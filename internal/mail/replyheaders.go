package mail

import (
	"io"
	"mime"
	netmail "net/mail"
	"regexp"
	"strings"

	"golang.org/x/net/html/charset"
)

// maxRefsTail is how many of the newest References ids are kept, besides the
// first (the thread root), when a chain grows long (RFC 5536 3.2.4 advice).
const maxRefsTail = 20

var messageIDRe = regexp.MustCompile(`<[^<>\s]+>`)

// MessageIDs returns the <msg-id> tokens of a References or In-Reply-To
// header value; comments and other junk between them are ignored.
func MessageIDs(v string) []string { return messageIDRe.FindAllString(v, -1) }

// ReferencesFor builds the References ids of a reply to a message (RFC 5322
// 3.6.4): the parent's References, else its In-Reply-To when that holds a
// single id, then the parent's own Message-ID. Without a Message-ID there is
// nothing to thread on, so it returns nil.
func ReferencesFor(parentRefs, parentInReplyTo, parentMessageID string) []string {
	id := MessageIDs(parentMessageID)
	if len(id) == 0 {
		return nil
	}
	refs := MessageIDs(parentRefs)
	if len(refs) == 0 {
		if irt := MessageIDs(parentInReplyTo); len(irt) == 1 {
			refs = irt
		}
	}
	if len(refs) == 0 || refs[len(refs)-1] != id[0] {
		refs = append(refs, id[0])
	}
	if len(refs) > maxRefsTail+1 {
		refs = append(refs[:1:1], refs[len(refs)-maxRefsTail:]...)
	}
	return refs
}

// ParseAddressList parses an RFC 5322 address-list (display names, quoted
// strings, comments, groups, RFC 2047 words) into "Name <addr>" strings. A
// malformed value yields nil: callers ignore it rather than fail.
func ParseAddressList(v string) []string {
	p := netmail.AddressParser{WordDecoder: &mime.WordDecoder{CharsetReader: func(label string, r io.Reader) (io.Reader, error) {
		return charset.NewReaderLabel(label, r)
	}}}
	list, err := p.ParseList(strings.Join(strings.Fields(v), " "))
	if err != nil {
		return nil
	}
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Address
		if a.Name != "" {
			out[i] = quoteName(a.Name) + " <" + a.Address + ">"
		}
	}
	return out
}

// quoteName quotes a display name that holds RFC 5322 specials.
func quoteName(n string) string {
	if !strings.ContainsAny(n, "()<>[]:;@\\,.\"") {
		return n
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(n) + `"`
}

// foldIDs writes "Name: id id ..." folding only between ids so no line exceeds
// 78 octets unless a single id is longer.
func foldIDs(name string, ids []string) string {
	var b strings.Builder
	line := len(name) + 1
	b.WriteString(name + ":")
	for i, id := range ids {
		if i > 0 && line+1+len(id) > 78 {
			b.WriteString("\r\n")
			line = 0
		}
		b.WriteString(" " + id)
		line += 1 + len(id)
	}
	return b.String()
}

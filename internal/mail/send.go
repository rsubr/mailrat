package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"mime"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/mailrat/mailrat/internal/gmail"
)

// SendService sends and replies to emails.
type SendService struct {
	client *gmail.Client
}

// NewSendService creates a SendService.
func NewSendService(client *gmail.Client) *SendService {
	return &SendService{client: client}
}

// Send sends an outbound message. Returns the new message ID.
func (s *SendService) Send(ctx context.Context, msg OutboundMessage) (string, error) {
	if err := checkRecipients(msg); err != nil {
		return "", err
	}
	raw, err := buildMessage(msg)
	if err != nil {
		return "", err
	}
	id, err := s.client.SendMessage(ctx, gmail.EncodeRaw(raw), msg.ThreadID)
	if err != nil {
		return "", err
	}
	return id, nil
}

// stripCRLF removes characters that could terminate a header line.
func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "", "\x00", "").Replace(s)
}

// splitAddressList splits an address list on commas outside quotes and <>.
func splitAddressList(s string) []string {
	var out []string
	var quoted, angle bool
	start := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && quoted:
			i++
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '<':
			angle = true
		case c == '>':
			angle = false
		case c == ',' && !angle:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// parseRecipients parses a recipient field token by token, so one half-typed
// address does not take the others down with it. bad holds the unparseable tokens.
func parseRecipients(v string) (ok []*netmail.Address, bad []string) {
	for _, tok := range splitAddressList(stripCRLF(v)) {
		if tok = strings.TrimSpace(tok); tok == "" {
			continue
		}
		if a, err := netmail.ParseAddress(tok); err == nil {
			ok = append(ok, a)
		} else {
			bad = append(bad, tok)
		}
	}
	return ok, bad
}

// addrHeader re-serialises an address list with quoted / RFC 2047 display
// names. Tokens that are not addresses (a half-typed recipient) are dropped
// rather than sent as a header Gmail rejects; Send refuses them up front.
func addrHeader(v string) string {
	list, _ := parseRecipients(v)
	parts := make([]string, len(list))
	for i, a := range list {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}

// checkRecipients rejects a message with an unparseable recipient or none at all.
func checkRecipients(msg OutboundMessage) error {
	n := 0
	for _, f := range []string{msg.To, msg.Cc, msg.Bcc} {
		ok, bad := parseRecipients(f)
		if len(bad) > 0 {
			return fmt.Errorf("invalid recipient %q", bad[0])
		}
		n += len(ok)
	}
	if n == 0 {
		return fmt.Errorf("no recipients")
	}
	return nil
}

func writeHeader(b *strings.Builder, name, value string) {
	b.WriteString(name + ": " + value + "\r\n")
}

func messageIDDomain(from string) string {
	if a, err := netmail.ParseAddress(stripCRLF(from)); err == nil {
		if _, d, ok := strings.Cut(a.Address, "@"); ok && d != "" {
			return d
		}
	}
	return "mailrat.local"
}

// encodeBody returns the transfer-encoding name and encoded body (CRLF lines).
func encodeBody(s string) (string, string) {
	nonASCII := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			nonASCII++
		}
	}
	if len(s) > 0 && nonASCII*3 > len(s) {
		enc := base64.StdEncoding.EncodeToString([]byte(s))
		var b strings.Builder
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
		return "base64", b.String()
	}
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	_, _ = w.Write([]byte(strings.ToValidUTF8(s, "\uFFFD")))
	_ = w.Close()
	out := buf.String()
	if !strings.HasSuffix(out, "\r\n") {
		out += "\r\n"
	}
	return "quoted-printable", out
}

func writePart(b *strings.Builder, boundary, ctype, body string) {
	cte, enc := encodeBody(body)
	if boundary != "" {
		b.WriteString("--" + boundary + "\r\n")
	}
	b.WriteString("Content-Type: " + ctype + "; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: " + cte + "\r\n\r\n")
	b.WriteString(enc)
}

// buildRaw is buildMessage for messages known to be valid (no attachments).
func buildRaw(msg OutboundMessage) string {
	raw, _ := buildMessage(msg)
	return raw
}

// buildMessage constructs an RFC 5322 message. HTML bodies are sanitised and
// sent as multipart/alternative (text/plain then text/html). With attachments
// the body becomes the first part of a multipart/mixed message.
func buildMessage(msg OutboundMessage) (string, error) {
	if err := CheckAttachmentsSize(msg.Attachments, len(msg.Body)); err != nil {
		return "", err
	}
	var b strings.Builder
	writeHeader(&b, "From", addrHeader(msg.From))
	if to := addrHeader(msg.To); to != "" {
		writeHeader(&b, "To", to)
	}
	if cc := addrHeader(msg.Cc); cc != "" {
		writeHeader(&b, "Cc", cc)
	}
	if bcc := addrHeader(msg.Bcc); bcc != "" {
		writeHeader(&b, "Bcc", bcc)
	}
	writeHeader(&b, "Subject", mime.QEncoding.Encode("UTF-8", stripCRLF(msg.Subject)))
	writeHeader(&b, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&b, "Message-ID", fmt.Sprintf("<%x.%x@%s>", time.Now().UnixNano(), randSuffix(), messageIDDomain(msg.From)))
	if irt := strings.TrimSpace(stripCRLF(msg.InReplyTo)); irt != "" {
		writeHeader(&b, "In-Reply-To", irt)
		refs := MessageIDs(stripCRLF(msg.References))
		if len(refs) == 0 {
			refs = []string{irt}
		}
		b.WriteString(foldIDs("References", refs) + "\r\n")
	}
	writeHeader(&b, "MIME-Version", "1.0")

	if len(msg.Attachments) == 0 {
		writeBody(&b, msg.Body)
		return b.String(), nil
	}
	mixed := fmt.Sprintf("mailrat_mixed_%x", time.Now().UnixNano())
	b.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mixed))
	b.WriteString("--" + mixed + "\r\n")
	writeBody(&b, msg.Body)
	used := map[string]bool{}
	for _, a := range msg.Attachments {
		data, err := readAttachment(a)
		if err != nil {
			return "", err
		}
		name := uniqueName(used, attachmentName(a))
		writeAttachment(&b, mixed, name, attachmentType(a, name, data), data)
	}
	b.WriteString("--" + mixed + "--\r\n")
	if b.Len() > MaxMessageBytes {
		return "", fmt.Errorf("attachments are too large: the message is %.1f MB, Gmail's limit is %d MB",
			float64(b.Len())/(1<<20), MaxMessageBytes>>20)
	}
	return b.String(), nil
}

// writeBody writes the body's Content-Type header, a blank line and the body:
// multipart/alternative for HTML, otherwise text/plain.
func writeBody(b *strings.Builder, body string) {
	if strings.HasPrefix(strings.TrimSpace(body), "<") {
		clean := SanitizeHTML(body)
		boundary := fmt.Sprintf("mailrat_%x", time.Now().UnixNano())
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary))
		writePart(b, boundary, "text/plain", htmlToText(clean))
		writePart(b, boundary, "text/html", clean)
		b.WriteString("--" + boundary + "--\r\n")
	} else {
		writePart(b, "", "text/plain", body)
	}
}

func randSuffix() uint64 {
	var v [8]byte
	_, _ = rand.Read(v[:])
	return binary.BigEndian.Uint64(v[:])
}

// htmlToText converts HTML to plain text for the text/plain MIME part.
// Links become "text (url)", list items "- ", br/p/div line breaks.
func htmlToText(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	var buf bytes.Buffer
	var walk func(n *html.Node)
	nl := func() {
		if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "\n") {
			buf.WriteByte('\n')
		}
	}
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			buf.WriteString(n.Data)
			return
		case html.ElementNode:
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Head:
				return
			case atom.Br:
				buf.WriteByte('\n')
				return
			case atom.Hr:
				nl()
				buf.WriteString("----\n")
				return
			case atom.Li:
				nl()
				buf.WriteString("- ")
			case atom.A:
				start := buf.Len()
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				href := ""
				for _, a := range n.Attr {
					if a.Key == "href" {
						href = a.Val
					}
				}
				text := strings.TrimSpace(buf.String()[start:])
				if href != "" && text != href && !strings.HasPrefix(href, "mailto:"+text) {
					buf.WriteString(" (" + href + ")")
				}
				return
			case atom.Blockquote:
				nl()
				start := buf.Len()
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				nl()
				quoted := quotePrefix(buf.String()[start:])
				buf.Truncate(start)
				buf.WriteString(quoted)
				return
			case atom.P, atom.Div, atom.Ul, atom.Ol, atom.Tr,
				atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre, atom.Table:
				nl()
			case atom.Td, atom.Th:
				if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "\n") {
					buf.WriteByte(' ')
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.P, atom.Div, atom.Li, atom.Tr,
				atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre:
				nl()
			}
		}
	}
	walk(doc)
	out := strings.ReplaceAll(buf.String(), "\u00a0", " ")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "\uFFFD")
	}
	return strings.TrimSpace(out)
}

// quotePrefix marks each line of a quoted block with "> ".
func quotePrefix(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, ">") {
			lines[i] = ">" + l
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

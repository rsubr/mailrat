package mail

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestParseAddressList(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"empty", "", nil},
		{"plain list", "a@x.com, Bob <b@x.com>", []string{"a@x.com", "Bob <b@x.com>"}},
		{"quoted comma", `"Doe, Jane" <j@x.com>, k@x.com`, []string{`"Doe, Jane" <j@x.com>`, "k@x.com"}},
		{"encoded word utf-8", "=?UTF-8?B?w4lsw6g=?= <e@x.com>", []string{"Élè <e@x.com>"}},
		{"encoded word latin1", "=?iso-8859-1?Q?Andr=E9?= <a@x.com>", []string{"André <a@x.com>"}},
		{"group", "Team: a@x.com, Bob <b@x.com>;", []string{"a@x.com", "Bob <b@x.com>"}},
		{"comment becomes name", "a@x.com (Ann)", []string{"Ann <a@x.com>"}},
		{"folded", "a@x.com,\r\n Bob <b@x.com>", []string{"a@x.com", "Bob <b@x.com>"}},
		{"malformed", "<<not an address", nil},
		{"half address", "a@x.com, bob", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseAddressList(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("<r%d@x>", i)
	}
	return out
}

func TestReferencesFor(t *testing.T) {
	long := strings.Join(ids(30), " ")
	tests := []struct {
		name, refs, irt, id string
		want                []string
	}{
		{"no message-id", "<a@x>", "", "", nil},
		{"first message", "", "", "<p@x>", []string{"<p@x>"}},
		{"parent references plus parent id", "<a@x> <b@x>", "<b@x>", "<p@x>", []string{"<a@x>", "<b@x>", "<p@x>"}},
		{"no references uses single in-reply-to", "", "<a@x>", "<p@x>", []string{"<a@x>", "<p@x>"}},
		{"ambiguous in-reply-to ignored", "", "<a@x> <b@x>", "<p@x>", []string{"<p@x>"}},
		{"folded and commented", "<a@x>\r\n (old) <b@x>", "", "<p@x>", []string{"<a@x>", "<b@x>", "<p@x>"}},
		{"parent id not repeated", "<a@x> <p@x>", "", "<p@x>", []string{"<a@x>", "<p@x>"}},
		{"junk ignored", "garbage <a@x> more", "", "<p@x>", []string{"<a@x>", "<p@x>"}},
	}
	tests = append(tests, struct {
		name, refs, irt, id string
		want                []string
	}{"trimmed to first plus last 20", long, "", "<p@x>", append([]string{"<r0@x>"}, append(slices.Clone(ids(30)[11:]), "<p@x>")...)})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ReferencesFor(tc.refs, tc.irt, tc.id)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := ReferencesFor(long, "", "<p@x>"); len(got) != 21 || got[0] != "<r0@x>" || got[20] != "<p@x>" {
		t.Fatalf("len %d first %q last %q", len(got), got[0], got[len(got)-1])
	}
}

func TestMIMEReferencesFolded(t *testing.T) {
	refs := strings.Join(ids(15), " ")
	raw := buildRaw(OutboundMessage{From: "me@x.com", To: "a@x.com", Body: "hi", InReplyTo: "<r14@x>", References: refs})
	head, _, _ := strings.Cut(raw, "\r\n\r\n")
	for _, l := range strings.Split(head, "\r\n") {
		if len(l) > 78 {
			t.Errorf("line over 78: %q", l)
		}
	}
	m := parseRaw(t, raw)
	if got := strings.Fields(m.Header.Get("References")); !slices.Equal(got, ids(15)) {
		t.Fatalf("References %q", got)
	}
	if !strings.Contains(raw, "\r\n <r") || m.Header.Get("In-Reply-To") != "<r14@x>" {
		t.Fatalf("not folded at whitespace or In-Reply-To lost:\n%s", head)
	}
}

func TestMIMEReferencesDefaultsAndInjection(t *testing.T) {
	m := parseRaw(t, buildRaw(OutboundMessage{From: "me@x.com", To: "a@x.com", Body: "hi", InReplyTo: "<p@x>"}))
	if m.Header.Get("References") != "<p@x>" {
		t.Fatalf("References %q", m.Header.Get("References"))
	}
	raw := buildRaw(OutboundMessage{From: "me@x.com", To: "a@x.com", Body: "hi", InReplyTo: "<p@x>",
		References: "<a@x>\r\nBcc: evil@x.com <b@x>"})
	if strings.Contains(raw, "\r\nBcc:") {
		t.Fatalf("header injection:\n%s", raw)
	}
	// a very long single id is never split
	long := "<" + strings.Repeat("x", 100) + "@x>"
	if got := parseRaw(t, buildRaw(OutboundMessage{From: "me@x.com", To: "a@x.com", Body: "hi", InReplyTo: long, References: "<a@x> " + long})).Header.Get("References"); !strings.HasSuffix(got, long) {
		t.Fatalf("long id split: %q", got)
	}
}

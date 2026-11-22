package gmail

import (
	"encoding/base64"
	"testing"

	gmailv1 "google.golang.org/api/gmail/v1"
)

func part(mime, ctype string, data []byte, parts ...*gmailv1.MessagePart) *gmailv1.MessagePart {
	p := &gmailv1.MessagePart{MimeType: mime, Parts: parts}
	if data != nil {
		p.Body = &gmailv1.MessagePartBody{Data: base64.URLEncoding.EncodeToString(data)}
	}
	if ctype != "" {
		p.Headers = []*gmailv1.MessagePartHeader{{Name: "Content-Type", Value: ctype}}
	}
	return p
}

func TestExtractBodyPrefersHTMLInAlternative(t *testing.T) {
	msg := part("multipart/alternative", "", nil,
		part("text/plain", "text/plain; charset=utf-8", []byte("line one\nline two")),
		part("text/html", "text/html; charset=utf-8", []byte("<p>line one</p><p>line two</p>")),
	)
	if got := extractBody(msg); got != "<p>line one</p><p>line two</p>" {
		t.Fatalf("got %q, want the HTML part", got)
	}
}

func TestExtractBodyNestedMixed(t *testing.T) {
	msg := part("multipart/mixed", "", nil,
		part("multipart/alternative", "", nil,
			part("text/plain", "", []byte("plain")),
			part("text/html", "", []byte("<table><tr><td>a</td><td>b</td></tr></table>")),
		),
		part("application/pdf", "", []byte("%PDF")),
	)
	if got := extractBody(msg); got != "<table><tr><td>a</td><td>b</td></tr></table>" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractBodyPlainOnly(t *testing.T) {
	msg := part("text/plain", "", []byte("a\n\nb <x@y.z>"))
	if got := extractBody(msg); got != "a\n\nb <x@y.z>" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractBodyCharset(t *testing.T) {
	// "café" in ISO-8859-1
	msg := part("text/html", "text/html; charset=ISO-8859-1", []byte("<p>caf\xe9</p>"))
	if got := extractBody(msg); got != "<p>café</p>" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractBodyUnpaddedBase64(t *testing.T) {
	p := &gmailv1.MessagePart{MimeType: "text/plain", Body: &gmailv1.MessagePartBody{
		Data: base64.RawURLEncoding.EncodeToString([]byte("ab"))}}
	if got := extractBody(p); got != "ab" {
		t.Fatalf("got %q", got)
	}
}

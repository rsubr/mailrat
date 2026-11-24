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

func TestExtractBodyUTF8MislabelledAsLatin1(t *testing.T) {
	// UTF-8 en dash under a windows-1252 header must not become "â€“".
	for _, ct := range []string{"text/html; charset=windows-1252", "text/html; charset=ISO-8859-1", "text/html; charset=us-ascii"} {
		msg := part("text/html", ct, []byte("<p>Coimbatore – 641 035</p>"))
		if got := extractBody(msg); got != "<p>Coimbatore – 641 035</p>" {
			t.Errorf("%s: got %q", ct, got)
		}
	}
}

func TestExtractBodyMetaCharset(t *testing.T) {
	// No charset in the header: the HTML meta declaration decides.
	msg := part("text/html", "text/html", []byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1252"><p>caf`+"\xe9 \x96 x</p>"))
	want := `<meta http-equiv="Content-Type" content="text/html; charset=windows-1252"><p>café – x</p>`
	if got := extractBody(msg); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractBodyUTF16(t *testing.T) {
	msg := part("text/plain", "text/plain; charset=utf-16le", []byte("h\x00i\x00 \x00\x13\x20"))
	if got := extractBody(msg); got != "hi –" {
		t.Fatalf("got %q", got)
	}
}

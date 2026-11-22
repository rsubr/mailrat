package mail

import (
	"strings"
	"testing"
)

func TestBuildRawHTMLIsMultipartAlternative(t *testing.T) {
	raw := buildRaw(OutboundMessage{
		From: "a@x.com", To: "b@x.com", Subject: "s",
		Body: `<div><b>Hi</b> &amp; bye</div><ul><li>one</li><li>two</li></ul>`,
	})
	if !strings.Contains(raw, "Content-Type: multipart/alternative") {
		t.Fatal("not multipart/alternative")
	}
	plain := strings.Index(raw, `text/plain`)
	htmlPart := strings.Index(raw, `text/html`)
	if plain < 0 || htmlPart < plain {
		t.Fatal("text/plain must precede text/html")
	}
	if !strings.Contains(raw, "Hi & bye\n- one\n- two") {
		t.Errorf("plain part wrong:\n%s", raw)
	}
	if !strings.Contains(raw, "<b>Hi</b>") {
		t.Error("html part missing")
	}
}

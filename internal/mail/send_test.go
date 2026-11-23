package mail

import (
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

func parseRaw(t *testing.T, raw string) *mail.Message {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	return m
}

func partBody(t *testing.T, p *multipart.Part) string {
	t.Helper()
	var r io.Reader = p
	if p.Header.Get("Content-Transfer-Encoding") == "quoted-printable" {
		r = quotedprintable.NewReader(p)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBuildRawHTMLRoundTrip(t *testing.T) {
	raw := buildRaw(OutboundMessage{
		From: "Zoë <a@x.com>", To: "b@x.com", Subject: "Héllo wörld",
		Body:      `<div><b>Hi</b> &amp; bye <a href="https://e.com/x">site</a></div><ul><li>one</li><li>two</li></ul><script>alert(1)</script>`,
		InReplyTo: "<abc@x.com>",
	})
	m := parseRaw(t, raw)
	for _, h := range []string{"Date", "Message-ID", "MIME-Version"} {
		if m.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	if _, err := m.Header.Date(); err != nil {
		t.Errorf("date: %v", err)
	}
	if m.Header.Get("In-Reply-To") != "<abc@x.com>" || m.Header.Get("References") != "<abc@x.com>" {
		t.Error("reply headers lost")
	}
	subj, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil || subj != "Héllo wörld" {
		t.Errorf("subject %q %v", subj, err)
	}
	from, err := m.Header.AddressList("From")
	if err != nil || from[0].Name != "Zoë" {
		t.Errorf("from %v %v", from, err)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("ct %s %v", mt, err)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	var types []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct, cp, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		types = append(types, ct)
		if !strings.EqualFold(cp["charset"], "utf-8") {
			t.Errorf("charset %v", cp)
		}
		body := partBody(t, p)
		switch ct {
		case "text/plain":
			want := "Hi & bye site (https://e.com/x)\n- one\n- two"
			if strings.ReplaceAll(body, "\r\n", "\n") != want+"\n" && strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n")) != want {
				t.Errorf("plain: %q", body)
			}
		case "text/html":
			if !strings.Contains(body, "<b>Hi</b>") || strings.Contains(body, "script") {
				t.Errorf("html: %q", body)
			}
		}
	}
	if len(types) != 2 || types[0] != "text/plain" || types[1] != "text/html" {
		t.Errorf("parts %v", types)
	}
}

func TestBuildRawHeaderInjection(t *testing.T) {
	raw := buildRaw(OutboundMessage{
		From: "a@x.com", To: "b@x.com\r\nBcc: evil@x.com", Cc: "c@x.com\nX-Evil: 1",
		Subject: "hi\r\nBcc: evil@x.com", InReplyTo: "<a@x>\r\nX-Evil: 1", Body: "plain",
	})
	m := parseRaw(t, raw)
	for _, h := range []string{"Bcc", "X-Evil"} {
		if v := m.Header.Get(h); v != "" {
			t.Errorf("injected %s=%q", h, v)
		}
	}
	head, _, _ := strings.Cut(raw, "\r\n\r\n")
	if strings.Count(head, "\r\nBcc:") != 0 {
		t.Errorf("injected line:\n%s", head)
	}
}

func TestBuildRawPlainAndBase64(t *testing.T) {
	m := parseRaw(t, buildRaw(OutboundMessage{From: "a@x.com", To: "b@x.com", Body: "héllo\nline2"}))
	if m.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
		t.Errorf("cte %q", m.Header.Get("Content-Transfer-Encoding"))
	}
	b, _ := io.ReadAll(quotedprintable.NewReader(m.Body))
	if strings.ReplaceAll(string(b), "\r\n", "\n") != "héllo\nline2\n" && !strings.HasPrefix(string(b), "héllo") {
		t.Errorf("body %q", b)
	}
	m = parseRaw(t, buildRaw(OutboundMessage{From: "a@x.com", To: "b@x.com", Body: "日本語日本語日本語"}))
	if m.Header.Get("Content-Transfer-Encoding") != "base64" {
		t.Errorf("cte %q", m.Header.Get("Content-Transfer-Encoding"))
	}
}

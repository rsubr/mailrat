package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

type gotPart struct {
	header map[string][]string
	ctype  string
	params map[string]string
	body   []byte
}

func readParts(t *testing.T, raw string) (topType string, parts []gotPart) {
	t.Helper()
	m := parseRaw(t, raw)
	topType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(topType, "multipart/") {
		return topType, nil
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return topType, parts
		}
		if err != nil {
			t.Fatal(err)
		}
		ct, pp, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		b, err := io.ReadAll(p) // multipart.Reader decodes quoted-printable only
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, gotPart{header: p.Header, ctype: ct, params: pp, body: b})
	}
}

func b64Decode(t *testing.T, s string) []byte {
	t.Helper()
	for _, l := range strings.Split(strings.TrimRight(s, "\r\n"), "\r\n") {
		if len(l) > 76 {
			t.Fatalf("base64 line of %d columns", len(l))
		}
	}
	out, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(s, "\r\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBuildAttachmentsStructure(t *testing.T) {
	dir := t.TempDir()
	diskData := bytes.Repeat([]byte("0123456789"), 500)
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, diskData, 0o600); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 100))
	msg := OutboundMessage{
		From: "a@x.com", To: "b@x.com", Subject: "s", Body: "<p>Hello</p>",
		Attachments: []Attachment{
			{Path: path},
			{Filename: "Résumé ü.txt", Data: []byte("héllo")},
			{Filename: "pic", Data: png},
			{Filename: "empty.bin"},
			{Filename: "report.pdf", Data: []byte("dup")},
		},
	}
	raw, err := buildMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	top, parts := readParts(t, raw)
	if top != "multipart/mixed" || len(parts) != 6 {
		t.Fatalf("top %s, %d parts\n%s", top, len(parts), raw)
	}
	if parts[0].ctype != "multipart/alternative" || !strings.Contains(string(parts[0].body), "text/html") {
		t.Errorf("body part: %s", parts[0].ctype)
	}
	type want struct {
		name, ctype string
		data        []byte
	}
	for i, w := range []want{
		{"report.pdf", "application/pdf", diskData},
		{"Résumé ü.txt", "text/plain", []byte("héllo")},
		{"pic", "image/png", png},
		{"empty.bin", "application/octet-stream", nil},
		{"report (2).pdf", "application/pdf", []byte("dup")},
	} {
		p := parts[i+1]
		if p.ctype != w.ctype || p.params["name"] != w.name {
			t.Errorf("part %d: type %s name %q", i, p.ctype, p.params["name"])
		}
		cd, params, err := mime.ParseMediaType(p.header["Content-Disposition"][0])
		if err != nil || cd != "attachment" || params["filename"] != w.name {
			t.Errorf("part %d disposition %v %v %v", i, cd, params, err)
		}
		if p.header["Content-Transfer-Encoding"][0] != "base64" {
			t.Errorf("part %d encoding", i)
		}
		if got := b64Decode(t, string(p.body)); !bytes.Equal(got, w.data) {
			t.Errorf("part %d bytes differ (%d vs %d)", i, len(got), len(w.data))
		}
	}
	if !strings.Contains(raw, "filename*=utf-8''R%C3%A9sum%C3%A9%20%C3%BC.txt") {
		t.Errorf("non-ASCII filename not RFC 2231 encoded:\n%s", raw[strings.Index(raw, "Content-Disposition"):][:120])
	}
}

func TestBuildAttachmentsPlainBody(t *testing.T) {
	raw, err := buildMessage(OutboundMessage{From: "a@x.com", To: "b@x.com", Body: "plain",
		Attachments: []Attachment{{Filename: "a.txt", Data: []byte("x")}}})
	if err != nil {
		t.Fatal(err)
	}
	top, parts := readParts(t, raw)
	if top != "multipart/mixed" || len(parts) != 2 || parts[0].ctype != "text/plain" {
		t.Fatalf("%s %+v", top, parts)
	}
}

func TestBuildNoAttachmentsUnchanged(t *testing.T) {
	for _, body := range []string{"plain", "<p>html</p>"} {
		msg := OutboundMessage{From: "a@x.com", To: "b@x.com", Body: body}
		top, _ := readParts(t, buildRaw(msg))
		want := "text/plain"
		if body[0] == '<' {
			want = "multipart/alternative"
		}
		if top != want {
			t.Errorf("%q: %s", body, top)
		}
	}
}

func TestAttachmentFilenameSanitising(t *testing.T) {
	cases := map[string]string{
		"a\r\nBcc: evil@x.com.txt": "aBcc: evil@x.com.txt",
		`x".txt`:                   "x'.txt",
		"../../etc/passwd":         "passwd",
		`C:\dir\f.doc`:             "f.doc",
		"":                         "attachment",
		"..":                       "attachment",
		"tab\there.txt":            "tabhere.txt",
	}
	for in, want := range cases {
		if got := attachmentName(Attachment{Filename: in}); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	raw, err := buildMessage(OutboundMessage{From: "a@x.com", To: "b@x.com", Body: "x",
		Attachments: []Attachment{{Filename: "a\r\nBcc: evil@x.com\r\n.txt", Data: []byte("x")}}})
	if err != nil {
		t.Fatal(err)
	}
	if m := parseRaw(t, raw); m.Header.Get("Bcc") != "" {
		t.Fatal("header injection through filename")
	}
	if strings.Contains(raw, "\r\nBcc: evil") {
		t.Fatal("injected header line")
	}
}

func TestAttachmentErrorsAndLimits(t *testing.T) {
	base := OutboundMessage{From: "a@x.com", To: "b@x.com", Body: "x"}

	m := base
	m.Attachments = []Attachment{{Path: filepath.Join(t.TempDir(), "missing.txt")}}
	if _, err := buildMessage(m); err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("missing file: %v", err)
	}
	m.Attachments = []Attachment{{Path: t.TempDir()}}
	if _, err := buildMessage(m); err == nil {
		t.Error("directory accepted")
	}

	big := make([]byte, 19<<20) // 19 MB -> ~25.3 MB encoded
	m.Attachments = []Attachment{{Filename: "big.bin", Data: big}}
	if _, err := buildMessage(m); err == nil || !strings.Contains(err.Error(), "25 MB") {
		t.Errorf("oversized: %v", err)
	}
	m.Attachments = []Attachment{{Filename: "ok.bin", Data: big[:17<<20]}}
	if _, err := buildMessage(m); err != nil {
		t.Errorf("17 MB should fit: %v", err)
	}
	// Several files that only exceed the limit together.
	m.Attachments = []Attachment{{Data: big[:10<<20]}, {Data: big[:10<<20]}}
	if err := CheckAttachmentsSize(m.Attachments, 0); err == nil {
		t.Error("sum not checked")
	}
}

func TestAttachmentTypeFallbacks(t *testing.T) {
	cases := []struct {
		a    Attachment
		name string
		data string
		want string
	}{
		{Attachment{MIMEType: "image/jpeg"}, "x.png", "", "image/jpeg"},
		{Attachment{MIMEType: "bad\r\ntype"}, "x.png", "", "image/png"},
		{Attachment{MIMEType: "multipart/mixed"}, "x.png", "", "image/png"},
		{Attachment{}, "noext", "%PDF-1.4 ...", "application/pdf"},
		{Attachment{}, "noext", "\x00\x01\x02\xff", "application/octet-stream"},
		{Attachment{}, "noext", "", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := attachmentType(c.a, c.name, []byte(c.data)); got != c.want {
			t.Errorf("%+v: got %s want %s", c.a, got, c.want)
		}
	}
}

func TestDraftRoundTripWithAttachment(t *testing.T) {
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(storage.Account{ID: "a", Email: "a@x.com", TokenJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	msg := OutboundMessage{From: "a@x.com", To: "b@x.com", Subject: "hi", Body: "<p>v1</p>",
		Attachments: []Attachment{{Filename: "n.txt", Data: []byte("payload")}}}
	if err := SaveLocalDraft(s, "local-1", "a", msg); err != nil {
		t.Fatal(err)
	}
	d, err := s.GetDraft("a", "local-1")
	if err != nil {
		t.Fatal(err)
	}
	_, parts := readParts(t, d.Content)
	if len(parts) != 2 || !bytes.Equal(b64Decode(t, string(parts[1].body)), []byte("payload")) {
		t.Fatalf("stored draft lacks attachment:\n%s", d.Content)
	}
	msg.Attachments = []Attachment{{Path: filepath.Join(t.TempDir(), "gone")}}
	if err := SaveLocalDraft(s, "local-1", "a", msg); err == nil {
		t.Error("unreadable attachment must fail the save")
	}
}

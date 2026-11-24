package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatAttachment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.pdf")
	if err := os.WriteFile(p, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := StatAttachment(p)
	if err != nil || a.Filename != "n.pdf" || a.Size != 5 || a.Path != p || a.MIMEType != "application/pdf" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := StatAttachment(dir); err == nil {
		t.Error("directory accepted")
	}
	if _, err := StatAttachment(filepath.Join(dir, "none")); err == nil {
		t.Error("missing file accepted")
	}
	big := filepath.Join(dir, "big")
	f, _ := os.Create(big)
	_ = f.Truncate(30 << 20) // sparse
	f.Close()
	if _, err := StatAttachment(big); err == nil {
		t.Error("30 MB file accepted")
	}
}

func TestCheckAttachmentsAndEqual(t *testing.T) {
	small := []OutAttachment{{Filename: "a", Data: []byte("x")}}
	if err := CheckAttachments(small, 100); err != nil {
		t.Fatal(err)
	}
	if err := CheckAttachments(append(small, OutAttachment{Data: make([]byte, 20<<20)}), 0); err == nil {
		t.Fatal("over limit accepted")
	}
	a, b := OutboundMsg{To: "x", Attachments: small}, OutboundMsg{To: "x", Attachments: []OutAttachment{{Filename: "a", Data: []byte("x")}}}
	if !a.Equal(b) {
		t.Error("equal msgs differ")
	}
	b.Attachments[0].Data = []byte("y")
	if a.Equal(b) || a.Equal(OutboundMsg{To: "x"}) {
		t.Error("different msgs equal")
	}
}

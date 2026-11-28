package ui

import "testing"

func TestAttachmentKind(t *testing.T) {
	for _, c := range []struct {
		name, mime string
		want       attachKind
	}{
		{"a.PDF", "", kindPDF}, {"a.docx", "", kindWord}, {"a.xlsx", "", kindExcel},
		{"a.csv", "", kindExcel}, {"a.pptx", "", kindPPT}, {"a.txt", "", kindText},
		{"a.zip", "", kindArchive}, {"a.mp3", "", kindAudio}, {"a.go", "", kindCode},
		{"noext", "image/png", kindImage}, {"noext", "text/plain", kindText},
		{"noext", "application/pdf", kindPDF}, {"noext", "application/octet-stream", kindFile},
	} {
		if got := attachmentKind(c.name, c.mime); got != c.want {
			t.Errorf("%s %s: got %v want %v", c.name, c.mime, got.Label, c.want.Label)
		}
	}
}

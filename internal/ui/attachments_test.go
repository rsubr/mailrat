package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	for in, want := range map[string]string{
		"report.pdf":         "report.pdf",
		"../../etc/passwd":   "passwd",
		`..\..\evil.exe`:     "evil.exe",
		"a:b*c?.txt":         "a_b_c_.txt",
		"":                   "attachment",
		"..":                 "attachment",
		" . ":                "attachment",
		"x\x00y.txt":         "x_y.txt",
		"/abs/path/file.txt": "file.txt",
	} {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q)=%q want %q", in, got, want)
		}
	}
}

func TestUniqueName(t *testing.T) {
	used := map[string]bool{}
	var got []string
	for _, n := range []string{"a.txt", "a.txt", "A.txt", "b", "b", "../a.txt"} {
		got = append(got, uniqueName(n, used))
	}
	want := []string{"a.txt", "a (1).txt", "A (2).txt", "b", "b (1)", "a (3).txt"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v want %v", got, want)
			break
		}
	}
}

func TestWriteTempAttachment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "open")
	p1, err := writeTempAttachment(root, "../x.txt", []byte("1"))
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := writeTempAttachment(root, "../x.txt", []byte("2"))
	if p1 == p2 || filepath.Base(p1) != "x.txt" {
		t.Fatalf("collision or bad name: %s %s", p1, p2)
	}
	if rel, _ := filepath.Rel(root, p1); filepath.IsAbs(rel) || rel[0] == '.' {
		t.Fatalf("escaped root: %s", p1)
	}
	if b, _ := os.ReadFile(p1); string(b) != "1" {
		t.Fatal("content")
	}
}

func TestFileURL(t *testing.T) {
	if got := fileURL("/tmp/a b.txt").String(); got != "file:///tmp/a%20b.txt" {
		t.Fatal(got)
	}
}

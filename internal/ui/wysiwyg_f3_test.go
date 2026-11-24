package ui

import (
	"strings"
	"testing"

	"github.com/mailrat/mailrat/internal/desktop"
)

func f3Editor(html string) *wyEditor {
	e := newWyEditor()
	e.setDoc(parseHTML(html))
	return e
}

func TestF3ExportKeepsGmailClasses(t *testing.T) {
	m := desktop.MessageItem{FromAddr: "a@x.com", Date: 1700000000000, Body: "<p>hi</p>"}
	out := parseHTML(composeHTML("<p>Sig</p>", quoteHTML(m))).toHTML()
	for _, want := range []string{`<div class="gmail_signature">`, `<div class="gmail_quote">`, `<div class="gmail_attr">`, "<blockquote>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if strings.Index(out, "gmail_signature") > strings.Index(out, "gmail_quote") {
		t.Errorf("signature must stay above the quote: %s", out)
	}
	// A reopened draft keeps the structure and does not nest or duplicate it.
	if again := parseHTML(out).toHTML(); again != out {
		t.Errorf("not stable:\n%s\n%s", out, again)
	}
	// The separator keeps its trailing space so "-- " detection works elsewhere.
	if !strings.Contains(out, "<p>-- </p>") {
		t.Errorf("separator mangled: %s", out)
	}
}

func TestF3ImportTablesAndPre(t *testing.T) {
	d := parseHTML(`<table><tr><td>Name</td><td>Bob</td></tr><tr><th>Age</th><td>7</td></tr></table>`)
	got := d.plainText()
	if got != "Name Bob\nAge 7" {
		t.Errorf("table: %q", got)
	}
	d = parseHTML("<pre>func f() {\n    return 1\n}\n\nend</pre>")
	if got := d.plainText(); got != "func f() {\n    return 1\n}\n\nend" {
		t.Errorf("pre: %q", got)
	}
}

func TestF3TypingAfterLinkDoesNotExtendIt(t *testing.T) {
	e := f3Editor(`<p>see <a href="https://x.example">link</a></p>`)
	e.moveTo(e.docEnd(), false)
	e.insertText(" after")
	if got := e.doc.toHTML(); !strings.Contains(got, `<a href="https://x.example">link</a> after`) {
		t.Fatalf("link extended: %s", got)
	}
	// Typing inside the link keeps it.
	e = f3Editor(`<p><a href="https://x.example">link</a></p>`)
	e.moveTo(wyPos{0, 2}, false)
	e.insertText("X")
	if got := e.doc.toHTML(); !strings.Contains(got, `>liXnk</a>`) {
		t.Fatalf("inside link: %s", got)
	}
	// Enter at the end of a link starts the next paragraph without it.
	e = f3Editor(`<p><a href="https://x.example">link</a></p>`)
	e.moveTo(e.docEnd(), false)
	e.enter()
	e.insertText("next")
	if got := e.doc.toHTML(); strings.Contains(got, "next</a>") {
		t.Fatalf("new paragraph continued the link: %s", got)
	}
}

func TestF3ClearFormattingKeepsLinks(t *testing.T) {
	e := f3Editor(`<p><b><a href="https://x.example">link</a></b></p>`)
	e.selectAll()
	e.clearFormatting()
	if got := e.doc.toHTML(); !strings.Contains(got, `<a href="https://x.example">link</a>`) || strings.Contains(got, "<b>") {
		t.Fatalf("got %s", got)
	}
}

func TestF3GraphemeCaretAndBackspace(t *testing.T) {
	tests := []struct{ name, text string }{
		{"flag", "a🇮🇳b"},
		{"zwj family", "a👨‍👩‍👧b"},
		{"combining", "aéb"},
		{"skin tone", "a👍🏽b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := f3Editor("<p>" + tc.text + "</p>")
			n := e.doc.Paras[0].length()
			e.moveTo(wyPos{0, 1 + clusterLen(tc.text, 1)}, false) // after the cluster at index 1
			if e.caret.Off != n-1 {
				t.Fatalf("setup: caret %d, len %d", e.caret.Off, n)
			}
			if p := e.left(e.caret, false); p.Off != leadLen(tc.text) {
				t.Errorf("left stops inside the cluster: %d", p.Off)
			}
			if p := e.right(wyPos{0, leadLen(tc.text)}, false); p.Off != n-1 {
				t.Errorf("right stops inside the cluster: %d", p.Off)
			}
			e.backspace(false)
			if got := e.doc.Paras[0].text(); got != strings.Replace(tc.text, tc.text[1:len(tc.text)-1], "", 1) {
				t.Errorf("backspace left %q", got)
			}
		})
	}
	e := f3Editor("<p>x🇮🇳y</p>")
	e.moveTo(wyPos{0, 1}, false)
	e.deleteForward(false)
	if got := e.doc.Paras[0].text(); got != "xy" {
		t.Errorf("delete forward left %q", got)
	}
}

// clusterLen is the rune length of the middle of s (all but first and last rune... clusters).
func clusterLen(s string, _ int) int { return len([]rune(s)) - 2 }
func leadLen(string) int             { return 1 }

func TestF3SignatureSwapKeepsUndoAndCaret(t *testing.T) {
	w := NewWysiwyg(nil)
	q := quoteHTML(desktop.MessageItem{FromAddr: "a@x.com", Date: 1700000000000, Body: "<p>orig</p>"})
	w.SetHTML(composeHTML("", q))
	tail := quoteParas(q)
	w.ed.setCollapsed(false)
	w.ed.insertText("typed first")
	w.ed.moveTo(wyPos{len(w.ed.doc.Paras) - 1, 2}, false) // inside the quote
	before := w.ed.doc.Paras[w.ed.caret.P].text()
	if !w.replaceSignature("", "<p>L1</p><p>L2</p>", tail) {
		t.Fatal("swap refused")
	}
	if got := w.ed.doc.Paras[w.ed.caret.P].text(); got != before || w.ed.caret.Off != 2 {
		t.Errorf("caret moved to %q off %d, want %q", got, w.ed.caret.Off, before)
	}
	if !w.ed.canUndo() {
		t.Fatal("undo history wiped by the signature arriving")
	}
	w.Undo()
	if got := w.ed.doc.Paras[0].text(); got != "" {
		t.Errorf("undo should revert the typing: %q", got)
	}
	if !strings.Contains(w.ed.doc.plainText(), "L1") {
		t.Error("undo must not take the signature away")
	}
}

func TestF3CleanLinkHostPortAndUserinfo(t *testing.T) {
	ok := map[string]string{
		"localhost:3000/x":     "https://localhost:3000/x",
		"example.com:8080":     "https://example.com:8080",
		"example.com:8080/a?b": "https://example.com:8080/a?b",
	}
	for in, want := range ok {
		if got, good := cleanLink(in); !good || got != want {
			t.Errorf("cleanLink(%q) = %q,%v", in, got, good)
		}
	}
	for _, in := range []string{"foo@bar.com/x", "javascript:alert(1)", "javascript:1", "data:text/html,x", "vbscript:x", "a:b"} {
		if got, good := cleanLink(in); good {
			t.Errorf("cleanLink(%q) accepted as %q", in, got)
		}
	}
}

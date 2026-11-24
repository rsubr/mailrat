package ui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/mailrat/mailrat/internal/richclip"
)

func TestG4PasteDocConversion(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bold italic", `<p><b>a</b> <i>b</i> <u>c</u> <s>d</s></p>`, `<p><b>a</b> <i>b</i> <u>c</u> <s>d</s></p>`},
		{"link", `<p>see <a href="https://x.example/p">here</a></p>`, `<p>see <a href="https://x.example/p">here</a></p>`},
		{"list", `<ul><li>one</li><li><b>two</b></li></ul><ol><li>3</li></ol>`, `<ul><li>one</li><li><b>two</b></li></ul><ol><li>3</li></ol>`},
		{"quote", `<blockquote>said</blockquote><p>after</p>`, `<blockquote><p>said</p></blockquote><p>after</p>`},
		{"nested inline", `<p><b><i>both</i> bold</b></p>`, `<p><b><i>both</i></b><b> bold</b></p>`},
		{"chrome wrapper", `<meta charset="utf-8"><html><body><!--StartFragment--><p>x</p><!--EndFragment--></body></html>`, `<p>x</p>`},
		{"script and handlers", `<p onclick="evil()">hi<script>alert(1)</script></p>`, `<p>hi</p>`},
		{"javascript link", `<p><a href="javascript:alert(1)">click</a></p>`, `<p>click</p>`},
		{"image dropped, text kept", `<p>a<img src="http://t/p.png">b</p><div>c</div>`, `<p>ab</p><p>c</p>`},
		{"table text kept", `<table><tr><td>A</td><td>B</td></tr></table>`, `<p>A B</p>`},
		{"heading text kept", `<h1>Title</h1><p>body</p>`, `<p>Title</p><p>body</p>`},
		{"colour and size dropped", `<p><span style="color:#ff0000;font-size:30px">red</span></p>`, `<p>red</p>`},
		{"gmail quote class not folded", `<div class="gmail_quote"><p>q</p></div>`, `<p>q</p>`},
		{"malformed", `<p><b>open <i>never closed`, `<p><b>open </b><b><i>never closed</i></b></p>`},
	}
	for _, c := range cases {
		d, ok := pasteDoc(c.in)
		if !ok {
			t.Errorf("%s: nothing pasted", c.name)
			continue
		}
		if got := d.toHTML(); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestG4PasteDocNothingVisible(t *testing.T) {
	for _, in := range []string{"", `<img src="x">`, `<script>x()</script>`, `<style>p{}</style>`, "   "} {
		if _, ok := pasteDoc(in); ok {
			t.Errorf("%q should fall back to plain", in)
		}
	}
}

func TestG4PasteDocSizeCap(t *testing.T) {
	big := "<p>" + strings.Repeat("x", wyMaxPasteHTML) + "</p>"
	if _, ok := pasteDoc(big); ok {
		t.Fatal("over the cap must fall back to plain")
	}
	if d, ok := pasteDoc("<p>" + strings.Repeat("x", wyMaxPasteHTML/2) + "</p>"); !ok || d.plainText() == "" {
		t.Fatal("large paste under the cap should work")
	}
}

func TestG4InsertDocSplicing(t *testing.T) {
	e := f3Editor("<p>ab</p>")
	e.moveTo(wyPos{0, 1}, false)
	d, _ := pasteDoc("<p>X</p><ul><li>Y</li></ul>")
	e.insertDoc(d)
	if got := e.doc.toHTML(); got != "<p>aX</p><ul><li>Yb</li></ul>" {
		t.Fatalf("got %s", got)
	}
	if e.caret != (wyPos{1, 1}) || e.hasSel() {
		t.Fatalf("caret %v", e.caret)
	}
	// Inline paste keeps the caret paragraph's kind and ends after the pasted text.
	e = f3Editor("<ul><li>ab</li></ul>")
	e.moveTo(wyPos{0, 1}, false)
	d, _ = pasteDoc("<p><b>Z</b></p>")
	e.insertDoc(d)
	if got := e.doc.toHTML(); got != "<ul><li>a<b>Z</b>b</li></ul>" || e.caret != (wyPos{0, 2}) {
		t.Fatalf("got %s caret %v", got, e.caret)
	}
}

// g4Paste pastes through the real shortcut path with the clipboard holding html and plain.
func g4Paste(t *testing.T, w *Wysiwyg, html, plain string, shift bool) {
	t.Helper()
	fake := &richclip.Fake{HTML: html, Text: plain}
	old := richReader
	richReader = fake
	t.Cleanup(func() { richReader = old })
	clip := fyne.CurrentApp().Clipboard()
	clip.SetContent(plain)
	if shift {
		w.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyV, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
		return
	}
	w.TypedShortcut(&fyne.ShortcutPaste{Clipboard: clip})
}

func TestG4EditorPasteRichKeepsFormatting(t *testing.T) {
	w, _, _ := wysSetup(t)
	w.SetHTML("<p>start:</p>")
	w.ed.moveTo(w.ed.docEnd(), false)
	g4Paste(t, w, `<p>a <b>bold</b> <a href="https://x.example">link</a></p>`, "a bold link", false)
	if got := w.HTML(); got != `<p>start:a <b>bold</b> <a href="https://x.example">link</a></p>` {
		t.Fatalf("got %s", got)
	}
}

func TestG4EditorPasteUndoOneStep(t *testing.T) {
	w, _, _ := wysSetup(t)
	w.SetHTML("<p>keep</p>")
	w.ed.moveTo(w.ed.docEnd(), false)
	g4Paste(t, w, "<p><b>one</b></p><p>two</p><ul><li>three</li></ul>", "one\ntwo\nthree", false)
	if len(w.ed.doc.Paras) != 3 {
		t.Fatalf("got %s", w.HTML())
	}
	w.Undo()
	if got := w.HTML(); got != "<p>keep</p>" {
		t.Fatalf("one undo should remove the whole paste, got %s", got)
	}
}

func TestG4EditorPasteReplacesSelection(t *testing.T) {
	w, _, _ := wysSetup(t)
	w.SetHTML("<p>hello big world</p>")
	w.ed.moveTo(wyPos{0, 6}, false)
	w.ed.moveTo(wyPos{0, 9}, true)
	g4Paste(t, w, "<i>small</i>", "small", false)
	if got := w.HTML(); got != "<p>hello <i>small</i> world</p>" {
		t.Fatalf("got %s", got)
	}
}

func TestG4EditorPastePlainFallbacks(t *testing.T) {
	w, _, _ := wysSetup(t)
	// No HTML on the clipboard: plain text exactly as before.
	g4Paste(t, w, "", "just text\nline2", false)
	if got := w.PlainText(); got != "just text\nline2" {
		t.Fatalf("got %q", got)
	}
	// HTML with nothing visible falls back to the plain flavour.
	w.SetHTML("")
	g4Paste(t, w, `<img src="x">`, "alt text", false)
	if got := w.PlainText(); got != "alt text" {
		t.Fatalf("got %q", got)
	}
	// Reader failure: plain.
	w.SetHTML("")
	old := richReader
	richReader = &richclip.Fake{ReadErr: richclip.ErrUnsupported, HTML: "<b>no</b>"}
	t.Cleanup(func() { richReader = old })
	fyne.CurrentApp().Clipboard().SetContent("plain only")
	w.TypedShortcut(&fyne.ShortcutPaste{Clipboard: fyne.CurrentApp().Clipboard()})
	if got := w.HTML(); got != "<p>plain only</p>" {
		t.Fatalf("got %s", got)
	}
}

func TestG4EditorShiftPasteIsPlain(t *testing.T) {
	w, _, _ := wysSetup(t)
	g4Paste(t, w, "<p><b>rich</b></p>", "rich", true)
	if got := w.HTML(); got != "<p>rich</p>" {
		t.Fatalf("got %s", got)
	}
}

func TestG4EditorPasteOverCapIsPlain(t *testing.T) {
	w, _, _ := wysSetup(t)
	big := "<p><b>" + strings.Repeat("x", wyMaxPasteHTML) + "</b></p>"
	g4Paste(t, w, big, "plain version", false)
	if got := w.HTML(); got != "<p>plain version</p>" {
		t.Fatalf("got %.60s", got)
	}
}

func TestG4EditorPasteStripsUnsafe(t *testing.T) {
	w, _, _ := wysSetup(t)
	g4Paste(t, w, `<p onclick="x()"><a href="javascript:alert(1)">bad</a><script>boom()</script></p>`, "bad", false)
	if got := w.HTML(); strings.Contains(got, "javascript") || strings.Contains(got, "script") || strings.Contains(got, "onclick") {
		t.Fatalf("unsafe markup survived: %s", got)
	}
}

// Reader selection copy (richclip Set) pasted into compose keeps its formatting.
func TestG4CopyPasteRoundTrip(t *testing.T) {
	w, _, _ := wysSetup(t)
	fake := &richclip.Fake{}
	_ = fake.Set(fmt.Sprintf(`<p>Hi <b>there</b>, <a href="%s">site</a></p>`, "https://s.example"), "Hi there, site")
	old := richReader
	richReader = fake
	t.Cleanup(func() { richReader = old })
	w.TypedShortcut(&fyne.ShortcutPaste{Clipboard: fyne.CurrentApp().Clipboard()})
	if got := w.HTML(); got != `<p>Hi <b>there</b>, <a href="https://s.example">site</a></p>` {
		t.Fatalf("got %s", got)
	}
}

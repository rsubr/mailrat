package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

func edWith(html string) *wyEditor {
	e := newWyEditor()
	e.setDoc(parseHTML(html))
	return e
}

func TestWyHTMLRoundTrip(t *testing.T) {
	cases := []string{
		`<p>hello <b>bold</b> <i>it</i> <u>un</u> <s>st</s></p>`,
		`<p><span style="color: #ff0000; font-size: 18px">big red</span></p>`,
		`<p>see <a href="https://example.com/a?b=1&amp;c=2">link</a></p>`,
		`<ul><li>one</li><li>two</li></ul><ol><li>a</li></ol>`,
		`<blockquote><p>quoted</p><p>more</p></blockquote><p>after</p>`,
		`<p>a &lt;tag&gt; &amp; more</p><p><br></p><p>x</p>`,
		`<p>&nbsp;lead and &nbsp;double&nbsp;</p>`,
	}
	for _, in := range cases {
		out := strings.ReplaceAll(strings.ReplaceAll(parseHTML(in).toHTML(), "<br/>", "<br>"), "\u00a0", "&nbsp;")
		if out != in {
			t.Errorf("round trip\n in: %s\nout: %s", in, out)
		}
	}
}

func TestWyParseAllowlist(t *testing.T) {
	d := parseHTML(`<div>x<script>alert(1)</script><img src=x onerror=y> <font color="blue" size="5">y</font>` +
		`<a href="javascript:alert(1)">bad</a><a href="mailto:a@b.co">ok</a></div><div>two<br>three</div>`)
	if got := d.plainText(); got != "x ybadok\ntwo\nthree" {
		t.Errorf("plain = %q", got)
	}
	out := d.toHTML()
	for _, bad := range []string{"script", "javascript", "onerror", "<img"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q leaked in %s", bad, out)
		}
	}
	if !strings.Contains(out, `href="mailto:a@b.co"`) || !strings.Contains(out, "color: #0000ff") ||
		!strings.Contains(out, "font-size: 24px") {
		t.Errorf("lost formatting: %s", out)
	}
}

func TestWyParseCleanup(t *testing.T) {
	if got := parseHTML("<p>a</p>\n  <p>b </p>\n").plainText(); got != "a\nb" {
		t.Errorf("got %q", got)
	}
	if got := parseHTML("").plainText(); got != "" || len(parseHTML("").Paras) != 1 {
		t.Errorf("empty doc wrong: %q", got)
	}
	if got := parseHTML("plain  text\nwrapped").plainText(); got != "plain text wrapped" {
		t.Errorf("got %q", got)
	}
}

func TestWyCleanLinkAndColor(t *testing.T) {
	for in, want := range map[string]string{
		"https://x.org": "https://x.org", "x.org/a": "https://x.org/a", "a@b.co": "mailto:a@b.co",
		"": "", "  http://x.org ": "http://x.org",
	} {
		if got, ok := cleanLink(in); !ok || got != want {
			t.Errorf("cleanLink(%q) = %q,%v", in, got, ok)
		}
	}
	for _, in := range []string{"javascript:alert(1)", "ftp://x", "a b", "nodots", "http://"} {
		if _, ok := cleanLink(in); ok {
			t.Errorf("cleanLink(%q) accepted", in)
		}
	}
	for in, want := range map[string]string{
		"#F00": "#ff0000", "#00FF00": "#00ff00", "rgb(1, 2, 3)": "#010203", "red": "#ff0000",
		"rgb(300,0,0)": "", "url(x)": "", "": "", "#12": "",
	} {
		if got := parseColor(in); got != want {
			t.Errorf("parseColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWyTypingEnterBackspace(t *testing.T) {
	e := newWyEditor()
	e.insertText("hello")
	e.enter()
	e.insertText("world")
	if got := e.doc.plainText(); got != "hello\nworld" {
		t.Fatalf("got %q", got)
	}
	e.moveTo(wyPos{1, 0}, false)
	e.backspace(false)
	if got := e.doc.plainText(); got != "helloworld" || e.caret != (wyPos{0, 5}) {
		t.Fatalf("got %q caret %v", got, e.caret)
	}
	e.deleteForward(false)
	if got := e.doc.plainText(); got != "helloorld" {
		t.Fatalf("got %q", got)
	}
	e.insertText("a\r\nb\tc")
	if got := e.doc.plainText(); got != "helloa\nb    corld" {
		t.Fatalf("paste got %q", got)
	}
}

func TestWyFormatSelection(t *testing.T) {
	e := edWith(`<p>one two three</p>`)
	e.moveTo(wyPos{0, 4}, false)
	e.moveTo(wyPos{0, 7}, true)
	e.toggleBold()
	e.setColor("#f00")
	e.setSize(20)
	if got := e.doc.toHTML(); got != `<p>one <b><span style="color: #ff0000; font-size: 20px">two</span></b> three</p>` {
		t.Fatalf("got %s", got)
	}
	if !e.styleAtCaret().Bold {
		t.Error("toolbar state should show bold")
	}
	e.toggleBold()
	if strings.Contains(e.doc.toHTML(), "<b>") {
		t.Error("second toggle should remove bold")
	}
	e.clearFormatting()
	if got := e.doc.toHTML(); got != `<p>one two three</p>` {
		t.Fatalf("clear got %s", got)
	}
}

func TestWyFormatAcrossParagraphsAndCaret(t *testing.T) {
	e := edWith(`<p>ab</p><p>cd</p>`)
	e.selectAll()
	e.toggleItalic()
	if got := e.doc.toHTML(); got != `<p><i>ab</i></p><p><i>cd</i></p>` {
		t.Fatalf("got %s", got)
	}
	e = newWyEditor()
	e.toggleBold() // collapsed caret: applies to following text
	e.insertText("x")
	e.toggleBold()
	e.insertText("y")
	if got := e.doc.toHTML(); got != `<p><b>x</b>y</p>` {
		t.Fatalf("got %s", got)
	}
}

func TestWyLinks(t *testing.T) {
	e := edWith(`<p>go here now</p>`)
	e.moveTo(wyPos{0, 3}, false)
	e.moveTo(wyPos{0, 7}, true)
	if !e.setLink("example.com") {
		t.Fatal("link rejected")
	}
	if got := e.doc.toHTML(); got != `<p>go <a href="https://example.com">here</a> now</p>` {
		t.Fatalf("got %s", got)
	}
	e.moveTo(wyPos{0, 5}, false)
	if e.linkAtCaret() != "https://example.com" {
		t.Fatal("link not detected at caret")
	}
	e.setLink("mailto:a@b.co") // edit in place, whole link
	if got := e.doc.toHTML(); !strings.Contains(got, `<a href="mailto:a@b.co">here</a>`) {
		t.Fatalf("edit got %s", got)
	}
	e.setLink("") // remove
	if got := e.doc.toHTML(); got != `<p>go here now</p>` {
		t.Fatalf("remove got %s", got)
	}
	if e.setLink("javascript:x") {
		t.Error("javascript link accepted")
	}
	e = newWyEditor() // nothing selected, no link: insert the URL as text
	e.setLink("https://x.org")
	if got := e.doc.toHTML(); got != `<p><a href="https://x.org">https://x.org</a></p>` {
		t.Fatalf("insert got %s", got)
	}
}

func TestWyLists(t *testing.T) {
	e := edWith(`<p>a</p><p>b</p><p>c</p>`)
	e.moveTo(wyPos{0, 0}, false)
	e.moveTo(wyPos{1, 1}, true)
	e.toggleKind(wyBullet)
	if got := e.doc.toHTML(); got != `<ul><li>a</li><li>b</li></ul><p>c</p>` {
		t.Fatalf("got %s", got)
	}
	e.toggleKind(wyNumber)
	if got := e.doc.plainText(); got != "1. a\n2. b\nc" {
		t.Fatalf("got %q", got)
	}
	e.toggleKind(wyNumber)
	if e.doc.Paras[0].Kind != wyPara {
		t.Error("toggle again should remove the list")
	}
	// Enter continues a list, Enter on an empty item leaves it
	e = edWith(`<ul><li>x</li></ul>`)
	e.moveTo(e.docEnd(), false)
	e.enter()
	e.insertText("y")
	e.enter()
	e.enter()
	if got := e.doc.toHTML(); got != `<ul><li>x</li><li>y</li></ul><p><br/></p>` {
		t.Fatalf("got %s", got)
	}
	// Backspace at the start of an item drops the bullet first
	e = edWith(`<ul><li>x</li><li>y</li></ul>`)
	e.moveTo(wyPos{1, 0}, false)
	e.backspace(false)
	if got := e.doc.toHTML(); got != `<ul><li>x</li></ul><p>y</p>` {
		t.Fatalf("got %s", got)
	}
}

func TestWyQuote(t *testing.T) {
	e := edWith(`<p>reply</p><blockquote><p>old</p></blockquote>`)
	if e.doc.Paras[1].Kind != wyQuote || e.doc.plainText() != "reply\n> old" {
		t.Fatalf("got %q", e.doc.plainText())
	}
	e.moveTo(wyPos{0, 0}, false)
	e.toggleKind(wyQuote)
	if got := e.doc.toHTML(); got != `<blockquote><p>reply</p><p>old</p></blockquote>` {
		t.Fatalf("got %s", got)
	}
}

func TestWyUndoRedo(t *testing.T) {
	e := newWyEditor()
	e.insertText("a")
	e.insertText("b")
	e.insertText("c") // typing coalesces into one step
	e.enter()
	e.insertText("d")
	e.doUndo()
	if got := e.doc.plainText(); got != "abc\n" {
		t.Fatalf("undo 1 got %q", got)
	}
	e.doUndo()
	if got := e.doc.plainText(); got != "abc" {
		t.Fatalf("undo 2 got %q", got)
	}
	e.doUndo()
	if got := e.doc.plainText(); got != "" || e.canUndo() {
		t.Fatalf("undo 3 got %q", got)
	}
	e.doRedo()
	e.doRedo()
	e.doRedo()
	if got := e.doc.plainText(); got != "abc\nd" || e.canRedo() {
		t.Fatalf("redo got %q", got)
	}
	e.selectAll()
	e.toggleBold()
	e.doUndo()
	if strings.Contains(e.doc.toHTML(), "<b>") {
		t.Error("undo should remove bold")
	}
	e.insertText("z") // new edit clears redo
	if e.canRedo() {
		t.Error("redo should be cleared")
	}
}

func TestWySelectionDeleteCut(t *testing.T) {
	e := edWith(`<p>one</p><p>two</p><p>three</p>`)
	e.moveTo(wyPos{0, 2}, false)
	e.moveTo(wyPos{2, 2}, true)
	if got := e.selectedText(); got != "e\ntwo\nth" {
		t.Fatalf("sel %q", got)
	}
	e.cut()
	if got := e.doc.plainText(); got != "onree" || e.caret != (wyPos{0, 2}) {
		t.Fatalf("got %q", got)
	}
	e.doUndo()
	if got := e.doc.plainText(); got != "one\ntwo\nthree" {
		t.Fatalf("undo got %q", got)
	}
}

func TestWyWordMotion(t *testing.T) {
	e := edWith(`<p>foo bar baz</p>`)
	if p := e.right(wyPos{0, 0}, true); p.Off != 4 {
		t.Errorf("right word = %d", p.Off)
	}
	if p := e.left(wyPos{0, 11}, true); p.Off != 8 {
		t.Errorf("left word = %d", p.Off)
	}
	if a, b := e.wordAt(wyPos{0, 5}); a.Off != 4 || b.Off != 7 {
		t.Errorf("wordAt = %d,%d", a.Off, b.Off)
	}
}

func TestWysiwygWidget(t *testing.T) {
	test.NewApp()
	win := test.NewWindow(nil)
	defer win.Close()
	w := NewWysiwyg(win)
	changed := 0
	w.OnChanged = func() { changed++ }
	bar := NewWysiwygToolbar(w)
	win.SetContent(container.NewBorder(bar, nil, nil, nil, w))
	w.Resize(fyne.NewSize(400, 200))
	w.SetHTML(`<p>hello <b>world</b></p><ul><li>item</li></ul>`)
	if w.PlainText() != "hello world\n- item" || w.IsEmpty() {
		t.Fatalf("plain %q", w.PlainText())
	}
	w.TypedRune('!')
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	w.TypedRune('x')
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if changed != 4 {
		t.Errorf("OnChanged called %d times", changed)
	}
	w.TypedShortcut(&fyne.ShortcutSelectAll{})
	w.Bold()
	if got := w.HTML(); !strings.Contains(got, "<b>") {
		t.Errorf("html %s", got)
	}
	w.Undo()
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if len(w.lines) < 3 {
		t.Errorf("lines = %d", len(w.lines))
	}
}

func TestWyBackspaceFirstParagraph(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bullet", `<ul><li>x</li></ul>`, `<p>x</p>`},
		{"number", `<ol><li>x</li></ol>`, `<p>x</p>`},
		{"quote", `<blockquote><p>x</p></blockquote>`, `<p>x</p>`},
		{"plain", `<p>x</p>`, `<p>x</p>`},
	}
	for _, c := range cases {
		e := edWith(c.in)
		e.moveTo(wyPos{0, 0}, false)
		e.backspace(false)
		if got := e.doc.toHTML(); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
	// with a selection, Backspace deletes it and leaves the block kind alone
	e := edWith(`<ul><li>xy</li></ul>`)
	e.moveTo(wyPos{0, 0}, false)
	e.moveTo(wyPos{0, 1}, true)
	e.backspace(false)
	if got := e.doc.toHTML(); got != `<ul><li>y</li></ul>` {
		t.Errorf("selection: got %s", got)
	}
}

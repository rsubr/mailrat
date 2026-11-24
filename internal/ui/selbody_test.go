package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/richclip"
)

const selSrc = `<p>Hello <b>bold</b> and <i>slanted</i> with a <a href="https://example.com/x?a=1&b=2">link</a>.</p>` +
	`<h1>Big head</h1><ul><li>one</li><li>two <u>under</u></li></ul><ol><li>first</li></ol>` +
	`<p>tail words</p><blockquote>quoted line</blockquote>`

func selDocOf(src string) *selBody {
	b := newSelBody(renderMail(src, renderOpts{bg: lightBG, fg: lightFG}).segments)
	return b
}

func TestSelFragmentAll(t *testing.T) {
	test.NewTempApp(t)
	b := selDocOf(selSrc)
	b.selectAll()
	h, p := b.Selection()
	for _, want := range []string{
		"<p>Hello <b>bold</b> and <i>slanted</i> with a <a href=\"https://example.com/x?a=1&amp;b=2\">link</a>.</p>",
		"<h2>Big head</h2>",
		"<ul><li>one</li><li>two <u>under</u></li></ul>",
		"<ol><li>first</li></ol>",
		"<p>tail words</p>",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html missing %q in\n%s", want, h)
		}
	}
	for _, want := range []string{"Hello bold and slanted with a link.", "Big head", "• one\n", "1. first", "tail words"} {
		if !strings.Contains(p, want) {
			t.Errorf("plain missing %q in\n%q", want, p)
		}
	}
	if strings.ContainsAny(p, " ▎") {
		t.Errorf("plain has layout artefacts: %q", p)
	}
}

func TestSelFragmentPartialInline(t *testing.T) {
	test.NewTempApp(t)
	b := selDocOf(`<p>Hello <b>bold</b> and <i>slanted</i> text</p><p>second para</p>`)
	a := selIdx(b, "ld and sla")
	b.setSel(a, a+len("ld and sla"))
	h, p := b.Selection()
	if want := "<p><b>ld</b> and <i>sla</i></p>"; h != want {
		t.Errorf("html = %s, want %s", h, want)
	}
	if p != "ld and sla" {
		t.Errorf("plain = %q", p)
	}
	// across paragraphs
	a = selIdx(b, "text")
	b.setSel(a, selIdx(b, "para")+4)
	h, p = b.Selection()
	if h != "<p>text</p><p>second para</p>" || p != "text\n\nsecond para" {
		t.Errorf("multi: %s | %q", h, p)
	}
}

func TestSelFragmentQuoteAndSafety(t *testing.T) {
	test.NewTempApp(t)
	b := selDocOf(`<p>before</p><blockquote>quoted <b>text</b></blockquote>`)
	res := renderMail(`<p>before</p><blockquote>quoted <b>text</b></blockquote>`, renderOpts{})
	q := newSelBody(res.quoted)
	q.selectAll()
	h, p := q.Selection()
	if h != "<blockquote><p>quoted <b>text</b></p></blockquote>" || p != "> quoted text" {
		t.Errorf("quote: %s | %q", h, p)
	}
	_ = b

	evil := selDocOf(`<p>a<script>alert(1)</script><a href="javascript:alert(1)" onclick="x()">bad</a> <b onclick="y()" style="background:url(http://t/x)">ok</b><img src="http://t/p.png"></p>`)
	evil.selectAll()
	h, _ = evil.Selection()
	for _, bad := range []string{"script", "javascript", "onclick", "style", "<img", "http://t"} {
		if strings.Contains(h, bad) {
			t.Errorf("copied html contains %q: %s", bad, h)
		}
	}
	if !strings.Contains(h, "<b>ok</b>") {
		t.Errorf("lost content: %s", h)
	}
}

// selFixture shows a body in the test window with a fake rich clipboard.
func selFixture(t *testing.T, src string) (*UI, *selBody, *richclip.Fake) {
	t.Helper()
	u, _ := newTestUI(t)
	fake := &richclip.Fake{}
	old := richClipboard
	richClipboard = fake
	t.Cleanup(func() { richClipboard = old })
	body := u.newBody(renderMail(src, renderOpts{bg: lightBG, fg: lightFG}).segments)
	u.win.SetContent(body)
	u.win.Resize(fyne.NewSize(420, 300))
	return u, body, fake
}

// selIdx is the rune offset of sub in the body (strings.Index counts bytes).
func selIdx(b *selBody, sub string) int {
	f := string(b.doc.flat)
	return len([]rune(f[:strings.Index(f, sub)]))
}

// selPt is the centre of the character at document offset off.
func selPt(b *selBody, off int) fyne.Position {
	li := b.lineOf(off)
	ln := b.lay.lines[li]
	return fyne.NewPos(b.xAt(li, off)+1, ln.y+ln.h/2)
}

func selPress(b *selBody, p fyne.Position) {
	b.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}, Button: desktop.MouseButtonPrimary})
}

func TestSelDragAndCopy(t *testing.T) {
	_, b, fake := selFixture(t, selSrc)
	from, to := selIdx(b, "bold"), selIdx(b, "Big head")+3
	selPress(b, selPt(b, from))
	b.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: selPt(b, to)}})
	b.DragEnd()
	if a, c := b.sel(); a != from || c != to {
		t.Fatalf("sel = %d..%d, want %d..%d", a, c, from, to)
	}
	b.TypedShortcut(&fyne.ShortcutCopy{Clipboard: test.NewClipboard()})
	if fake.Calls != 1 {
		t.Fatalf("clipboard writes = %d", fake.Calls)
	}
	want := "<p><b>bold</b> and <i>slanted</i> with a <a href=\"https://example.com/x?a=1&amp;b=2\">link</a>.</p><h2>Big</h2>"
	if fake.HTML != want {
		t.Errorf("html = %s\nwant   %s", fake.HTML, want)
	}
	if fake.Text != "bold and slanted with a link.\n\nBig" {
		t.Errorf("text = %q", fake.Text)
	}
}

func TestSelClickCounts(t *testing.T) {
	_, b, fake := selFixture(t, `<p>alpha beta gamma</p><p>second line</p>`)
	at := selPt(b, selIdx(b, "beta")+1)
	selPress(b, at)
	selPress(b, at)
	b.TypedShortcut(&fyne.ShortcutCopy{})
	if fake.Text != "beta" || fake.HTML != "<p>beta</p>" {
		t.Errorf("word: %q %q", fake.Text, fake.HTML)
	}
	selPress(b, at)
	b.TypedShortcut(&fyne.ShortcutCopy{})
	if fake.Text != "alpha beta gamma" {
		t.Errorf("paragraph: %q", fake.Text)
	}
	b.lastClick = time.Now().Add(-time.Hour)
	selPress(b, at)
	if b.hasSel() {
		t.Error("single click should collapse the selection")
	}
}

func TestSelKeyboard(t *testing.T) {
	u, b, fake := selFixture(t, `<p>alpha beta</p><p>second</p>`)
	b.TypedShortcut(&fyne.ShortcutSelectAll{})
	b.TypedShortcut(&fyne.ShortcutCopy{})
	if fake.Text != "alpha beta\n\nsecond" {
		t.Errorf("select all: %q", fake.Text)
	}
	selPress(b, selPt(b, 0))
	b.lastClick = time.Time{}
	b.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	for i := 0; i < 5; i++ {
		b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	}
	b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	b.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	b.TypedShortcut(&fyne.ShortcutCopy{})
	if !strings.HasPrefix(fake.Text, "alpha beta\n\nsec") {
		t.Errorf("shift selection: %q", fake.Text)
	}
	_ = u
}

func TestSelCopyFallsBackToPlain(t *testing.T) {
	u, b, fake := selFixture(t, `<p>plain me</p>`)
	fake.Err = richclip.ErrUnsupported
	b.selectAll()
	b.TypedShortcut(&fyne.ShortcutCopy{})
	if got := u.app.Clipboard().Content(); got != "plain me" {
		t.Errorf("fallback clipboard = %q", got)
	}
}

func TestSelLinkTapWithoutSelection(t *testing.T) {
	_, b, _ := selFixture(t, `<p>go <a href="https://example.com/">there</a></p>`)
	p := selPt(b, selIdx(b, "there")+1)
	if b.linkAt(p) == nil {
		t.Fatal("no link under pointer")
	}
	if b.linkAt(selPt(b, 0)) != nil {
		t.Fatal("link reported on plain text")
	}
}

func TestSelWrapsAndSizes(t *testing.T) {
	_, b, _ := selFixture(t, `<p>`+strings.Repeat("word ", 80)+`</p>`)
	if n := len(b.lay.lines); n < 3 {
		t.Fatalf("expected wrapping, got %d line(s)", n)
	}
	if b.MinSize().Height < float32(len(b.lay.lines))*10 {
		t.Fatalf("min height %v for %d lines", b.MinSize().Height, len(b.lay.lines))
	}
}

func TestSelGolden(t *testing.T) {
	freezeNow(t, goldenNow)
	u, b, _ := selFixture(t, selSrc)
	b.setSel(selIdx(b, "bold"), selIdx(b, "two"))
	golden(t, "reader_sel_range", u.win.Canvas().Capture())
}

package ui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// f4Long builds an editor with n paragraphs of about 200 bytes each.
func f4Long(tb testing.TB, n int) *Wysiwyg {
	tb.Helper()
	test.NewApp()
	win := test.NewWindow(nil)
	tb.Cleanup(win.Close)
	w := NewWysiwyg(win)
	win.SetContent(container.NewBorder(nil, nil, nil, nil, w))
	w.Resize(fyne.NewSize(700, 400))
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "<p>Paragraph %d: the quick brown fox jumps over the lazy dog and then <b>bold words here</b> keep going on and on until the line wraps around twice or so.</p>", i)
	}
	w.SetHTML(sb.String())
	return w
}

func BenchmarkWysiwygTypeRune(b *testing.B) {
	w := f4Long(b, 300)
	w.ed.moveTo(wyPos{P: 150, Off: 20}, false)
	w.after(false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.TypedRune('x')
	}
}

// Typing in the middle of a long document must produce the same lines as a
// from-scratch layout (the per-paragraph cache must never go stale).
func TestF4CachedLayoutMatchesFresh(t *testing.T) {
	w := f4Long(t, 40)
	w.ed.moveTo(wyPos{P: 20, Off: 10}, false)
	check := func(step string) {
		t.Helper()
		got := fmt.Sprintf("%+v", w.lines)
		moreY := w.moreY
		gotObjs := f4Objs(w.page.Objects)
		w.lcache, w.ocache = nil, nil
		want := fmt.Sprintf("%+v", w.layoutLines(w.width-wyScrollBar))
		if got != want || moreY != w.moreY {
			t.Fatalf("%s: cached layout differs from fresh", step)
		}
		w.lines = w.layoutLines(w.width - wyScrollBar)
		if wantObjs := f4Objs(w.draw()); gotObjs != wantObjs {
			t.Fatalf("%s: reused canvas objects differ from freshly built ones", step)
		}
	}
	for _, r := range "hello world, this is long enough to wrap the line again and again" {
		w.TypedRune(r)
	}
	check("typing")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	w.TypedRune('z')
	check("enter")
	w.Bold()
	w.TypedRune('q')
	check("bold")
	w.Undo()
	check("undo")
	w.Resize(fyne.NewSize(500, 400))
	check("resize")
	w.TypedShortcut(&fyne.ShortcutSelectAll{})
	check("select all")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	w.ed.moveTo(wyPos{P: 5, Off: 3}, false)
	w.ed.moveTo(wyPos{P: 7, Off: 9}, true)
	w.after(false)
	check("partial selection")
	w.TypedRune('k')
	check("replace selection")
	w.ed.moveTo(wyPos{P: 10, Off: 2}, false)
	w.ed.moveTo(wyPos{P: 10, Off: 9}, true)
	w.after(false)
	if !w.SetLink("https://example.com") {
		t.Fatal("link rejected")
	}
	check("link")
	w.SetLink("")
	check("unlink")
	w.SetHTML("<p>mine</p><p>more</p><div class=\"gmail_quote\"><p>quoted one</p><p>quoted two</p></div>")
	check("quote fixture")
	w.ToggleQuote()
	check("toggle quote")
	w.ToggleQuote()
	check("toggle quote back")
}

// f4Objs describes the drawn objects: kind, geometry, text and colour.
func f4Objs(objs []fyne.CanvasObject) string {
	var sb strings.Builder
	for _, o := range objs {
		fmt.Fprintf(&sb, "%T %v %v", o, o.Position(), o.Size())
		switch c := o.(type) {
		case *canvas.Text:
			fmt.Fprintf(&sb, " %q %v %v %v", c.Text, c.Color, c.TextSize, c.TextStyle)
		case *canvas.Rectangle:
			fmt.Fprintf(&sb, " %v", c.FillColor)
		case *canvas.Line:
			fmt.Fprintf(&sb, " %v %v %v", c.StrokeColor, c.Position1, c.Position2)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

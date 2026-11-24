package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// wysSetup hosts an editor with its formatting toolbar in a window, like compose does.
func wysSetup(t *testing.T) (*Wysiwyg, fyne.CanvasObject, fyne.Window) {
	t.Helper()
	testUI(t) // test app, theme and window registry
	win := newWindow(t, nil, 760, 320)
	w := NewWysiwyg(win)
	bar := NewWysiwygToolbar(w)
	win.SetContent(container.NewBorder(bar, nil, nil, nil, w))
	win.Canvas().Focus(w)
	return w, bar, win
}

// wysWalk visits o and everything below it, including widget internals.
func wysWalk(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	f(o)
	switch c := o.(type) {
	case *fyne.Container:
		for _, ch := range c.Objects {
			wysWalk(ch, f)
		}
	case *container.Scroll:
		wysWalk(c.Content, f)
	case fyne.Widget:
		for _, ch := range test.WidgetRenderer(c).Objects() {
			wysWalk(ch, f)
		}
	}
}

func wysButton(t *testing.T, root fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	var found *widget.Button
	wysWalk(root, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && found == nil && b.Text == text {
			found = b
		}
	})
	if found == nil {
		t.Fatalf("no button %q", text)
	}
	return found
}

// wysIconButtons returns the two icon-only buttons of the toolbar: undo, redo.
func wysIconButtons(t *testing.T, bar fyne.CanvasObject) (undo, redo *widget.Button) {
	t.Helper()
	var bs []*widget.Button
	wysWalk(bar, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && b.Text == "" {
			bs = append(bs, b)
		}
	})
	if len(bs) != 2 {
		t.Fatalf("%d icon buttons", len(bs))
	}
	return bs[0], bs[1]
}

// wysOverlay returns the topmost overlay of win's canvas.
func wysOverlay(t *testing.T, win fyne.Window) fyne.CanvasObject {
	t.Helper()
	o := win.Canvas().Overlays().Top()
	if o == nil {
		t.Fatal("no overlay open")
	}
	return o
}

func wysTap(b *widget.Button) { test.Tap(b) }

func wysCtrl(w *Wysiwyg, k fyne.KeyName) {
	w.TypedShortcut(&desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierControl})
}

func wysSelectAll(w *Wysiwyg) { w.TypedShortcut(&fyne.ShortcutSelectAll{}) }

func wysHas(t *testing.T, w *Wysiwyg, want ...string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(w.HTML(), s) {
			t.Errorf("HTML %q lacks %q", w.HTML(), s)
		}
	}
}

func wysHasNot(t *testing.T, w *Wysiwyg, bad ...string) {
	t.Helper()
	for _, s := range bad {
		if strings.Contains(w.HTML(), s) {
			t.Errorf("HTML %q should not contain %q", w.HTML(), s)
		}
	}
}

func TestWyUIInlineToolbarButtons(t *testing.T) {
	w, bar, win := wysSetup(t)
	for _, c := range []struct{ btn, tag string }{{"B", "b"}, {"I", "i"}, {"U", "u"}, {"S", "s"}} {
		w.SetHTML("<p>word</p>")
		wysSelectAll(w)
		b := wysButton(t, bar, c.btn)
		wysTap(b)
		wysHas(t, w, "<"+c.tag+">word</"+c.tag+">")
		if b.Importance != widget.HighImportance {
			t.Errorf("%s not highlighted while selection has it", c.btn)
		}
		if win.Canvas().Focused() != fyne.Focusable(w) {
			t.Errorf("%s did not return focus to the editor", c.btn)
		}
		wysTap(b)
		wysHasNot(t, w, "<"+c.tag+">")
		if b.Importance == widget.HighImportance {
			t.Errorf("%s still highlighted after toggling off", c.btn)
		}
	}
}

func TestWyUIBoldAtCaretAppliesToNextTyping(t *testing.T) {
	w, bar, _ := wysSetup(t)
	test.Type(w, "a ")
	wysTap(wysButton(t, bar, "B"))
	test.Type(w, "bold")
	wysTap(wysButton(t, bar, "B"))
	test.Type(w, " c")
	wysHas(t, w, "<p>a <b>bold</b> c</p>")
}

func TestWyUIShortcuts(t *testing.T) {
	w, bar, _ := wysSetup(t)
	for _, c := range []struct {
		key fyne.KeyName
		btn string
		tag string
	}{{fyne.KeyB, "B", "b"}, {fyne.KeyI, "I", "i"}, {fyne.KeyU, "U", "u"}} {
		w.SetHTML("<p>word</p>")
		wysSelectAll(w)
		wysCtrl(w, c.key)
		wysHas(t, w, "<"+c.tag+">word</"+c.tag+">")
		if wysButton(t, bar, c.btn).Importance != widget.HighImportance {
			t.Errorf("Ctrl+%s did not update the toolbar", c.btn)
		}
		wysCtrl(w, c.key)
		wysHasNot(t, w, "<"+c.tag+">")
	}
	// Super (Cmd) works, Alt and Shift alone are ignored.
	w.SetHTML("<p>word</p>")
	wysSelectAll(w)
	w.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierAlt})
	w.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierShift})
	wysHasNot(t, w, "<b>")
	w.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierSuper})
	wysHas(t, w, "<b>word</b>")
}

func TestWyUIKeyboardSelectionThenFormat(t *testing.T) {
	w, _, _ := wysSetup(t)
	w.SetHTML("<p>one two</p>")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	w.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	for range 3 {
		w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	}
	w.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	wysCtrl(w, fyne.KeyB)
	wysHas(t, w, "<p><b>one</b> two</p>")
}

func TestWyUIUndoRedoButtonsAndShortcuts(t *testing.T) {
	w, bar, _ := wysSetup(t)
	undo, redo := wysIconButtons(t, bar)
	if !undo.Disabled() || !redo.Disabled() {
		t.Fatal("undo/redo enabled on an empty editor")
	}
	test.Type(w, "abc")
	if undo.Disabled() || !redo.Disabled() {
		t.Fatalf("after typing undo disabled=%v redo disabled=%v", undo.Disabled(), redo.Disabled())
	}
	wysSelectAll(w)
	wysCtrl(w, fyne.KeyB)
	wysHas(t, w, "<b>abc</b>")

	wysTap(undo) // formatting step
	wysHasNot(t, w, "<b>")
	if redo.Disabled() {
		t.Error("redo not enabled after undo")
	}
	wysTap(undo) // typing step (coalesced)
	if !w.IsEmpty() || !undo.Disabled() {
		t.Errorf("not back to empty: %q undo disabled=%v", w.PlainText(), undo.Disabled())
	}
	wysTap(redo)
	wysTap(redo)
	wysHas(t, w, "<b>abc</b>")
	if !redo.Disabled() {
		t.Error("redo still enabled with nothing to redo")
	}

	w.TypedShortcut(&fyne.ShortcutUndo{})
	wysHasNot(t, w, "<b>")
	w.TypedShortcut(&fyne.ShortcutRedo{})
	wysHas(t, w, "<b>abc</b>")
	test.Type(w, "d") // a new edit drops the redo history
	w.TypedShortcut(&fyne.ShortcutUndo{})
	w.TypedShortcut(&fyne.ShortcutUndo{})
	w.TypedShortcut(&fyne.ShortcutRedo{})
	w.TypedShortcut(&fyne.ShortcutRedo{})
	if !redo.Disabled() {
		t.Error("redo should be exhausted")
	}
}

func TestWyUIListsAndQuote(t *testing.T) {
	w, bar, _ := wysSetup(t)
	bullets, numbers, quote := wysButton(t, bar, "• List"), wysButton(t, bar, "1. List"), wysButton(t, bar, "Quote")
	high := func(b *widget.Button) bool { return b.Importance == widget.HighImportance }

	wysTap(bullets)
	test.Type(w, "a")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	test.Type(w, "b")
	wysHas(t, w, "<ul><li>a</li><li>b</li></ul>")
	if !high(bullets) || high(numbers) || high(quote) {
		t.Error("only the bullet button should be active")
	}
	// Enter on an empty item leaves the list.
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	test.Type(w, "after")
	wysHas(t, w, "<ul><li>a</li><li>b</li></ul>", "<p>after</p>")
	if high(bullets) {
		t.Error("bullet button active outside the list")
	}

	// Switching kinds replaces rather than nests; tapping again removes.
	wysSelectAll(w)
	wysTap(numbers)
	wysHas(t, w, "<ol>")
	wysHasNot(t, w, "<ul>")
	if !high(numbers) || high(bullets) {
		t.Error("number button should be the only active list button")
	}
	wysTap(numbers)
	wysHasNot(t, w, "<ol>", "<li>")

	wysTap(quote)
	wysHas(t, w, "<blockquote>")
	if !high(quote) {
		t.Error("quote button inactive inside a quote")
	}
	wysTap(quote)
	wysHasNot(t, w, "<blockquote>")
	if high(quote) {
		t.Error("quote button still active")
	}
}

func TestWyUIBackspaceLeavesListItem(t *testing.T) {
	w, _, _ := wysSetup(t)
	w.SetHTML("<p>x</p><ul><li>a</li></ul>")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	wysHas(t, w, "<p>a</p>")
	wysHasNot(t, w, "<li>")
}

func TestWyUIBackspaceFirstParagraph(t *testing.T) {
	for _, c := range []struct{ in, want, not string }{
		{"<ul><li>a</li></ul>", "<p>a</p>", "<li>"},
		{"<ol><li>a</li></ol>", "<p>a</p>", "<li>"},
		{"<blockquote><p>a</p></blockquote>", "<p>a</p>", "<blockquote>"},
		{"<p>a</p>", "<p>a</p>", ""},
		{"<ul><li>ab</li></ul>", "<ul><li>b</li></ul>", ""}, // selection: delete only
	} {
		w, _, _ := wysSetup(t)
		w.SetHTML(c.in)
		w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
		if c.in == "<ul><li>ab</li></ul>" {
			w.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
			w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
			w.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
		}
		w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
		wysHas(t, w, c.want)
		if c.not != "" {
			wysHasNot(t, w, c.not)
		}
	}
}

func TestWyUILinkDialogApplyCancelRemove(t *testing.T) {
	w, bar, win := wysSetup(t)
	w.SetHTML("<p>go here now</p>")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	w.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	for range 4 {
		w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	}
	w.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})

	link := wysButton(t, bar, "Link")
	wysTap(link)
	ov := wysOverlay(t, win)
	var entry *widget.Entry
	wysWalk(ov, func(o fyne.CanvasObject) {
		if e, ok := o.(*escEntry); ok {
			entry = &e.Entry
		}
	})
	if entry == nil {
		t.Fatal("link dialog has no URL entry")
	}
	// Cancel changes nothing.
	test.Tap(wysButton(t, ov, "Cancel"))
	wysHasNot(t, w, "<a ")
	if win.Canvas().Overlays().Top() != nil {
		t.Fatal("dialog still open after Cancel")
	}

	wysTap(link)
	ov = wysOverlay(t, win)
	wysWalk(ov, func(o fyne.CanvasObject) {
		if e, ok := o.(*escEntry); ok {
			entry = &e.Entry
		}
	})
	entry.SetText("https://example.com/a?b=1&c=2")
	test.Tap(wysButton(t, ov, "Apply"))
	wysHas(t, w, `<p>go <a href="https://example.com/a?b=1&amp;c=2">here</a> now</p>`)
	if win.Canvas().Focused() != fyne.Focusable(w) {
		t.Error("focus not returned to editor after the dialog")
	}

	// Caret inside the link: the toolbar shows it and Ctrl+K prefills the URL.
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	for range 5 {
		w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	}
	if link.Importance != widget.HighImportance {
		t.Error("Link button not highlighted inside a link")
	}
	wysCtrl(w, fyne.KeyK)
	ov = wysOverlay(t, win)
	wysWalk(ov, func(o fyne.CanvasObject) {
		if e, ok := o.(*escEntry); ok {
			entry = &e.Entry
		}
	})
	if entry.Text != "https://example.com/a?b=1&c=2" {
		t.Errorf("prefilled URL %q", entry.Text)
	}
	entry.SetText("")
	test.Tap(wysButton(t, ov, "Apply"))
	wysHasNot(t, w, "<a ")
	wysHas(t, w, "<p>go here now</p>")
}

func TestWyUILinkDialogRejectsUnsafeScheme(t *testing.T) {
	w, bar, win := wysSetup(t)
	w.SetHTML("<p>text</p>")
	wysSelectAll(w)
	wysTap(wysButton(t, bar, "Link"))
	ov := wysOverlay(t, win)
	wysWalk(ov, func(o fyne.CanvasObject) {
		if e, ok := o.(*escEntry); ok {
			e.SetText("javascript:alert(1)")
		}
	})
	test.Tap(wysButton(t, ov, "Apply"))
	wysHasNot(t, w, "javascript", "<a ")
	var msg bool
	wysWalk(wysOverlay(t, win), func(o fyne.CanvasObject) {
		if l, ok := o.(*widget.Label); ok && strings.Contains(l.Text, "Only http, https and mailto") {
			msg = true
		}
	})
	if !msg {
		t.Error("no explanation shown for a rejected link")
	}
}

func TestWyUIColorMenuAndClear(t *testing.T) {
	w, bar, win := wysSetup(t)
	w.SetHTML("<p>paint</p>")
	wysSelectAll(w)
	wysTap(wysButton(t, bar, "Color"))
	var red *canvas.Text
	wysWalk(wysOverlay(t, win), func(o fyne.CanvasObject) {
		if tx, ok := o.(*canvas.Text); ok && tx.Text == "Red" {
			red = tx
		}
	})
	if red == nil {
		t.Fatal("colour menu has no Red entry")
	}
	test.TapCanvas(win.Canvas(), fyne.CurrentApp().Driver().AbsolutePositionForObject(red).Add(fyne.NewPos(2, 2)))
	wysHas(t, w, "color: #d32f2f")
	if win.Canvas().Focused() != fyne.Focusable(w) {
		t.Error("focus not returned after choosing a colour")
	}

	w.SetHTML(`<p><b><i>messy</i></b> <span style="color: #d32f2f">red</span></p>`)
	wysSelectAll(w)
	wysTap(wysButton(t, bar, "Clear"))
	if got := w.HTML(); got != "<p>messy red</p>" {
		t.Errorf("Clear left %q", got)
	}
}

func TestWyUISizeSelectAndToolbarFollowsCaret(t *testing.T) {
	w, bar, _ := wysSetup(t)
	var sel *widget.Select
	wysWalk(bar, func(o fyne.CanvasObject) {
		if s, ok := o.(*widget.Select); ok {
			sel = s
		}
	})
	if sel == nil {
		t.Fatal("no size select")
	}
	w.SetHTML("<p>normal</p>")
	wysSelectAll(w)
	sel.SetSelected("Large")
	wysHas(t, w, "font-size: 18px")

	w.SetHTML(`<p>plain <span style="font-size: 28px"><b>huge</b></span></p>`)
	b := wysButton(t, bar, "B")
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if sel.Selected != "Normal" || b.Importance == widget.HighImportance {
		t.Errorf("plain text shows size %q bold=%v", sel.Selected, b.Importance == widget.HighImportance)
	}
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if sel.Selected != "Huge" || b.Importance != widget.HighImportance {
		t.Errorf("end of bold huge text shows size %q bold=%v", sel.Selected, b.Importance == widget.HighImportance)
	}
	// Choosing from the select moves the caret size without echoing back as a change.
	changed := 0
	w.OnChanged = func() { changed++ }
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	if changed != 0 {
		t.Errorf("moving the caret reported %d content changes", changed)
	}
}

func TestWyUIOnChangedOnlyForEdits(t *testing.T) {
	w, bar, _ := wysSetup(t)
	changed := 0
	w.OnChanged = func() { changed++ }
	test.Type(w, "ab")
	wysSelectAll(w)
	w.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if changed != 2 {
		t.Fatalf("typing+selection+nav reported %d changes, want 2", changed)
	}
	wysSelectAll(w)
	wysTap(wysButton(t, bar, "B"))
	wysTap(wysButton(t, bar, "• List"))
	if changed != 4 {
		t.Errorf("toolbar commands reported %d changes in total, want 4", changed)
	}
}

func TestWyUIGoldenToolbarAndContent(t *testing.T) {
	w, _, win := wysSetup(t)
	win.Canvas().Unfocus()
	w.SetHTML(`<p>Hello <b>bold</b>, <i>italic</i> and <a href="https://example.com">a link</a>.</p>` +
		`<ul><li>first</li><li>second</li></ul><ol><li>one</li><li>two</li></ol>` +
		`<blockquote><p>quoted text</p></blockquote><p><span style="color: #d32f2f">red</span> ` +
		`<span style="font-size: 18px">large</span></p>`)
	golden(t, "wysiwyg_toolbar_content", win.Canvas().Capture())
}

func TestF3LinkDialogFocusesURLAndEscCloses(t *testing.T) {
	w, bar, win := wysSetup(t)
	w.SetHTML("<p>text</p>")
	wysSelectAll(w)
	wysTap(wysButton(t, bar, "Link"))
	ov := wysOverlay(t, win)
	var entry *escEntry
	wysWalk(ov, func(o fyne.CanvasObject) {
		if e, ok := o.(*escEntry); ok {
			entry = e
		}
	})
	if entry == nil || win.Canvas().Focused() != fyne.Focusable(entry) {
		t.Fatalf("URL field not focused: %v", win.Canvas().Focused())
	}
	test.Type(entry, "https://a.example")
	if entry.Text != "https://a.example" {
		t.Fatalf("typing went elsewhere: %q", entry.Text)
	}
	win.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if win.Canvas().Overlays().Top() != nil {
		t.Fatal("Esc must close the link dialog")
	}
	wysHasNot(t, w, "<a ")
	if win.Canvas().Focused() != fyne.Focusable(w) {
		t.Error("focus not returned to the editor")
	}
}

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestF4EscEntryEscapeRunsClose(t *testing.T) {
	test.NewApp()
	closed := 0
	e := newEscEntry(func() { closed++ })
	e.SetText("abc")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if closed != 1 || e.Text != "abc" {
		t.Fatalf("closed=%d text=%q", closed, e.Text)
	}
}

func TestF4EscEntryOtherKeysPassThrough(t *testing.T) {
	test.NewApp()
	closed := 0
	e := newEscEntry(func() { closed++ })
	test.Type(e, "hi")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) // single-line entry: Enter is not Escape and must not close
	if closed != 0 || e.Text != "h" {
		t.Fatalf("closed=%d text=%q", closed, e.Text)
	}
	submitted := ""
	e.OnSubmitted = func(s string) { submitted = s }
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if submitted != "h" || closed != 0 {
		t.Fatalf("Enter must reach the embedded Entry: submitted=%q closed=%d", submitted, closed)
	}
}

func TestF4EscEntryLinkDialogClosesOnEscape(t *testing.T) {
	w, _, win := wysSetup(t)
	w.SetHTML("<p>word</p>")
	w.EditLink()
	ov := wysOverlay(t, win)
	var entry *escEntry
	wysWalk(ov, func(o fyne.CanvasObject) {
		if e, ok := o.(*escEntry); ok {
			entry = e
		}
	})
	if entry == nil || win.Canvas().Focused() != fyne.Focusable(entry) {
		t.Fatal("link dialog has no focused escEntry")
	}
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if win.Canvas().Overlays().Top() != nil {
		t.Fatal("dialog still open after Escape")
	}
	if win.Canvas().Focused() != fyne.Focusable(w) {
		t.Fatal("focus not returned to the editor")
	}
}

func TestF4EscEntryContactsSearchClosesWindow(t *testing.T) {
	u, _ := winContactsUI(t)
	es := winFind[*escEntry](u.contactsWin.Content())
	if len(es) == 0 {
		t.Fatal("contacts window has no escEntry search field")
	}
	es[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if u.contactsWin != nil {
		t.Fatal("contacts window still open after Escape")
	}
}

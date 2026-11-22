package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// contactLabel is how a contact is shown and inserted: "Name <email>" or the bare address.
func contactLabel(c desktop.ContactItem) string {
	if c.Name != "" {
		return c.Name + " <" + c.Email + ">"
	}
	return c.Email
}

// suggester is a contact dropdown with keyboard selection (Down/Up move, Enter or Tab
// accept, Escape closes), shared by the recipient fields and @mentions in the body.
type suggester struct {
	canvas fyne.Canvas
	pick   func(desktop.ContactItem)
	items  []desktop.ContactItem
	sel    int
	popup  *widget.PopUp
}

func (s *suggester) active() bool { return s.popup != nil }

func (s *suggester) hide() {
	if s.popup != nil {
		s.popup.Hide()
		s.popup = nil
	}
	s.items = nil
}

// show lists items in a dropdown under the anchor, with the first one selected.
func (s *suggester) show(anchor fyne.CanvasObject, items []desktop.ContactItem) {
	s.hide()
	if len(items) == 0 {
		return
	}
	s.items, s.sel = items, 0
	s.render(anchor)
}

func (s *suggester) render(anchor fyne.CanvasObject) {
	if s.popup != nil {
		s.popup.Hide()
	}
	list := container.NewVBox()
	for i, c := range s.items {
		c := c
		b := widget.NewButton(contactLabel(c), func() { s.accept(c) })
		b.Alignment = widget.ButtonAlignLeading
		b.Importance = widget.LowImportance
		if i == s.sel {
			b.Importance = widget.MediumImportance
		}
		list.Add(b)
	}
	s.popup = widget.NewPopUp(list, s.canvas)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	s.popup.ShowAtPosition(pos.Add(fyne.NewPos(0, anchor.Size().Height)))
	s.popup.Resize(fyne.NewSize(anchor.Size().Width, list.MinSize().Height))
}

func (s *suggester) accept(c desktop.ContactItem) {
	s.hide()
	s.pick(c)
}

// key handles a key press while the dropdown is open and reports whether it was consumed.
func (s *suggester) key(k fyne.KeyName, anchor fyne.CanvasObject) bool {
	if !s.active() {
		return false
	}
	switch k {
	case fyne.KeyDown:
		s.sel = (s.sel + 1) % len(s.items)
	case fyne.KeyUp:
		s.sel = (s.sel + len(s.items) - 1) % len(s.items)
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyTab:
		s.accept(s.items[s.sel])
		return true
	case fyne.KeyEscape:
		s.hide()
		return true
	default:
		return false
	}
	s.render(anchor)
	return true
}

// suggestEntry is a single-line entry whose key presses go to its suggester first.
type suggestEntry struct {
	widget.Entry
	sg *suggester
	// quiet suppresses suggestions for programmatic SetText calls.
	quiet bool
}

// setQuietly sets the text without triggering suggestions.
func (e *suggestEntry) setQuietly(s string) {
	e.quiet = true
	e.SetText(s)
	e.quiet = false
}

func newSuggestEntry(sg *suggester) *suggestEntry {
	e := &suggestEntry{sg: sg}
	e.ExtendBaseWidget(e)
	return e
}

func (e *suggestEntry) TypedKey(k *fyne.KeyEvent) {
	if !e.sg.key(k.Name, e) {
		e.Entry.TypedKey(k)
	}
}

// AcceptsTab lets Tab reach TypedKey (to accept a suggestion) only while the dropdown is open.
func (e *suggestEntry) AcceptsTab() bool { return e.sg.active() }

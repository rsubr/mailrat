package ui

import (
	"strings"

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

// contactInsert is what is typed into a recipient field for c: like the label,
// but with a display name that contains address specials (a comma, say) quoted
// so the field still parses as one address.
func contactInsert(c desktop.ContactItem) string {
	if c.Name == "" {
		return c.Email
	}
	name := c.Name
	if strings.ContainsAny(name, `()<>[]:;@\,"`) {
		name = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name) + `"`
	}
	return name + " <" + c.Email + ">"
}

// suggestHost is the inline strip of suggestions shown in the window layout. It is
// deliberately not a popup/overlay: Fyne gives an overlay its own focus manager, so
// the moment a PopUp opened the entry lost keyboard focus and typing stopped.
type suggestHost struct {
	box *fyne.Container
	cur *suggester
}

func newSuggestHost() *suggestHost {
	h := &suggestHost{box: container.NewVBox()}
	h.box.Hide()
	return h
}

// suggester is a contact dropdown with keyboard selection (Down/Up move, Enter or Tab
// accept, Escape closes), shared by the recipient fields and @mentions in the body.
type suggester struct {
	host  *suggestHost
	pick  func(desktop.ContactItem)
	items []desktop.ContactItem
	sel   int
}

func (s *suggester) active() bool { return len(s.items) > 0 }

func (s *suggester) hide() {
	if s.items != nil && s.host.cur == s {
		s.host.box.Objects = nil
		s.host.box.Hide()
		s.host.cur = nil
	}
	s.items = nil
}

// show lists items in the host strip, with the first one selected.
func (s *suggester) show(items []desktop.ContactItem) {
	s.hide()
	if len(items) == 0 {
		return
	}
	if c := s.host.cur; c != nil && c != s {
		c.hide()
	}
	s.host.cur = s
	s.items, s.sel = items, 0
	s.render()
}

func (s *suggester) render() {
	s.host.box.Objects = nil
	for i, c := range s.items {
		c := c
		b := widget.NewButton(contactLabel(c), func() { s.accept(c) })
		b.Alignment = widget.ButtonAlignLeading
		b.Importance = widget.LowImportance
		if i == s.sel {
			b.Importance = widget.MediumImportance
		}
		s.host.box.Add(b)
	}
	s.host.box.Show()
	s.host.box.Refresh()
}

func (s *suggester) accept(c desktop.ContactItem) {
	s.hide()
	s.pick(c)
}

// key handles a key press while the dropdown is open and reports whether it was consumed.
func (s *suggester) key(k fyne.KeyName) bool {
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
	s.render()
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
	if !e.sg.key(k.Name) {
		e.Entry.TypedKey(k)
	}
}

// AcceptsTab lets Tab reach TypedKey (to accept a suggestion) only while the dropdown is open.
func (e *suggestEntry) AcceptsTab() bool { return e.sg.active() }

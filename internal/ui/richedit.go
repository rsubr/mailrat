package ui

import (
	"regexp"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// bodyEntry is the compose body: a multi-line entry that offers @/+ mention
// suggestions and carries the formatting operations used by the toolbar.
type bodyEntry struct {
	widget.Entry
	sg *suggester
	// anchor is the object the mention dropdown opens under (Fyne exposes no caret position).
	anchor fyne.CanvasObject
}

func newBodyEntry(sg *suggester) *bodyEntry {
	e := &bodyEntry{sg: sg}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.ExtendBaseWidget(e)
	return e
}

func (e *bodyEntry) TypedKey(k *fyne.KeyEvent) {
	if !e.sg.key(k.Name, e.anchor) {
		e.Entry.TypedKey(k)
	}
}

// AcceptsTab is true only while the mention dropdown is open; otherwise Tab moves focus.
func (e *bodyEntry) AcceptsTab() bool { return e.sg.active() }

type memClipboard struct{ s string }

func (c *memClipboard) Content() string     { return c.s }
func (c *memClipboard) SetContent(s string) { c.s = s }

// insert replaces the selection (if any) with s and leaves the cursor after it.
func (e *bodyEntry) insert(s string) {
	e.TypedShortcut(&fyne.ShortcutPaste{Clipboard: &memClipboard{s}})
}

// wrap surrounds the selection with open/close; with no selection it inserts an
// empty pair and puts the cursor between the tags.
func (e *bodyEntry) wrap(open, close string) {
	sel := e.SelectedText()
	e.insert(open + sel + close)
	if sel == "" {
		for range []rune(close) {
			e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
		}
	}
}

// prefixLines starts the selected lines (or the current one) with prefix.
func (e *bodyEntry) prefixLines(prefix string) {
	if sel := e.SelectedText(); sel != "" {
		lines := strings.Split(sel, "\n")
		for i, l := range lines {
			lines[i] = prefix + l
		}
		e.insert(strings.Join(lines, "\n"))
		return
	}
	before := []rune(e.Text)[:e.CursorTextOffset()]
	if n := len(before); n > 0 && before[n-1] != '\n' {
		prefix = "\n" + prefix
	}
	e.insert(prefix)
}

// clearFormatting strips markup from the selection.
func (e *bodyEntry) clearFormatting() {
	if sel := e.SelectedText(); sel != "" {
		e.insert(stripMarkup(sel))
	}
}

// mentionAt finds an unfinished @name or +name just before the cursor. The trigger
// must start the text or follow whitespace, as in Gmail.
func mentionAt(text []rune, cursor int) (query string, ok bool) {
	if cursor > len(text) {
		cursor = len(text)
	}
	for i := cursor - 1; i >= 0; i-- {
		r := text[i]
		if r == '@' || r == '+' {
			if i == 0 || unicode.IsSpace(text[i-1]) {
				return string(text[i+1 : cursor]), true
			}
			return "", false
		}
		if unicode.IsSpace(r) {
			return "", false
		}
	}
	return "", false
}

var addrRe = regexp.MustCompile(`[^\s<>,;"]+@[^\s<>,;"]+`)

// addRecipient returns the To field with c appended unless c is already in To or Cc.
func addRecipient(to, cc string, c desktop.ContactItem) (string, bool) {
	have := strings.ToLower(to + " " + cc)
	for _, a := range addrRe.FindAllString(have, -1) {
		if a == strings.ToLower(c.Email) {
			return to, false
		}
	}
	to = strings.TrimRight(strings.TrimSpace(to), ", ")
	if to == "" {
		return contactLabel(c), true
	}
	return to + ", " + contactLabel(c), true
}

// mentionMarkup is the body text inserted for a picked contact: a mailto link.
func mentionMarkup(c desktop.ContactItem) string {
	name := c.Name
	if name == "" {
		name, _, _ = strings.Cut(c.Email, "@")
	}
	name = strings.NewReplacer("[", "", "]", "").Replace(name)
	return "[url=mailto:" + c.Email + "]@" + name + "[/url] "
}

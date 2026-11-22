package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

func TestSplitRecipient(t *testing.T) {
	cases := []struct{ in, head, token string }{
		{"ali", "", "ali"},
		{"a@x.com, bo", "a@x.com, ", "bo"},
		{"A <a@x.com>,  ", "A <a@x.com>, ", ""},
	}
	for _, c := range cases {
		h, tok := splitRecipient(c.in)
		if h != c.head || tok != c.token {
			t.Errorf("splitRecipient(%q) = %q,%q want %q,%q", c.in, h, tok, c.head, c.token)
		}
	}
}

func TestSuggesterKeyboard(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("t")
	var picked []string
	sg := &suggester{host: newSuggestHost(), pick: func(c desktop.ContactItem) { picked = append(picked, c.Email) }}
	e := newSuggestEntry(sg)
	w.SetContent(e)
	items := []desktop.ContactItem{{Name: "Ann", Email: "ann@x.com"}, {Email: "bob@x.com"}}

	if sg.key(fyne.KeyDown) {
		t.Fatal("key consumed while closed")
	}
	sg.show(items)
	if !sg.active() || !e.AcceptsTab() {
		t.Fatal("dropdown should be open and accept Tab")
	}
	sg.key(fyne.KeyDown)
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(picked) != 1 || picked[0] != "bob@x.com" || sg.active() {
		t.Fatalf("picked %v active=%v", picked, sg.active())
	}
	sg.show(items)
	sg.key(fyne.KeyUp) // wraps to last
	sg.key(fyne.KeyTab)
	if picked[1] != "bob@x.com" {
		t.Fatalf("Up should wrap to last, got %v", picked)
	}
	sg.show(items)
	sg.key(fyne.KeyEscape)
	if sg.active() {
		t.Fatal("Escape should close")
	}
	if contactLabel(items[0]) != "Ann <ann@x.com>" || contactLabel(items[1]) != "bob@x.com" {
		t.Fatal("bad label")
	}
}

// Regression: suggestions used to open in an overlay popup, which took keyboard
// focus from the entry so nothing could be typed after the second character.
func TestRecipientTypingKeepsFocusWithSuggestions(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("t")
	u := &UI{inline: true, be: &fakeBackend{found: []desktop.ContactItem{{Name: "Raja", Email: "raja@x.com"}}}}
	host := newSuggestHost()
	e := u.recipientEntry(host, func() string { return "" }, "To")
	w.SetContent(e)
	w.Canvas().Focus(e)
	test.Type(e, "rajaram")
	if e.Text != "rajaram" {
		t.Fatalf("typing blocked: %q", e.Text)
	}
	if w.Canvas().Focused() != fyne.Focusable(e) || !host.box.Visible() {
		t.Fatal("entry lost focus or suggestions hidden")
	}
}

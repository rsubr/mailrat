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
	sg := &suggester{canvas: w.Canvas(), pick: func(c desktop.ContactItem) { picked = append(picked, c.Email) }}
	e := newSuggestEntry(sg)
	w.SetContent(e)
	items := []desktop.ContactItem{{Name: "Ann", Email: "ann@x.com"}, {Email: "bob@x.com"}}

	if sg.key(fyne.KeyDown, e) {
		t.Fatal("key consumed while closed")
	}
	sg.show(e, items)
	if !sg.active() || !e.AcceptsTab() {
		t.Fatal("dropdown should be open and accept Tab")
	}
	sg.key(fyne.KeyDown, e)
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(picked) != 1 || picked[0] != "bob@x.com" || sg.active() {
		t.Fatalf("picked %v active=%v", picked, sg.active())
	}
	sg.show(e, items)
	sg.key(fyne.KeyUp, e) // wraps to last
	sg.key(fyne.KeyTab, e)
	if picked[1] != "bob@x.com" {
		t.Fatalf("Up should wrap to last, got %v", picked)
	}
	sg.show(e, items)
	sg.key(fyne.KeyEscape, e)
	if sg.active() {
		t.Fatal("Escape should close")
	}
	if contactLabel(items[0]) != "Ann <ann@x.com>" || contactLabel(items[1]) != "bob@x.com" {
		t.Fatal("bad label")
	}
}

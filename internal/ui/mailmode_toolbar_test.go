package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// The mail-mode toggle must be a real, visible, tappable button at the top
// right of the rendered reader toolbar, found by walking the window content.
func TestMailToggleRenderedRightmostInWindow(t *testing.T) {
	u := mmOpen(t)
	d := fyne.CurrentApp().Driver()
	win := u.win.Canvas().Size()
	type item struct {
		tip string
		b   *tooltipButton
	}
	collect := func() (items []item, search fyne.CanvasObject) {
		keysWalk(u.win.Content(), func(o fyne.CanvasObject) {
			if b, ok := o.(*tooltipButton); ok && d.AbsolutePositionForObject(b).X > u.sidebar.Size().Width+u.list.Size().Width {
				items = append(items, item{b.tip, b})
			}
			if o == fyne.CanvasObject(u.search) {
				search = o
			}
		})
		return
	}
	items, search := collect()
	if search == nil {
		t.Fatal("search field not reachable from window content")
	}
	var tips []string
	for _, it := range items {
		tips = append(tips, it.tip)
	}
	want := []string{"Archive", "Trash", "Mark unread", "Spam", "Star", "Labels", "Switch mail to dark mode"}
	toolbar := []string{}
	for _, tip := range tips {
		if slices.Contains(want, tip) {
			toolbar = append(toolbar, tip)
		}
	}
	if !slices.Equal(toolbar, want) {
		t.Fatalf("toolbar order %v, want %v (all %v)", toolbar, want, tips)
	}
	var toggle *tooltipButton
	sp := d.AbsolutePositionForObject(search)
	for _, it := range items {
		bp := d.AbsolutePositionForObject(it.b)
		sz := it.b.Size()
		if !it.b.Visible() || sz.Width <= 0 || sz.Height <= 0 || bp.X < 0 || bp.Y < 0 || bp.X+sz.Width > win.Width || bp.Y+sz.Height > win.Height {
			t.Errorf("%q not visible inside window %v: pos %v size %v", it.tip, win, bp, sz)
		}
		if it.tip == "Switch mail to dark mode" {
			toggle = it.b
			continue
		}
		if slices.Contains(want, it.tip) && bp.X > sp.X {
			t.Errorf("%q is right of the search field", it.tip)
		}
	}
	tp := d.AbsolutePositionForObject(toggle)
	if tp.X < sp.X+search.Size().Width {
		t.Errorf("toggle %v not right of search %v+%v", tp, sp, search.Size())
	}
	for _, it := range items {
		if d.AbsolutePositionForObject(it.b).X > tp.X {
			t.Errorf("%q is right of the mail toggle", it.tip)
		}
	}
	if toggle.Disabled() {
		t.Error("toggle must never be disabled")
	}
	test.Tap(toggle)
	if u.mailLight() {
		t.Fatal("tap did not switch mail to dark")
	}
	items, _ = collect()
	last := items[len(items)-1]
	if last.tip != "Switch mail to light mode" {
		t.Fatalf("after tap right-most is %q", last.tip)
	}
	if isDark() {
		t.Fatal("app theme must be unchanged")
	}
}

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

// tlAbs returns target's absolute rect inside root (root at 0,0), or false.
func tlAbs(root, target fyne.CanvasObject) (fyne.Position, fyne.Size, bool) {
	var find func(o fyne.CanvasObject, off fyne.Position) (fyne.Position, bool)
	find = func(o fyne.CanvasObject, off fyne.Position) (fyne.Position, bool) {
		p := off.Add(o.Position())
		if o == target {
			return p, true
		}
		var kids []fyne.CanvasObject
		switch c := o.(type) {
		case *fyne.Container:
			kids = c.Objects
		case fyne.Widget:
			kids = test.WidgetRenderer(c).Objects()
		}
		for _, k := range kids {
			if q, ok := find(k, p); ok {
				return q, true
			}
		}
		return fyne.Position{}, false
	}
	p, ok := find(root, fyne.Position{})
	return p, target.Size(), ok
}

// findSplit returns the first *container.Split under o.
func findSplit(o fyne.CanvasObject) *container.Split {
	if sp, ok := o.(*container.Split); ok {
		return sp
	}
	var kids []fyne.CanvasObject
	switch c := o.(type) {
	case *fyne.Container:
		kids = c.Objects
	case fyne.Widget:
		kids = test.WidgetRenderer(c).Objects()
	}
	for _, k := range kids {
		if sp := findSplit(k); sp != nil {
			return sp
		}
	}
	return nil
}

// The list pane's minimum width must be enough for the widest row layout
// (star, clip, date and a long sender), at any window size.
func TestThreadRowFitsListPaneMin(t *testing.T) {
	u, _ := navUI(t)
	item := desktop.ThreadItem{ID: "x", Participants: "Alexander Hamilton-Washington <a@x.com>, Bob <b@x.com>", Subject: "Hello", Snippet: "world", Date: msAt(1), IsUnread: true, IsStarred: true, HasAttachments: true, MessageCount: 3}
	row := newThreadRow(false)
	u.setRow(row, item)
	sp := findSplit(u.win.Content())
	if sp == nil {
		t.Fatal("no split in main window")
	}
	pane := sp.Leading.MinSize().Width
	rowMin := row.MinSize().Width
	if rowMin > pane {
		t.Fatalf("row min width %v exceeds list pane min %v", rowMin, pane)
	}
	if rowMin > listMinWidth {
		t.Fatalf("row min width %v exceeds listMinWidth %v", rowMin, listMinWidth)
	}
	w := pane
	newWindow(t, row, w, 80)
	row.Resize(fyne.NewSize(w, 80))
	hp, hs, _ := tlAbs(row, row.head)
	if hs.Width < 0 {
		t.Errorf("sender width %v negative", hs.Width)
	}
	for name, o := range map[string]fyne.CanvasObject{"star": row.star, "clip": row.clip, "when": row.when} {
		p, s, ok := tlAbs(row, o)
		if !ok {
			t.Errorf("%s not found", name)
		}
		if !o.Visible() {
			continue
		}
		if p.X < hp.X+hs.Width-0.5 {
			t.Errorf("%s at x=%v overlaps sender ending %v", name, p.X, hp.X+hs.Width)
		}
		if p.X+s.Width > w+0.5 {
			t.Errorf("%s right edge %v beyond pane min %v", name, p.X+s.Width, w)
		}
	}
}

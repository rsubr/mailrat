package ui

import (
	"errors"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// g2Buttons lists the reader toolbar's tooltip buttons in left-to-right order.
func g2Buttons(u *UI) (out []*tooltipButton) {
	keysWalk(u.reader, func(o fyne.CanvasObject) {
		if b, ok := o.(*tooltipButton); ok {
			out = append(out, b)
		}
	})
	return out
}

func g2Tips(u *UI) (tips []string) {
	for _, b := range g2Buttons(u) {
		tips = append(tips, b.tip)
	}
	return tips
}

func g2Find(t *testing.T, u *UI, tip string) *tooltipButton {
	t.Helper()
	for _, b := range g2Buttons(u) {
		if b.tip == tip {
			return b
		}
	}
	t.Fatalf("no %q button in %v", tip, g2Tips(u))
	return nil
}

func TestG2ToolbarOrderAndDisabled(t *testing.T) {
	u, _ := f1UI(t)
	tips := g2Tips(u)
	if len(tips) < 3 || tips[len(tips)-1] != "Switch mail to light mode" && tips[len(tips)-1] != "Switch mail to dark mode" {
		t.Fatalf("mail toggle must stay right-most: %v", tips)
	}
	si, li := slices.Index(tips, "Star"), slices.Index(tips, "Labels")
	if si < 0 || li != si+1 || si < slices.Index(tips, "Spam") {
		t.Fatalf("want Spam, Star, Labels in order: %v", tips)
	}
	for _, tip := range []string{"Star", "Labels", "Archive"} {
		if !g2Find(t, u, tip).Disabled() {
			t.Fatalf("%s must be disabled with no conversation open", tip)
		}
	}
	f1Open(u, "t3")
	for _, tip := range []string{"Star", "Labels", "Archive"} {
		if g2Find(t, u, tip).Disabled() {
			t.Fatalf("%s must be enabled with a conversation open", tip)
		}
	}
}

func TestG2StarToggleFlipsIconAndRow(t *testing.T) {
	u, fb := f1UI(t)
	f1Open(u, "t3")
	star := g2Find(t, u, "Star")
	before := star.Icon
	test.Tap(star)
	if !slices.Contains(fb.labelled, "t3:STARRED:true") || !u.selected.IsStarred {
		t.Fatalf("star not applied: %v", fb.labelled)
	}
	un := g2Find(t, u, "Unstar")
	if un != star || un.Icon == before {
		t.Fatalf("icon/tooltip must flip: %v", g2Tips(u))
	}
	rowStarred := false
	for _, th := range u.threads {
		if th.ID == "t3" {
			rowStarred = th.IsStarred
		}
	}
	if !rowStarred {
		t.Fatal("list row must show the star")
	}
	test.Tap(un)
	if !slices.Contains(fb.labelled, "t3:STARRED:false") || g2Find(t, u, "Star") != star {
		t.Fatalf("unstar: %v %v", fb.labelled, g2Tips(u))
	}
}

func TestG2StarOptimisticAndRollback(t *testing.T) {
	u, fb := f1UI(t)
	f1Open(u, "t3")
	seen := false
	fb.hook = map[string]func(){}
	u.be.(*f1Backend).fakeBackend.errs = map[string]error{"SetThreadLabel": errors.New("boom")}
	// optimistic: while the call is in flight the row is already starred
	orig := u.be
	u.be = &g2Probe{Backend: orig, during: func() { seen = u.selected.IsStarred && u.starBtn.tip == "Unstar" }}
	test.Tap(g2Find(t, u, "Star"))
	if !seen {
		t.Fatal("star must flip before the backend answers")
	}
	if u.selected.IsStarred || u.starBtn.tip != "Star" {
		t.Fatal("failed call must roll the star back")
	}
	for _, th := range u.threads {
		if th.ID == "t3" && th.IsStarred {
			t.Fatal("row must roll back")
		}
	}
	if u.win.Canvas().Overlays().Top() == nil {
		t.Fatal("error dialog expected on the main window")
	}
}

type g2Probe struct {
	Backend
	during func()
}

func (p *g2Probe) SetThreadLabel(a, id, l string, on bool) error {
	p.during()
	return p.Backend.SetThreadLabel(a, id, l, on)
}

func TestG2LabelButtonOpensPickerPreticked(t *testing.T) {
	u, fb := f1UI(t)
	f1Open(u, "t1") // carries L1 only
	test.Tap(g2Find(t, u, "Labels"))
	checks := keysChecks(u)
	if c := checks["Inquiries"]; c == nil || !c.Checked {
		t.Fatalf("open thread's label must be pre-ticked: %v", checks)
	}
	if c := checks["Travel"]; c == nil || c.Checked {
		t.Fatal("other labels must start unticked")
	}
	fb.labelled = nil
	test.Tap(checks["Travel"])
	test.Tap(keysButton(t, u, "Apply"))
	if !slices.Contains(fb.labelled, "t1:L2:true") {
		t.Fatalf("apply: %v", fb.labelled)
	}
}

func TestG2ToolbarFitsReaderPane(t *testing.T) {
	u, _ := f1UI(t)
	f1Open(u, "t3")
	tb := u.toolbar()
	t.Logf("toolbar min %v reader %v", tb.MinSize().Width, u.reader.Size().Width)
	if min, w := tb.MinSize().Width, u.reader.Size().Width; min > w {
		t.Fatalf("reader toolbar min width %v exceeds reader pane %v", min, w)
	}
	if got := u.reader.MinSize().Width; got > u.reader.Size().Width {
		t.Fatalf("reader min %v exceeds pane %v", got, u.reader.Size().Width)
	}
}

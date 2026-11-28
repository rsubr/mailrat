package ui

import (
	"testing"

	"fyne.io/fyne/v2"
)

// The list pane header and the reader toolbar share topBarHeight, so their
// separator lines meet across the panes; their content must fit inside it.
func TestTopBarsAligned(t *testing.T) {
	u, _, _ := testUI(t)
	u.list.Select(2)
	for name, bar := range map[string]fyne.CanvasObject{"reader toolbar": u.toolbar(), "list header": u.listHeader().(*fyne.Container).Objects[0]} {
		c := bar.(*fyne.Container)
		if h := c.MinSize().Height; h != topBarHeight {
			t.Errorf("%s height %v, want %v", name, h, topBarHeight)
		}
		if h := c.Objects[0].MinSize().Height; h+c.Objects[1].MinSize().Height > topBarHeight {
			t.Errorf("%s content %v does not fit in %v", name, h, topBarHeight)
		}
	}
}

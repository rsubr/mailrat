package ui

import "image/color"

var starGold = color.NRGBA{0xff, 0xc1, 0x07, 0xff} // as the list row star

// starButton builds the reader toolbar's star toggle; syncStarButton keeps it
// in step with the open conversation.
func (u *UI) starButton() *tooltipButton {
	u.starBtn = iconButton("star", "Star", u.toggleStar)
	u.syncStarButton()
	return u.starBtn
}

func (u *UI) syncStarButton() {
	b := u.starBtn
	if b == nil {
		return
	}
	if u.selected != nil && u.selected.IsStarred {
		b.SetIcon(icon("starfill", starGold))
		b.tip = "Unstar"
	} else {
		b.SetIcon(icon("star", fg()))
		b.tip = "Star"
	}
}

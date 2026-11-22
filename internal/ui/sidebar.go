package ui

import (
	"image/color"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// Labels never listed in the sidebar (tabs and legacy star variants).
var hiddenLabels = map[string]bool{
	"UNREAD": true, "CATEGORY_PERSONAL": true, "CATEGORY_SOCIAL": true,
	"CATEGORY_PROMOTIONS": true, "CATEGORY_UPDATES": true, "CATEGORY_FORUMS": true,
	"YELLOW_STAR": true, "BLUE_STAR": true, "RED_STAR": true, "PURPLE_STAR": true,
	"GREEN_STAR": true, "RED_BANGBANG": true, "ORANGE_GUILLEMET": true,
	"GREEN_CHECK": true, "BLUE_INFO": true, "PURPLE_QUESTION": true,
	"GREEN_CIRCLE": true, "ORANGE_CIRCLE": true, "RED_CIRCLE": true, "PURPLE_CIRCLE": true,
	"CHAT": true,
}

var (
	favoriteOrder = []string{"STARRED", "DRAFT", "SENT", "IMPORTANT", "ALL_MAIL", "SPAM", "TRASH"}
)

// mailboxes returns the sidebar's "Favorites" entries followed by user labels.
func (u *UI) mailboxes() (favorites, user []desktop.LabelItem) {
	byID := map[string]desktop.LabelItem{}
	for _, l := range u.labels {
		byID[l.ID] = l
	}
	seen := map[string]bool{"INBOX": true}
	for _, id := range favoriteOrder {
		seen[id] = true
		l, ok := byID[id]
		if !ok {
			if id == "IMPORTANT" || id == "ALL_MAIL" {
				continue // only show these if the account has them
			}
			l = desktop.LabelItem{ID: id, Name: u.labelName(id), Type: "system"}
		}
		favorites = append(favorites, l)
	}
	for _, l := range u.labels {
		switch {
		case seen[l.ID] || hiddenLabels[l.ID]:
		case l.Type == "user":
			user = append(user, l)
		default:
			favorites = append(favorites, l)
		}
	}
	sort.SliceStable(user, func(i, j int) bool { return strings.ToLower(user[i].Name) < strings.ToLower(user[j].Name) })
	return
}

func (u *UI) refreshSidebar() {
	if u.sidebar == nil {
		return
	}
	u.sidebar.Objects = []fyne.CanvasObject{u.buildSidebar()}
	u.sidebar.Refresh()
}

func (u *UI) buildSidebar() fyne.CanvasObject {
	bg := canvas.NewRectangle(col(colorSidebar))

	compose := iconButton("compose", "New message", func() { u.compose(desktop.OutboundMsg{}) })
	top := container.NewBorder(nil, nil, nil, compose, spacer(1, 1))

	tiles := []fyne.CanvasObject{u.accountTile(unified, "Unified", "inbox", color.NRGBA{0x0a, 0x84, 0xff, 0xff})}
	for _, a := range u.accounts {
		name := a.DisplayName
		if name == "" {
			name = strings.SplitN(a.Email, "@", 2)[0]
		}
		tiles = append(tiles, u.accountTile(a.ID, name, "", avatarColor(a.Email)))
	}
	tileGrid := container.NewGridWithColumns(2, tiles...)

	favs, user := u.mailboxes()
	nav := []fyne.CanvasObject{sectionHeader("Inboxes"), u.navRow(desktop.LabelItem{ID: "INBOX", Name: u.labelName("INBOX"), Type: "system"})}
	nav = append(nav, sectionHeader("Favorites"))
	for _, l := range favs {
		nav = append(nav, u.navRow(l))
	}
	if len(user) > 0 {
		nav = append(nav, sectionHeader("Labels"))
		for _, l := range user {
			nav = append(nav, u.navRow(l))
		}
	}
	scroll := container.NewVScroll(container.NewVBox(nav...))

	head := container.NewVBox(container.NewPadded(top), container.NewPadded(tileGrid))
	body := container.NewBorder(head, u.sidebarFooter(), nil, nil, scroll)
	return container.NewStack(bg, body)
}

func (u *UI) accountTile(id, name, glyph string, tint color.Color) fyne.CanvasObject {
	var badge fyne.CanvasObject
	if glyph != "" {
		img := canvas.NewImageFromResource(icon(glyph, color.White))
		img.FillMode = canvas.ImageFillContain
		badge = container.NewGridWrap(fyne.NewSize(26, 26), container.NewStack(canvas.NewCircle(tint), container.NewPadded(img)))
	} else {
		badge = avatar(initials(name), 26, tint)
	}
	label := widget.NewLabel(name)
	label.Truncation = fyne.TextTruncateEllipsis
	content := container.NewVBox(container.NewHBox(badge), label)
	return newTapRow(content, u.activeAccount == id, func() { u.selectAccount(id) })
}

func (u *UI) navRow(l desktop.LabelItem) fyne.CanvasObject {
	glyph, tint := "tag", subtle()
	if g, ok := labelIcons[l.ID]; ok {
		glyph, tint = g.icon, g.col
	} else if c, ok := parseHex(l.BackgroundColor); ok && l.Type == "user" {
		tint = c
	}
	img := canvas.NewImageFromResource(icon(glyph, tint))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(20, 20))

	name := widget.NewLabel(l.Name)
	name.Truncation = fyne.TextTruncateEllipsis
	var right fyne.CanvasObject = spacer(0, 0)
	if n := u.unread[l.ID]; n > 0 {
		c := canvas.NewText(strconv.Itoa(n), subtle())
		c.TextSize = 13
		right = container.NewCenter(c)
	}
	row := container.NewBorder(nil, nil, container.NewCenter(img), right, name)
	return newTapRow(row, u.activeLabel == l.ID && u.query == "", func() { u.selectLabel(l.ID) })
}

func (u *UI) sidebarFooter() fyne.CanvasObject {
	settings := iconButton("settings", "Settings", u.openSettings)
	status := canvas.NewText(u.syncStatus, subtle())
	status.TextSize = 12
	var items []fyne.CanvasObject
	items = append(items, settings, status, layout.NewSpacer())
	if u.errCount > 0 {
		warn := widget.NewButtonWithIcon(strconv.Itoa(u.errCount), icon("warning", color.NRGBA{0xff, 0x9f, 0x0a, 0xff}), u.showErrorLog)
		warn.Importance = widget.LowImportance
		items = append(items, warn)
	}
	items = append(items, iconButton("sync", "Check for new mail", u.syncNow))
	return container.NewVBox(hline(), container.NewPadded(container.NewHBox(items...)))
}

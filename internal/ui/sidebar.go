package ui

import (
	"image/color"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
	"CHAT": true, "IMPORTANT": true,
}

var (
	favoriteOrder = []string{"STARRED", "DRAFT", "SENT", "ALL_MAIL", "SPAM", "TRASH"}
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
			if id == "ALL_MAIL" {
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
	// Sorting by full name keeps children directly after their parent.
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

	if u.sidebarMini {
		return u.buildMiniSidebar(bg)
	}
	compose := iconButton("compose", "New message", func() { u.compose(desktop.OutboundMsg{}) })
	top := container.NewBorder(nil, nil, iconButton("menu", "Collapse sidebar", u.toggleSidebar), compose, spacer(1, 1))

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
	nav := []fyne.CanvasObject{sectionHeader("Inboxes"), u.navRow(desktop.LabelItem{ID: "INBOX", Name: u.labelName("INBOX"), Type: "system"}, 0, false)}
	nav = append(nav, sectionHeader("Favorites"))
	for _, l := range favs {
		nav = append(nav, u.navRow(l, 0, false))
	}
	if len(user) > 0 {
		nav = append(nav, u.labelsHeader())
		if !u.labelsHidden {
			nav = append(nav, u.userLabelRows(user)...)
		}
	}
	scroll := container.NewVScroll(container.NewVBox(nav...))

	head := container.NewVBox(container.NewPadded(top), container.NewPadded(tileGrid))
	body := container.NewBorder(head, u.sidebarFooter(), nil, nil, scroll)
	return container.NewStack(bg, body)
}

// labelsHeader is the "Labels" section title with a chevron that hides the
// section; the choice persists in preferences.
func (u *UI) labelsHeader() fyne.CanvasObject {
	glyph := "chevrondown"
	if u.labelsHidden {
		glyph = "chevron"
	}
	chev := canvas.NewImageFromResource(icon(glyph, subtle()))
	chev.FillMode = canvas.ImageFillContain
	chev.SetMinSize(fyne.NewSize(14, 14))
	return newTapRow(container.NewHBox(container.NewCenter(chev), sectionHeader("Labels")), false, u.toggleLabels)
}

func (u *UI) toggleLabels() {
	u.labelsHidden = !u.labelsHidden
	u.app.Preferences().SetBool(prefLabelsHidden, u.labelsHidden)
	u.refreshSidebar()
}

// buildMiniSidebar is the icon-only sidebar: menu toggle, compose, accounts,
// mailboxes (no user labels) and footer actions.
func (u *UI) buildMiniSidebar(bg fyne.CanvasObject) fyne.CanvasObject {
	pick := func(id string, badge fyne.CanvasObject) fyne.CanvasObject {
		return newTapRow(container.NewCenter(badge), u.activeAccount == id, func() { u.selectAccount(id) })
	}
	head := container.NewVBox(
		container.NewCenter(iconButton("menu", "Expand sidebar", u.toggleSidebar)),
		container.NewCenter(iconButton("compose", "New message", func() { u.compose(desktop.OutboundMsg{}) })),
		pick(unified, u.accountBadge("Unified", "inbox", color.NRGBA{0x0a, 0x84, 0xff, 0xff})),
	)
	for _, a := range u.accounts {
		name := a.DisplayName
		if name == "" {
			name = a.Email
		}
		head.Add(pick(a.ID, u.accountBadge(name, "", avatarColor(a.Email))))
	}
	favs, _ := u.mailboxes()
	nav := []fyne.CanvasObject{u.miniNavRow(desktop.LabelItem{ID: "INBOX", Name: u.labelName("INBOX"), Type: "system"})}
	for _, l := range favs {
		nav = append(nav, u.miniNavRow(l))
	}
	footer := container.NewVBox(hline(), container.NewCenter(iconButton("person", "Contacts", u.openContacts)), container.NewCenter(iconButton("settings", "Settings", u.openSettings)), container.NewCenter(u.darkToggle()), container.NewCenter(u.syncButton()))
	body := container.NewBorder(head, footer, nil, nil, container.NewVScroll(container.NewVBox(nav...)))
	return container.NewStack(bg, body)
}

func (u *UI) miniNavRow(l desktop.LabelItem) fyne.CanvasObject {
	glyph, tint := "tag", subtle()
	if g, ok := labelIcons[l.ID]; ok {
		glyph, tint = g.icon, g.col
	}
	img := canvas.NewImageFromResource(icon(glyph, tint))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(20, 28))
	return newTapRow(container.NewCenter(img), u.activeLabel == l.ID && u.query == "", func() { u.selectLabel(l.ID) })
}

func (u *UI) accountBadge(name, glyph string, tint color.Color) fyne.CanvasObject {
	if glyph != "" {
		img := canvas.NewImageFromResource(icon(glyph, color.White))
		img.FillMode = canvas.ImageFillContain
		return container.NewGridWrap(fyne.NewSize(26, 26), container.NewStack(canvas.NewCircle(tint), container.NewPadded(img)))
	}
	return avatar(initials(name), 26, tint)
}

func (u *UI) accountTile(id, name, glyph string, tint color.Color) fyne.CanvasObject {
	badge := u.accountBadge(name, glyph, tint)
	label := widget.NewLabel(name)
	label.Truncation = fyne.TextTruncateEllipsis
	content := container.NewVBox(spacer(0, 2), container.NewHBox(badge), tight(label), spacer(0, 2))
	tile := newTapRow(content, u.activeAccount == id, func() { u.selectAccount(id) })
	tile.idle = col(theme.ColorNameBackground)
	tile.radius = 9
	return tile
}

// navRow is one mailbox line. depth indents nested labels; hasKids shows the
// disclosure chevron Mimestream uses for expandable mailboxes.
func (u *UI) navRow(l desktop.LabelItem, depth int, hasKids bool) fyne.CanvasObject {
	glyph, tint := "tag", subtle()
	if g, ok := labelIcons[l.ID]; ok {
		glyph, tint = g.icon, g.col
	} else if c, ok := parseHex(l.BackgroundColor); ok && l.Type == "user" {
		tint = c
	}
	img := canvas.NewImageFromResource(icon(glyph, tint))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(20, 20))

	name := widget.NewLabel(displayLeaf(l.Name))
	name.Truncation = fyne.TextTruncateEllipsis
	var right fyne.CanvasObject = spacer(0, 0)
	if n := u.unread[l.ID]; n > 0 {
		c := canvas.NewText(strconv.Itoa(n), subtle())
		c.TextSize = 13
		right = container.NewCenter(c)
	}

	// Fixed 14px disclosure column keeps icons aligned whether or not a chevron shows.
	var disclosure fyne.CanvasObject = spacer(14, 1)
	if hasKids {
		glyph := "chevron"
		if u.openGroups[l.Name] {
			glyph = "chevrondown"
		}
		chev := canvas.NewImageFromResource(icon(glyph, subtle()))
		chev.FillMode = canvas.ImageFillContain
		chev.SetMinSize(fyne.NewSize(14, 14))
		disclosure = container.NewCenter(chev)
	}
	lead := container.NewHBox(spacer(float32(depth)*16, 1), disclosure, container.NewCenter(img))
	row := container.NewBorder(nil, nil, lead, right, tight(name))
	r := newTapRow(row, u.activeLabel == l.ID && u.query == "", func() {
		if hasKids {
			u.openGroups[l.Name] = !u.openGroups[l.Name]
		}
		u.selectLabel(l.ID)
	})
	return r
}

// displayLeaf shows only the last path segment of a nested label ("Work/Receipts" → "Receipts").
func displayLeaf(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// userLabelRows lays out user labels as a tree following Gmail's "Parent/Child" naming.
func (u *UI) userLabelRows(user []desktop.LabelItem) []fyne.CanvasObject {
	var rows []fyne.CanvasObject
	for _, l := range user {
		depth := strings.Count(l.Name, "/")
		kids := false
		for _, o := range user {
			if strings.HasPrefix(o.Name, l.Name+"/") {
				kids = true
				break
			}
		}
		if !u.pathOpen(l.Name) {
			continue
		}
		rows = append(rows, u.navRow(l, depth, kids))
	}
	return rows
}

// pathOpen reports whether every ancestor of a nested label is expanded.
func (u *UI) pathOpen(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] == '/' && !u.openGroups[name[:i]] {
			return false
		}
	}
	return true
}

func (u *UI) sidebarFooter() fyne.CanvasObject {
	settings := iconButton("settings", "Settings", u.openSettings)
	status := canvas.NewText(u.syncStatus, subtle())
	status.TextSize = 12
	var items []fyne.CanvasObject
	items = append(items, iconButton("person", "Contacts", u.openContacts), settings, u.darkToggle(), status, layout.NewSpacer())
	if u.errCount > 0 {
		warn := widget.NewButtonWithIcon(strconv.Itoa(u.errCount), icon("warning", color.NRGBA{0xff, 0x9f, 0x0a, 0xff}), u.showErrorLog)
		warn.Importance = widget.LowImportance
		items = append(items, warn)
	}
	items = append(items, u.syncButton())
	return container.NewVBox(hline(), container.NewPadded(container.NewHBox(items...)))
}

// darkToggle shows the glyph for the mode a tap switches to.
func (u *UI) darkToggle() fyne.CanvasObject {
	glyph := "moon"
	if isDark() {
		glyph = "sun"
	}
	return iconButton(glyph, "Toggle dark/light mode", u.toggleDark)
}

// syncButton is the sync glyph; it spins while a sync is in progress.
func (u *UI) syncButton() fyne.CanvasObject {
	if u.syncAnim != nil {
		u.syncAnim.Stop()
		u.syncAnim = nil
	}
	b := iconButton("sync", "Check for new mail", u.syncNow)
	if u.syncStatus != "" {
		const frames = 12
		last := -1
		u.syncAnim = fyne.NewAnimation(time.Second, func(p float32) {
			if f := int(p*frames) % frames; f != last {
				last = f
				b.SetIcon(iconRot("sync", fg(), f*360/frames))
			}
		})
		u.syncAnim.RepeatCount = fyne.AnimationRepeatForever
		u.syncAnim.Curve = fyne.AnimationLinear
		u.syncAnim.Start()
	}
	return b
}

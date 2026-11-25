package ui

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	fdesktop "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// now is the clock for relative dates and key-sequence timeouts; tests freeze it.
var now = time.Now

// listHeader is the bold mailbox title with the filter glyph on the right.
func (u *UI) listHeader() fyne.CanvasObject {
	u.filterBtn = newTooltipButton(nil, "Filter conversations", func() { u.popMenu(u.filterBtn, u.filterItems()...) })
	u.filterBtn.Importance = widget.LowImportance
	u.refreshFilterBtn()
	row := container.NewBorder(nil, nil, nil, container.NewCenter(u.filterBtn), u.listTitle)
	u.listBar = container.NewStack()
	u.refreshListBar()
	u.listErrLbl = widget.NewLabel("")
	u.listErrLbl.Wrapping = fyne.TextWrapWord
	u.listErrLbl.Hide()
	return container.NewVBox(container.NewPadded(row), u.listBar, u.listErrLbl, hline())
}

func (u *UI) refreshListHeader() {
	if u.listTitle == nil {
		return
	}
	u.refreshEmpty()
	u.refreshFilterBtn()
	title := u.labelName(u.activeLabel)
	switch {
	case u.query != "":
		title = "Search: " + u.query
	case u.activeAccount != unified:
		title += ": " + u.accountName(u.activeAccount)
	}
	u.listTitle.Segments = []widget.RichTextSegment{&widget.TextSegment{
		Text: title,
		Style: widget.RichTextStyle{
			SizeName:  theme.SizeNameSubHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
			ColorName: theme.ColorNameForeground,
		},
	}}
	u.listTitle.Truncation = fyne.TextTruncateEllipsis
	u.listTitle.Refresh()
	if u.listErrLbl == nil {
		return
	}
	if u.listErr != "" {
		u.listErrLbl.SetText("Could not load mail: " + u.listErr)
		u.listErrLbl.Show()
	} else {
		u.listErrLbl.Hide()
	}
}

// threadRow is one cell of the list, shaped like Gmail's. With the sidebar
// expanded it has two lines (sender + time; "subject - snippet" + chips); with
// the sidebar minimal it is a single wide line (sender | subject - snippet | time).
// Unread rows are bold on a clear background; read rows are tinted.
type threadRow struct {
	widget.BaseWidget
	bg      *canvas.Rectangle
	dotFill *canvas.Circle
	check   *widget.Check
	head    *widget.RichText // sender names; truncates
	count   *widget.RichText // message count, always fully shown
	when    *widget.Label
	subj    *widget.RichText // subject and snippet are separate so each truncates
	snip    *widget.RichText
	body    fyne.CanvasObject
	subjW   float32 // natural subject width, set by setRow
	clip    *canvas.Image
	star    *canvas.Image
	chips   *fyne.Container
	root    fyne.CanvasObject
}

const wideSenderWidth float32 = 190

func newThreadRow(wide bool) *threadRow {
	r := &threadRow{
		bg:      canvas.NewRectangle(color.Transparent),
		dotFill: canvas.NewCircle(color.Transparent),
		check:   widget.NewCheck("", nil),
		head:    widget.NewRichText(),
		count:   widget.NewRichText(),
		when:    widget.NewLabel(""),
		subj:    widget.NewRichText(),
		snip:    widget.NewRichText(),
		clip:    canvas.NewImageFromResource(icon("clip", subtle())),
		star:    canvas.NewImageFromResource(icon("star", color.NRGBA{0xff, 0xc1, 0x07, 0xff})),
		chips:   container.NewHBox(),
	}
	r.star.FillMode = canvas.ImageFillContain
	r.star.SetMinSize(fyne.NewSize(14, 14))
	r.head.Truncation = fyne.TextTruncateEllipsis
	r.subj.Truncation = fyne.TextTruncateEllipsis
	r.snip.Truncation = fyne.TextTruncateEllipsis
	sender := container.NewBorder(nil, nil, nil, tight(r.count), container.New(shrinkable{}, tight(r.head)))
	r.body = container.New(subjectLine{&r.subjW}, tight(r.subj), tight(r.snip))
	r.clip.FillMode = canvas.ImageFillContain
	r.clip.SetMinSize(fyne.NewSize(14, 14))

	gutter := container.NewGridWrap(fyne.NewSize(10, 10), r.dotFill)
	left := container.NewHBox(container.NewCenter(container.NewGridWrap(fyne.NewSize(18, 18), r.check)), container.NewVBox(spacer(0, 11), gutter))
	var content fyne.CanvasObject
	if wide {
		right := container.NewHBox(r.chips, container.NewCenter(r.star), container.NewCenter(r.clip), tight(r.when))
		content = container.NewBorder(nil, nil, container.New(fixedWidth{wideSenderWidth}, sender), right, container.New(shrinkable{}, tight(r.body)))
	} else {
		headRow := container.NewBorder(nil, nil, nil, container.NewHBox(container.NewCenter(r.star), container.NewCenter(r.clip), tight(r.when)), sender)
		content = container.NewVBox(headRow, container.NewBorder(nil, nil, nil, r.chips, container.New(shrinkable{}, tight(r.body))))
	}
	r.root = container.NewStack(r.bg, container.New(layout.NewCustomPaddedLayout(3, 3, 2, 6), container.NewBorder(nil, nil, left, nil, content)))
	r.ExtendBaseWidget(r)
	return r
}

func (r *threadRow) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(r.root) }

func (u *UI) setRow(r *threadRow, t desktop.ThreadItem) {
	r.check.OnChanged = nil
	r.check.SetChecked(u.checked[threadKey(t)])
	r.check.OnChanged = func(on bool) { u.setChecked(t, on) }
	if t.IsUnread {
		r.dotFill.FillColor = col(theme.ColorNamePrimary)
		r.bg.FillColor = color.Transparent
	} else {
		r.dotFill.FillColor = color.Transparent
		c := color.NRGBAModel.Convert(col(colorChip)).(color.NRGBA)
		c.A = 0x70
		r.bg.FillColor = c
	}
	if u.checked[threadKey(t)] {
		r.bg.FillColor = col(theme.ColorNameSelection)
	}
	r.dotFill.Refresh()
	r.bg.Refresh()

	r.head.Segments = []widget.RichTextSegment{&widget.TextSegment{
		Text:  displayParticipants(t.Participants),
		Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: t.IsUnread}, ColorName: theme.ColorNameForeground},
	}}
	r.head.Refresh()
	r.count.Segments = nil
	if t.MessageCount > 1 {
		r.count.Segments = []widget.RichTextSegment{&widget.TextSegment{
			Text:  fmt.Sprintf("  %d", t.MessageCount),
			Style: widget.RichTextStyle{ColorName: theme.ColorNamePlaceHolder},
		}}
	}
	r.count.Refresh()
	r.when.TextStyle = fyne.TextStyle{Bold: t.IsUnread}
	r.when.SetText(formatListDate(t.Date, now()))
	r.subj.Segments = []widget.RichTextSegment{&widget.TextSegment{Text: t.Subject, Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: t.IsUnread}, ColorName: theme.ColorNameForeground}}}
	r.snip.Segments = []widget.RichTextSegment{&widget.TextSegment{Text: " - " + t.Snippet, Style: widget.RichTextStyle{ColorName: theme.ColorNamePlaceHolder}}}
	r.subjW = fyne.MeasureText(t.Subject, theme.TextSize(), fyne.TextStyle{Bold: t.IsUnread}).Width + 2*theme.InnerPadding()
	r.subj.Refresh()
	r.snip.Refresh()
	if t.HasAttachments {
		r.clip.Show()
	} else {
		r.clip.Hide()
	}
	if t.IsStarred {
		r.star.Show()
	} else {
		r.star.Hide()
	}

	r.chips.Objects = nil
	shown := 0
	for _, id := range t.LabelIDs {
		l, ok := u.findLabel(id)
		if !ok || l.Type != "user" || shown == 2 {
			continue
		}
		bgc, fgc := color.Color(col(colorChip)), color.Color(fg())
		if c, ok := parseHex(l.BackgroundColor); ok {
			bgc = c
			fgc = color.NRGBA{0, 0, 0, 0xd0}
			if c2, ok := parseHex(l.TextColor); ok {
				fgc = c2
			}
		}
		r.chips.Add(chip(l.Name, bgc, fgc))
		shown++
	}
	r.chips.Refresh()
	r.root.Refresh() // re-run the row layout: new text changes the right side's width
}

func (u *UI) findLabel(id string) (desktop.LabelItem, bool) {
	for _, l := range u.labels {
		if l.ID == id {
			return l, true
		}
	}
	return desktop.LabelItem{}, false
}

// displayParticipants shortens "Ben Johnson <b@x>, Neil <n@x>" to "Ben, Neil".
func displayParticipants(s string) string {
	var out []string
	for _, p := range splitAddrs(s) {
		n := displayName(p)
		if i := indexSpace(n); i > 0 && len(splitAddrs(s)) > 1 {
			n = n[:i]
		}
		out = append(out, n)
	}
	return joinComma(out)
}

// listCell is one list slot: a thread row, or (past the last thread) the
// footer with its Fetch more button, loading bar and end-of-list label.
type listCell struct {
	widget.BaseWidget
	row   *threadRow
	foot  *fyne.Container
	text  *widget.Label
	btn   *widget.Button
	bar   *widget.ProgressBarInfinite
	stack *fyne.Container
}

func (u *UI) newListCell() *listCell {
	c := &listCell{row: newThreadRow(u.sidebarMini), text: widget.NewLabel(""), bar: widget.NewProgressBarInfinite()}
	c.text.Alignment = fyne.TextAlignCenter
	c.btn = widget.NewButton("Fetch more", func() {
		u.olderFailed = false
		u.loadMore()
	})
	c.foot = container.NewVBox(c.text, c.bar, c.btn)
	c.foot.Hide() // list sizes rows from this template; the footer must not set the height
	c.stack = container.NewStack(c.row, c.foot)
	c.ExtendBaseWidget(c)
	return c
}

func (c *listCell) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(c.stack) }

// showFooter switches the cell to the footer for the current load state.
func (u *UI) showFooter(c *listCell) {
	c.row.Hide()
	c.foot.Show()
	c.text.Hide()
	c.bar.Hide()
	c.btn.Hide()
	switch {
	case u.loadingMore:
		c.text.SetText("Loading older emails…")
		c.text.Show()
		c.bar.Show()
	case u.allLoaded:
		c.text.SetText("No more emails")
		c.text.Show()
	default:
		if u.olderFailed {
			c.text.SetText("Could not fetch older emails")
			c.text.Show()
		}
		c.btn.Show()
	}
}

func (u *UI) newThreadList() *keyList {
	l := &keyList{u: u}
	l.Length = func() int {
		if len(u.threads) == 0 {
			return 0
		}
		return len(u.threads) + 1 // footer
	}
	l.CreateItem = func() fyne.CanvasObject { return u.newListCell() }
	l.UpdateItem = func(i widget.ListItemID, o fyne.CanvasObject) {
		c := o.(*listCell)
		if i >= len(u.threads) {
			u.showFooter(c)
			return
		}
		c.foot.Hide()
		c.row.Show()
		u.setRow(c.row, u.threads[i])
		if i >= len(u.threads)-5 && !u.olderFailed {
			u.loadMore()
		}
	}
	l.ExtendBaseWidget(l)
	l.OnSelected = func(i widget.ListItemID) {
		if i >= len(u.threads) {
			l.Unselect(i)
			return
		}
		if mods := currentModifiers(); mods&(fyne.KeyModifierShift|fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0 {
			l.Unselect(i) // a modifier click ticks rows; it must not open the thread
			u.modClick(i, mods)
			return
		}
		u.cursor, u.cursorNext = i, false
		t := u.threads[i]
		if u.selected != nil && u.selected.ID == t.ID && u.selected.AccountID == t.AccountID {
			return
		}
		u.openThread(t)
	}
	return l
}

func currentModifiers() fyne.KeyModifier {
	if d, ok := fyne.CurrentApp().Driver().(fdesktop.Driver); ok {
		return d.CurrentKeyModifiers()
	}
	return 0
}

// anchorRow is where a shift-click range starts: the last clicked row, else
// the open thread, else -1.
func (u *UI) anchorRow() int {
	if c := u.cursor; c >= 0 {
		if u.cursorNext {
			c++
		}
		if c < len(u.threads) {
			return c
		}
	}
	if u.selected != nil {
		for i, t := range u.threads {
			if threadKey(t) == threadKey(*u.selected) {
				return i
			}
		}
	}
	return -1
}

// modClick handles a modifier-click on row i: Ctrl/Cmd toggles it and moves
// the anchor; Shift ticks anchor..i (replacing the ticks, or adding with
// Ctrl/Cmd) and leaves the anchor where it was.
func (u *UI) modClick(i int, mods fyne.KeyModifier) {
	additive := mods&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0
	if mods&fyne.KeyModifierShift == 0 {
		u.cursor, u.cursorNext = i, false
		t := u.threads[i]
		u.setChecked(t, !u.checked[threadKey(t)])
		return
	}
	a := u.anchorRow()
	if a < 0 {
		a = i
		u.cursor, u.cursorNext = i, false
	}
	lo, hi := min(a, i), max(a, i)
	if !additive || u.checked == nil {
		u.checked = map[string]bool{}
	}
	for _, t := range u.threads[lo : hi+1] {
		u.checked[threadKey(t)] = true
	}
	u.list.Refresh()
	u.refreshListBar()
}

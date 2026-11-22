package ui

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// listHeader is the bold mailbox title with the filter glyph on the right.
func (u *UI) listHeader() fyne.CanvasObject {
	img := canvas.NewImageFromResource(icon("filter", subtle()))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(26, 26))
	row := container.NewBorder(nil, nil, nil, container.NewCenter(img), u.listTitle)
	return container.NewVBox(container.NewPadded(row), hline())
}

func (u *UI) refreshListHeader() {
	if u.listTitle == nil {
		return
	}
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
}

// threadRow is one cell of the list: unread dot, sender + count + time,
// subject, and a snippet line with an optional label chip.
type threadRow struct {
	widget.BaseWidget
	dotFill *canvas.Circle
	head    *widget.RichText
	when    *widget.Label
	subject *widget.Label
	snippet *widget.Label
	clip    *canvas.Image
	star    *canvas.Image
	chips   *fyne.Container
	root    fyne.CanvasObject
}

func newThreadRow() *threadRow {
	r := &threadRow{
		dotFill: canvas.NewCircle(color.Transparent),
		head:    widget.NewRichText(),
		when:    widget.NewLabel(""),
		subject: widget.NewLabel(""),
		snippet: widget.NewLabel(""),
		clip:    canvas.NewImageFromResource(icon("clip", subtle())),
		star:    canvas.NewImageFromResource(icon("star", color.NRGBA{0xff, 0xc1, 0x07, 0xff})),
		chips:   container.NewHBox(),
	}
	r.star.FillMode = canvas.ImageFillContain
	r.star.SetMinSize(fyne.NewSize(14, 14))
	r.head.Truncation = fyne.TextTruncateEllipsis
	r.subject.Truncation = fyne.TextTruncateEllipsis
	r.snippet.Truncation = fyne.TextTruncateEllipsis
	r.snippet.Importance = widget.LowImportance
	r.when.Importance = widget.LowImportance
	r.clip.FillMode = canvas.ImageFillContain
	r.clip.SetMinSize(fyne.NewSize(14, 14))

	gutter := container.NewGridWrap(fyne.NewSize(10, 10), r.dotFill)
	left := container.NewVBox(spacer(0, 11), gutter)
	headRow := container.NewBorder(nil, nil, nil, container.NewHBox(container.NewCenter(r.star), container.NewCenter(r.clip), tight(r.when)), tight(r.head))
	snipRow := container.NewBorder(nil, nil, nil, r.chips, tight(r.snippet))
	r.root = container.New(layout.NewCustomPaddedLayout(3, 3, 2, 6), container.NewBorder(nil, nil, left, nil, container.NewVBox(headRow, tight(r.subject), snipRow)))
	r.ExtendBaseWidget(r)
	return r
}

func (r *threadRow) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(r.root) }

func (u *UI) setRow(r *threadRow, t desktop.ThreadItem) {
	if t.IsUnread {
		r.dotFill.FillColor = col(theme.ColorNamePrimary)
	} else {
		r.dotFill.FillColor = color.Transparent
	}
	r.dotFill.Refresh()

	segs := []widget.RichTextSegment{&widget.TextSegment{
		Text:  displayParticipants(t.Participants),
		Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}, ColorName: theme.ColorNameForeground, Inline: true},
	}}
	if t.MessageCount > 1 {
		segs = append(segs, &widget.TextSegment{
			Text:  fmt.Sprintf("  %d", t.MessageCount),
			Style: widget.RichTextStyle{ColorName: theme.ColorNamePlaceHolder, Inline: true},
		})
	}
	r.head.Segments = segs
	r.head.Refresh()
	r.when.SetText(formatListDate(t.Date, time.Now()))
	r.subject.SetText(t.Subject)
	r.snippet.SetText(t.Snippet)
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

func (u *UI) newThreadList() *keyList {
	l := &keyList{u: u}
	l.Length = func() int { return len(u.threads) }
	l.CreateItem = func() fyne.CanvasObject { return newThreadRow() }
	l.UpdateItem = func(i widget.ListItemID, o fyne.CanvasObject) {
		if i >= len(u.threads) {
			return
		}
		u.setRow(o.(*threadRow), u.threads[i])
		if i >= len(u.threads)-5 {
			u.loadMore()
		}
	}
	l.ExtendBaseWidget(l)
	l.OnSelected = func(i widget.ListItemID) {
		if i < len(u.threads) {
			u.cursor, u.cursorNext = i, false
			t := u.threads[i]
			if u.selected != nil && u.selected.ID == t.ID && u.selected.AccountID == t.AccountID {
				return
			}
			u.openThread(t)
		}
	}
	return l
}

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

// threadRow is one cell of the list, shaped like Gmail's. With the sidebar
// expanded it has two lines (sender + time; "subject - snippet" + chips); with
// the sidebar minimal it is a single wide line (sender | subject - snippet | time).
// Unread rows are bold on a clear background; read rows are tinted.
type threadRow struct {
	widget.BaseWidget
	bg      *canvas.Rectangle
	dotFill *canvas.Circle
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
	left := container.NewVBox(spacer(0, 11), gutter)
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
	if t.IsUnread {
		r.dotFill.FillColor = col(theme.ColorNamePrimary)
		r.bg.FillColor = color.Transparent
	} else {
		r.dotFill.FillColor = color.Transparent
		c := color.NRGBAModel.Convert(col(colorChip)).(color.NRGBA)
		c.A = 0x70
		r.bg.FillColor = c
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
	r.when.SetText(formatListDate(t.Date, time.Now()))
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
	l.CreateItem = func() fyne.CanvasObject { return newThreadRow(u.sidebarMini) }
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

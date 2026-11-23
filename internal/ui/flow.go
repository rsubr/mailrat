package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// flowLayout places children left to right and wraps like text, centring each
// on its row. wrapW is the width MinSize assumes; Layout uses the real width.
type flowLayout struct{ wrapW float32 }

func (f flowLayout) rows(o []fyne.CanvasObject, w float32) (h float32, pos []fyne.Position) {
	var x, y, rowH float32
	for _, c := range o {
		m := c.MinSize()
		if x > 0 && x+m.Width > w {
			x, y, rowH = 0, y+rowH, 0
		}
		pos = append(pos, fyne.NewPos(x, y))
		x += m.Width
		rowH = max(rowH, m.Height)
	}
	return y + rowH, pos
}

func (f flowLayout) MinSize(o []fyne.CanvasObject) fyne.Size {
	h, _ := f.rows(o, f.wrapW)
	return fyne.NewSize(0, h)
}

func (f flowLayout) Layout(o []fyne.CanvasObject, s fyne.Size) {
	_, pos := f.rows(o, s.Width)
	// Centre each child vertically within its row.
	for i := 0; i < len(o); {
		j := i
		for j < len(o) && pos[j].Y == pos[i].Y {
			j++
		}
		var rowH float32
		for _, c := range o[i:j] {
			rowH = max(rowH, c.MinSize().Height)
		}
		for k := i; k < j; k++ {
			m := o[k].MinSize()
			o[k].Resize(m)
			o[k].Move(fyne.NewPos(pos[k].X, pos[k].Y+(rowH-m.Height)/2))
		}
		i = j
	}
}

// subjectFlow lays the subject out word by word so label chips can follow its
// last word and wrap along with it.
func subjectFlow(subject string, size float32, chips []fyne.CanvasObject, wrapW float32) fyne.CanvasObject {
	var objs []fyne.CanvasObject
	for _, w := range strings.Fields(subject) {
		t := canvas.NewText(w+" ", fg())
		t.TextSize, t.TextStyle = size, fyne.TextStyle{Bold: true}
		objs = append(objs, t)
	}
	return container.New(flowLayout{wrapW}, append(objs, chips...)...)
}

// tapText is clickable text with a pointer cursor.
type tapText struct {
	widget.BaseWidget
	t    *canvas.Text
	fn   func()
	l, r float32
}

func newTapText(s string, c color.Color, l, r float32, fn func()) *tapText {
	t := canvas.NewText(s, c)
	t.TextSize = 13
	b := &tapText{t: t, fn: fn, l: l, r: r}
	b.ExtendBaseWidget(b)
	return b
}

func (b *tapText) Tapped(*fyne.PointEvent) { b.fn() }
func (b *tapText) Cursor() desktop.Cursor  { return desktop.PointerCursor }
func (b *tapText) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.New(layout.NewCustomPaddedLayout(1, 1, b.l, b.r), b.t))
}

// labelColors returns a Gmail label's chip colours: the label's own when the
// API provided them, else Gmail's neutral grey used for system/uncoloured labels.
func labelColors(bg, fgHex string) (color.Color, color.Color) {
	b, ok := parseHex(bg)
	if !ok {
		b = color.NRGBA{0xe7, 0xe7, 0xe7, 0xff}
	}
	if f, ok := parseHex(fgHex); ok {
		return b, f
	}
	if contrast(b, color.White) > contrast(b, color.Black) {
		return b, color.White
	}
	return b, color.NRGBA{0x2b, 0x2b, 0x2b, 0xff}
}

// labelChip is a Gmail-style label tag: the name opens the label, × removes it.
func labelChip(name string, bg, fgc color.Color, open, remove func()) fyne.CanvasObject {
	r := canvas.NewRectangle(bg)
	r.CornerRadius = 5
	row := container.New(layout.NewCustomPaddedLayout(0, 0, 0, 0),
		container.NewHBox(newTapText(name, fgc, 7, 0, open), newTapText("×", fgc, 0, 6, remove)))
	return container.New(layout.NewCustomPaddedLayout(0, 0, 0, 6), container.NewStack(r, row))
}

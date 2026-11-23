package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/net/html"
)

// Fyne rich text has no tables, so rows used to be flattened into " │ "
// joined lines and columns never lined up. A data table (every cell inline
// content, no nested tables) is laid out as a real grid instead.

// tableSegment is a rich-text block holding one grid of cells.
type tableSegment struct {
	rows [][][]widget.RichTextSegment // row → cell → segments
	cols int
}

func (t *tableSegment) Inline() bool { return false }
func (t *tableSegment) Textual() string {
	var b strings.Builder
	for _, row := range t.rows {
		for _, c := range row {
			for _, s := range c {
				b.WriteString(s.Textual())
			}
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
	}
	return b.String()
}
func (t *tableSegment) Visual() fyne.CanvasObject {
	var cells []fyne.CanvasObject
	for _, row := range t.rows {
		for i := 0; i < t.cols; i++ {
			rt := widget.NewRichText()
			if i < len(row) {
				rt.Segments = row[i]
			}
			rt.Wrapping = fyne.TextWrapOff
			cells = append(cells, rt)
		}
	}
	return container.New(tableLayout{t.cols}, cells...)
}
func (t *tableSegment) Update(fyne.CanvasObject)  {}
func (t *tableSegment) Select(_, _ fyne.Position) {}
func (t *tableSegment) SelectedText() string      { return "" }
func (t *tableSegment) Unselect()                 {}

// tableLayout sizes each column to its widest cell and each row to its tallest.
type tableLayout struct{ cols int }

func (l tableLayout) sizes(o []fyne.CanvasObject) (colW, rowH []float32) {
	colW = make([]float32, l.cols)
	for i, c := range o {
		m := c.MinSize()
		colW[i%l.cols] = max(colW[i%l.cols], m.Width)
		if i/l.cols >= len(rowH) {
			rowH = append(rowH, 0)
		}
		rowH[i/l.cols] = max(rowH[i/l.cols], m.Height)
	}
	return
}

func (l tableLayout) MinSize(o []fyne.CanvasObject) fyne.Size {
	colW, rowH := l.sizes(o)
	var w, h float32
	for _, v := range colW {
		w += v + theme.Padding()
	}
	for _, v := range rowH {
		h += v
	}
	return fyne.NewSize(w, h)
}

func (l tableLayout) Layout(o []fyne.CanvasObject, _ fyne.Size) {
	colW, rowH := l.sizes(o)
	var y float32
	for r, h := range rowH {
		var x float32
		for c := 0; c < l.cols && r*l.cols+c < len(o); c++ {
			cell := o[r*l.cols+c]
			cell.Move(fyne.NewPos(x, y))
			cell.Resize(fyne.NewSize(colW[c], h))
			x += colW[c] + theme.Padding()
		}
		y += h
	}
}

// table renders n as a grid when it is a data table, reporting whether it did.
func (r *htmlRenderer) table(n *html.Node, st textStyle) bool {
	var trs []*html.Node
	var collect func(*html.Node)
	collect = func(p *html.Node) {
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type != html.ElementNode:
			case c.Data == "tr":
				trs = append(trs, c)
			case c.Data == "tbody" || c.Data == "thead" || c.Data == "tfoot":
				collect(c)
			}
		}
	}
	collect(n)
	seg := &tableSegment{}
	for _, tr := range trs {
		var row [][]widget.RichTextSegment
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") || skipElement(c) {
				continue
			}
			if hasBlock(c) {
				return false
			}
			sub := &htmlRenderer{opts: r.opts}
			cs := sub.styleFor(c, st)
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				sub.walk(cc, cs)
			}
			sub.flush()
			r.out.blocked += sub.out.blocked
			row = append(row, sub.out.segments)
		}
		if len(row) > 0 {
			seg.rows = append(seg.rows, row)
			seg.cols = max(seg.cols, len(row))
		}
	}
	if seg.cols < 2 {
		return false
	}
	r.block(seg)
	return true
}

package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// tipDelay is how long the pointer rests on a button before its tooltip shows.
const tipDelay = 500 * time.Millisecond

// tooltipButton is a Button with a hover tooltip (Fyne has none built in).
type tooltipButton struct {
	widget.Button
	tip   string
	timer *time.Timer
	over  *tipOverlay
}

func newTooltipButton(ic fyne.Resource, tip string, fn func()) *tooltipButton {
	b := &tooltipButton{tip: tip}
	b.Icon, b.OnTapped = ic, fn
	b.ExtendBaseWidget(b)
	return b
}

func (b *tooltipButton) MouseIn(e *desktop.MouseEvent) {
	b.Button.MouseIn(e)
	if b.tip == "" || b.timer != nil || b.over != nil {
		return
	}
	b.timer = time.AfterFunc(tipDelay, func() { fyne.Do(b.showTip) })
}

func (b *tooltipButton) MouseOut() {
	b.Button.MouseOut()
	b.hideTip()
}

func (b *tooltipButton) Tapped(e *fyne.PointEvent) {
	b.hideTip()
	b.Button.Tapped(e)
}

func (b *tooltipButton) showTip() {
	b.timer = nil
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil || !b.Visible() {
		return
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	b.over = newTipOverlay(b, c, pos)
	c.Overlays().Add(b.over)
}

func (b *tooltipButton) hideTip() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	if b.over != nil {
		o := b.over
		b.over = nil
		o.canvas.Overlays().Remove(o)
	}
}

// tipOverlay is a transparent full-canvas layer carrying the tooltip box. It
// has to cover the button because an overlay captures all pointer events; it
// dismisses itself as soon as the pointer leaves the button and forwards taps.
type tipOverlay struct {
	widget.BaseWidget
	btn    *tooltipButton
	canvas fyne.Canvas
	box    fyne.CanvasObject
	pos    fyne.Position
}

func newTipOverlay(b *tooltipButton, c fyne.Canvas, pos fyne.Position) *tipOverlay {
	bg := canvas.NewRectangle(col(theme.ColorNameInputBackground))
	bg.StrokeColor, bg.StrokeWidth, bg.CornerRadius = col(theme.ColorNameSeparator), 1, 4
	t := canvas.NewText(b.tip, fg())
	t.TextSize = 12
	box := &tipBox{bg: bg, text: t}
	o := &tipOverlay{btn: b, canvas: c, box: box, pos: pos}
	o.ExtendBaseWidget(o)
	o.Resize(c.Size())
	return o
}

func (o *tipOverlay) inButton(p fyne.Position) bool {
	s := o.btn.Size()
	return p.X >= o.pos.X && p.X < o.pos.X+s.Width && p.Y >= o.pos.Y && p.Y < o.pos.Y+s.Height
}

func (o *tipOverlay) MouseIn(e *desktop.MouseEvent) { o.MouseMoved(e) }
func (o *tipOverlay) MouseMoved(e *desktop.MouseEvent) {
	if !o.inButton(e.AbsolutePosition) {
		o.btn.hideTip()
	}
}
func (o *tipOverlay) MouseOut()              { o.btn.hideTip() }
func (o *tipOverlay) Cursor() desktop.Cursor { return desktop.DefaultCursor }
func (o *tipOverlay) Tapped(e *fyne.PointEvent) {
	o.btn.hideTip()
	if o.inButton(e.AbsolutePosition) && !o.btn.Disabled() {
		o.btn.Button.Tapped(e)
	}
}

func (o *tipOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &tipOverlayRenderer{o: o}
}

type tipOverlayRenderer struct{ o *tipOverlay }

func (r *tipOverlayRenderer) Layout(size fyne.Size) {
	m := r.o.box.MinSize()
	x := min(r.o.pos.X, max(size.Width-m.Width, 0))
	r.o.box.Resize(m)
	r.o.box.Move(fyne.NewPos(x, r.o.pos.Y+r.o.btn.Size().Height+4))
}
func (r *tipOverlayRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *tipOverlayRenderer) Refresh()                     {}
func (r *tipOverlayRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.o.box} }
func (r *tipOverlayRenderer) Destroy()                     {}

// tipBox is the small bordered label.
type tipBox struct {
	widget.BaseWidget
	bg   *canvas.Rectangle
	text *canvas.Text
}

func (t *tipBox) CreateRenderer() fyne.WidgetRenderer {
	t.ExtendBaseWidget(t)
	return &tipBoxRenderer{t: t}
}

type tipBoxRenderer struct{ t *tipBox }

func (r *tipBoxRenderer) Layout(s fyne.Size) {
	r.t.bg.Resize(s)
	r.t.text.Move(fyne.NewPos(8, 3))
	r.t.text.Resize(fyne.NewSize(s.Width-16, s.Height-6))
}
func (r *tipBoxRenderer) MinSize() fyne.Size {
	m := r.t.text.MinSize()
	return fyne.NewSize(m.Width+16, m.Height+6)
}
func (r *tipBoxRenderer) Refresh()                     {}
func (r *tipBoxRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.t.bg, r.t.text} }
func (r *tipBoxRenderer) Destroy()                     {}

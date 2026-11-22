package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// col resolves a theme colour for the active variant (including custom names).
func col(n fyne.ThemeColorName) color.Color {
	s := fyne.CurrentApp().Settings()
	return s.Theme().Color(n, s.ThemeVariant())
}

func fg() color.Color     { return col(theme.ColorNameForeground) }
func subtle() color.Color { return col(colorSubtle) }

// ── tapRow ──────────────────────────────────────────────────────────────────

// tapRow is a tappable, hoverable row with a rounded selection background.
// It backs sidebar entries and account tiles.
type tapRow struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	onTap    func()
	selected bool
	hovered  bool
	radius   float32
	idle     color.Color // fill when neither selected nor hovered
	bg       *canvas.Rectangle
}

func newTapRow(content fyne.CanvasObject, selected bool, onTap func()) *tapRow {
	r := &tapRow{content: content, selected: selected, onTap: onTap, radius: 7, bg: canvas.NewRectangle(color.Transparent)}
	r.ExtendBaseWidget(r)
	return r
}

func (r *tapRow) Tapped(*fyne.PointEvent) {
	if r.onTap != nil {
		r.onTap()
	}
}
func (r *tapRow) MouseIn(*desktop.MouseEvent)    { r.hovered = true; r.Refresh() }
func (r *tapRow) MouseMoved(*desktop.MouseEvent) {}
func (r *tapRow) MouseOut()                      { r.hovered = false; r.Refresh() }
func (r *tapRow) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (r *tapRow) CreateRenderer() fyne.WidgetRenderer {
	return &tapRowRenderer{row: r, objs: []fyne.CanvasObject{r.bg, r.content}}
}

type tapRowRenderer struct {
	row  *tapRow
	objs []fyne.CanvasObject
}

func (t *tapRowRenderer) Layout(size fyne.Size) {
	t.row.bg.Resize(size)
	pad := theme.Padding() * 2
	t.row.content.Move(fyne.NewPos(pad, 0))
	t.row.content.Resize(fyne.NewSize(size.Width-2*pad, size.Height))
}

func (t *tapRowRenderer) MinSize() fyne.Size {
	m := t.row.content.MinSize()
	return fyne.NewSize(m.Width+theme.Padding()*4, m.Height)
}

func (t *tapRowRenderer) Refresh() {
	bg := t.row.bg
	bg.CornerRadius = t.row.radius
	switch {
	case t.row.selected:
		bg.FillColor = col(theme.ColorNameSelection)
	case t.row.hovered:
		c := col(theme.ColorNameSelection).(color.NRGBA)
		c.A = 0x70
		bg.FillColor = c
	case t.row.idle != nil:
		bg.FillColor = t.row.idle
	default:
		bg.FillColor = color.Transparent
	}
	bg.Refresh()
	t.row.content.Refresh()
}

func (t *tapRowRenderer) Objects() []fyne.CanvasObject { return t.objs }
func (t *tapRowRenderer) Destroy()                     {}

// ── small visual pieces ─────────────────────────────────────────────────────

// avatar is a filled circle with centred initials.
func avatar(text string, d float32, fill color.Color) fyne.CanvasObject {
	c := canvas.NewCircle(fill)
	t := canvas.NewText(text, color.White)
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = d * 0.38
	return container.NewGridWrap(fyne.NewSize(d, d), container.NewStack(c, container.NewCenter(t)))
}

// chip is a rounded label tag such as "Travel".
func chip(text string, bg, fgc color.Color) fyne.CanvasObject {
	r := canvas.NewRectangle(bg)
	r.CornerRadius = 5
	t := canvas.NewText(text, fgc)
	t.TextSize = 13
	return container.NewStack(r, container.New(layout.NewCustomPaddedLayout(1, 1, 7, 7), t))
}

// dot is the blue unread marker.
func dot(d float32, c color.Color) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(d, d), canvas.NewCircle(c))
}

func spacer(w, h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// sectionHeader is the small grey "Inboxes" / "Favorites" caption.
func sectionHeader(text string) fyne.CanvasObject {
	t := canvas.NewText(text, subtle())
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = 13
	return container.New(layout.NewCustomPaddedLayout(10, 2, 12, 0), t)
}

// iconButton is a borderless toolbar glyph.
func iconButton(name string, tip string, fn func()) *widget.Button {
	b := widget.NewButtonWithIcon("", icon(name, fg()), fn)
	b.Importance = widget.LowImportance
	return b
}

// vline is a 1px vertical divider.
func hline() fyne.CanvasObject { return widget.NewSeparator() }

func bold(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

// tight trims a Label's generous built-in padding so list rows stay compact.
func tight(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(-5, -5, -2, -2), o)
}

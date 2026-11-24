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

// isDark reports whether the active theme is rendering dark.
func isDark() bool {
	r, _, _, _ := col(theme.ColorNameBackground).RGBA()
	return r < 0x8000
}

func fg() color.Color     { return col(theme.ColorNameForeground) }
func subtle() color.Color { return col(colorSubtle) }

// ── tapRow ──────────────────────────────────────────────────────────────────

// tapRow is a tappable, hoverable row with a rounded selection background.
// It backs sidebar entries and account tiles.
type tapRow struct {
	widget.BaseWidget
	content     fyne.CanvasObject
	onTap       func()
	onSecondary func(abs fyne.Position)
	selected    bool
	hovered     bool
	radius      float32
	idle        color.Color // fill when neither selected nor hovered
	bg          *canvas.Rectangle
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
func (r *tapRow) TappedSecondary(e *fyne.PointEvent) {
	if r.onSecondary != nil {
		r.onSecondary(e.AbsolutePosition)
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

// fixedWidth pins its single child to w wide, keeping the child's own height.
type fixedWidth struct{ w float32 }

func (f fixedWidth) MinSize(o []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.w, o[0].MinSize().Height)
}
func (f fixedWidth) Layout(o []fyne.CanvasObject, s fyne.Size) {
	o[0].Move(fyne.NewPos(0, 0))
	o[0].Resize(fyne.NewSize(f.w, s.Height))
}

// shrinkable lets its child be narrower than its natural width so truncating
// text clips with an ellipsis instead of pushing neighbours out.
type shrinkable struct{}

func (shrinkable) MinSize(o []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, o[0].MinSize().Height)
}
func (shrinkable) Layout(o []fyne.CanvasObject, s fyne.Size) {
	// Too narrow for even an ellipsis, truncating text paints outside its box; drop it.
	o[0].Move(fyne.NewPos(0, 0))
	o[0].Resize(s)
	if s.Width < 24 {
		o[0].Hide()
	} else {
		o[0].Show()
	}
}

// minWidth keeps its single child at least w wide, however narrow its parent
// split is dragged, so rows never lay out below their fixed parts.
type minWidth struct{ w float32 }

func (m minWidth) MinSize(o []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(max(m.w, o[0].MinSize().Width), o[0].MinSize().Height)
}
func (minWidth) Layout(o []fyne.CanvasObject, s fyne.Size) {
	o[0].Move(fyne.NewPos(0, 0))
	o[0].Resize(s)
}

// subjectLine places subject then snippet on one line: the subject keeps its
// natural width up to the whole line, the snippet takes what is left.
type subjectLine struct{ subjW *float32 }

func (subjectLine) MinSize(o []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, max(o[0].MinSize().Height, o[1].MinSize().Height))
}
func (l subjectLine) Layout(o []fyne.CanvasObject, s fyne.Size) {
	sw := min(*l.subjW, s.Width)
	o[0].Move(fyne.NewPos(0, 0))
	o[0].Resize(fyne.NewSize(sw, s.Height))
	// A sliver too narrow for text would paint outside its box; drop it.
	if s.Width-sw < 40 {
		o[1].Hide()
		return
	}
	o[1].Show()
	o[1].Move(fyne.NewPos(sw, 0))
	o[1].Resize(fyne.NewSize(s.Width-sw, s.Height))
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
func iconButton(name string, tip string, fn func()) *tooltipButton {
	b := newTooltipButton(icon(name, fg()), tip, fn)
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

// escEntry is an Entry whose Escape key runs close instead of being swallowed.
type escEntry struct {
	widget.Entry
	close func()
}

func (e *escEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		e.close()
		return
	}
	e.Entry.TypedKey(k)
}

// newEscEntry is an Entry that closes via close on Escape instead of swallowing it.
func newEscEntry(close func()) *escEntry {
	e := &escEntry{close: close}
	e.ExtendBaseWidget(e)
	return e
}

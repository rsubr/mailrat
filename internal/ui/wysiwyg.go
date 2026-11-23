package ui

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Wysiwyg is a custom rich-text editing widget (Fyne has none). The document model,
// editing and HTML conversion live in wysiwyg_model.go and wysiwyg_html.go; this file
// only lays the model out, draws it and turns input events into model calls.
type Wysiwyg struct {
	widget.BaseWidget
	// OnChanged is called after every content change, OnCursor after any change of
	// caret, selection or formatting at the caret (the toolbar listens to it).
	OnChanged func()
	OnCursor  func()

	win         fyne.Window
	ed          *wyEditor
	scroll      *container.Scroll
	page        *fyne.Container
	pageLay     *wyPageLayout
	lines       []wyLine
	focused     bool
	shift, ctrl bool
	dragging    bool
	width       float32
}

const (
	wyPad       = 6
	wyScrollBar = 14
	wyListIndex = 26
	wyQuoteDent = 16
	wyParaGap   = 3
)

// NewWysiwyg returns an empty editor; win hosts its link dialog.
func NewWysiwyg(win fyne.Window) *Wysiwyg {
	w := &Wysiwyg{win: win, ed: newWyEditor(), pageLay: &wyPageLayout{}}
	w.page = container.New(w.pageLay)
	w.scroll = container.NewVScroll(w.page)
	w.ExtendBaseWidget(w)
	return w
}

// SetHTML replaces the content with the allowlisted subset of html and clears undo history.
func (w *Wysiwyg) SetHTML(html string) {
	w.ed.setDoc(parseHTML(html))
	w.rebuild()
	w.notifyCursor()
}

// HTML returns the content as clean, escaped, sanitised HTML.
func (w *Wysiwyg) HTML() string { return w.ed.doc.toHTML() }

// PlainText returns the content as text, with "- ", "1. " and "> " line prefixes.
func (w *Wysiwyg) PlainText() string { return w.ed.doc.plainText() }

// IsEmpty reports whether the editor holds no text.
func (w *Wysiwyg) IsEmpty() bool {
	return len(w.ed.doc.Paras) == 1 && w.ed.doc.Paras[0].length() == 0
}

// --- layout ----------------------------------------------------------------

type wySeg struct {
	off  int
	text string
	st   wyStyle
	x, w float32
}

type wyLine struct {
	p          int
	start, end int
	y, h       float32
	x0         float32
	segs       []wySeg
	first      bool
	marker     string
	quote      bool
	last       bool
}

type wyPageLayout struct{ h float32 }

func (l *wyPageLayout) Layout([]fyne.CanvasObject, fyne.Size) {}
func (l *wyPageLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(10, l.h) }

func wySize(st wyStyle) float32 {
	if st.Size > 0 {
		return float32(st.Size)
	}
	return theme.TextSize()
}

func wyFont(st wyStyle) fyne.TextStyle { return fyne.TextStyle{Bold: st.Bold, Italic: st.Italic} }

func measure(text string, st wyStyle) fyne.Size {
	s, _ := fyne.CurrentApp().Driver().RenderedTextSize(text, wySize(st), wyFont(st), nil)
	return s
}

type wyPiece struct {
	off  int
	text string
	st   wyStyle
}

// pieces splits a paragraph into word-sized chunks (a word plus its trailing spaces).
func pieces(p wyParagraph) []wyPiece {
	var out []wyPiece
	off := 0
	for _, r := range p.Runs {
		rs := []rune(r.Text)
		a := 0
		for i, c := range rs {
			if c == ' ' && (i+1 == len(rs) || rs[i+1] != ' ') {
				out = append(out, wyPiece{off + a, string(rs[a : i+1]), r.wyStyle})
				a = i + 1
			}
		}
		if a < len(rs) {
			out = append(out, wyPiece{off + a, string(rs[a:]), r.wyStyle})
		}
		off += len(rs)
	}
	return out
}

func (w *Wysiwyg) layoutLines(width float32) []wyLine {
	var lines []wyLine
	y := float32(wyPad)
	num := 0
	def := measure("Ag", wyStyle{}).Height
	for pi, p := range w.ed.doc.Paras {
		x0 := float32(wyPad)
		marker := ""
		if p.Kind == wyNumber {
			num++
		} else {
			num = 0
		}
		switch p.Kind {
		case wyBullet:
			x0, marker = wyPad+wyListIndex, "•"
		case wyNumber:
			x0, marker = wyPad+wyListIndex, strconv.Itoa(num)+"."
		case wyQuote:
			x0 = wyPad + wyQuoteDent
		}
		avail := width - x0 - wyPad
		cur := wyLine{p: pi, y: y, x0: x0, first: true, marker: marker, quote: p.Kind == wyQuote}
		x := x0
		flush := func(end int) {
			cur.end = end
			if cur.h == 0 {
				cur.h = def
			}
			lines = append(lines, cur)
			y += cur.h
			cur = wyLine{p: pi, y: y, x0: x0, start: end, quote: p.Kind == wyQuote}
			x = x0
		}
		for _, pc := range pieces(p) {
			text, off := pc.text, pc.off
			for {
				sz := measure(text, pc.st)
				if x+sz.Width-x0 > avail && len(cur.segs) > 0 {
					flush(off)
					continue
				}
				if x+sz.Width-x0 > avail && len([]rune(text)) > 1 { // single word wider than the line
					rs := []rune(text)
					n := 1
					for n < len(rs) && x+measure(string(rs[:n+1]), pc.st).Width-x0 <= avail {
						n++
					}
					sz = measure(string(rs[:n]), pc.st)
					cur.segs = append(cur.segs, wySeg{off, string(rs[:n]), pc.st, x, sz.Width})
					cur.h = max(cur.h, sz.Height)
					flush(off + n)
					text, off = string(rs[n:]), off+n
					continue
				}
				cur.segs = append(cur.segs, wySeg{off, text, pc.st, x, sz.Width})
				cur.h = max(cur.h, sz.Height)
				x += sz.Width
				break
			}
		}
		cur.last = true
		flush(p.length())
		y += wyParaGap
	}
	return lines
}

// lineOf returns the index of the line holding pos.
func (w *Wysiwyg) lineOf(pos wyPos) int {
	for i, l := range w.lines {
		if l.p == pos.P && (pos.Off < l.end || l.last) {
			return i
		}
	}
	return len(w.lines) - 1
}

func segPrefixX(s wySeg, n int) float32 {
	if n <= 0 {
		return s.x
	}
	return s.x + measure(string([]rune(s.text)[:n]), s.st).Width
}

// xAt is the x coordinate of off within line l.
func xAt(l wyLine, off int) float32 {
	x := l.x0
	for _, s := range l.segs {
		n := len([]rune(s.text))
		if off <= s.off+n {
			return segPrefixX(s, off-s.off)
		}
		x = s.x + s.w
	}
	return x
}

func (w *Wysiwyg) caretXY() (x, y, h float32) {
	l := w.lines[w.lineOf(w.ed.caret)]
	return xAt(l, w.ed.caret.Off), l.y, l.h
}

// hit maps a point in page coordinates to a document position.
func (w *Wysiwyg) hit(x, y float32) wyPos {
	li := len(w.lines) - 1
	for i, l := range w.lines {
		if y < l.y+l.h {
			li = i
			break
		}
	}
	l := w.lines[li]
	pos := wyPos{l.p, l.start}
	for _, s := range l.segs {
		if x < s.x {
			break
		}
		rs := []rune(s.text)
		pos = wyPos{l.p, s.off + len(rs)}
		if x < s.x+s.w {
			for k := 0; k < len(rs); k++ {
				if x < (segPrefixX(s, k)+segPrefixX(s, k+1))/2 {
					return wyPos{l.p, s.off + k}
				}
			}
		}
	}
	if !l.last && pos.Off == l.end && pos.Off > l.start { // stay on this visual line
		pos.Off--
	}
	return pos
}

// --- drawing ---------------------------------------------------------------

func (w *Wysiwyg) rebuild() {
	if w.width <= 0 {
		return
	}
	w.lines = w.layoutLines(w.width - wyScrollBar)
	last := w.lines[len(w.lines)-1]
	w.pageLay.h = last.y + last.h + wyPad
	w.page.Objects = w.draw()
	w.page.Refresh()
	w.scroll.Refresh()
}

func (w *Wysiwyg) draw() []fyne.CanvasObject {
	var objs []fyne.CanvasObject
	s, t := w.ed.selRange()
	selCol := col(theme.ColorNameSelection)
	for _, l := range w.lines {
		if w.ed.hasSel() && l.p >= s.P && l.p <= t.P {
			a, b := l.start, l.end
			if l.p == s.P {
				a = max(a, s.Off)
			}
			if l.p == t.P {
				b = min(b, t.Off)
			}
			x1, x2 := xAt(l, a), xAt(l, b)
			if l.last && l.p < t.P {
				x2 += 4 // show the paragraph break
			}
			if x2 > x1 && (b > a || l.last) {
				r := canvas.NewRectangle(selCol)
				r.Move(fyne.NewPos(x1, l.y))
				r.Resize(fyne.NewSize(x2-x1, l.h))
				objs = append(objs, r)
			}
		}
		if l.quote {
			bar := canvas.NewRectangle(col(theme.ColorNameDisabled))
			bar.Move(fyne.NewPos(l.x0-10, l.y))
			bar.Resize(fyne.NewSize(3, l.h))
			objs = append(objs, bar)
		}
		if l.first && l.marker != "" {
			ts := measure(l.marker, wyStyle{})
			m := canvas.NewText(l.marker, fg())
			m.TextSize = wySize(wyStyle{})
			m.Move(fyne.NewPos(l.x0-ts.Width-6, l.y+l.h-ts.Height))
			objs = append(objs, m)
		}
		for _, sg := range l.segs {
			c := fg()
			if sg.st.Color != "" {
				c = wyColor(sg.st.Color)
			}
			if sg.st.Link != "" {
				c = col(theme.ColorNameHyperlink)
			}
			sz := measure(sg.text, sg.st)
			tx := canvas.NewText(sg.text, c)
			tx.TextSize, tx.TextStyle = wySize(sg.st), wyFont(sg.st)
			tx.Move(fyne.NewPos(sg.x, l.y+l.h-sz.Height))
			objs = append(objs, tx)
			if sg.st.Underline || sg.st.Link != "" {
				ly := l.y + l.h - 2
				objs = append(objs, wyLineObj(c, sg.x, ly, sg.x+sg.w, ly))
			}
			if sg.st.Strike {
				ly := l.y + l.h - sz.Height/2
				objs = append(objs, wyLineObj(c, sg.x, ly, sg.x+sg.w, ly))
			}
		}
	}
	if w.focused && !w.ed.hasSel() {
		x, y, h := w.caretXY()
		c := canvas.NewRectangle(fg())
		c.Move(fyne.NewPos(x, y))
		c.Resize(fyne.NewSize(1.5, h))
		objs = append(objs, c)
	}
	return objs
}

func wyColor(hex string) color.Color {
	var r, g, b uint8
	if len(hex) == 7 {
		v, _ := strconv.ParseUint(hex[1:], 16, 32)
		r, g, b = uint8(v>>16), uint8(v>>8), uint8(v)
	}
	return color.NRGBA{r, g, b, 255}
}

func wyLineObj(c color.Color, x1, y1, x2, y2 float32) *canvas.Line {
	l := canvas.NewLine(c)
	l.StrokeWidth = 1
	l.Position1, l.Position2 = fyne.NewPos(x1, y1), fyne.NewPos(x2, y2)
	return l
}

// --- widget plumbing -------------------------------------------------------

type wyRenderer struct {
	w      *Wysiwyg
	bg     *canvas.Rectangle
	border *canvas.Rectangle
}

func (w *Wysiwyg) CreateRenderer() fyne.WidgetRenderer {
	return &wyRenderer{w: w, bg: canvas.NewRectangle(col(theme.ColorNameInputBackground)),
		border: canvas.NewRectangle(color.Transparent)}
}

func (r *wyRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.border.Resize(size)
	r.w.scroll.Resize(size)
	if size.Width != r.w.width {
		r.w.width = size.Width
		r.w.rebuild()
	}
}
func (r *wyRenderer) MinSize() fyne.Size { return fyne.NewSize(260, 140) }
func (r *wyRenderer) Refresh() {
	r.bg.FillColor = col(theme.ColorNameInputBackground)
	r.border.StrokeColor = col(theme.ColorNameInputBorder)
	r.border.StrokeWidth = 1
	r.w.rebuild()
	canvas.Refresh(r.bg)
}
func (r *wyRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.w.scroll, r.border}
}
func (r *wyRenderer) Destroy() {}

func (w *Wysiwyg) FocusGained() { w.focused = true; w.rebuild() }
func (w *Wysiwyg) FocusLost()   { w.focused = false; w.shift, w.ctrl = false, false; w.rebuild() }

func (w *Wysiwyg) Cursor() desktop.Cursor { return desktop.TextCursor }

func (w *Wysiwyg) KeyDown(k *fyne.KeyEvent) { w.setMods(k.Name, true) }
func (w *Wysiwyg) KeyUp(k *fyne.KeyEvent)   { w.setMods(k.Name, false) }

func (w *Wysiwyg) setMods(k fyne.KeyName, down bool) {
	switch k {
	case desktop.KeyShiftLeft, desktop.KeyShiftRight:
		w.shift = down
	case desktop.KeyControlLeft, desktop.KeyControlRight, desktop.KeyAltLeft, desktop.KeyAltRight:
		w.ctrl = down
	}
}

// after redraws, keeps the caret in view and tells listeners; changed marks a content edit.
func (w *Wysiwyg) after(changed bool) {
	w.rebuild()
	if len(w.lines) > 0 {
		_, y, h := w.caretXY()
		off, vis := w.scroll.Offset.Y, w.scroll.Size().Height
		switch {
		case y < off:
			w.scroll.Offset.Y = max(y-wyPad, 0)
		case y+h > off+vis:
			w.scroll.Offset.Y = y + h + wyPad - vis
		}
		w.scroll.Refresh()
	}
	if changed && w.OnChanged != nil {
		w.OnChanged()
	}
	w.notifyCursor()
}

func (w *Wysiwyg) notifyCursor() {
	if w.OnCursor != nil {
		w.OnCursor()
	}
}

func (w *Wysiwyg) TypedRune(r rune) {
	w.ed.insertText(string(r))
	w.after(true)
}

func (w *Wysiwyg) TypedKey(k *fyne.KeyEvent) {
	e := w.ed
	nav := func(p wyPos) { e.moveTo(p, w.shift); w.after(false) }
	edit := func(f func()) { f(); w.after(true) }
	switch k.Name {
	case fyne.KeyLeft:
		if e.hasSel() && !w.shift && !w.ctrl {
			s, _ := e.selRange()
			nav(s)
		} else {
			nav(e.left(e.caret, w.ctrl))
		}
	case fyne.KeyRight:
		if e.hasSel() && !w.shift && !w.ctrl {
			_, t := e.selRange()
			nav(t)
		} else {
			nav(e.right(e.caret, w.ctrl))
		}
	case fyne.KeyUp, fyne.KeyDown, fyne.KeyPageUp, fyne.KeyPageDown:
		x, y, h := w.caretXY()
		d := float32(1)
		if k.Name == fyne.KeyUp || k.Name == fyne.KeyPageUp {
			d = -1
		}
		step := h
		if k.Name == fyne.KeyPageUp || k.Name == fyne.KeyPageDown {
			step = max(w.scroll.Size().Height-h, h)
		}
		ty := y + h/2 + d*step
		switch {
		case ty < 0:
			nav(wyPos{})
		case ty > w.pageLay.h:
			nav(e.docEnd())
		default:
			nav(w.hit(x, ty))
		}
	case fyne.KeyHome:
		if w.ctrl {
			nav(wyPos{})
		} else {
			nav(wyPos{e.caret.P, w.lines[w.lineOf(e.caret)].start})
		}
	case fyne.KeyEnd:
		if w.ctrl {
			nav(e.docEnd())
		} else {
			l := w.lines[w.lineOf(e.caret)]
			end := l.end
			if !l.last && end > l.start {
				end--
			}
			nav(wyPos{l.p, end})
		}
	case fyne.KeyBackspace:
		edit(func() { e.backspace(w.ctrl) })
	case fyne.KeyDelete:
		edit(func() { e.deleteForward(w.ctrl) })
	case fyne.KeyReturn, fyne.KeyEnter:
		edit(e.enter)
	}
}

func (w *Wysiwyg) TypedShortcut(sh fyne.Shortcut) {
	e := w.ed
	switch s := sh.(type) {
	case *fyne.ShortcutCopy:
		if e.hasSel() {
			s.Clipboard.SetContent(e.selectedText())
		}
	case *fyne.ShortcutCut:
		if e.hasSel() {
			s.Clipboard.SetContent(e.selectedText())
			e.cut()
			w.after(true)
		}
	case *fyne.ShortcutPaste:
		e.insertText(s.Clipboard.Content())
		w.after(true)
	case *fyne.ShortcutSelectAll:
		e.selectAll()
		w.after(false)
	case *fyne.ShortcutUndo:
		w.Undo()
	case *fyne.ShortcutRedo:
		w.Redo()
	case *desktop.CustomShortcut:
		if s.Modifier != fyne.KeyModifierControl && s.Modifier != fyne.KeyModifierSuper {
			return
		}
		switch s.KeyName {
		case fyne.KeyB:
			w.Bold()
		case fyne.KeyI:
			w.Italic()
		case fyne.KeyU:
			w.Underline()
		case fyne.KeyK:
			w.EditLink()
		}
	}
}

func (w *Wysiwyg) pagePos(p fyne.Position) (float32, float32) { return p.X, p.Y + w.scroll.Offset.Y }

func (w *Wysiwyg) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button != desktop.MouseButtonPrimary || len(w.lines) == 0 {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(w); c != nil {
		c.Focus(w)
	}
	w.dragging = true
	w.ed.moveTo(w.hit(w.pagePos(ev.Position)), ev.Modifier&fyne.KeyModifierShift != 0)
	w.after(false)
}

func (w *Wysiwyg) MouseUp(*desktop.MouseEvent) {}

func (w *Wysiwyg) Tapped(*fyne.PointEvent) {}

func (w *Wysiwyg) Dragged(ev *fyne.DragEvent) {
	if w.dragging && len(w.lines) > 0 {
		w.ed.moveTo(w.hit(w.pagePos(ev.Position)), true)
		w.after(false)
	}
}

func (w *Wysiwyg) DragEnd() { w.dragging = false }

func (w *Wysiwyg) DoubleTapped(ev *fyne.PointEvent) {
	if len(w.lines) == 0 {
		return
	}
	a, b := w.ed.wordAt(w.hit(w.pagePos(ev.Position)))
	w.ed.moveTo(a, false)
	w.ed.moveTo(b, true)
	w.after(false)
}

// --- commands (used by the toolbar and shortcuts) --------------------------

func (w *Wysiwyg) cmd(f func()) { f(); w.after(true) }

func (w *Wysiwyg) Bold()             { w.cmd(w.ed.toggleBold) }
func (w *Wysiwyg) Italic()           { w.cmd(w.ed.toggleItalic) }
func (w *Wysiwyg) Underline()        { w.cmd(w.ed.toggleUnderline) }
func (w *Wysiwyg) Strike()           { w.cmd(w.ed.toggleStrike) }
func (w *Wysiwyg) ClearFormat()      { w.cmd(w.ed.clearFormatting) }
func (w *Wysiwyg) Bullets()          { w.cmd(func() { w.ed.toggleKind(wyBullet) }) }
func (w *Wysiwyg) Numbers()          { w.cmd(func() { w.ed.toggleKind(wyNumber) }) }
func (w *Wysiwyg) Quote()            { w.cmd(func() { w.ed.toggleKind(wyQuote) }) }
func (w *Wysiwyg) SetColor(c string) { w.cmd(func() { w.ed.setColor(c) }) }
func (w *Wysiwyg) SetSize(px int)    { w.cmd(func() { w.ed.setSize(px) }) }
func (w *Wysiwyg) Undo()             { w.ed.doUndo(); w.after(true) }
func (w *Wysiwyg) Redo()             { w.ed.doRedo(); w.after(true) }

// SetLink links the selection (or the link at the caret) to url; "" removes the link.
func (w *Wysiwyg) SetLink(url string) bool {
	ok := w.ed.setLink(url)
	w.after(true)
	return ok
}

// EditLink asks for a URL for the selection or the link at the caret.
func (w *Wysiwyg) EditLink() {
	if w.win == nil {
		return
	}
	url := widget.NewEntry()
	url.SetText(w.ed.linkAtCaret())
	url.SetPlaceHolder("https://example.com (empty removes the link)")
	dialog.ShowForm("Link", "Apply", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("URL", url)},
		func(ok bool) {
			if ok && !w.SetLink(url.Text) {
				dialog.ShowInformation("Link", "Only http, https and mailto links are supported.", w.win)
			}
			w.win.Canvas().Focus(w)
		}, w.win)
}

// --- toolbar ---------------------------------------------------------------

var wyColors = []struct{ name, hex string }{
	{"Default", ""}, {"Black", "#000000"}, {"Red", "#d32f2f"}, {"Orange", "#ef6c00"},
	{"Green", "#2e7d32"}, {"Blue", "#1565c0"}, {"Purple", "#6a1b9a"}, {"Gray", "#757575"},
}

var wySizes = []struct {
	name string
	px   int
}{{"Small", 11}, {"Normal", 0}, {"Large", 18}, {"Huge", 28}}

// NewWysiwygToolbar returns a formatting toolbar bound to w. Toggle buttons show
// their state at the caret.
func NewWysiwygToolbar(w *Wysiwyg) fyne.CanvasObject {
	type toggle struct {
		b  *widget.Button
		on func(wyStyle) bool
	}
	var toggles []toggle
	var blocks []*widget.Button
	var kinds []wyKind
	refocus := func() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(w); c != nil {
			c.Focus(w)
		}
	}
	tog := func(label string, on func(wyStyle) bool, do func()) *widget.Button {
		b := widget.NewButton(label, func() { do(); refocus() })
		toggles = append(toggles, toggle{b, on})
		return b
	}
	block := func(label string, k wyKind, do func()) *widget.Button {
		b := widget.NewButton(label, func() { do(); refocus() })
		blocks, kinds = append(blocks, b), append(kinds, k)
		return b
	}

	colorBtn := widget.NewButton("Color", nil)
	colorBtn.OnTapped = func() {
		var items []*fyne.MenuItem
		for _, c := range wyColors {
			items = append(items, fyne.NewMenuItem(c.name, func() { w.SetColor(c.hex); refocus() }))
		}
		cv := fyne.CurrentApp().Driver().CanvasForObject(colorBtn)
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(colorBtn)
		widget.NewPopUpMenu(fyne.NewMenu("", items...), cv).ShowAtPosition(pos.Add(fyne.NewPos(0, colorBtn.Size().Height)))
	}

	var sizeNames []string
	for _, s := range wySizes {
		sizeNames = append(sizeNames, s.name)
	}
	updating := false
	sizeSel := widget.NewSelect(sizeNames, func(name string) {
		if updating {
			return
		}
		for _, s := range wySizes {
			if s.name == name {
				w.SetSize(s.px)
			}
		}
		refocus()
	})
	sizeSel.PlaceHolder = "Size"

	undo := widget.NewButtonWithIcon("", theme.ContentUndoIcon(), func() { w.Undo(); refocus() })
	redo := widget.NewButtonWithIcon("", theme.ContentRedoIcon(), func() { w.Redo(); refocus() })

	bar := container.NewHBox(
		undo, redo,
		tog("B", func(s wyStyle) bool { return s.Bold }, w.Bold),
		tog("I", func(s wyStyle) bool { return s.Italic }, w.Italic),
		tog("U", func(s wyStyle) bool { return s.Underline }, w.Underline),
		tog("S", func(s wyStyle) bool { return s.Strike }, w.Strike),
		colorBtn, sizeSel,
		tog("Link", func(s wyStyle) bool { return s.Link != "" }, w.EditLink),
		block("• List", wyBullet, w.Bullets),
		block("1. List", wyNumber, w.Numbers),
		block("Quote", wyQuote, w.Quote),
		widget.NewButton("Clear", func() { w.ClearFormat(); refocus() }),
	)

	update := func() {
		st := w.ed.styleAtCaret()
		for _, t := range toggles {
			t.b.Importance = widget.MediumImportance
			if t.on(st) {
				t.b.Importance = widget.HighImportance
			}
			t.b.Refresh()
		}
		k := w.ed.blockKind()
		for i, b := range blocks {
			b.Importance = widget.MediumImportance
			if kinds[i] == k {
				b.Importance = widget.HighImportance
			}
			b.Refresh()
		}
		updating = true
		name := ""
		for _, s := range wySizes {
			if s.px == st.Size {
				name = s.name
			}
		}
		if name == "" && st.Size != 0 {
			name = strconv.Itoa(st.Size)
		}
		sizeSel.SetSelected(name)
		updating = false
		for b, ok := range map[*widget.Button]bool{undo: w.ed.canUndo(), redo: w.ed.canRedo()} {
			if ok {
				b.Enable()
			} else {
				b.Disable()
			}
		}
	}
	w.OnCursor = update
	update()
	return container.NewHScroll(bar)
}

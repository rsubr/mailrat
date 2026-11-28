package ui

import (
	"image/color"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// multiClick is how close in time clicks must be to count as a double or triple click.
const multiClick = 450 * time.Millisecond

// selBody is a selectable, wrapped rich-text view of one message body.
type selBody struct {
	widget.BaseWidget
	doc    *selDoc
	lay    *selLayout
	layW   float32
	anchor int // selection start; caret is the moving end
	caret  int
	shift  bool
	hover  bool // pointer is over a link

	clicks    int
	lastClick time.Time

	// copyFn receives the HTML fragment and plain text of a copy.
	copyFn func(html, text string)
	// active is told when this body starts a selection so others can clear theirs.
	active func(*selBody)
	// th paints this body (the mail view palette); nil follows the app theme.
	th fyne.Theme
	// key and rune forward keys that are not selection moves to the app shortcuts.
	key  func(fyne.KeyName) bool
	rune func(rune)
}

// setBands paints the renderer's author backgrounds behind the text. Call it
// right after newSelBody, before the widget is shown.
func (b *selBody) setBands(bs []mailBand) {
	b.doc.setBands(bs)
	b.lay = nil
}

func newSelBody(segs []widget.RichTextSegment) *selBody {
	b := &selBody{doc: buildSelDoc(segs)}
	b.ExtendBaseWidget(b)
	return b
}

// col resolves a theme colour in the body's palette.
func (b *selBody) col(n fyne.ThemeColorName) color.Color {
	if b.th == nil {
		return col(n)
	}
	return b.th.Color(n, theme.VariantLight)
}

func (b *selBody) ensure(w float32) {
	if w <= 0 {
		w = 360
	}
	if b.lay == nil || b.layW != w {
		b.lay, b.layW = b.doc.layout(w), w
	}
}

func (b *selBody) MinSize() fyne.Size {
	b.ensure(b.Size().Width)
	return fyne.NewSize(60, b.lay.height)
}

func (b *selBody) Resize(s fyne.Size) {
	b.BaseWidget.Resize(s)
	b.ensure(s.Width)
}

// ── selection ───────────────────────────────────────────────────────────────

func (b *selBody) sel() (int, int) { return min(b.anchor, b.caret), max(b.anchor, b.caret) }
func (b *selBody) hasSel() bool    { return b.anchor != b.caret }

func (b *selBody) setSel(a, c int) {
	b.anchor, b.caret = a, c
	b.Refresh()
}

func (b *selBody) selectAll() { b.setSel(0, b.doc.total()) }

// Selection is the selected part as an HTML fragment and as plain text.
func (b *selBody) Selection() (html, text string) {
	a, c := b.sel()
	return b.doc.fragment(a, c)
}

func (b *selBody) copySel() {
	if !b.hasSel() || b.copyFn == nil {
		return
	}
	h, t := b.Selection()
	b.copyFn(h, t)
}

func selClass(r rune) int {
	switch {
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 0
	case unicode.IsSpace(r):
		return 1
	}
	return 2
}

func (b *selBody) wordAt(off int) (int, int) {
	f := b.doc.flat
	if len(f) == 0 {
		return 0, 0
	}
	off = min(off, len(f)-1)
	if f[off] == '\n' {
		return off, off
	}
	c := selClass(f[off])
	lo, hi := off, off+1
	for lo > 0 && f[lo-1] != '\n' && selClass(f[lo-1]) == c {
		lo--
	}
	for hi < len(f) && f[hi] != '\n' && selClass(f[hi]) == c {
		hi++
	}
	return lo, hi
}

func (b *selBody) paraAt(off int) (int, int) {
	for _, p := range b.doc.paras {
		if off <= p.start+p.n {
			return p.start, p.start + p.n
		}
	}
	return 0, 0
}

// ── geometry ────────────────────────────────────────────────────────────────

func (b *selBody) lineAt(y float32) int {
	b.ensure(b.Size().Width)
	ls := b.lay.lines
	for i := range ls {
		if y < ls[i].y+ls[i].h+theme.Padding() {
			return i
		}
	}
	return len(ls) - 1
}

func (b *selBody) lineOf(off int) int {
	b.ensure(b.Size().Width)
	ls, at := b.lay.lines, 0
	for i := range ls {
		if ls[i].start <= off {
			at = i
		}
	}
	return at
}

// xAt is the x of document offset off on line li.
func (b *selBody) xAt(li, off int) float32 {
	ln := b.lay.lines[li]
	x := theme.InnerPadding()
	for _, f := range ln.frags {
		if off <= f.docStart {
			return f.x
		}
		if off < f.docEnd {
			return f.x + b.doc.measure(b.doc.paras[ln.para].runs[f.run], f.from, f.from+off-f.docStart)
		}
		x = f.x + f.w
	}
	return x
}

// offsetAt is the document offset nearest point p.
func (b *selBody) offsetAt(p fyne.Position) int {
	if len(b.doc.paras) == 0 {
		return 0
	}
	li := b.lineAt(p.Y)
	ln := b.lay.lines[li]
	for _, f := range ln.frags {
		if p.X >= f.x+f.w {
			continue
		}
		r := b.doc.paras[ln.para].runs[f.run]
		prev := float32(0)
		for k := f.from + 1; k <= f.to; k++ {
			w := b.doc.measure(r, f.from, k)
			if p.X < f.x+(prev+w)/2 {
				return f.docStart + k - 1 - f.from
			}
			prev = w
		}
		return f.docEnd
	}
	return ln.textEnd
}

func (b *selBody) linkAt(p fyne.Position) (u *selRun) {
	if len(b.doc.paras) == 0 {
		return nil
	}
	b.ensure(b.Size().Width)
	ln := b.lay.lines[b.lineAt(p.Y)]
	if p.Y < ln.y || p.Y > ln.y+ln.h {
		return nil
	}
	for _, f := range ln.frags {
		if p.X >= f.x && p.X < f.x+f.w {
			if r := &b.doc.paras[ln.para].runs[f.run]; r.link != nil {
				return r
			}
		}
	}
	return nil
}

// ── input ───────────────────────────────────────────────────────────────────

func (b *selBody) MouseDown(e *desktop.MouseEvent) {
	if e.Button != desktop.MouseButtonPrimary {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
		c.Focus(b)
	}
	if b.active != nil {
		b.active(b)
	}
	if t := time.Now(); t.Sub(b.lastClick) < multiClick {
		b.clicks++
	} else {
		b.clicks = 1
	}
	b.lastClick = time.Now()
	off := b.offsetAt(e.Position)
	switch {
	case b.clicks == 2:
		b.setSel(b.wordAt(off))
	case b.clicks >= 3:
		b.setSel(b.paraAt(off))
	case e.Modifier&fyne.KeyModifierShift != 0:
		b.setSel(b.anchor, off)
	default:
		b.setSel(off, off)
	}
}

func (b *selBody) MouseUp(*desktop.MouseEvent) {}

func (b *selBody) Dragged(e *fyne.DragEvent) {
	if b.clicks == 1 {
		b.setSel(b.anchor, b.offsetAt(e.Position))
	}
}

func (b *selBody) DragEnd() {}

// Tapped opens a link when the click did not select anything.
func (b *selBody) Tapped(e *fyne.PointEvent) {
	if b.hasSel() {
		return
	}
	if r := b.linkAt(e.Position); r != nil {
		_ = fyne.CurrentApp().OpenURL(r.link)
	}
}

func (b *selBody) TappedSecondary(e *fyne.PointEvent) {
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	cp := fyne.NewMenuItem("Copy", b.copySel)
	cp.Disabled = !b.hasSel()
	menu := fyne.NewMenu("", cp, fyne.NewMenuItem("Select All", b.selectAll))
	widget.ShowPopUpMenuAtPosition(menu, c, e.AbsolutePosition)
}

func (b *selBody) MouseIn(*desktop.MouseEvent) {}
func (b *selBody) MouseOut()                   { b.hover = false }
func (b *selBody) MouseMoved(e *desktop.MouseEvent) {
	b.hover = b.linkAt(e.Position) != nil
}

func (b *selBody) Cursor() desktop.Cursor {
	if b.hover {
		return desktop.PointerCursor
	}
	return desktop.TextCursor
}

func (b *selBody) FocusGained() {}
func (b *selBody) FocusLost()   { b.shift = false }

func (b *selBody) KeyDown(e *fyne.KeyEvent) {
	if e.Name == desktop.KeyShiftLeft || e.Name == desktop.KeyShiftRight {
		b.shift = true
	}
}

func (b *selBody) KeyUp(e *fyne.KeyEvent) {
	if e.Name == desktop.KeyShiftLeft || e.Name == desktop.KeyShiftRight {
		b.shift = false
	}
}

func (b *selBody) TypedRune(r rune) {
	if b.rune != nil {
		b.rune(r)
	}
}

// TypedKey extends the selection on Shift+arrows/Home/End; any other key
// goes to the app's Gmail shortcuts, and Escape also clears the selection.
func (b *selBody) TypedKey(e *fyne.KeyEvent) {
	if b.shift && b.moveCaret(e.Name) {
		return
	}
	if e.Name == fyne.KeyEscape {
		b.setSel(b.caret, b.caret)
	}
	if b.key != nil {
		b.key(e.Name)
	}
}

func (b *selBody) moveCaret(k fyne.KeyName) bool {
	b.ensure(b.Size().Width)
	c, li := b.caret, b.lineOf(b.caret)
	switch k {
	case fyne.KeyLeft:
		c = max(c-1, 0)
	case fyne.KeyRight:
		c = min(c+1, b.doc.total())
	case fyne.KeyHome:
		c = b.lay.lines[li].start
	case fyne.KeyEnd:
		c = b.lay.lines[li].textEnd
	case fyne.KeyUp, fyne.KeyDown:
		to := li - 1
		if k == fyne.KeyDown {
			to = li + 1
		}
		if to < 0 || to >= len(b.lay.lines) {
			return true
		}
		t := b.lay.lines[to]
		c = b.offsetAt(fyne.NewPos(b.xAt(li, c), t.y+t.h/2))
	default:
		return false
	}
	b.setSel(b.anchor, c)
	return true
}

func (b *selBody) TypedShortcut(s fyne.Shortcut) {
	switch s.(type) {
	case *fyne.ShortcutCopy:
		b.copySel()
	case *fyne.ShortcutSelectAll:
		b.selectAll()
	}
}

// ── rendering ───────────────────────────────────────────────────────────────

func (b *selBody) CreateRenderer() fyne.WidgetRenderer {
	return &selRenderer{b: b}
}

type selRenderer struct {
	b    *selBody
	objs []fyne.CanvasObject
}

func (r *selRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *selRenderer) Destroy()                     {}
func (r *selRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *selRenderer) Layout(s fyne.Size)           { r.b.ensure(s.Width); r.Refresh() }

func (r *selRenderer) Refresh() {
	b := r.b
	b.ensure(b.Size().Width)
	var bands, rects, texts []fyne.CanvasObject
	for _, br := range b.lay.bands {
		rc := canvas.NewRectangle(br.col)
		rc.Move(fyne.NewPos(0, br.y))
		rc.Resize(fyne.NewSize(b.layW, br.h))
		bands = append(bands, rc)
	}
	for _, q := range b.lay.quotes {
		rc := quoteBar()
		rc.Move(fyne.NewPos(quoteBarPad, q.y))
		rc.Resize(fyne.NewSize(1, q.h))
		bands = append(bands, rc)
	}
	a, c := b.sel()
	selCol := b.col(theme.ColorNameSelection)
	for li, ln := range b.lay.lines {
		if ln.obj != nil {
			ln.obj.Move(fyne.NewPos(theme.InnerPadding(), ln.y))
			texts = append(texts, ln.obj)
			continue
		}
		if a < c && a < ln.end+1 && c > ln.start {
			x1 := b.xAt(li, max(a, ln.start))
			x2 := b.xAt(li, min(c, ln.textEnd))
			if c > ln.textEnd { // selection runs on past the line: show the newline
				x2 += theme.Padding()
			}
			rc := canvas.NewRectangle(selCol)
			rc.Move(fyne.NewPos(x1, ln.y))
			rc.Resize(fyne.NewSize(max(x2-x1, 0), ln.h))
			rects = append(rects, rc)
		}
		runs := b.doc.paras[ln.para].runs
		for _, f := range ln.frags {
			run := runs[f.run]
			if run.kind == runBar {
				continue
			}
			sz, st := runStyle(run)
			cn := run.color
			if cn == "" {
				cn = theme.ColorNameForeground
			}
			var cl color.Color = b.col(cn)
			t := canvas.NewText(string(run.text[f.from:f.to]), cl)
			t.TextSize, t.TextStyle = sz, st
			t.Move(fyne.NewPos(f.x, ln.y))
			t.Resize(fyne.NewSize(f.w, ln.h))
			texts = append(texts, t)
		}
	}
	r.objs = append(append(bands, rects...), texts...)
}

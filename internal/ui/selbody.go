package ui

import (
	"html"
	"image/color"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Fyne's RichText cannot select text and exposes no layout, so a message body
// is shown by selBody: it takes the renderer's segments, lays them out itself
// (word wrap, so it knows where every character is) and owns selection. Copy
// is built from those same segments, which renderMail has already reduced to
// a safe subset, so the clipboard never carries raw remote HTML.

type runKind uint8

const (
	runText   runKind = iota
	runBar            // blockquote bar the renderer prefixes to quoted paragraphs
	runMarker         // list bullet or number
)

type selRun struct {
	text                          []rune
	kind                          runKind
	bold, italic, underline, mono bool
	size                          fyne.ThemeSizeName
	color                         fyne.ThemeColorName
	link                          *url.URL
}

type selPara struct {
	runs    []selRun
	quote   int
	item    bool // list item
	ordered bool
	listLvl int // nesting level of a list item, from 1
	heading int // 0, or 2 / 3 for the <h2> / <h3> a heading size maps to
	obj     fyne.CanvasObject
	otext   string // plain text of a non-text block (image, table)
	runOff  []int  // rune offset of each run within the paragraph
	start   int    // document offset
	n       int    // runes
}

// selDoc is a message body: paragraphs separated by one implicit newline.
type selDoc struct {
	paras []*selPara
	flat  []rune // every rune, paragraphs joined by '\n'
	// segPara[i] is the paragraph segment i starts in (len(segs) holds the count).
	segPara []int
	bands   []selBand
	lh      map[lhKey]float32 // line height per size and style
	mw      map[mwKey]float32 // measured widths
}

// selBand is a painted background over paragraphs [p0, p1).
type selBand struct {
	p0, p1 int
	col    color.Color
}

// bandPad is the vertical padding inside a band, above and below its text.
const bandPad = 6

// setBands maps the renderer's segment bands onto paragraphs.
func (d *selDoc) setBands(bs []mailBand) {
	d.bands = nil
	for _, b := range bs {
		if b.to > len(d.segPara)-1 || b.from < 0 {
			continue
		}
		if p0, p1 := d.segPara[b.from], d.segPara[b.to]; p1 > p0 {
			d.bands = append(d.bands, selBand{p0, p1, b.col})
		}
	}
}

var (
	barRe    = regexp.MustCompile(`^▎+ $`)
	markerRe = regexp.MustCompile(`^\x{a0}+(•|\d+\.) $`)
)

// buildSelDoc turns renderer segments into the selection model. It relies on
// how render.go marks quote bars and list markers (a leading placeholder-coloured
// "▎" run, and an nbsp-indented "• " / "1. " run); render_test pins that.
func buildSelDoc(segs []widget.RichTextSegment) *selDoc {
	d := &selDoc{}
	var cur *selPara
	end := func() {
		if cur != nil && len(cur.runs) > 0 {
			d.paras = append(d.paras, cur)
		}
		cur = nil
	}
	add := func(r selRun) {
		if cur == nil {
			cur = &selPara{}
		}
		cur.runs = append(cur.runs, r)
	}
	for _, s := range segs {
		d.segPara = append(d.segPara, len(d.paras))
		switch v := s.(type) {
		case *widget.TextSegment:
			r := selRun{text: []rune(v.Text), bold: v.Style.TextStyle.Bold, italic: v.Style.TextStyle.Italic,
				underline: v.Style.TextStyle.Underline, mono: v.Style.TextStyle.Monospace,
				size: v.Style.SizeName, color: v.Style.ColorName}
			first := cur == nil || len(cur.runs) == 0
			switch {
			case first && v.Style.ColorName == theme.ColorNamePlaceHolder && barRe.MatchString(v.Text):
				r.kind = runBar
				add(r)
				cur.quote = len(r.text) - 1
			case first && markerRe.MatchString(v.Text):
				r.kind = runMarker
				add(r)
				cur.item, cur.ordered = true, v.Text[len(v.Text)-2] == '.'
				cur.listLvl = strings.Count(v.Text, " ") / 4
			default:
				if !v.Inline() && v.Text == " " && cur != nil {
					end() // the renderer's paragraph terminator after a link
					continue
				}
				add(r)
				cur.quote = max(cur.quote, v.Style.QuotingDepth)
				switch v.Style.SizeName {
				case theme.SizeNameHeadingText:
					cur.heading = 2
				case theme.SizeNameSubHeadingText:
					cur.heading = 3
				}
			}
			if !v.Inline() {
				end()
			}
		case *widget.HyperlinkSegment:
			add(selRun{text: []rune(v.Text), link: v.URL, color: theme.ColorNameHyperlink})
		case *colLink:
			add(selRun{text: []rune(v.Text), link: v.URL, color: v.col})
		default:
			end()
			d.paras = append(d.paras, &selPara{obj: s.Visual(), otext: s.Textual()})
		}
	}
	end()
	d.segPara = append(d.segPara, len(d.paras))
	off := 0
	for i, p := range d.paras {
		p.start = off
		if p.obj != nil {
			p.n = len([]rune(p.otext))
			d.flat = append(d.flat, []rune(p.otext)...)
		}
		for _, r := range p.runs {
			p.runOff = append(p.runOff, p.n)
			p.n += len(r.text)
			d.flat = append(d.flat, r.text...)
		}
		off += p.n + 1
		if i < len(d.paras)-1 {
			d.flat = append(d.flat, '\n')
		}
	}
	return d
}

func (d *selDoc) total() int {
	if len(d.paras) == 0 {
		return 0
	}
	p := d.paras[len(d.paras)-1]
	return p.start + p.n
}

// slice returns the part of run r of p inside doc range [a, b) as a run.
func (p *selPara) slice(i, a, b int) (selRun, bool) {
	r := p.runs[i]
	lo, hi := max(a-p.start-p.runOff[i], 0), min(b-p.start-p.runOff[i], len(r.text))
	if hi <= lo {
		return r, false
	}
	r.text = r.text[lo:hi]
	return r, true
}

// fragment is the selection [a, b) as an HTML fragment and as plain text.
func (d *selDoc) fragment(a, b int) (h, plain string) {
	var hb, pb strings.Builder
	var lists []bool // open lists, innermost last; true = ordered
	closeTo := func(n int) {
		for len(lists) > n {
			tag := "ul"
			if lists[len(lists)-1] {
				tag = "ol"
			}
			hb.WriteString("</li></" + tag + ">")
			lists = lists[:len(lists)-1]
		}
	}
	var prev *selPara
	for _, p := range d.paras {
		if min(b, p.start+p.n) <= max(a, p.start) {
			continue
		}
		if p.obj != nil {
			closeTo(0)
			if t := strings.TrimSpace(p.otext); t != "" {
				hb.WriteString("<p>" + html.EscapeString(t) + "</p>")
				sepFor(&pb, prev, p)
				pb.WriteString(t)
				prev = p
			}
			continue
		}
		var inner, text strings.Builder
		for i := range p.runs {
			r, ok := p.slice(i, a, b)
			if !ok {
				continue
			}
			switch r.kind {
			case runBar:
				text.WriteString(strings.Repeat(">", len(r.text)-1) + " ")
			case runMarker:
				text.WriteString(strings.ReplaceAll(string(r.text), " ", " "))
			default:
				text.WriteString(string(r.text))
				inner.WriteString(selRunHTML(r, p.heading > 0))
			}
		}
		sepFor(&pb, prev, p)
		pb.WriteString(strings.TrimRight(text.String(), " \n"))
		prev = p
		if strings.TrimSpace(inner.String()) == "" {
			continue
		}
		if p.item {
			lvl := max(p.listLvl, 1)
			closeTo(lvl)
			if len(lists) == lvl && lists[lvl-1] != p.ordered {
				closeTo(lvl - 1) // a bulleted list followed by a numbered one
			}
			if len(lists) == lvl {
				hb.WriteString("</li>")
			}
			for len(lists) < lvl {
				tag := "<ul>"
				if p.ordered {
					tag = "<ol>"
				}
				hb.WriteString(tag)
				lists = append(lists, p.ordered)
				if len(lists) < lvl {
					hb.WriteString("<li>")
				}
			}
			hb.WriteString("<li>" + inner.String())
			continue
		}
		closeTo(0)
		hb.WriteString(strings.Repeat("<blockquote>", p.quote))
		switch {
		case p.heading > 0:
			hb.WriteString("<h" + strconv.Itoa(p.heading) + ">" + inner.String() + "</h" + strconv.Itoa(p.heading) + ">")
		default:
			hb.WriteString("<p>" + inner.String() + "</p>")
		}
		hb.WriteString(strings.Repeat("</blockquote>", p.quote))
	}
	closeTo(0)
	return hb.String(), pb.String()
}

// sepFor writes the plain-text gap before p: a line inside a list or quote, a blank line otherwise.
func sepFor(b *strings.Builder, prev, p *selPara) {
	switch {
	case prev == nil:
	case prev.item && p.item, prev.quote > 0 && p.quote > 0:
		b.WriteString("\n")
	default:
		b.WriteString("\n\n")
	}
}

// selRunHTML renders one run as inline HTML. Only the styles the renderer
// produces are emitted; colours are left to the paste target.
func selRunHTML(r selRun, inHeading bool) string {
	s := strings.ReplaceAll(html.EscapeString(string(r.text)), "\n", "<br>")
	if r.mono {
		s = "<code>" + s + "</code>"
	}
	if r.underline && r.link == nil {
		s = "<u>" + s + "</u>"
	}
	if r.italic {
		s = "<i>" + s + "</i>"
	}
	if r.bold && !inHeading {
		s = "<b>" + s + "</b>"
	}
	if r.link != nil && (r.link.Scheme == "http" || r.link.Scheme == "https" || r.link.Scheme == "mailto") {
		s = `<a href="` + html.EscapeString(r.link.String()) + `">` + s + `</a>`
	}
	return s
}

// ── layout ──────────────────────────────────────────────────────────────────

type selFrag struct {
	run, from, to    int // runes [from, to) of the run
	docStart, docEnd int
	x, w             float32
}

type selLine struct {
	para       int
	y, h       float32
	frags      []selFrag
	start, end int // document offsets; end includes a trailing newline
	textEnd    int // end of the last visible rune
	obj        fyne.CanvasObject
}

type selLayout struct {
	lines  []selLine
	height float32
	bands  []bandRect
	quotes []bandRect // one continuous quote bar per run of quoted paragraphs
}

// bandRect is a band's full-width rectangle: y and height.
type bandRect struct {
	y, h float32
	col  color.Color
}

func runStyle(r selRun) (float32, fyne.TextStyle) {
	n := r.size
	if n == "" {
		n = theme.SizeNameText
	}
	return theme.Size(n), fyne.TextStyle{Bold: r.bold, Italic: r.italic, Monospace: r.mono, Underline: r.underline}
}

func (d *selDoc) measure(r selRun, from, to int) float32 {
	if to <= from {
		return 0
	}
	if r.kind == runBar { // the bar is drawn as a rectangle: one fixed indent, whatever the depth
		return float32(quoteBarPad+1+quoteBarGap) * float32(to-from) / float32(len(r.text))
	}
	sz, st := runStyle(r)
	k := mwKey{string(r.text[from:to]), sz, st}
	if w, ok := d.mw[k]; ok {
		return w
	}
	if d.mw == nil {
		d.mw = map[mwKey]float32{}
	}
	w := fyne.MeasureText(k.s, sz, st).Width
	d.mw[k] = w
	return w
}

// mwKey caches the width of a piece of text: layouts repeat the same words.
type mwKey struct {
	s  string
	sz float32
	st fyne.TextStyle
}

type lhKey struct {
	sz float32
	st fyne.TextStyle
}

func (d *selDoc) lineHeight(r selRun) float32 {
	sz, st := runStyle(r)
	k := lhKey{sz, st}
	if h, ok := d.lh[k]; ok {
		return h
	}
	if d.lh == nil {
		d.lh = map[lhKey]float32{}
	}
	h := fyne.MeasureText("Ag", sz, st).Height
	d.lh[k] = h
	return h
}

func selBlank(r rune) bool { return r == ' ' || r == '\t' }

type selAtom struct{ run, from, to int }

func (d *selDoc) layout(width float32) *selLayout {
	l := &selLayout{bands: make([]bandRect, len(d.bands))}
	// bands open before their first paragraph and close after their last, outer first.
	opens, closes := map[int][]int{}, map[int][]int{}
	for i, b := range d.bands {
		l.bands[i].col = b.col
		opens[b.p0] = append(opens[b.p0], i)
		closes[b.p1-1] = append(closes[b.p1-1], i)
	}
	base := fyne.MeasureText("Ag", theme.TextSize(), fyne.TextStyle{}).Height
	// Inset like RichText's inner padding so text sits where it used to.
	ox := theme.InnerPadding()
	y := ox
	width -= 2 * ox
	for pi, p := range d.paras {
		if pi > 0 {
			y += theme.Padding()
		}
		for _, bi := range opens[pi] {
			l.bands[bi].y = y
			y += bandPad
		}
		closeBands := func() {
			for k := len(closes[pi]) - 1; k >= 0; k-- {
				bi := closes[pi][k]
				y += bandPad
				l.bands[bi].h = y - l.bands[bi].y
			}
		}
		if p.obj != nil {
			m := p.obj.MinSize()
			if _, ok := p.obj.(*widget.Separator); ok {
				m.Width = width
			}
			p.obj.Resize(m)
			l.lines = append(l.lines, selLine{para: pi, y: y, h: m.Height, start: p.start, end: p.start + p.n, textEnd: p.start, obj: p.obj})
			y += m.Height
			closeBands()
			continue
		}
		cur := selLine{para: pi, y: y, start: p.start, end: p.start, textEnd: p.start}
		x, startX := ox, ox
		contX := ox // wrapped lines of a quoted paragraph hang under the text, right of the bar
		if len(p.runs) > 0 && p.runs[0].kind == runBar {
			contX += quoteBarPad + 1 + quoteBarGap
		}
		made := 0
		finish := func() {
			made++
			if cur.h == 0 {
				cur.h = base
			}
			l.lines = append(l.lines, cur)
			y += cur.h
			cur = selLine{para: pi, y: y, start: cur.end, end: cur.end, textEnd: cur.end}
			x, startX = contX, contX
		}
		put := func(a selAtom, w float32) {
			r := p.runs[a.run]
			cur.h = max(cur.h, d.lineHeight(r))
			ds, de := p.start+p.runOff[a.run]+a.from, p.start+p.runOff[a.run]+a.to
			if n := len(cur.frags); n > 0 && cur.frags[n-1].run == a.run && cur.frags[n-1].to == a.from {
				f := &cur.frags[n-1]
				f.to, f.docEnd, f.w = a.to, de, f.w+w
			} else {
				cur.frags = append(cur.frags, selFrag{run: a.run, from: a.from, to: a.to, docStart: ds, docEnd: de, x: x, w: w})
			}
			x += w
			cur.end, cur.textEnd = de, de
		}
		// Tokens: runs of blanks, newlines, and words (non-blank text, possibly across runs).
		var word []selAtom
		var wordW float32
		flushWord := func() {
			if len(word) == 0 {
				return
			}
			if x > startX && x+wordW > width+ox {
				finish()
			}
			if wordW > width { // longer than the line: break between characters
				for _, a := range word {
					r := p.runs[a.run]
					for k := a.from; k < a.to; k++ {
						rw := d.measure(r, k, k+1)
						if x > startX && x+rw > width+ox {
							finish()
						}
						put(selAtom{a.run, k, k + 1}, rw)
					}
				}
			} else {
				for _, a := range word {
					put(a, d.measure(p.runs[a.run], a.from, a.to))
				}
			}
			word, wordW = nil, 0
		}
		for ri, r := range p.runs {
			for i := 0; i < len(r.text); {
				j := i + 1
				switch c := r.text[i]; {
				case c == '\n':
					flushWord()
					cur.end = p.start + p.runOff[ri] + j
					finish()
				case selBlank(c):
					for j < len(r.text) && selBlank(r.text[j]) {
						j++
					}
					flushWord()
					put(selAtom{ri, i, j}, d.measure(r, i, j))
				default:
					for j < len(r.text) && !selBlank(r.text[j]) && r.text[j] != '\n' {
						j++
					}
					w := d.measure(r, i, j)
					word, wordW = append(word, selAtom{ri, i, j}), wordW+w
				}
				i = j
			}
		}
		flushWord()
		cur.end = p.start + p.n
		if len(cur.frags) > 0 || made == 0 {
			finish()
		}
		closeBands()
	}
	l.height = y + ox
	var open *bandRect
	for pi, p := range d.paras {
		if len(p.runs) == 0 || p.runs[0].kind != runBar {
			open = nil
			continue
		}
		top, bot := float32(-1), float32(0)
		for _, ln := range l.lines {
			if ln.para == pi {
				if top < 0 {
					top = ln.y
				}
				bot = ln.y + ln.h
			}
		}
		if open == nil {
			l.quotes = append(l.quotes, bandRect{y: top})
			open = &l.quotes[len(l.quotes)-1]
		}
		open.h = bot - open.y
	}
	return l
}

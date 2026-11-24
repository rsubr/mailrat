package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Pure-Go document model for the WYSIWYG editor (see wysiwyg.go). It has no Fyne
// dependency so it can be unit-tested directly.

// wyStyle is the inline formatting of a run of text.
type wyStyle struct {
	Bold, Italic, Underline, Strike bool
	Color                           string // "#rrggbb", "" = default
	Size                            int    // px, 0 = default
	Link                            string // http, https or mailto URL
}

type wyRun struct {
	Text string
	wyStyle
}

type wyKind int

const (
	wyPara wyKind = iota
	wyBullet
	wyNumber
	wyQuote
)

// wyParagraph is one line-level block. Consecutive paragraphs of the same non-wyPara
// kind form one list or one blockquote.
type wyParagraph struct {
	Kind wyKind
	Runs []wyRun
	// Fold marks the first paragraph of the quoted original in a reply/forward:
	// it and everything after it form the quote, which the editor can hide.
	Fold bool
	// Sig marks the first paragraph of the signature (the "-- " line); the
	// signature runs to the quote or the end. It round-trips as gmail_signature.
	Sig bool
}

// wyDoc always holds at least one paragraph.
type wyDoc struct{ Paras []wyParagraph }

func newWyDoc() wyDoc { return wyDoc{Paras: []wyParagraph{{}}} }

// foldAt is the index of the first quote paragraph, or -1 when there is no quote.
// Index 0 never counts: the editor always keeps one visible paragraph.
func (d wyDoc) foldAt() int {
	for i := 1; i < len(d.Paras); i++ {
		if d.Paras[i].Fold {
			return i
		}
	}
	return -1
}

func (d wyDoc) clone() wyDoc {
	c := wyDoc{Paras: make([]wyParagraph, len(d.Paras))}
	for i, p := range d.Paras {
		c.Paras[i] = wyParagraph{Kind: p.Kind, Runs: append([]wyRun(nil), p.Runs...), Fold: p.Fold, Sig: p.Sig}
	}
	return c
}

func (p wyParagraph) length() int {
	n := 0
	for _, r := range p.Runs {
		n += len([]rune(r.Text))
	}
	return n
}

func (p wyParagraph) text() string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// cutRuns returns copies of the parts of runs inside rune range [a,b).
func cutRuns(runs []wyRun, a, b int) []wyRun {
	var out []wyRun
	pos := 0
	for _, r := range runs {
		rs := []rune(r.Text)
		lo, hi := max(a-pos, 0), min(b-pos, len(rs))
		if lo < hi {
			out = append(out, wyRun{Text: string(rs[lo:hi]), wyStyle: r.wyStyle})
		}
		pos += len(rs)
	}
	return out
}

// normRuns drops empty runs and merges neighbours with equal style.
func normRuns(runs []wyRun) []wyRun {
	var out []wyRun
	for _, r := range runs {
		if r.Text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].wyStyle == r.wyStyle {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

// charStyle is the style of the character at rune index i.
func (p wyParagraph) charStyle(i int) wyStyle {
	pos := 0
	for _, r := range p.Runs {
		n := len([]rune(r.Text))
		if i < pos+n {
			return r.wyStyle
		}
		pos += n
	}
	return wyStyle{}
}

// styleAt is the style typing at off would get: that of the previous character.
func (p wyParagraph) styleAt(off int) wyStyle {
	switch {
	case off > 0:
		return p.charStyle(off - 1)
	case len(p.Runs) > 0:
		return p.Runs[0].wyStyle
	}
	return wyStyle{}
}

type wyPos struct{ P, Off int }

func (a wyPos) less(b wyPos) bool { return a.P < b.P || (a.P == b.P && a.Off < b.Off) }

type wySnap struct {
	doc           wyDoc
	anchor, caret wyPos
}

const wyMaxUndo = 200

// wyEditor is the editing state: document, selection and undo history.
type wyEditor struct {
	doc           wyDoc
	anchor, caret wyPos
	pending       *wyStyle // style for the next typed text at a collapsed caret
	undo, redo    []wySnap
	last          string // kind of the last edit, for coalescing typing
	collapsed     bool   // the quote (from doc.foldAt) is hidden and out of reach of caret and selection
}

func newWyEditor() *wyEditor { return &wyEditor{doc: newWyDoc()} }

// setDoc replaces the document, resetting selection and history.
func (e *wyEditor) setDoc(d wyDoc) {
	if len(d.Paras) == 0 {
		d = newWyDoc()
	}
	e.doc = d
	e.collapsed = d.foldAt() >= 0
	e.anchor, e.caret = wyPos{}, wyPos{}
	e.pending, e.undo, e.redo, e.last = nil, nil, nil, ""
}

func (e *wyEditor) snap() wySnap { return wySnap{e.doc.clone(), e.anchor, e.caret} }

// begin records an undo point before a mutation. Consecutive "type" or "del" edits share one.
func (e *wyEditor) begin(kind string) {
	if !((kind == "type" || kind == "del") && e.last == kind) {
		e.undo = append(e.undo, e.snap())
		if len(e.undo) > wyMaxUndo {
			e.undo = e.undo[1:]
		}
	}
	e.redo = nil
	e.last = kind
}

func (e *wyEditor) restore(s wySnap) {
	e.doc, e.anchor, e.caret = s.doc.clone(), s.anchor, s.caret
	e.anchor, e.caret = e.clamp(e.anchor), e.clamp(e.caret)
	e.pending, e.last = nil, ""
}

// visible is the number of paragraphs the caret can reach.
func (e *wyEditor) visible() int {
	if f := e.doc.foldAt(); e.collapsed && f >= 0 {
		return f
	}
	return len(e.doc.Paras)
}

// setCollapsed hides or shows the quote; a caret inside a newly hidden quote moves to the end of the text above.
func (e *wyEditor) setCollapsed(c bool) {
	e.collapsed = c && e.doc.foldAt() >= 0
	e.anchor, e.caret = e.clamp(e.anchor), e.clamp(e.caret)
	e.pending = nil
}

func (e *wyEditor) canUndo() bool { return len(e.undo) > 0 }
func (e *wyEditor) canRedo() bool { return len(e.redo) > 0 }

func (e *wyEditor) doUndo() {
	if n := len(e.undo); n > 0 {
		e.redo = append(e.redo, e.snap())
		e.restore(e.undo[n-1])
		e.undo = e.undo[:n-1]
	}
}

func (e *wyEditor) doRedo() {
	if n := len(e.redo); n > 0 {
		e.undo = append(e.undo, e.snap())
		e.restore(e.redo[n-1])
		e.redo = e.redo[:n-1]
	}
}

// --- selection -------------------------------------------------------------

func (e *wyEditor) hasSel() bool { return e.anchor != e.caret }

func (e *wyEditor) selRange() (wyPos, wyPos) {
	if e.caret.less(e.anchor) {
		return e.caret, e.anchor
	}
	return e.anchor, e.caret
}

func (e *wyEditor) selectedText() string {
	s, t := e.selRange()
	var lines []string
	for i := s.P; i <= t.P; i++ {
		a, b := 0, e.doc.Paras[i].length()
		if i == s.P {
			a = s.Off
		}
		if i == t.P {
			b = t.Off
		}
		lines = append(lines, wyParagraph{Runs: cutRuns(e.doc.Paras[i].Runs, a, b)}.text())
	}
	return strings.Join(lines, "\n")
}

func (e *wyEditor) clamp(p wyPos) wyPos {
	if n := e.visible() - 1; p.P > n {
		p = wyPos{n, e.doc.Paras[n].length()}
	}
	p.P = max(0, p.P)
	p.Off = max(0, min(p.Off, e.doc.Paras[p.P].length()))
	return p
}

// moveTo puts the caret at p; with extend the anchor stays.
func (e *wyEditor) moveTo(p wyPos, extend bool) {
	e.caret = e.clamp(p)
	if !extend {
		e.anchor = e.caret
	}
	e.pending, e.last = nil, ""
}

func (e *wyEditor) selectAll() {
	e.anchor = wyPos{}
	e.moveTo(e.docEnd(), true)
}

func (e *wyEditor) docEnd() wyPos {
	n := e.visible() - 1
	return wyPos{n, e.doc.Paras[n].length()}
}

// clusterStarts are the rune offsets where a grapheme cluster (an emoji with its
// joiners and skin tone, a flag, a letter with its accents) starts, plus len(rs).
func clusterStarts(rs []rune) []int {
	b := []int{0}
	g := uniseg.NewGraphemes(string(rs))
	for g.Next() {
		b = append(b, b[len(b)-1]+utf8.RuneCountInString(g.Str()))
	}
	return b
}

// prevCluster is the start of the cluster that ends at or before off.
func prevCluster(rs []rune, off int) int {
	prev := 0
	for _, b := range clusterStarts(rs) {
		if b >= off {
			break
		}
		prev = b
	}
	return prev
}

// nextCluster is the end of the cluster that starts at or after off.
func nextCluster(rs []rune, off int) int {
	for _, b := range clusterStarts(rs) {
		if b > off {
			return b
		}
	}
	return len(rs)
}

func (e *wyEditor) left(p wyPos, word bool) wyPos {
	if p.Off == 0 {
		if p.P > 0 {
			return wyPos{p.P - 1, e.doc.Paras[p.P-1].length()}
		}
		return p
	}
	rs := []rune(e.doc.Paras[p.P].text())
	if !word {
		return wyPos{p.P, prevCluster(rs, p.Off)}
	}
	i := p.Off
	for i > 0 && unicode.IsSpace(rs[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(rs[i-1]) {
		i--
	}
	return wyPos{p.P, i}
}

func (e *wyEditor) right(p wyPos, word bool) wyPos {
	n := e.doc.Paras[p.P].length()
	if p.Off >= n {
		if p.P < e.visible()-1 {
			return wyPos{p.P + 1, 0}
		}
		return p
	}
	rs := []rune(e.doc.Paras[p.P].text())
	if !word {
		return wyPos{p.P, nextCluster(rs, p.Off)}
	}
	i := p.Off
	for i < n && !unicode.IsSpace(rs[i]) {
		i++
	}
	for i < n && unicode.IsSpace(rs[i]) {
		i++
	}
	return wyPos{p.P, i}
}

// wordAt returns the word range around p (or the whitespace run when not on a word).
func (e *wyEditor) wordAt(p wyPos) (wyPos, wyPos) {
	rs := []rune(e.doc.Paras[p.P].text())
	a, b := p.Off, p.Off
	if len(rs) == 0 {
		return p, p
	}
	i := min(p.Off, len(rs)-1)
	sp := unicode.IsSpace(rs[i])
	for a > 0 && unicode.IsSpace(rs[a-1]) == sp {
		a--
	}
	b = i
	for b < len(rs) && unicode.IsSpace(rs[b]) == sp {
		b++
	}
	return wyPos{p.P, a}, wyPos{p.P, b}
}

// --- editing ---------------------------------------------------------------

// deleteSel removes the selection and collapses the caret to its start.
func (e *wyEditor) deleteSel() {
	if !e.hasSel() {
		return
	}
	s, t := e.selRange()
	first := e.doc.Paras[s.P]
	last := e.doc.Paras[t.P]
	runs := append(cutRuns(first.Runs, 0, s.Off), cutRuns(last.Runs, t.Off, last.length())...)
	first.Runs = normRuns(runs)
	e.doc.Paras = append(e.doc.Paras[:s.P+1], e.doc.Paras[t.P+1:]...)
	e.doc.Paras[s.P] = first
	e.anchor, e.caret = s, s
}

// styleAtCaret is the formatting reported to the toolbar and used for typing.
func (e *wyEditor) styleAtCaret() wyStyle {
	if e.pending != nil {
		return *e.pending
	}
	if e.hasSel() {
		s, _ := e.selRange()
		p := e.doc.Paras[s.P]
		if s.Off < p.length() {
			return p.charStyle(s.Off)
		}
		return p.styleAt(s.Off)
	}
	return e.doc.Paras[e.caret.P].styleAt(e.caret.Off)
}

// typingStyle is the style typed text gets: like styleAtCaret, but text typed
// right after a link does not extend it.
func (e *wyEditor) typingStyle() wyStyle {
	st := e.styleAtCaret()
	if st.Link != "" && e.pending == nil && !e.hasSel() {
		p := e.doc.Paras[e.caret.P]
		if e.caret.Off >= p.length() || p.charStyle(e.caret.Off).Link != st.Link {
			st.Link = ""
		}
	}
	return st
}

func (e *wyEditor) blockKind() wyKind { return e.doc.Paras[e.caret.P].Kind }

// insertText types s at the caret, replacing the selection. "\n" starts a new paragraph.
func (e *wyEditor) insertText(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return
	}
	kind := "type"
	if len([]rune(s)) > 1 {
		kind = "paste"
	}
	e.begin(kind)
	st := e.typingStyle()
	e.deleteSel()
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			e.split()
		}
		e.insertRun(strings.ReplaceAll(line, "\t", "    "), st)
	}
	e.pending = nil
}

func (e *wyEditor) insertRun(text string, st wyStyle) {
	if text == "" {
		return
	}
	p := &e.doc.Paras[e.caret.P]
	runs := cutRuns(p.Runs, 0, e.caret.Off)
	runs = append(runs, wyRun{Text: text, wyStyle: st})
	runs = append(runs, cutRuns(p.Runs, e.caret.Off, p.length())...)
	p.Runs = normRuns(runs)
	e.caret.Off += len([]rune(text))
	e.anchor = e.caret
}

// split breaks the paragraph at the caret.
func (e *wyEditor) split() {
	i := e.caret.P
	p := e.doc.Paras[i]
	head := wyParagraph{Kind: p.Kind, Runs: normRuns(cutRuns(p.Runs, 0, e.caret.Off)), Fold: p.Fold, Sig: p.Sig}
	tail := wyParagraph{Kind: p.Kind, Runs: normRuns(cutRuns(p.Runs, e.caret.Off, p.length()))}
	e.doc.Paras = append(e.doc.Paras[:i+1], e.doc.Paras[i:]...)
	e.doc.Paras[i], e.doc.Paras[i+1] = head, tail
	e.caret = wyPos{i + 1, 0}
	e.anchor = e.caret
}

// enter starts a new paragraph; on an empty list or quote line it leaves the block instead.
func (e *wyEditor) enter() {
	e.begin("enter")
	st := e.typingStyle()
	e.deleteSel()
	p := &e.doc.Paras[e.caret.P]
	if p.length() == 0 && p.Kind != wyPara {
		p.Kind = wyPara
		return
	}
	e.split()
	if st != (wyStyle{}) {
		e.pending = &st
	}
}

func (e *wyEditor) mergeNext(i int) {
	a, b := e.doc.Paras[i], e.doc.Paras[i+1]
	a.Runs = normRuns(append(append([]wyRun(nil), a.Runs...), b.Runs...))
	e.doc.Paras[i] = a
	e.doc.Paras = append(e.doc.Paras[:i+1], e.doc.Paras[i+2:]...)
}

func (e *wyEditor) backspace(word bool) {
	if !e.hasSel() {
		p := e.caret
		if p.Off == 0 && e.doc.Paras[p.P].Kind != wyPara {
			e.begin("block")
			e.doc.Paras[p.P].Kind = wyPara
			return
		}
		if p.P == 0 && p.Off == 0 {
			return
		}
		e.anchor = e.left(p, word)
	}
	e.begin("del")
	e.delRange()
}

func (e *wyEditor) deleteForward(word bool) {
	if !e.hasSel() {
		if e.caret == e.docEnd() {
			return
		}
		e.anchor = e.right(e.caret, word)
	}
	e.begin("del")
	e.delRange()
}

// delRange deletes the selection, merging paragraphs when it spans breaks.
func (e *wyEditor) delRange() {
	e.deleteSel()
	e.pending = nil
}

// cut removes the selection as an undoable step.
func (e *wyEditor) cut() {
	if e.hasSel() {
		e.begin("cut")
		e.deleteSel()
	}
}

// --- formatting ------------------------------------------------------------

// mapRange applies f to the style of every character in [s,t).
func (e *wyEditor) mapRange(s, t wyPos, f func(*wyStyle)) {
	for i := s.P; i <= t.P; i++ {
		p := &e.doc.Paras[i]
		a, b := 0, p.length()
		if i == s.P {
			a = s.Off
		}
		if i == t.P {
			b = t.Off
		}
		if a >= b {
			continue
		}
		mid := cutRuns(p.Runs, a, b)
		for k := range mid {
			f(&mid[k].wyStyle)
		}
		runs := cutRuns(p.Runs, 0, a)
		runs = append(runs, mid...)
		runs = append(runs, cutRuns(p.Runs, b, p.length())...)
		p.Runs = normRuns(runs)
	}
}

// all reports whether every selected character satisfies pred.
func (e *wyEditor) all(pred func(wyStyle) bool) bool {
	s, t := e.selRange()
	seen := false
	for i := s.P; i <= t.P; i++ {
		p := e.doc.Paras[i]
		a, b := 0, p.length()
		if i == s.P {
			a = s.Off
		}
		if i == t.P {
			b = t.Off
		}
		for k := a; k < b; k++ {
			seen = true
			if !pred(p.charStyle(k)) {
				return false
			}
		}
	}
	return seen
}

// format applies f to the selection, or to the typing style at a collapsed caret.
func (e *wyEditor) format(f func(*wyStyle)) {
	if !e.hasSel() {
		st := e.styleAtCaret()
		f(&st)
		e.pending = &st
		return
	}
	e.begin("fmt")
	s, t := e.selRange()
	e.mapRange(s, t, f)
}

func (e *wyEditor) toggle(get func(wyStyle) bool, set func(*wyStyle, bool)) {
	var on bool
	if e.hasSel() {
		on = e.all(get)
	} else {
		on = get(e.styleAtCaret())
	}
	e.format(func(s *wyStyle) { set(s, !on) })
}

func (e *wyEditor) toggleBold() {
	e.toggle(func(s wyStyle) bool { return s.Bold }, func(s *wyStyle, v bool) { s.Bold = v })
}
func (e *wyEditor) toggleItalic() {
	e.toggle(func(s wyStyle) bool { return s.Italic }, func(s *wyStyle, v bool) { s.Italic = v })
}
func (e *wyEditor) toggleUnderline() {
	e.toggle(func(s wyStyle) bool { return s.Underline }, func(s *wyStyle, v bool) { s.Underline = v })
}
func (e *wyEditor) toggleStrike() {
	e.toggle(func(s wyStyle) bool { return s.Strike }, func(s *wyStyle, v bool) { s.Strike = v })
}

// setColor sets the text colour ("#rrggbb"); anything invalid or "" resets to default.
func (e *wyEditor) setColor(c string) {
	c = parseColor(c)
	e.format(func(s *wyStyle) { s.Color = c })
}

// setSize sets the font size in px (clamped to 8..96); 0 resets to default.
func (e *wyEditor) setSize(px int) {
	if px != 0 {
		px = max(8, min(px, 96))
	}
	e.format(func(s *wyStyle) { s.Size = px })
}

func (e *wyEditor) clearFormatting() {
	e.format(func(s *wyStyle) { *s = wyStyle{Link: s.Link} }) // links are content, not formatting
}

// linkExtent returns the range of the link at a collapsed caret.
func (e *wyEditor) linkExtent() (s, t wyPos, url string) {
	p := e.doc.Paras[e.caret.P]
	url = p.styleAt(e.caret.Off).Link
	if url == "" || e.hasSel() {
		return s, t, ""
	}
	a, b := e.caret.Off, e.caret.Off
	for a > 0 && p.charStyle(a-1).Link == url {
		a--
	}
	for b < p.length() && p.charStyle(b).Link == url {
		b++
	}
	return wyPos{e.caret.P, a}, wyPos{e.caret.P, b}, url
}

// linkAtCaret returns the URL of the link at the caret or selection start, or "".
func (e *wyEditor) linkAtCaret() string { return e.styleAtCaret().Link }

// setLink links the selection (or the link at the caret) to raw; "" removes the link.
// With nothing selected and no link at the caret the URL itself is inserted as the text.
func (e *wyEditor) setLink(raw string) bool {
	url, ok := cleanLink(raw)
	if !ok {
		return false
	}
	set := func(s *wyStyle) { s.Link = url }
	switch {
	case e.hasSel():
		e.format(set)
	default:
		if s, t, cur := e.linkExtent(); cur != "" {
			e.begin("fmt")
			e.mapRange(s, t, set)
		} else if url != "" {
			e.begin("type")
			e.last = ""
			st := e.styleAtCaret()
			st.Link = url
			e.insertRun(url, st)
		}
	}
	return true
}

// toggleKind turns the selected paragraphs into k, or back to plain when all already are k.
func (e *wyEditor) toggleKind(k wyKind) {
	e.begin("block")
	s, t := e.selRange()
	allK := true
	for i := s.P; i <= t.P; i++ {
		allK = allK && e.doc.Paras[i].Kind == k
	}
	for i := s.P; i <= t.P; i++ {
		if allK {
			e.doc.Paras[i].Kind = wyPara
		} else {
			e.doc.Paras[i].Kind = k
		}
	}
}

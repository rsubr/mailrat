package ui

import (
	"image/color"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Fyne's RichText cannot paint backgrounds, so the renderer reports author
// backgrounds as bands: a solid rectangle behind the paragraphs of a
// block-level element, full width of the body. Nested bands paint inner over
// outer. Limits, by design: side-by-side cells still stack, inline highlights
// (span/font backgrounds) and background images/gradients are not painted
// (a solid colour beside the image is used), and cells of grid tables are
// unpainted.

// mailBand is a painted background over segments [from, to) of the result.
type mailBand struct {
	from, to int
	col      color.NRGBA
	depth    int
}

// maxBands caps band openings per message; beyond it text is simply unpainted.
const maxBands = 2000

var bgHex = regexp.MustCompile(`^[0-9a-f]{6}$`)

// bandTag reports whether a background on this element is painted.
func bandTag(tag string) bool {
	switch tag {
	case "body", "table", "tr", "td", "th", "div", "p", "blockquote", "center", "section", "article",
		"header", "footer", "main", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "pre":
		return true
	}
	return false
}

// bgOf is the solid background colour n declares (bgcolor, background-color or
// the colour in a background shorthand). Translucent and unknown values give none.
func bgOf(n *html.Node) (color.NRGBA, bool) {
	var out color.NRGBA
	ok := false
	set := func(v string) {
		v = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.ToLower(v), "!important")))
		if bgHex.MatchString(v) {
			v = "#" + v
		}
		if strings.HasPrefix(v, "rgba") || strings.HasPrefix(v, "hsl") || strings.Contains(v, "(") && !strings.HasPrefix(v, "rgb(") {
			return
		}
		if c, k := parseCSSColor(v); k {
			out, ok = color.NRGBAModel.Convert(c).(color.NRGBA), true
		}
	}
	if v := attr(n, "bgcolor"); v != "" {
		set(v)
	}
	for _, decl := range strings.Split(attr(n, "style"), ";") {
		k, v, found := strings.Cut(decl, ":")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "background-color":
			set(v)
		case "background":
			for _, t := range strings.Fields(v) {
				if strings.Contains(t, "(") && !strings.HasPrefix(strings.ToLower(t), "rgb(") {
					continue
				}
				set(t)
			}
		}
	}
	return out, ok
}

// near reports whether two colours are visually the same.
func near(a, b color.Color) bool {
	x, y := color.NRGBAModel.Convert(a).(color.NRGBA), color.NRGBAModel.Convert(b).(color.NRGBA)
	d := func(p, q uint8) int { return max(int(p)-int(q), int(q)-int(p)) }
	return d(x.R, y.R) <= 3 && d(x.G, y.G) <= 3 && d(x.B, y.B) <= 3
}

// openBand starts painting col behind everything walked until the returned
// closer runs. It returns st unchanged and no closer when nothing is painted.
func (r *htmlRenderer) openBand(col color.NRGBA, st textStyle) (textStyle, func()) {
	var under color.Color = r.opts.bg
	if st.bg != nil {
		under = st.bg
	}
	if r.flat || r.nbands >= maxBands || near(col, under) {
		return st, nil
	}
	r.flush()
	idx := len(r.bands)
	r.bands = append(r.bands, mailBand{from: len(r.out.segments), to: -1, col: col, depth: r.bdepth})
	r.nbands++
	r.bdepth++
	st.bg = col
	return st, func() {
		r.flush()
		r.bdepth--
		r.bands[idx].to = len(r.out.segments)
	}
}

// band paints n's own background when it has one.
func (r *htmlRenderer) band(n *html.Node, st textStyle) (textStyle, func()) {
	if !bandTag(n.Data) {
		return st, nil
	}
	if c, ok := bgOf(n); ok {
		return r.openBand(c, st)
	}
	return st, nil
}

// rowBand paints a row whose inline cells all declare the same background
// (walkRow lays those cells out on one line, so one band covers them).
func (r *htmlRenderer) rowBand(tr *html.Node, st textStyle) (textStyle, func()) {
	var col color.NRGBA
	have := false
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") || skipElement(c) || strings.TrimSpace(nodeText(c)) == "" {
			continue
		}
		bg, ok := bgOf(c)
		if hasBlock(c) || !ok || have && bg != col {
			return st, nil
		}
		col, have = bg, true
	}
	if !have {
		return st, nil
	}
	return r.openBand(col, st)
}

// cleanBands drops empty bands and merges neighbours of the same depth and
// colour that touch, so a table of identical rows is one rectangle.
func cleanBands(in []mailBand) []mailBand {
	var out []mailBand
	last := map[[2]int]int{} // (depth, packed colour) -> index in out
	for _, b := range in {
		if b.to <= b.from {
			continue
		}
		key := [2]int{b.depth, int(b.col.R)<<16 | int(b.col.G)<<8 | int(b.col.B)}
		if i, ok := last[key]; ok && out[i].to == b.from {
			out[i].to = b.to
			continue
		}
		last[key] = len(out)
		out = append(out, b)
	}
	return out
}

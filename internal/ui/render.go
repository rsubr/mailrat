package ui

import (
	"fmt"
	"image/color"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/net/html"
)

// renderOpts controls HTML mail rendering.
type renderOpts struct {
	bg, fg color.Color // pane background / default text colour, for contrast repair
	images bool        // fetch remote images
}

// renderResult is the rendered body plus what was held back.
type renderResult struct {
	segments []widget.RichTextSegment
	blocked  int // remote images not loaded
}

// minContrast is the WCAG ratio below which an author-chosen text colour is
// replaced. 3.5 keeps muted greys that are still legible and rejects black on
// black / white on white.
const minContrast = 3.5

type textStyle struct {
	bold, italic, mono, pre bool
	col                     color.Color
	size                    fyne.ThemeSizeName
	link                    *url.URL
	quote                   int
}

type htmlRenderer struct {
	opts renderOpts
	out  renderResult
	cur  []widget.RichTextSegment
	// quote is the blockquote depth of the paragraph being built.
	quote int
	// list state: ordered flag and next number; 0 means not in a list.
	ordered bool
	n       int
	inList  bool
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"')]+[^\s<>"').,;:!?]`)

// renderMail renders an email body (HTML or plain text) as styled rich text.
// Author colours are kept only when readable on the current pane background,
// so mail that ignores dark mode never ends up black-on-black. Author
// backgrounds are dropped for the same reason.
func renderMail(src string, o renderOpts) renderResult {
	r := &htmlRenderer{opts: o}
	if !strings.Contains(src, "<") {
		r.plain(src)
		return r.out
	}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		r.plain(tagRe.ReplaceAllString(src, ""))
		return r.out
	}
	r.walk(doc, textStyle{})
	r.flush()
	return r.out
}

func (r *htmlRenderer) plain(s string) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	for _, para := range strings.Split(s, "\n\n") {
		last := 0
		for _, m := range urlRe.FindAllStringIndex(para, -1) {
			r.text(para[last:m[0]], textStyle{pre: true})
			if u, err := url.Parse(para[m[0]:m[1]]); err == nil {
				r.cur = append(r.cur, &widget.HyperlinkSegment{Text: para[m[0]:m[1]], URL: u})
			}
			last = m[1]
		}
		r.text(para[last:], textStyle{pre: true})
		r.flush()
	}
}

// flush ends the current paragraph.
func (r *htmlRenderer) flush() {
	if len(r.cur) == 0 {
		return
	}
	empty := true
	for _, s := range r.cur {
		if strings.TrimSpace(s.Textual()) != "" {
			empty = false
		}
	}
	segs := r.cur
	r.cur = nil
	if empty {
		return
	}
	if r.quote > 0 {
		bar := &widget.TextSegment{Text: strings.Repeat("▎", r.quote) + " ", Style: widget.RichTextStyle{Inline: true, ColorName: theme.ColorNamePlaceHolder}}
		segs = append([]widget.RichTextSegment{bar}, segs...)
	}
	// End the paragraph: the last run must be a block so the next one starts a new line.
	if t, ok := segs[len(segs)-1].(*widget.TextSegment); ok {
		t.Text = strings.TrimRight(t.Text, " ")
		t.Style.Inline = false
	} else {
		segs = append(segs, &widget.TextSegment{Text: " ", Style: widget.RichTextStyle{Inline: false}})
	}
	r.out.segments = append(r.out.segments, segs...)
}

func (r *htmlRenderer) block(seg widget.RichTextSegment) {
	r.flush()
	r.out.segments = append(r.out.segments, seg)
}

var spaceRun = regexp.MustCompile(`[ \t\r\n\f\x{a0}]+`)

func (r *htmlRenderer) text(s string, st textStyle) {
	r.quote = st.quote
	if !st.pre {
		s = spaceRun.ReplaceAllString(s, " ")
		// Drop a leading space at paragraph start or after another space.
		if s == " " || strings.HasPrefix(s, " ") {
			if len(r.cur) == 0 || strings.HasSuffix(r.curText(), " ") || strings.HasSuffix(r.curText(), "\n") {
				s = strings.TrimLeft(s, " ")
			}
		}
	}
	if s == "" {
		return
	}
	if st.link != nil {
		r.cur = append(r.cur, &widget.HyperlinkSegment{Text: s, URL: st.link})
		return
	}
	ts := &widget.TextSegment{Text: s, Style: widget.RichTextStyle{
		Inline:       true,
		TextStyle:    fyne.TextStyle{Bold: st.bold || st.size != "", Italic: st.italic, Monospace: st.mono},
		SizeName:     st.size,
		QuotingDepth: st.quote,
		ColorName:    r.colorName(st.col),
	}}
	r.cur = append(r.cur, ts)
}

func (r *htmlRenderer) curText() string {
	if len(r.cur) == 0 {
		return ""
	}
	return r.cur[len(r.cur)-1].Textual()
}

// colorName maps an author colour to a theme colour name, repairing contrast.
func (r *htmlRenderer) colorName(c color.Color) fyne.ThemeColorName {
	if c == nil {
		return theme.ColorNameForeground
	}
	return hexName(readable(c, r.opts.bg, r.opts.fg))
}

func hexName(c color.Color) fyne.ThemeColorName {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fyne.ThemeColorName(fmt.Sprintf("#%02x%02x%02x", n.R, n.G, n.B))
}

// parseHexName is the inverse of hexName, used by the theme.
func parseHexName(n fyne.ThemeColorName) (color.Color, bool) {
	s := string(n)
	if len(s) != 7 || s[0] != '#' {
		return nil, false
	}
	return parseHex(s)
}

func (r *htmlRenderer) walk(n *html.Node, st textStyle) {
	switch n.Type {
	case html.TextNode:
		r.text(n.Data, st)
		return
	case html.ElementNode:
		if skipElement(n) {
			return
		}
		st = r.styleFor(n, st)
		switch n.Data {
		case "br":
			r.text("\n", textStyle{pre: true, quote: st.quote})
			return
		case "hr":
			r.block(&widget.SeparatorSegment{})
			return
		case "img":
			r.image(n, st)
			return
		case "ul", "ol":
			r.flush()
			savedOrdered, savedN, savedIn := r.ordered, r.n, r.inList
			r.ordered, r.n, r.inList = n.Data == "ol", 0, true
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				r.walk(c, st)
			}
			r.flush()
			r.ordered, r.n, r.inList = savedOrdered, savedN, savedIn
			return
		case "li":
			r.flush()
			marker := "• "
			if r.ordered {
				r.n++
				marker = strconv.Itoa(r.n) + ". "
			}
			r.cur = append(r.cur, &widget.TextSegment{Text: marker, Style: widget.RichTextStyle{Inline: true, ColorName: theme.ColorNameForeground}})
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				r.walk(c, st)
			}
			r.flush()
			return
		}
	}
	block := n.Type == html.ElementNode && isBlock(n.Data) || n.Type == html.ElementNode && (n.Data == "td" || n.Data == "th")
	if block {
		r.flush()
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.walk(c, st)
	}
	if block {
		r.flush()
	}
}

func (r *htmlRenderer) image(n *html.Node, st textStyle) {
	src, alt := attr(n, "src"), strings.TrimSpace(attr(n, "alt"))
	w, _ := strconv.Atoi(strings.TrimSuffix(attr(n, "width"), "px"))
	h, _ := strconv.Atoi(strings.TrimSuffix(attr(n, "height"), "px"))
	if (w > 0 && w <= 2) || (h > 0 && h <= 2) {
		return // tracking pixel
	}
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		if alt != "" {
			r.text(alt, st)
		}
		return
	}
	if !r.opts.images {
		r.out.blocked++
		if alt != "" {
			r.text("["+alt+"]", st)
		}
		return
	}
	r.block(&widget.ImageSegment{Source: storage.NewURI(src), Title: alt})
}

func skipElement(n *html.Node) bool {
	switch n.Data {
	case "script", "style", "head", "title", "template", "noscript", "meta", "link", "svg", "object", "iframe":
		return true
	}
	s := strings.ToLower(strings.ReplaceAll(attr(n, "style"), " ", ""))
	return strings.Contains(s, "display:none") || strings.Contains(s, "visibility:hidden") ||
		strings.Contains(s, "max-height:0") || strings.Contains(s, "font-size:0")
}

func (r *htmlRenderer) styleFor(n *html.Node, st textStyle) textStyle {
	switch n.Data {
	case "b", "strong", "th":
		st.bold = true
	case "i", "em", "cite", "dfn":
		st.italic = true
	case "code", "tt", "kbd", "samp":
		st.mono = true
	case "pre":
		st.mono, st.pre = true, true
	case "blockquote":
		st.quote++
	case "h1", "h2":
		st.size = theme.SizeNameHeadingText
	case "h3", "h4", "h5", "h6":
		st.size = theme.SizeNameSubHeadingText
	case "a":
		if u, err := url.Parse(strings.TrimSpace(attr(n, "href"))); err == nil &&
			(u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "mailto") {
			st.link = u
		}
	case "font":
		if c, ok := parseCSSColor(attr(n, "color")); ok {
			st.col = c
		}
	}
	if style := attr(n, "style"); style != "" {
		for _, decl := range strings.Split(style, ";") {
			k, v, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			k, v = strings.ToLower(strings.TrimSpace(k)), strings.ToLower(strings.TrimSpace(v))
			switch k {
			case "color":
				if c, ok := parseCSSColor(v); ok {
					st.col = c
				}
			case "font-weight":
				if v == "bold" || v == "bolder" || v >= "600" && len(v) == 3 {
					st.bold = true
				}
			case "font-style":
				st.italic = v == "italic" || v == "oblique"
			}
		}
	}
	return st
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// ── colour handling ─────────────────────────────────────────────────────────

var namedColors = map[string]color.NRGBA{
	"black": {0, 0, 0, 255}, "white": {255, 255, 255, 255}, "red": {255, 0, 0, 255},
	"blue": {0, 0, 255, 255}, "green": {0, 128, 0, 255}, "gray": {128, 128, 128, 255},
	"grey": {128, 128, 128, 255}, "silver": {192, 192, 192, 255}, "navy": {0, 0, 128, 255},
	"maroon": {128, 0, 0, 255}, "purple": {128, 0, 128, 255}, "orange": {255, 165, 0, 255},
	"yellow": {255, 255, 0, 255}, "teal": {0, 128, 128, 255}, "darkgray": {169, 169, 169, 255},
	"darkgrey": {169, 169, 169, 255}, "dimgray": {105, 105, 105, 255},
}

var rgbFn = regexp.MustCompile(`^rgba?\(\s*(\d+)\s*[, ]\s*(\d+)\s*[, ]\s*(\d+)`)

func parseCSSColor(s string) (color.Color, bool) {
	s = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.ToLower(s), "!important")))
	if c, ok := namedColors[s]; ok {
		return c, true
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) == 6 {
			if v, err := strconv.ParseUint(h, 16, 32); err == nil {
				return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
			}
		}
		return nil, false
	}
	if m := rgbFn.FindStringSubmatch(s); m != nil {
		c := func(i int) uint8 {
			v, _ := strconv.Atoi(m[i])
			if v > 255 {
				v = 255
			}
			return uint8(v)
		}
		return color.NRGBA{c(1), c(2), c(3), 255}, true
	}
	return nil, false
}

func luminance(c color.Color) float64 {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(n.R) + 0.7152*lin(n.G) + 0.0722*lin(n.B)
}

func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// readable returns c if it is legible on bg. Otherwise it tries c with its
// lightness mirrored (keeping hue, so a navy link becomes a light blue in dark
// mode) and finally falls back to the default foreground.
func readable(c, bg, fallback color.Color) color.Color {
	if contrast(c, bg) >= minContrast {
		return c
	}
	if flipped := flipLightness(c); contrast(flipped, bg) >= minContrast {
		return flipped
	}
	return fallback
}

func flipLightness(c color.Color) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	h, s, l := rgbToHSL(n)
	return hslToRGB(h, s, 1-l)
}

func rgbToHSL(c color.NRGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslToRGB(h, s, l float64) color.NRGBA {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return color.NRGBA{v, v, v, 255}
	}
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	to := func(v float64) uint8 { return uint8(math.Round(v * 255)) }
	return color.NRGBA{to(hue(h + 1.0/3)), to(hue(h)), to(hue(h - 1.0/3)), 255}
}

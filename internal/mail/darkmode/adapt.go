// Package darkmode adapts already-sanitised message HTML for a dark UI in the
// manner of Gmail for Android: light backgrounds are darkened (hue kept),
// already-dark areas are left alone, and every text run is guaranteed WCAG
// contrast against its effective background. It is pure and deterministic.
// Light mode is not handled here: callers render the original HTML as-is.
package darkmode

import (
	"bytes"
	"errors"
	"image/color"
	"math"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const (
	// MaxInput is the largest accepted HTML body in bytes.
	MaxInput = 8 << 20
	maxDepth = 3000

	minNormal = 4.5 // WCAG AA, normal text
	minLarge  = 3.0 // WCAG AA, >= 24px or >= 18.66px bold
	// lightBg is the OKLab lightness above which a background is considered
	// "light" and gets darkened; darker backgrounds are kept as designed.
	lightBg = 0.62
	// textFlip is the OKLab lightness below which text counts as "dark" and
	// is lightness-mirrored.
	textFlip = 0.50
	// target adds headroom over the minimum so rounding never dips below it.
	target = 0.25
)

var (
	ErrTooLarge = errors.New("darkmode: input too large")
	ErrTooDeep  = errors.New("darkmode: input nested too deeply")
)

// ImageInfo is optional knowledge about an image, usually from decoded bytes.
type ImageInfo struct {
	Known    bool // false: unknown, fall back to the attribute heuristic
	HasAlpha bool // image has transparent pixels
	Dark     bool // opaque pixels are predominantly dark (mean luminance low)
}

// Theme describes the app's dark surface. Nil colours take defaults.
type Theme struct {
	Background color.Color // app dark surface; default #1e1e1e
	Foreground color.Color // default text; default #f2f2f7
	Accent     color.Color // links without an author colour; default #0a84ff
	// DropBackgrounds removes every author background instead of darkening
	// it, and treats the surface as the effective background everywhere.
	// Use it when the renderer cannot paint block backgrounds (Fyne RichText).
	DropBackgrounds bool
	// KeepColors is the light-mode pass: <style> rules and legacy attributes
	// are still inlined and every colour is rewritten as an opaque #rrggbb
	// (backgrounds composited over their parents), but nothing is remapped or
	// repaired. The renderer paints the backgrounds and does its own contrast
	// guard. DropBackgrounds is ignored.
	KeepColors bool
	// ImageHint, if set, is asked about each <img src>; Known results replace
	// the attribute heuristic.
	ImageHint func(src string) ImageInfo
}

// Stats counts what Adapt changed.
type Stats struct {
	BgRemapped    int // elements whose background was darkened, replaced or dropped
	TextRemapped  int // elements whose text colour was changed or forced
	LinksRemapped int // <a> elements whose colour was set
	ImagesChipped int // <img> given a light chip background
	Unsupported   []string
}

// Result is the adapted document.
type Result struct {
	HTML  string
	Stats Stats
}

type ctx struct {
	bgOrig, bgAd color.NRGBA  // effective background as authored / as adapted
	fgSrc        *color.NRGBA // authored inherited text colour (nil = default)
	fgAd         color.NRGBA  // adapted inherited text colour
	px           float64
	bold         bool
}

type adapter struct {
	th              Theme
	surface, fg, ac color.NRGBA
	surfL, fgL      float64
	sheet           *sheet
	st              Stats
	seen            map[string]bool
}

func (a *adapter) note(s string) {
	if a.seen[s] || len(a.st.Unsupported) >= 32 {
		return
	}
	a.seen[s] = true
	a.st.Unsupported = append(a.st.Unsupported, s)
}

func orDefault(c color.Color, def color.NRGBA) color.NRGBA {
	if c == nil {
		return def
	}
	return opaque(c)
}

// Adapt returns a dark-mode version of src. It never panics on malformed
// input; HTML that cannot be handled safely yields an error.
func Adapt(src string, th Theme) (Result, error) {
	if len(src) > MaxInput {
		return Result{}, ErrTooLarge
	}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return Result{}, err
	}
	a := &adapter{th: th, sheet: newSheet(), seen: map[string]bool{}}
	a.surface = orDefault(th.Background, color.NRGBA{0x1e, 0x1e, 0x1e, 255})
	a.fg = orDefault(th.Foreground, color.NRGBA{0xf2, 0xf2, 0xf7, 255})
	a.ac = orDefault(th.Accent, color.NRGBA{0x0a, 0x84, 0xff, 255})
	a.surfL = math.Min(toLCH(a.surface).L, 0.36)
	a.fgL = toLCH(a.fg).L
	a.collectStyles(doc, 0)
	root := ctx{
		bgOrig: color.NRGBA{255, 255, 255, 255}, bgAd: a.surface,
		fgAd: a.fg, px: 16,
	}
	if err := a.walk(doc, root, 0); err != nil {
		return Result{}, err
	}
	var b bytes.Buffer
	if err := html.Render(&b, doc); err != nil {
		return Result{}, err
	}
	return Result{HTML: b.String(), Stats: a.st}, nil
}

// collectStyles feeds every <style> text into the sheet and removes the
// element: its rules are inlined per element during the walk.
func (a *adapter) collectStyles(n *html.Node, depth int) {
	if depth > maxDepth {
		return
	}
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && c.Data == "style" {
			var sb strings.Builder
			for t := c.FirstChild; t != nil; t = t.NextSibling {
				if t.Type == html.TextNode {
					sb.WriteString(t.Data)
				}
			}
			a.sheet.add(sb.String(), a.note)
			n.RemoveChild(c)
		} else {
			a.collectStyles(c, depth+1)
		}
		c = next
	}
}

func (a *adapter) walk(n *html.Node, c ctx, depth int) error {
	if depth > maxDepth {
		return ErrTooDeep
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "head", "script", "noscript", "template":
			return nil
		}
		c = a.element(n, c)
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if err := a.walk(ch, c, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// mapBg darkens light backgrounds (hue kept, chroma damped, lightness mapped
// linearly so white becomes the app surface); dark backgrounds are kept.
func (a *adapter) mapBg(c color.NRGBA) color.NRGBA {
	l := toLCH(c)
	if l.L < lightBg {
		return c
	}
	t := (l.L - lightBg) / (1 - lightBg)
	mid := 0.36
	l.L = mid - t*(mid-a.surfL)
	l.C *= 0.8
	return fromLCH(l)
}

// adaptText maps an authored text colour onto bg with at least min contrast.
func (a *adapter) adaptText(s, bg color.NRGBA, min float64) color.NRGBA {
	l := toLCH(s)
	out := s
	flipped := false
	if l.L < textFlip {
		l.L, l.C, flipped = 1-l.L, l.C*0.9, true
	}
	if l.C < 0.04 && (flipped || l.L > a.fgL) {
		out = a.fg // near-neutral: the theme foreground, never pure white
	} else if flipped {
		out = fromLCH(l)
	}
	return ensure(out, bg, min+target)
}

func (a *adapter) softBorder(c *color.NRGBA, bg color.NRGBA) color.NRGBA {
	bl := toLCH(bg).L
	l := lch{L: bl + 0.16}
	if c != nil {
		l = toLCH(*c)
		l.L = math.Max(bl+0.08, math.Min(bl+0.22, 1-l.L))
		l.C *= 0.5
	}
	return fromLCH(l)
}

var borderProps = map[string]bool{
	"border": true, "border-color": true, "border-top": true, "border-right": true,
	"border-bottom": true, "border-left": true, "border-top-color": true,
	"border-right-color": true, "border-bottom-color": true, "border-left-color": true,
	"outline": true, "outline-color": true,
}

func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ', '\t', '\n', '\r':
			if depth == 0 {
				if i > start {
					out = append(out, s[start:i])
				}
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func isImageToken(v string) bool {
	v = strings.ToLower(v)
	return strings.Contains(v, "url(") || strings.Contains(v, "gradient(") || strings.Contains(v, "image(") || strings.Contains(v, "image-set(")
}

func parsePx(v string, parent float64) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	kw := map[string]float64{"xx-small": 9, "x-small": 10, "small": 13, "medium": 16, "large": 18, "x-large": 24, "xx-large": 32, "xxx-large": 48}
	if f, ok := kw[v]; ok {
		return f, true
	}
	switch v {
	case "larger":
		return parent * 1.2, true
	case "smaller":
		return parent / 1.2, true
	}
	for _, u := range []struct {
		s string
		f float64
	}{{"px", 1}, {"pt", 4.0 / 3}, {"rem", 16}, {"em", parent}, {"%", parent / 100}} {
		if strings.HasSuffix(v, u.s) {
			f, err := strconv.ParseFloat(strings.TrimSuffix(v, u.s), 64)
			if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
				return 0, false
			}
			return f * u.f, true
		}
	}
	return 0, false
}

func (a *adapter) element(n *html.Node, c ctx) ctx {
	ds := a.sheet.cascade(n, attrVal(n, "style"))
	has := map[string]bool{}
	for _, d := range ds {
		has[d.prop] = true
	}
	// legacy presentational attributes, lowest priority
	legacy := func(prop, attr string) {
		if v := attrVal(n, attr); v != "" && !has[prop] {
			ds = append(ds, decl{prop: prop, val: v})
		}
	}
	if !has["background"] {
		legacy("background-color", "bgcolor")
		if v := attrVal(n, "background"); v != "" && !has["background-image"] {
			ds = append(ds, decl{prop: "background-image", val: "url(" + v + ")"})
		}
	}
	switch n.Data {
	case "font":
		legacy("color", "color")
		if px, ok := map[string]float64{"1": 10, "2": 13, "3": 16, "4": 18, "5": 24, "6": 32, "7": 48}[strings.TrimSpace(attrVal(n, "size"))]; ok && !has["font-size"] {
			ds = append(ds, decl{prop: "font-size", val: strconv.FormatFloat(px, 'f', -1, 64) + "px"})
		}
	case "body":
		legacy("color", "text")
	}
	legacy("border-color", "bordercolor")

	// ---- classify declarations
	var keep, borders []decl
	var bgc color.NRGBA
	bgk, hasImg := kInvalid, false
	var colDecl *decl
	px, bold := c.px, c.bold
	switch n.Data {
	case "h1":
		px, bold = px*2, true
	case "h2":
		px, bold = px*1.5, true
	case "h3":
		px, bold = px*1.17, true
	case "h5":
		px, bold = px*0.83, true
	case "h6":
		px, bold = px*0.67, true
	case "h4", "b", "strong", "th":
		bold = true
	case "small", "sub", "sup":
		px *= 0.83
	case "big":
		px *= 1.2
	}
	for i := range ds {
		d := ds[i]
		lv := strings.ToLower(d.val)
		if strings.Contains(lv, "expression(") || strings.Contains(lv, "javascript:") {
			continue
		}
		if strings.Contains(lv, "var(") && (d.prop == "color" || strings.HasPrefix(d.prop, "background") || borderProps[d.prop]) {
			a.note("css variables")
			continue
		}
		switch {
		case d.prop == "color":
			colDecl = &ds[i]
		case d.prop == "background-color":
			if col, k := parseColor(d.val, true); k != kInvalid {
				bgc, bgk = col, k
			} else {
				a.note("color " + d.val)
			}
		case d.prop == "background":
			bgk, hasImg = kInvalid, false
			for _, t := range splitTop(d.val) {
				if isImageToken(t) {
					hasImg = true
				} else if col, k := parseColor(t, false); k != kInvalid {
					bgc, bgk = col, k
				}
			}
		case d.prop == "background-image":
			hasImg = isImageToken(d.val)
		case borderProps[d.prop]:
			borders = append(borders, d)
		default:
			switch d.prop {
			case "font-size":
				if f, ok := parsePx(d.val, c.px); ok {
					px = f
				}
			case "font-weight":
				if w, err := strconv.Atoi(strings.TrimSpace(lv)); err == nil {
					bold = w >= 600
				} else if lv == "bold" || lv == "bolder" {
					bold = true
				} else if lv == "normal" || lv == "lighter" {
					bold = false
				}
			}
			keep = append(keep, decl{prop: d.prop, val: d.val})
		}
	}

	// ---- background
	nc := c
	nc.px, nc.bold = px, bold
	var writes []decl
	switch {
	case bgk == kOK && bgc.A > 0:
		comp := over(bgc, c.bgOrig)
		nc.bgOrig = comp
		if a.th.KeepColors {
			nc.bgAd = comp
			writes = append(writes, decl{prop: "background-color", val: hexOf(comp)})
		} else if !a.th.DropBackgrounds {
			nc.bgAd = a.mapBg(comp)
			writes = append(writes, decl{prop: "background-color", val: hexOf(nc.bgAd)})
		}
		a.st.BgRemapped++
		if hasImg {
			a.note("background-image replaced by solid colour")
		}
	case hasImg:
		if !a.th.DropBackgrounds && !a.th.KeepColors {
			nc.bgAd = a.surface
			writes = append(writes, decl{prop: "background-color", val: hexOf(nc.bgAd)})
		}
		a.st.BgRemapped++
		a.note("background-image replaced by solid colour")
	}

	// ---- text colour
	large := px >= 24 || (bold && px >= 18.66)
	min := minNormal
	if large {
		min = minLarge
	}
	var declared *color.NRGBA
	src := c.fgSrc
	if colDecl != nil {
		if col, k := parseColor(colDecl.val, false); k == kOK {
			o := over(col, nc.bgAd)
			declared, src = &o, &o
		} else if k == kInvalid {
			a.note("color " + colDecl.val)
		}
	}
	if declared == nil && n.Data == "a" && attrVal(n, "href") != "" {
		ac := a.ac
		src = &ac
	}
	var adFg color.NRGBA
	switch {
	case a.th.KeepColors:
		adFg = c.fgAd
		if declared != nil {
			adFg = *declared
		}
	case src == nil:
		adFg = ensure(a.fg, nc.bgAd, min+target)
	default:
		adFg = a.adaptText(*src, nc.bgAd, min)
	}
	nc.fgSrc, nc.fgAd = src, adFg
	if declared != nil || adFg != c.fgAd {
		writes = append(writes, decl{prop: "color", val: hexOf(adFg)})
		if declared == nil || *declared != adFg {
			a.st.TextRemapped++
		}
		if n.Data == "a" {
			a.st.LinksRemapped++
		}
	}

	// ---- borders
	hadBorderColor := false
	for _, d := range borders {
		if a.th.KeepColors {
			keep = append(keep, d)
			continue
		}
		toks := splitTop(d.val)
		for i, t := range toks {
			if col, k := parseColor(t, false); k == kOK {
				col = over(col, nc.bgAd)
				toks[i] = hexOf(a.softBorder(&col, nc.bgAd))
				hadBorderColor = true
			}
		}
		keep = append(keep, decl{prop: d.prop, val: strings.Join(toks, " ")})
	}
	if !hadBorderColor && !a.th.KeepColors {
		switch {
		case n.Data == "hr", n.Data == "table" && attrVal(n, "border") != "":
			keep = append(keep, decl{prop: "border-color", val: hexOf(a.softBorder(nil, nc.bgAd))})
		case n.Data == "blockquote":
			keep = append(keep, decl{prop: "border-left-color", val: hexOf(a.softBorder(nil, nc.bgAd))})
		}
	}

	// ---- images
	if n.Data == "img" && !a.th.KeepColors && a.wantChip(n, c, keep) {
		keep = append(keep, decl{prop: "background-color", val: "#ffffff"},
			decl{prop: "padding", val: "4px"}, decl{prop: "border-radius", val: "6px"})
		a.st.ImagesChipped++
	}

	// ---- write back
	var sb strings.Builder
	for _, d := range append(keep, writes...) {
		if sb.Len() > 0 {
			sb.WriteByte(';')
		}
		sb.WriteString(d.prop + ":" + d.val)
	}
	attrs := n.Attr[:0:0]
	for _, at := range n.Attr {
		switch at.Key {
		case "style", "bgcolor", "background", "text", "link", "vlink", "alink", "bordercolor":
			continue
		case "color":
			if n.Data == "font" {
				continue
			}
		}
		attrs = append(attrs, at)
	}
	if sb.Len() > 0 {
		attrs = append(attrs, html.Attribute{Key: "style", Val: sb.String()})
	}
	n.Attr = attrs
	return nc
}

// wantChip decides whether an <img> gets a light chip behind it: dark logos
// on transparent backgrounds vanish on a darkened surface. With a Known hint
// the decision is HasAlpha && Dark. Otherwise (heuristic, conservative): a
// small (declared max side 3..320px) png/svg/gif authored on a light
// background that is now dark. Photos (jpeg/webp) and big images are never
// chipped, and images authored on dark backgrounds (likely light logos) too.
func (a *adapter) wantChip(n *html.Node, c ctx, decls []decl) bool {
	for _, d := range decls {
		if d.prop == "background-color" || d.prop == "background" {
			return false
		}
	}
	src := attrVal(n, "src")
	if a.th.ImageHint != nil {
		if h := a.th.ImageHint(src); h.Known {
			return h.HasAlpha && h.Dark && toLCH(c.bgAd).L < 0.7
		}
	}
	if toLCH(c.bgAd).L >= 0.7 || toLCH(c.bgOrig).L < lightBg {
		return false
	}
	p := strings.ToLower(src)
	if strings.HasPrefix(p, "data:image/") {
		if !(strings.HasPrefix(p, "data:image/png") || strings.HasPrefix(p, "data:image/svg") || strings.HasPrefix(p, "data:image/gif")) {
			return false
		}
	} else {
		if i := strings.IndexAny(p, "?#"); i >= 0 {
			p = p[:i]
		}
		if !(strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".svg") || strings.HasSuffix(p, ".gif")) {
			return false
		}
	}
	side := 0.0
	for _, k := range []string{"width", "height"} {
		v := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(attrVal(n, k))), "px")
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > side {
			side = f
		}
	}
	for _, d := range decls {
		if d.prop == "width" || d.prop == "height" {
			if f, ok := parsePx(d.val, 16); ok && !strings.HasSuffix(d.val, "%") && f > side {
				side = f
			}
		}
	}
	return side >= 3 && side <= 320
}

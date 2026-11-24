package darkmode

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// colorKind classifies the result of parsing a CSS/legacy colour value.
type colorKind int

const (
	kInvalid     colorKind = iota
	kOK                    // a concrete colour (alpha may be < 255)
	kTransparent           // the keyword "transparent"
	kInherit               // inherit / currentColor / unset
)

func nrgba(c color.Color) color.NRGBA { return color.NRGBAModel.Convert(c).(color.NRGBA) }

func opaque(c color.Color) color.NRGBA {
	n := nrgba(c)
	n.A = 255
	return n
}

func hexOf(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func lin(v uint8) float64 {
	f := float64(v) / 255
	if f <= 0.04045 {
		return f / 12.92
	}
	return math.Pow((f+0.055)/1.055, 2.4)
}

// relLum is the WCAG 2.x relative luminance of the (alpha-ignored) colour.
func relLum(c color.Color) float64 {
	n := nrgba(c)
	return 0.2126*lin(n.R) + 0.7152*lin(n.G) + 0.0722*lin(n.B)
}

// ContrastRatio is the WCAG 2.x contrast ratio (1..21) between two colours.
// Alpha is ignored: pass opaque colours (composite translucent ones first).
func ContrastRatio(a, b color.Color) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// over composites fg (possibly translucent) onto the opaque colour bg.
func over(fg, bg color.NRGBA) color.NRGBA {
	if fg.A == 255 {
		return fg
	}
	a := float64(fg.A) / 255
	mix := func(f, b uint8) uint8 { return uint8(math.Round(float64(f)*a + float64(b)*(1-a))) }
	return color.NRGBA{mix(fg.R, bg.R), mix(fg.G, bg.G), mix(fg.B, bg.B), 255}
}

// ---- OKLab / OKLCH -------------------------------------------------------

type lch struct{ L, C, H float64 }

func toLCH(c color.Color) lch {
	n := nrgba(c)
	r, g, b := lin(n.R), lin(n.G), lin(n.B)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	a := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	bb := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	return lch{L, math.Hypot(a, bb), math.Atan2(bb, a)}
}

func oklabLin(L, C, H float64) (r, g, b float64) {
	a, bb := C*math.Cos(H), C*math.Sin(H)
	l := L + 0.3963377774*a + 0.2158037573*bb
	m := L - 0.1055613458*a - 0.0638541728*bb
	s := L - 0.0894841775*a - 1.2914855480*bb
	l, m, s = l*l*l, m*m*m, s*s*s
	r = 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	g = -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	b = -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	return
}

func inGamut(r, g, b float64) bool {
	const e = 1e-4
	return r >= -e && r <= 1+e && g >= -e && g <= 1+e && b >= -e && b <= 1+e
}

func gamma(v float64) uint8 {
	v = math.Max(0, math.Min(1, v))
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(math.Round(v * 255))
}

// fromLCH converts to sRGB, reducing chroma until the colour is in gamut.
func fromLCH(x lch) color.NRGBA {
	if x.L >= 1 {
		return color.NRGBA{255, 255, 255, 255}
	}
	if x.L <= 0 {
		return color.NRGBA{0, 0, 0, 255}
	}
	c := x.C
	r, g, b := oklabLin(x.L, c, x.H)
	if !inGamut(r, g, b) {
		lo, hi := 0.0, c
		for i := 0; i < 24; i++ {
			mid := (lo + hi) / 2
			if r, g, b = oklabLin(x.L, mid, x.H); inGamut(r, g, b) {
				lo = mid
			} else {
				hi = mid
			}
		}
		r, g, b = oklabLin(x.L, lo, x.H)
	}
	return color.NRGBA{gamma(r), gamma(g), gamma(b), 255}
}

// EnsureContrast returns fg unchanged if it already has at least min contrast
// against bg; otherwise the closest colour (same OKLCH hue and chroma where
// the gamut allows) that does. If min is unreachable it returns the better of
// black and white. bg is treated as opaque.
func EnsureContrast(fg, bg color.Color, min float64) color.Color {
	return ensure(opaque(fg), opaque(bg), min)
}

func ensure(fg, bg color.NRGBA, min float64) color.NRGBA {
	if ContrastRatio(fg, bg) >= min {
		return fg
	}
	white, black := color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 255}
	ext, extL := white, 1.0
	if ContrastRatio(black, bg) > ContrastRatio(white, bg) {
		ext, extL = black, 0.0
	}
	if ContrastRatio(ext, bg) < min {
		return ext
	}
	f := toLCH(fg)
	startL := toLCH(bg).L
	pass := func(t float64) (color.NRGBA, bool) {
		c := fromLCH(lch{startL + (extL-startL)*t, f.C, f.H})
		return c, ContrastRatio(c, bg) >= min
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 24; i++ {
		mid := (lo + hi) / 2
		if _, ok := pass(mid); ok {
			hi = mid
		} else {
			lo = mid
		}
	}
	if c, ok := pass(hi); ok {
		return c
	}
	return ext
}

// ---- parsing -------------------------------------------------------------

// parseColor parses a CSS or legacy HTML-attribute colour. legacy allows
// bare hex such as "ffffff" (bgcolor="ffffff").
func parseColor(s string, legacy bool) (color.NRGBA, colorKind) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSpace(strings.TrimSuffix(s, "!important"))
	switch s {
	case "":
		return color.NRGBA{}, kInvalid
	case "transparent":
		return color.NRGBA{}, kTransparent
	case "inherit", "currentcolor", "unset", "revert":
		return color.NRGBA{}, kInherit
	}
	if strings.HasPrefix(s, "#") || (legacy && isHex(s) && (len(s) == 6 || len(s) == 3)) {
		return parseHex(strings.TrimPrefix(s, "#"))
	}
	if i := strings.IndexByte(s, '('); i > 0 && strings.HasSuffix(s, ")") {
		return parseFunc(s[:i], s[i+1:len(s)-1])
	}
	if h, ok := namedColors[s]; ok {
		return parseHex(h)
	}
	return color.NRGBA{}, kInvalid
}

func isHex(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return s != ""
}

func parseHex(h string) (color.NRGBA, colorKind) {
	if !isHex(h) {
		return color.NRGBA{}, kInvalid
	}
	switch len(h) {
	case 3, 4:
		var e strings.Builder
		for _, r := range h {
			e.WriteRune(r)
			e.WriteRune(r)
		}
		h = e.String()
	case 6, 8:
	default:
		return color.NRGBA{}, kInvalid
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, kInvalid
	}
	if len(h) == 6 {
		return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, kOK
	}
	return color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}, kOK
}

func parseFunc(name, args string) (color.NRGBA, colorKind) {
	args = strings.NewReplacer(",", " ", "/", " ").Replace(args)
	f := strings.Fields(args)
	if len(f) < 3 || len(f) > 4 {
		return color.NRGBA{}, kInvalid
	}
	num := func(s string, scale float64) (float64, bool) { // "50%" -> 0.5*scale
		pct := strings.HasSuffix(s, "%")
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		if pct {
			return v / 100 * scale, true
		}
		return v, true
	}
	alpha := 1.0
	if len(f) == 4 {
		a, ok := num(f[3], 1)
		if !ok {
			return color.NRGBA{}, kInvalid
		}
		alpha = math.Max(0, math.Min(1, a))
	}
	clamp := func(v float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(255, v)))) }
	var r, g, b float64
	switch name {
	case "rgb", "rgba":
		var ok1, ok2, ok3 bool
		r, ok1 = num(f[0], 255)
		g, ok2 = num(f[1], 255)
		b, ok3 = num(f[2], 255)
		if !ok1 || !ok2 || !ok3 {
			return color.NRGBA{}, kInvalid
		}
	case "hsl", "hsla":
		h, ok1 := num(strings.TrimSuffix(f[0], "deg"), 1)
		s, ok2 := num(f[1], 1)
		l, ok3 := num(f[2], 1)
		if !ok1 || !ok2 || !ok3 || !strings.HasSuffix(f[1], "%") || !strings.HasSuffix(f[2], "%") {
			return color.NRGBA{}, kInvalid
		}
		s, l = math.Max(0, math.Min(1, s)), math.Max(0, math.Min(1, l))
		h = math.Mod(math.Mod(h, 360)+360, 360) / 360
		q := l + s - l*s
		if l < 0.5 {
			q = l * (1 + s)
		}
		p := 2*l - q
		hue := func(t float64) float64 {
			t = math.Mod(t+1, 1)
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
		r, g, b = hue(h+1.0/3)*255, hue(h)*255, hue(h-1.0/3)*255
	default:
		return color.NRGBA{}, kInvalid
	}
	return color.NRGBA{clamp(r), clamp(g), clamp(b), uint8(math.Round(alpha * 255))}, kOK
}

var namedColors = func() map[string]string {
	const t = "aliceblue f0f8ff antiquewhite faebd7 aqua 00ffff aquamarine 7fffd4 azure f0ffff beige f5f5dc bisque ffe4c4 black 000000 blanchedalmond ffebcd blue 0000ff blueviolet 8a2be2 brown a52a2a burlywood deb887 cadetblue 5f9ea0 chartreuse 7fff00 chocolate d2691e coral ff7f50 cornflowerblue 6495ed cornsilk fff8dc crimson dc143c cyan 00ffff darkblue 00008b darkcyan 008b8b darkgoldenrod b8860b darkgray a9a9a9 darkgreen 006400 darkgrey a9a9a9 darkkhaki bdb76b darkmagenta 8b008b darkolivegreen 556b2f darkorange ff8c00 darkorchid 9932cc darkred 8b0000 darksalmon e9967a darkseagreen 8fbc8f darkslateblue 483d8b darkslategray 2f4f4f darkslategrey 2f4f4f darkturquoise 00ced1 darkviolet 9400d3 deeppink ff1493 deepskyblue 00bfff dimgray 696969 dimgrey 696969 dodgerblue 1e90ff firebrick b22222 floralwhite fffaf0 forestgreen 228b22 fuchsia ff00ff gainsboro dcdcdc ghostwhite f8f8ff gold ffd700 goldenrod daa520 gray 808080 green 008000 greenyellow adff2f grey 808080 honeydew f0fff0 hotpink ff69b4 indianred cd5c5c indigo 4b0082 ivory fffff0 khaki f0e68c lavender e6e6fa lavenderblush fff0f5 lawngreen 7cfc00 lemonchiffon fffacd lightblue add8e6 lightcoral f08080 lightcyan e0ffff lightgoldenrodyellow fafad2 lightgray d3d3d3 lightgreen 90ee90 lightgrey d3d3d3 lightpink ffb6c1 lightsalmon ffa07a lightseagreen 20b2aa lightskyblue 87cefa lightslategray 778899 lightslategrey 778899 lightsteelblue b0c4de lightyellow ffffe0 lime 00ff00 limegreen 32cd32 linen faf0e6 magenta ff00ff maroon 800000 mediumaquamarine 66cdaa mediumblue 0000cd mediumorchid ba55d3 mediumpurple 9370db mediumseagreen 3cb371 mediumslateblue 7b68ee mediumspringgreen 00fa9a mediumturquoise 48d1cc mediumvioletred c71585 midnightblue 191970 mintcream f5fffa mistyrose ffe4e1 moccasin ffe4b5 navajowhite ffdead navy 000080 oldlace fdf5e6 olive 808000 olivedrab 6b8e23 orange ffa500 orangered ff4500 orchid da70d6 palegoldenrod eee8aa palegreen 98fb98 paleturquoise afeeee palevioletred db7093 papayawhip ffefd5 peachpuff ffdab9 peru cd853f pink ffc0cb plum dda0dd powderblue b0e0e6 purple 800080 rebeccapurple 663399 red ff0000 rosybrown bc8f8f royalblue 4169e1 saddlebrown 8b4513 salmon fa8072 sandybrown f4a460 seagreen 2e8b57 seashell fff5ee sienna a0522d silver c0c0c0 skyblue 87ceeb slateblue 6a5acd slategray 708090 slategrey 708090 snow fffafa springgreen 00ff7f steelblue 4682b4 tan d2b48c teal 008080 thistle d8bfd8 tomato ff6347 turquoise 40e0d0 violet ee82ee wheat f5deb3 white ffffff whitesmoke f5f5f5 yellow ffff00 yellowgreen 9acd32"
	f := strings.Fields(t)
	m := make(map[string]string, len(f)/2)
	for i := 0; i+1 < len(f); i += 2 {
		m[f[i]] = f[i+1]
	}
	return m
}()

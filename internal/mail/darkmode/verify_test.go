package darkmode

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The verifier below re-parses the OUTPUT and recomputes contrast without
// using the transform's cascade, parser or colour code paths other than the
// WCAG ratio. Adapted output must only contain #rrggbb colours.

var testTheme = Theme{}

func hexColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
}

type vctx struct {
	fg, bg color.NRGBA
	px     float64
	bold   bool
}

// verify returns violations (empty = every text run readable).
func verify(out string) []string {
	doc, err := html.Parse(strings.NewReader(out))
	if err != nil {
		return []string{"unparseable output: " + err.Error()}
	}
	var bad []string
	root := vctx{fg: color.NRGBA{0xf2, 0xf2, 0xf7, 255}, bg: color.NRGBA{0x1e, 0x1e, 0x1e, 255}, px: 16}
	var rec func(n *html.Node, c vctx)
	rec = func(n *html.Node, c vctx) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "head", "script", "style", "title", "noscript", "template":
				return
			case "h1":
				c.px, c.bold = 32, true
			case "h2":
				c.px, c.bold = 24, true
			}
			for _, a := range n.Attr {
				for _, banned := range []string{"bgcolor", "background"} {
					if a.Key == banned {
						bad = append(bad, "legacy attr "+a.Key)
					}
				}
				if a.Key != "style" {
					continue
				}
				for _, d := range strings.Split(a.Val, ";") {
					k, v, _ := strings.Cut(d, ":")
					k = strings.TrimSpace(strings.ToLower(k))
					if strings.HasPrefix(k, "background") && (strings.Contains(v, "gradient(") || strings.Contains(v, "url(")) {
						bad = append(bad, "background image left: "+d)
					}
					switch k {
					case "color", "background-color":
						col, ok := hexColor(v)
						if !ok {
							bad = append(bad, "non-hex "+k+": "+v)
						} else if k == "color" {
							c.fg = col
						} else {
							c.bg = col
						}
					case "font-size":
						v = strings.TrimSpace(v)
						if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil && strings.HasSuffix(v, "px") {
							c.px = f
						}
					case "font-weight":
						c.bold = strings.TrimSpace(v) == "bold"
					}
				}
			}
		}
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
			need := 4.5
			if c.px >= 24 || (c.bold && c.px >= 18.66) {
				need = 3
			}
			if r := ContrastRatio(c.fg, c.bg); r < need {
				bad = append(bad, fmt.Sprintf("%q fg %v on bg %v ratio %.2f < %.1f", strings.TrimSpace(n.Data), hexOf(c.fg), hexOf(c.bg), r, need))
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			rec(ch, c)
		}
	}
	rec(doc, root)
	return bad
}

func mustAdapt(t *testing.T, src string, th Theme) Result {
	t.Helper()
	r, err := Adapt(src, th)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	if bad := verify(r.HTML); len(bad) > 0 {
		t.Fatalf("unreadable output for %q:\n%s\n%s", src, strings.Join(bad, "\n"), r.HTML)
	}
	return r
}

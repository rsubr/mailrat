package darkmode

import (
	"fmt"
	"image/color"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestAdaptCases(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		th    Theme
		has   []string // substrings required in output
		not   []string // substrings forbidden
		check func(t *testing.T, r Result)
	}{
		{name: "white newsletter black text",
			in:  `<body bgcolor="#ffffff"><table bgcolor="#fff"><tr><td style="color:#000000">Hello</td></tr></table></body>`,
			not: []string{"bgcolor", "#ffffff", "#000000"},
			check: func(t *testing.T, r Result) {
				if r.Stats.BgRemapped < 2 || r.Stats.TextRemapped < 1 {
					t.Errorf("stats %+v", r.Stats)
				}
			}},
		{name: "white bg becomes the surface",
			in:  `<div style="background:#fff;color:#111">x</div>`,
			has: []string{"background-color:#1e1e1e"}},
		{name: "dark text on transparent bg",
			in:  `<p style="color:#222;background-color:transparent">unreadable before</p><p style="color:rgba(0,0,0,.6)">alpha</p>`,
			not: []string{"#222"}},
		{name: "light grey on white",
			in: `<div style="background:white"><span style="color:#cccccc">faint</span></div>`},
		{name: "brand red link on white",
			in:  `<div bgcolor="#ffffff" style="background:#fff"><a href="https://x.test" style="color:#c0392b">Shop</a></div>`,
			not: []string{"#c0392b"},
			check: func(t *testing.T, r Result) {
				if r.Stats.LinksRemapped != 1 {
					t.Errorf("links %d", r.Stats.LinksRemapped)
				}
			}},
		{name: "brand red link on dark",
			in:  `<div style="background:#101010"><a href="https://x.test" style="color:#8b0000">Shop</a></div>`,
			has: []string{"background-color:#101010"}},
		{name: "default link gets accent",
			in: `<p>see <a href="https://x.test">this</a></p>`,
			check: func(t *testing.T, r Result) {
				if r.Stats.LinksRemapped != 1 {
					t.Errorf("links %d: %s", r.Stats.LinksRemapped, r.HTML)
				}
			}},
		{name: "navy banner white text kept",
			in:  `<table><tr><td bgcolor="#0b3d91" style="color:#ffffff">Banner</td></tr></table>`,
			has: []string{"background-color:#0b3d91"},
			check: func(t *testing.T, r Result) {
				if r.Stats.BgRemapped != 1 {
					t.Errorf("stats %+v", r.Stats)
				}
			}},
		{name: "nested tables mixed bgcolor",
			in: `<table bgcolor="#eeeeee"><tr><td bgcolor="#ffffff"><table bgcolor="#003366"><tr><td><font color="#000">dark on navy</font></td>
				<td bgcolor="#ffcc00"><font color="#000000">black on yellow</font></td></tr></table>plain in white</td></tr></table>`},
		{name: "font color 000",
			in:  `<font color=#000>old school</font><font color="000000">bare hex</font>`,
			not: []string{`color="`}},
		{name: "inline important",
			in: `<style>.a{color:#000 !important}</style><p class="a" style="color:#fff">x</p><div style="background:#fff !important;background:#000">y</div>`},
		{name: "rgba hsl",
			in: `<div style="background:hsl(210,40%,96%)"><p style="color:hsla(0,0%,0%,.8)">a</p><p style="color:rgb(10 10 10 / 50%)">b</p><p style="color:rgb(100%,100%,100%)">c</p></div>`},
		{name: "style class colours",
			in: `<html><head><style>
				/* c */ body{background:#ffffff;color:#333}
				.t{color:#222222}
				td.hdr{background-color:#f4f4f4;color:#000}
				#x{color:#111}
				.a .b{color:#000}
				@media (prefers-color-scheme:dark){.t{color:#fff}}
			</style></head><body><table><tr><td class="hdr">H</td></tr></table><p class="t">T</p><p id="x">X</p><div class="a"><span class="b">nested</span></div></body></html>`,
			not: []string{"<style"},
			check: func(t *testing.T, r Result) {
				joined := strings.Join(r.Stats.Unsupported, "|")
				if !strings.Contains(joined, "selector .a .b") || !strings.Contains(joined, "@media") {
					t.Errorf("unsupported not reported: %v", r.Stats.Unsupported)
				}
			}},
		{name: "gradient background image",
			in:  `<div style="background-image:linear-gradient(#fff,#ccc);color:#000">grad</div><table background="https://x.test/bg.jpg"><tr><td style="color:#333">img</td></tr></table>`,
			not: []string{"gradient", "bg.jpg"},
			check: func(t *testing.T, r Result) {
				if len(r.Stats.Unsupported) == 0 {
					t.Error("expected unsupported note")
				}
			}},
		{name: "blockquote",
			in:  `<blockquote style="border-left:3px solid #cccccc;color:#555">quoted</blockquote><blockquote>bare</blockquote>`,
			not: []string{"#cccccc"},
			has: []string{"border-left-color"}},
		{name: "plain text",
			in: "On Mon, X wrote:\n> quoted <b> & stuff\nhello"},
		{name: "hr softened",
			in:  `<hr><hr style="border-top:1px solid #000">`,
			not: []string{"#000"}},
		{name: "logo png chip",
			in:  `<div style="background:#fff"><img src="https://x.test/logo.png" width="120" height="40" alt="logo"></div>`,
			has: []string{"padding:4px", "border-radius:6px"},
			check: func(t *testing.T, r Result) {
				if r.Stats.ImagesChipped != 1 {
					t.Errorf("chipped %d", r.Stats.ImagesChipped)
				}
			}},
		{name: "photo and big png not chipped",
			in:  `<img src="https://x.test/p.jpg" width="100"><img src="https://x.test/hero.png" width="600"><img src="https://x.test/u.png">`,
			not: []string{"padding"}},
		{name: "light logo on dark banner not chipped",
			in:  `<table><tr><td bgcolor="#000033"><img src="https://x.test/logo.png" width="100"></td></tr></table>`,
			not: []string{"padding"}},
		{name: "image hint overrides",
			in: `<img src="https://x.test/photo.jpg" width="600"><img src="https://x.test/white.png" width="50">`,
			th: Theme{ImageHint: func(s string) ImageInfo {
				if strings.HasSuffix(s, "photo.jpg") {
					return ImageInfo{Known: true, HasAlpha: true, Dark: true}
				}
				return ImageInfo{Known: true, HasAlpha: true, Dark: false}
			}},
			check: func(t *testing.T, r Result) {
				if r.Stats.ImagesChipped != 1 {
					t.Errorf("chipped %d: %s", r.Stats.ImagesChipped, r.HTML)
				}
			}},
		{name: "large text uses 3:1 but still passes",
			in: `<div style="background:#fff"><h1 style="color:#888888">Big</h1><p style="color:#888888">small</p></div>`},
		{name: "drop backgrounds mode",
			in:  `<table bgcolor="#ffffff"><tr><td bgcolor="#0b3d91" style="color:#ffffff">x</td><td bgcolor="#f0f0f0" style="color:#000">y</td></tr></table>`,
			th:  Theme{DropBackgrounds: true},
			not: []string{"background-color", "bgcolor"}},
		{name: "custom theme",
			in: `<p style="color:#000">t</p>`,
			th: Theme{Background: color.NRGBA{0, 0, 0, 255}, Foreground: color.NRGBA{200, 200, 200, 255}}},
		{name: "unsafe values dropped",
			in:  `<div style="width:expression(alert(1));color:#000;background:url(javascript:x)">x</div>`,
			not: []string{"expression", "javascript"}},
		{name: "invalid colours and vars",
			in: `<p style="color:var(--x);background:foo">a</p><p style="color:notacolor">b</p>`},
		{name: "malformed html",
			in: `<table><td style="color:#000"><b><i>unclosed<div style="background:#fff;color:">x</p></b>&nbsp;<`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := mustAdapt(t, tc.in, tc.th)
			for _, s := range tc.has {
				if !strings.Contains(r.HTML, s) {
					t.Errorf("missing %q in %s", s, r.HTML)
				}
			}
			for _, s := range tc.not {
				if strings.Contains(r.HTML, s) {
					t.Errorf("unexpected %q in %s", s, r.HTML)
				}
			}
			if tc.check != nil {
				tc.check(t, r)
			}
			// idempotent enough: adapting the output again stays readable
			mustAdapt(t, r.HTML, tc.th)
		})
	}
}

func TestDeterministic(t *testing.T) {
	in := `<style>.a{color:#123}.b{background:#eee}</style><div class="a b"><p style="color:#000">x</p></div>`
	a := mustAdapt(t, in, testTheme)
	for i := 0; i < 20; i++ {
		if b := mustAdapt(t, in, testTheme); b.HTML != a.HTML {
			t.Fatalf("nondeterministic:\n%s\n%s", a.HTML, b.HTML)
		}
	}
}

func TestLimits(t *testing.T) {
	if _, err := Adapt(strings.Repeat("a", MaxInput+1), testTheme); err != ErrTooLarge {
		t.Errorf("want ErrTooLarge, got %v", err)
	}
	deep := strings.Repeat("<div>", maxDepth+100) + "x"
	if r, err := Adapt(deep, testTheme); err == nil {
		// html.Parse may cap nesting itself; then the result must be readable.
		if bad := verify(r.HTML); len(bad) > 0 {
			t.Errorf("deep output unreadable: %v", bad[:1])
		}
	}
}

func TestEnsureContrastProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	rc := func() color.NRGBA {
		return color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255}
	}
	for i := 0; i < 5000; i++ {
		fg, bg := rc(), rc()
		for _, min := range []float64{3, 4.5} {
			got := EnsureContrast(fg, bg, min)
			if r := ContrastRatio(got, bg); r < min {
				t.Fatalf("fg %v bg %v min %v -> %v ratio %.3f", fg, bg, min, got, r)
			}
		}
	}
	// unchanged when already fine
	if got := EnsureContrast(color.White, color.Black, 4.5); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("changed passing colour: %v", got)
	}
}

func TestContrastRatioKnown(t *testing.T) {
	if r := ContrastRatio(color.White, color.Black); r < 20.99 || r > 21.01 {
		t.Errorf("white/black %v", r)
	}
	if r := ContrastRatio(color.NRGBA{0x77, 0x77, 0x77, 255}, color.White); r < 4.4 || r > 4.5 {
		t.Errorf("#777/white %v", r) // known ~4.48
	}
	if r := ContrastRatio(color.Black, color.Black); r != 1 {
		t.Errorf("same %v", r)
	}
}

func TestParseColor(t *testing.T) {
	ok := map[string]color.NRGBA{
		"#fff": {255, 255, 255, 255}, "#000000": {0, 0, 0, 255}, "#11223344": {0x11, 0x22, 0x33, 0x44},
		"#f008": {255, 0, 0, 0x88}, "RED": {255, 0, 0, 255}, "rgb(1,2,3)": {1, 2, 3, 255},
		"rgb(100% 0% 0%)": {255, 0, 0, 255}, "rgba(0,0,0,.5)": {0, 0, 0, 128},
		"hsl(120,100%,50%)": {0, 255, 0, 255}, "hsl(0deg 100% 50% / 50%)": {255, 0, 0, 128},
		"rebeccapurple": {0x66, 0x33, 0x99, 255}, "  #FfF !important": {255, 255, 255, 255},
	}
	for in, want := range ok {
		if got, k := parseColor(in, false); k != kOK || got != want {
			t.Errorf("%q -> %v %v, want %v", in, got, k, want)
		}
	}
	if got, k := parseColor("ffcc00", true); k != kOK || got != (color.NRGBA{255, 204, 0, 255}) {
		t.Errorf("legacy bare hex: %v %v", got, k)
	}
	if _, k := parseColor("ffcc00", false); k != kInvalid {
		t.Error("bare hex must be invalid in CSS")
	}
	for in, want := range map[string]colorKind{"transparent": kTransparent, "inherit": kInherit, "currentColor": kInherit,
		"": kInvalid, "#12": kInvalid, "#gggggg": kInvalid, "rgb(1,2)": kInvalid, "rgb(a,b,c)": kInvalid, "hsl(1,2,3)": kInvalid, "foo(1,2,3)": kInvalid, "var(--x)": kInvalid} {
		if _, k := parseColor(in, false); k != want {
			t.Errorf("%q kind %v want %v", in, k, want)
		}
	}
}

func TestPropertyRandomPairs(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	form := []func(c color.NRGBA) string{
		func(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) },
		func(c color.NRGBA) string { return fmt.Sprintf("rgb(%d,%d,%d)", c.R, c.G, c.B) },
		func(c color.NRGBA) string { return fmt.Sprintf("rgba(%d,%d,%d,0.%d)", c.R, c.G, c.B, 1+rng.Intn(9)) },
	}
	for i := 0; i < 400; i++ {
		var b strings.Builder
		for j := 0; j < 6; j++ {
			bg := color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255}
			fg := color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255}
			fmt.Fprintf(&b, `<div style="background:%s"><p style="color:%s">t%d <a href="https://x.test" style="color:%s">l</a></p></div>`,
				form[rng.Intn(3)](bg), form[rng.Intn(3)](fg), j, form[rng.Intn(3)](fg))
		}
		for _, th := range []Theme{testTheme, {DropBackgrounds: true}} {
			mustAdapt(t, b.String(), th)
		}
	}
}

func TestPerformance1MB(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<style>.c{color:#333}.r{background:#f6f6f6}</style><table bgcolor="#fff">`)
	for b.Len() < 1<<20 {
		b.WriteString(`<tr class="r"><td bgcolor="#ffffff" style="color:#222;font-size:14px"><font color="#444">cell</font> <a href="https://x.test" class="c">link</a><img src="https://x.test/a.png" width="40"></td></tr>`)
	}
	b.WriteString("</table>")
	start := time.Now()
	r := mustAdapt(t, b.String(), testTheme)
	d := time.Since(start)
	limit := time.Second
	if raceEnabled {
		limit = 10 * time.Second
	}
	if d > limit {
		t.Errorf("took %v for %d bytes", d, len(b.String()))
	}
	t.Logf("1MB in %v (out %d bytes)", d, len(r.HTML))
}

func FuzzAdapt(f *testing.F) {
	for _, s := range []string{
		"", "plain text", `<p style="color:#000">x</p>`, `<td bgcolor=#fff><font color=#000>x</font></td>`,
		`<style>.a{color:red}</style><p class=a>x</p>`, `<div style="background:linear-gradient(red,blue);color:#fff">`,
		`<a href=x style="color:rgba(0,0,0,.5)">`, `<table><tr><td style="background:hsl(0,0%,100%)">`, `<style>{{{}}} @media{`, "<<<>>>&&;;;(((",
		`<p style="color:;background:;;:;color:#">`, `<img src="a.png" width="5">`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := Adapt(s, testTheme)
		if err != nil {
			return
		}
		if bad := verify(r.HTML); len(bad) > 0 {
			t.Fatalf("unreadable: %v\nin: %q\nout: %s", bad[0], s, r.HTML)
		}
	})
}

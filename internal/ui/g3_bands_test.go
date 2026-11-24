package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/richclip"
)

// g3Band is a rendered band with the text it sits behind.
type g3Band struct {
	col   color.NRGBA
	depth int
	text  string
}

func g3Bands(res renderResult) []g3Band {
	var out []g3Band
	for _, b := range res.bands {
		var sb strings.Builder
		for _, s := range res.segments[b.from:b.to] {
			sb.WriteString(s.Textual())
		}
		out = append(out, g3Band{b.col, b.depth, sb.String()})
	}
	return out
}

func g3Hex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// g3Find is the innermost band whose text contains s.
func g3Find(bs []g3Band, s string) *g3Band {
	var f *g3Band
	for i := range bs {
		if strings.Contains(bs[i].text, s) {
			f = &bs[i]
		}
	}
	return f
}

type g3Fix struct {
	name, src string
	// want: text -> authored band colour ("" = must not be painted), light mode.
	want map[string]string
}

var g3Fixtures = []g3Fix{
	{"newsletter header and footer", `<body bgcolor="#ffffff"><table width="100%"><tr><td bgcolor="#e8f0fe" style="color:#1a237e"><b>Header band</b></td></tr><tr><td style="color:#202124">Body copy here</td></tr><tr><td bgcolor="#f1f3f4" style="color:#5f6368">Footer band</td></tr></table></body>`,
		map[string]string{"Header band": "#e8f0fe", "Footer band": "#f1f3f4", "Body copy": ""}},
	{"navy banner white text", `<table><tr><td bgcolor="#0b3d91" style="color:#ffffff;font-size:20px">Big sale</td></tr></table><p>after</p>`,
		map[string]string{"Big sale": "#0b3d91", "after": ""}},
	{"light grey on near-white band", `<div style="background-color:#f5f5f5"><span style="color:#cccccc">Faint footer</span></div>`,
		map[string]string{"Faint footer": "#f5f5f5"}},
	{"nested table in table in td", `<table bgcolor="#dddddd"><tr><td><table bgcolor="#bbccee"><tr><td bgcolor="#ffffcc" style="color:#333">Deep</td></tr></table></td></tr></table>`,
		map[string]string{"Deep": "#ffffcc"}},
	{"body bgcolor", `<body bgcolor="#f4f4f4"><p>Hello grey</p></body>`, map[string]string{"Hello grey": "#f4f4f4"}},
	{"transparent", `<p style="background-color:transparent;color:#222">Dark on nothing</p>`, map[string]string{"Dark on nothing": ""}},
	{"gradient only", `<div style="background:linear-gradient(#ffffff,#000000);color:#fff">Gradient words</div>`, map[string]string{"Gradient words": ""}},
	{"image plus fallback colour", `<div style="background-image:url(https://x.test/a.png);background-color:#336699;color:#fff">Hero text</div>`, map[string]string{"Hero text": "#336699"}},
	{"background shorthand colour", `<div style="background:#aa3355 url(x.png) no-repeat;color:#fff">Shorthand</div>`, map[string]string{"Shorthand": "#aa3355"}},
	{"blockquote", `<p>Reply above</p><blockquote style="background-color:#eeeeee;color:#333">Quoted old text</blockquote>`, map[string]string{"Reply above": ""}},
	{"plain text", "Hello,\nplain words\n> quoted line\n", map[string]string{}},
	{"logo chip", `<table><tr><td bgcolor="#102a43"><img src="https://x.test/logo.png" alt="Logo" width="40" height="40"> <span style="color:#fff">Acme Inc</span></td></tr></table>`,
		map[string]string{"Acme": "#102a43"}},
	{"link on banner", `<table><tr><td bgcolor="#0b3d91" style="color:#ffffff"><a href="https://x.test" style="color:#ffffff">Footer link</a></td></tr></table>`, map[string]string{"Footer link": "#0b3d91"}},
}

func wc0(s string) color.NRGBA { c, _ := parseCSSColor(s); return c.(color.NRGBA) }

func g3Lum(c color.NRGBA) float64 { return mmLum(c) }

// (a) a band exists behind the right text with the right colour (light: as
// authored; dark: remapped, dark colours kept), and nothing else is painted.
func TestG3BandsMatrix(t *testing.T) {
	newApp(t)
	for _, f := range g3Fixtures {
		for _, light := range []bool{true, false} {
			res := mailRender(f.src, newMailTheme(light), true)
			bs := g3Bands(res)
			for text, want := range f.want {
				b := g3Find(bs, text)
				if want == "" {
					if b != nil && g3Hex(b.col) != "" && strings.Contains(b.text, text) && text != "after" && text != "Body copy" {
						t.Errorf("%s light=%v: %q must not be painted, got %s", f.name, light, text, g3Hex(b.col))
					}
					continue
				}
				if b == nil && !light && g3Lum(wc0(want)) > 0.85 {
					continue // near-white darkens to the surface itself: nothing to paint
				}
				if b == nil {
					t.Errorf("%s light=%v: no band behind %q (bands %v)", f.name, light, text, bs)
					continue
				}
				got := g3Hex(b.col)
				w, _ := parseCSSColor(want)
				wc := w.(color.NRGBA)
				switch {
				case light && got != want:
					t.Errorf("%s light: band %s, want authored %s", f.name, got, want)
				case !light && g3Lum(wc) < 0.1 && got != want:
					t.Errorf("%s dark: dark band %s changed to %s", f.name, want, got)
				case !light && g3Lum(b.col) > g3Lum(wc):
					t.Errorf("%s dark: band %s got lighter (%s)", f.name, want, got)
				}
			}
			need := 4.5
			if light {
				need = minContrast
			}
			if r, w := mmMin(res, newMailTheme(light)); r < need {
				t.Errorf("%s light=%v: contrast %.2f < %.1f on %q", f.name, light, r, need, w)
			}
		}
	}
}

func TestG3BandsNested(t *testing.T) {
	newApp(t)
	res := mailRender(g3Fixtures[3].src, newMailTheme(true), false)
	bs := g3Bands(res)
	if len(bs) != 3 || bs[0].depth != 0 || bs[1].depth != 1 || bs[2].depth != 2 ||
		g3Hex(bs[0].col) != "#dddddd" || g3Hex(bs[1].col) != "#bbccee" || g3Hex(bs[2].col) != "#ffffcc" {
		t.Fatalf("nested bands %v", bs)
	}
}

// White on a band stays white; the white link on it is repaired only if it can't be read.
func TestG3LightKeepsWhiteOnBand(t *testing.T) {
	newApp(t)
	res := mailRender(g3Fixtures[1].src, newMailTheme(true), false)
	if n := mmColour(res.segments, "Big sale"); n != "#ffffff" {
		t.Errorf("white on navy = %q", n)
	}
	res = mailRender(g3Fixtures[12].src, newMailTheme(true), false)
	var link *colLink
	for _, s := range res.segments {
		if l, ok := s.(*colLink); ok {
			link = l
		}
	}
	if link == nil || link.col != "#ffffff" {
		t.Fatalf("link on navy band = %+v, want white", link)
	}
}

// Quoted text is drawn unpainted in the fold, so it is judged on the pane and gets no band.
func TestG3QuotedHasNoBand(t *testing.T) {
	newApp(t)
	src := `<div style="background-color:#0b3d91;color:#fff">Top band</div><div class="gmail_quote"><div style="background-color:#0b3d91;color:#ffffff">Old navy</div></div>`
	for _, light := range []bool{true, false} {
		res := mailRender(src, newMailTheme(light), false)
		if len(res.quoted) == 0 {
			t.Fatal("no quote folded")
		}
		for _, b := range g3Bands(res) {
			if strings.Contains(b.text, "Old") {
				t.Errorf("light=%v: quoted text inside band %v", light, b)
			}
		}
		if r, w := mmMin(res, newMailTheme(light)); r < minContrast {
			t.Errorf("light=%v: contrast %.2f on %q", light, r, w)
		}
	}
}

// g3Body builds a laid-out body widget for html at width w.
func g3Body(t *testing.T, html string, light bool, w float32) (*selBody, renderResult) {
	t.Helper()
	th := newMailTheme(light)
	res := mailRender(html, th, true)
	b := newSelBody(res.segments)
	b.th = th
	b.setBands(res.bands)
	b.Resize(fyne.NewSize(w, 10))
	b.Resize(fyne.NewSize(w, b.MinSize().Height))
	return b, res
}

func g3Rects(b *selBody) (rs []*canvas.Rectangle) {
	for _, o := range test_renderer(b).Objects() {
		if r, ok := o.(*canvas.Rectangle); ok {
			rs = append(rs, r)
		}
	}
	return
}

func test_renderer(b *selBody) fyne.WidgetRenderer {
	r := &selRenderer{b: b}
	r.Refresh()
	return r
}

// (e)(f) bands stay inside the widget, resize re-lays them out, nothing stale.
func TestG3BandGeometryAndResize(t *testing.T) {
	newApp(t)
	src := g3Fixtures[0].src + `<table><tr><td bgcolor="#0b3d91" style="color:#fff">` + strings.Repeat("long banner words ", 30) + `</td></tr></table>`
	for _, light := range []bool{true, false} {
		b, res := g3Body(t, src, light, 700)
		n := len(res.bands)
		if n < 3 || len(b.lay.bands) != n {
			t.Fatalf("bands %d, laid out %d", n, len(b.lay.bands))
		}
		last := b.lay.bands[n-1]
		hWide := last.h
		for _, w := range []float32{300, 700, 1400} {
			b.Resize(fyne.NewSize(w, b.MinSize().Height))
			if len(b.lay.bands) != n {
				t.Fatalf("w=%v: %d bands", w, len(b.lay.bands))
			}
			for i, br := range b.lay.bands {
				if br.h <= 2*bandPad || br.y < 0 || br.y+br.h > b.lay.height {
					t.Errorf("w=%v band %d: y=%v h=%v height=%v", w, i, br.y, br.h, b.lay.height)
				}
			}
			rs := g3Rects(b)
			if len(rs) != n {
				t.Errorf("w=%v: %d rects, want %d (no stale ones)", w, len(rs), n)
			}
			for _, r := range rs {
				if r.Position().X != 0 || r.Size().Width != w {
					t.Errorf("w=%v: band x=%v width=%v beyond content width", w, r.Position().X, r.Size().Width)
				}
			}
			if w == 300 && b.lay.bands[n-1].h <= hWide {
				t.Errorf("light=%v: narrow band %v not taller than wide %v", light, b.lay.bands[n-1].h, hWide)
			}
		}
	}
}

// (c) selection paints over the band and under the text; copy is colour-free sanitized HTML.
func TestG3SelectionAndCopyAcrossBands(t *testing.T) {
	newApp(t)
	for _, light := range []bool{true, false} {
		fake := &richclip.Fake{}
		b, _ := g3Body(t, g3Fixtures[0].src, light, 700)
		b.copyFn = func(h, tx string) { _ = fake.Set(h, tx) }
		b.selectAll()
		var kinds []string
		for _, o := range test_renderer(b).Objects() {
			switch o.(type) {
			case *canvas.Rectangle:
				kinds = append(kinds, "r")
			case *canvas.Text:
				kinds = append(kinds, "t")
			}
		}
		k := strings.Join(kinds, "")
		first := strings.Index(k, "t")
		if first < 0 || strings.Contains(k[first:], "r") || len(g3Rects(b)) < len(b.lay.bands)+2 {
			t.Errorf("light=%v: draw order %q (bands, selection, then text expected)", light, k)
		}
		b.copySel()
		if fake.Calls != 1 || strings.ContainsAny(fake.HTML, "#") || strings.Contains(fake.HTML, "color") || strings.Contains(fake.HTML, "background") ||
			!strings.Contains(fake.Text, "Header band") || !strings.Contains(fake.Text, "Footer band") || !strings.Contains(fake.HTML, "<p>") {
			t.Errorf("light=%v: copy html=%q text=%q", light, fake.HTML, fake.Text)
		}
	}
}

func g3BigMail(rows int) string {
	var sb strings.Builder
	sb.WriteString(`<html><body><table>`)
	for i := 0; i < rows; i++ {
		c := []string{"#e8f0fe", "#fff3cd", "#0b3d91"}[i%3]
		fmt.Fprintf(&sb, `<tr><td bgcolor="%s" style="color:#%s">Row %d of the newsletter with some words in it</td></tr>`, c, map[bool]string{true: "ffffff", false: "202124"}[i%3 == 2], i)
	}
	sb.WriteString(`</table></body></html>`)
	return sb.String()
}

// (d) a 1 MB mail renders fast and the band count is capped.
func TestG3Perf1MBAndBandCap(t *testing.T) {
	newApp(t)
	g3Body(t, "<p>warm the font cache</p>", true, 300)
	src := g3BigMail(10000)
	for len(src) < 1<<20 {
		src += "<p>" + strings.Repeat("filler text ", 20) + "</p>"
	}
	for _, light := range []bool{true, false} {
		th := newMailTheme(light)
		start := time.Now()
		res := mailRender(src, th, true)
		b := newSelBody(res.segments)
		b.th = th
		b.setBands(res.bands)
		b.Resize(fyne.NewSize(900, 10))
		_ = b.MinSize()
		d := time.Since(start)
		if len(res.bands) == 0 || len(res.bands) > maxBands || len(b.lay.bands) > maxBands {
			t.Errorf("light=%v: %d bands, laid out %d (cap %d)", light, len(res.bands), len(b.lay.bands), maxBands)
		}
		if d > time.Second && !raceOn {
			t.Errorf("light=%v: 1 MB mail took %v", light, d)
		}
		t.Logf("light=%v: %d bytes, %d bands, %v", light, len(src), len(res.bands), d)
		if r, w := mmMin(res, th); r < map[bool]float64{true: minContrast, false: 4.5}[light] {
			t.Errorf("light=%v: past the cap contrast %.2f on %q", light, r, w)
		}
	}
}

// Malformed HTML never panics and always yields consistent geometry.
func FuzzG3Bands(f *testing.F) {
	for _, fx := range g3Fixtures {
		f.Add(fx.src)
	}
	f.Add(`<td bgcolor=><table bgcolor="#zzz"><tr><td style="background:">x</div></table></td>`)
	f.Add(`<div style="background-color:rgb(1,2"><p bgcolor="#fff" style="background:url(">`)
	f.Add(`<table bgcolor="red"><tr bgcolor="blue"><td bgcolor="red"><blockquote style="background:#000">`)
	f.Fuzz(func(t *testing.T, src string) {
		a := test_newApp(t)
		_ = a
		for _, light := range []bool{true, false} {
			b, res := g3Body(t, src, light, 320)
			for _, br := range res.bands {
				if br.from < 0 || br.to > len(res.segments) || br.to <= br.from {
					t.Fatalf("bad band %+v of %d segments", br, len(res.segments))
				}
			}
			for _, br := range b.lay.bands {
				if br.y < 0 || br.y+br.h > b.lay.height+0.5 {
					t.Fatalf("band out of layout: %+v height %v", br, b.lay.height)
				}
			}
		}
	})
}

func test_newApp(t *testing.T) fyne.App { t.Helper(); newApp(t); return fyne.CurrentApp() }

const g3GoldenHTML = `<table width="100%" bgcolor="#ffffff"><tr><td bgcolor="#0b3d91" style="color:#ffffff;font-size:20px"><b>Spring sale banner</b></td></tr>
<tr><td bgcolor="#e8f0fe" style="color:#1a237e">Header strip on light blue, dark blue text.</td></tr>
<tr><td style="color:#000000">Dear Neil,<br>Black newsletter text on white.</td></tr>
<tr><td><table width="100%" bgcolor="#fff3cd"><tr><td bgcolor="#ffe08a" style="color:#5c3b00">Nested offer: yellow table with an amber cell.</td></tr></table></td></tr>
<tr><td style="color:#cccccc" bgcolor="#f5f5f5">Light grey footer text on a near white band.</td></tr>
<tr><td bgcolor="#0b3d91" style="color:#ffffff">Footer: <a href="https://x.test" style="color:#ffffff">white link on navy</a> and <a href="https://y.test">default link</a></td></tr>
<tr><td style="color:#222">Dark text with no background set.</td></tr></table>
<p>Earlier in the thread:</p><blockquote style="color:#333">Quoted reply text<br>second quoted line</blockquote>`

func g3Golden(t *testing.T, mailLight bool, name string) {
	freezeNow(t, goldenNow)
	u := mmOpen(t)
	u.mailMode = map[bool]string{true: "light", false: "dark"}[mailLight]
	u.messages = []desktop.MessageItem{
		{ID: "m1", ThreadID: "t3", AccountID: "a1", Subject: "Spring sale", FromAddr: "Shop <s@x.com>", ToAddr: "Neil <n@x.com>", Body: g3GoldenHTML, Date: msAt(5), IsUnread: true},
	}
	u.msgCursor = 0
	u.refreshReader()
	readerShot(t, u, name)
}

func TestG3Goldens(t *testing.T) {
	t.Run("dark", func(t *testing.T) { g3Golden(t, false, "g3_mail_dark_bands") })
	t.Run("light", func(t *testing.T) { g3Golden(t, true, "g3_mail_light_bands") })
}

// The golden fixture meets the contrast rules in both modes.
func TestG3GoldenContrast(t *testing.T) {
	newApp(t)
	for _, light := range []bool{true, false} {
		th := newMailTheme(light)
		res := mailRender(g3GoldenHTML, th, false)
		if r, w := mmMin(res, th); r < map[bool]float64{true: minContrast, false: 4.5}[light] {
			t.Errorf("light=%v: %.2f on %q", light, r, w)
		}
		if len(res.bands) < 5 {
			t.Errorf("light=%v: only %d bands", light, len(res.bands))
		}
	}
}

package ui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/richclip"
)

// mmLum and mmRatio are an independent WCAG 2.x contrast implementation.
func mmLum(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		x := float64(v>>8) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func mmRatio(a, b color.Color) float64 {
	la, lb := mmLum(a), mmLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// mmMin is the lowest text/background contrast over every text and link
// segment, against the band painted behind it (innermost containing band; the
// pane for quoted text). Bars are decorative and skipped.
func mmMin(res renderResult, th mailTheme) (min float64, worst string) {
	min = 100
	pane := th.Color(theme.ColorNameBackground, theme.VariantLight)
	check := func(s widget.RichTextSegment, name fyne.ThemeColorName, bg color.Color) {
		if strings.TrimSpace(s.Textual()) == "" {
			return
		}
		if r := mmRatio(th.Color(name, theme.VariantLight), bg); r < min {
			min, worst = r, s.Textual()
		}
	}
	name := func(s widget.RichTextSegment) (fyne.ThemeColorName, bool) {
		switch v := s.(type) {
		case *widget.TextSegment:
			if v.Style.ColorName == theme.ColorNamePlaceHolder && barRe.MatchString(v.Text) {
				return "", false
			}
			if v.Style.ColorName == "" {
				return theme.ColorNameForeground, true
			}
			return v.Style.ColorName, true
		case *colLink:
			return v.col, true
		case *widget.HyperlinkSegment:
			return theme.ColorNameHyperlink, true
		}
		return "", false
	}
	for i, s := range res.segments {
		if n, ok := name(s); ok {
			check(s, n, mmBgAt(res.bands, i, pane))
		}
	}
	for _, s := range res.quoted {
		if n, ok := name(s); ok {
			check(s, n, pane)
		}
	}
	return
}

// mmBgAt is the colour painted behind segment i.
func mmBgAt(bands []mailBand, i int, pane color.Color) color.Color {
	bg := pane
	for _, b := range bands {
		if b.from <= i && i < b.to {
			bg = b.col
		}
	}
	return bg
}

var mmFixtures = map[string]string{
	"white newsletter black text": `<body bgcolor="#ffffff"><table bgcolor="#fff"><tr><td style="color:#000000">Weekly digest</td></tr></table></body>`,
	"dark text on transparent":    `<p style="color:#222;background-color:transparent">Dark on nothing</p>`,
	"light grey on white":         `<div style="background:white"><span style="color:#cccccc">Faint footer</span></div>`,
	"navy banner white text":      `<table><tr><td bgcolor="#0b3d91" style="color:#ffffff;background:#0b3d91">Big sale banner</td></tr></table>`,
	"brand red links":             `<p><a href="https://x.test" style="color:#c0392b">Shop now</a> <a href="https://y.test" style="color:#8b0000">More</a></p>`,
	"font color tags":             `<p><font color="#ffff00">Yellow text</font> <font color="#000080">Navy text</font></p>`,
	"blockquote":                  `<p>Reply above</p><blockquote style="color:#333333">Quoted <b style="color:#000">old</b> text</blockquote>`,
	"plain text":                  "Hello,\nplain words\n> quoted line\n",
	"logo chip image":             `<p><img src="https://x.test/logo.png" alt="Logo" width="40" height="40"> <span style="color:#444">Acme Inc</span></p>`,
}

func TestMailRenderContrast(t *testing.T) {
	newApp(t)
	for name, src := range mmFixtures {
		for _, light := range []bool{false, true} {
			th := newMailTheme(light)
			res := mailRender(src, th, true)
			need := 4.5
			if light {
				need = minContrast
			}
			if len(res.segments)+len(res.quoted) == 0 {
				t.Errorf("%s light=%v: nothing rendered", name, light)
				continue
			}
			if r, w := mmMin(res, th); r < need {
				t.Errorf("%s light=%v: contrast %.2f < %.1f on %q", name, light, r, need, w)
			}
		}
	}
}

func newApp(t *testing.T) { t.Helper(); test.NewTempApp(t) }

// Light mode must not run the dark adapter: the author's colour survives as is
// (a readable brand colour is untouched), while dark mode changes it.
func TestMailLightIsAsAuthored(t *testing.T) {
	newApp(t)
	src := `<p style="color:#8b0000">Maroon</p>`
	if n := mmColour(mailRender(src, newMailTheme(true), false).segments, "Maroon"); n != "#8b0000" {
		t.Errorf("light colour = %q, want original #8b0000", n)
	}
	if n := mmColour(mailRender(src, newMailTheme(false), false).segments, "Maroon"); n == "#8b0000" || n == "" {
		t.Errorf("dark colour = %q, want adapted", n)
	}
}

// White text on a navy band stays white (it is painted on navy); only text
// that would be unreadable on its effective background is repaired.
func TestMailLightGuardUsesPaintedBackground(t *testing.T) {
	newApp(t)
	th := newMailTheme(true)
	src := `<table><tr><td bgcolor="#0b3d91" style="color:#ffffff">Banner</td></tr></table><p style="color:#ffffff">Ghost</p><table><tr><td bgcolor="#0b3d91" style="color:#102060">Murky</td></tr></table>`
	res := mailRender(src, th, false)
	if n := mmColour(res.segments, "Banner"); n != "#ffffff" {
		t.Errorf("white on navy band repaired to %q", n)
	}
	for _, w := range []string{"Ghost", "Murky"} {
		if n := mmColour(res.segments, w); n == "#ffffff" || n == "#102060" || n == "" {
			t.Errorf("%s not repaired: %q", w, n)
		}
	}
}

func mmColour(segs []widget.RichTextSegment, text string) string {
	for _, s := range segs {
		if ts, ok := s.(*widget.TextSegment); ok && strings.Contains(ts.Text, text) {
			return string(ts.Style.ColorName)
		}
	}
	return ""
}

func mmOpen(t *testing.T, opts ...testOpt) *UI {
	t.Helper()
	u, _ := newTestUI(t, opts...)
	u.list.Select(2)
	return u
}

func TestMailToggleTopRightFlipsMode(t *testing.T) {
	u := mmOpen(t)
	if !u.mailLight() {
		t.Fatal("light app theme must default to a light mail view")
	}
	b := readerTipButton(u.reader, "Switch mail to dark mode")
	if b == nil {
		t.Fatal("no toggle")
	}
	d := fyne.CurrentApp().Driver()
	bp, sp := d.AbsolutePositionForObject(b), d.AbsolutePositionForObject(u.search)
	if bp.Y > 60 || bp.X <= sp.X+u.search.Size().Width-1 || bp.X+b.Size().Width < u.reader.Size().Width-40 {
		t.Fatalf("toggle at %v, search at %v, reader width %v: not top-right", bp, sp, u.reader.Size().Width)
	}
	b.OnTapped()
	if u.mailLight() || u.app.Preferences().String(prefMailMode) != "dark" {
		t.Fatalf("not dark/persisted: %q", u.mailMode)
	}
	if readerTipButton(u.reader, "Switch mail to light mode") == nil {
		t.Fatal("tooltip did not flip")
	}
	u.typedRune('v')
	if !u.mailLight() {
		t.Fatal("v shortcut did not toggle")
	}
}

func TestMailDefaultFollowsAppTheme(t *testing.T) {
	if u := mmOpen(t, withDark()); u.mailLight() {
		t.Error("dark app should default to dark mail")
	}
}

func TestMailToggleKeepsScroll(t *testing.T) {
	u := mmOpen(t)
	u.messages[2].Body = "<p>" + strings.Repeat("line<br>", 200) + "</p>"
	u.refreshReader()
	u.convScroll.ScrollToOffset(fyne.NewPos(0, 120))
	u.toggleMailMode()
	if y := u.convScroll.Offset.Y; y != 120 {
		t.Errorf("scroll offset %v after toggle, want 120", y)
	}
}

// Copy carries the sanitized original structure, never the adapted colours.
func TestMailCopyHasNoColours(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := mmOpen(t)
		fake := &richclip.Fake{}
		old := richClipboard
		richClipboard = fake
		t.Cleanup(func() { richClipboard = old })
		u.mailMode = map[bool]string{true: "dark", false: "light"}[dark]
		th := newMailTheme(!dark)
		b := u.newBody(mailRender(`<p style="color:#000">Black <font color="#c0392b">red</font> words</p>`, th, false).segments)
		b.th = th
		b.setSel(0, len(b.doc.flat))
		b.copySel()
		if fake.Calls != 1 || strings.Contains(fake.HTML, "#") || strings.Contains(fake.HTML, "color") || !strings.Contains(fake.Text, "Black red words") {
			t.Errorf("dark=%v copy html=%q text=%q", dark, fake.HTML, fake.Text)
		}
	}
}

const mmGoldenHTML = `<table width="100%" bgcolor="#ffffff"><tr><td bgcolor="#0b3d91" style="color:#ffffff;font-size:20px"><b>Spring sale banner</b></td></tr>
<tr><td style="color:#000000">Dear Neil,<br>Black newsletter text on white.</td></tr>
<tr><td style="color:#cccccc">Light grey footer text, barely there on white.</td></tr>
<tr><td style="color:#222">Dark text with no background set.</td></tr>
<tr><td><a href="https://x.test" style="color:#c0392b">Brand red link</a> and <font color="#000080">navy font tag</font> and <font color="#ffcc00">yellow font tag</font>.</td></tr></table>
<p>Earlier in the thread:</p><blockquote style="color:#333">Quoted reply text<br>second quoted line</blockquote>`

func mmGolden(t *testing.T, appDark, mailLight bool, name string) {
	freezeNow(t, goldenNow)
	opts := []testOpt{}
	if appDark {
		opts = append(opts, withDark())
	}
	u := mmOpen(t, opts...)
	u.mailMode = map[bool]string{true: "light", false: "dark"}[mailLight]
	u.messages = []desktop.MessageItem{
		{ID: "m1", ThreadID: "t3", AccountID: "a1", Subject: "Spring sale", FromAddr: "Shop <s@x.com>", ToAddr: "Neil <n@x.com>", Body: mmGoldenHTML, Date: msAt(5), IsUnread: true,
			Attachments: []desktop.AttachmentItem{{Name: "invoice.pdf", MimeType: "application/pdf", Size: 345678, AttachmentID: "x1"}}},
		{ID: "m2", ThreadID: "t3", AccountID: "a1", Subject: "Spring sale", FromAddr: "Pat <p@x.com>", ToAddr: "Neil <n@x.com>", Body: "Thanks, plain text here.\n\nOn Tue, Neil wrote:\n> older plain line\n", Date: msAt(2)},
	}
	u.msgCursor = 1
	u.refreshReader()
	readerShot(t, u, name)
}

func TestMailGoldens(t *testing.T) {
	t.Run("dark_appdark", func(t *testing.T) { mmGolden(t, true, false, "reader_mail_dark_appdark") })
	t.Run("light_appdark", func(t *testing.T) { mmGolden(t, true, true, "reader_mail_light_appdark") })
	t.Run("dark_applight", func(t *testing.T) { mmGolden(t, false, false, "reader_mail_dark_applight") })
	t.Run("light_applight", func(t *testing.T) { mmGolden(t, false, true, "reader_mail_light_applight") })
}

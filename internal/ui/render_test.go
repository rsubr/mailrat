package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

var (
	darkBG  = color.NRGBA{0x1e, 0x1e, 0x1e, 0xff}
	lightBG = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	darkFG  = color.NRGBA{0xf2, 0xf2, 0xf7, 0xff}
	lightFG = color.NRGBA{0x1c, 0x1c, 0x1e, 0xff}
)

// texts flattens rendered segments into (text, colour name) pairs.
func texts(res renderResult) (out [][2]string) {
	var walk func(segs []widget.RichTextSegment)
	walk = func(segs []widget.RichTextSegment) {
		for _, s := range segs {
			switch v := s.(type) {
			case *widget.ParagraphSegment:
				walk(v.Texts)
			case *widget.ListSegment:
				walk(v.Items)
			case *widget.TextSegment:
				out = append(out, [2]string{v.Text, string(v.Style.ColorName)})
			case *widget.HyperlinkSegment:
				out = append(out, [2]string{v.Text, "link:" + v.URL.String()})
			}
		}
	}
	walk(res.segments)
	walk(res.quoted)
	return
}

func colourOf(t *testing.T, name string) color.Color {
	t.Helper()
	c, ok := parseHexName(fyne.ThemeColorName(name))
	if !ok {
		t.Fatalf("not a hex colour name: %q", name)
	}
	return c
}

func TestDarkModeFixesBlackText(t *testing.T) {
	src := `<html><body style="background:#fff"><p style="color:#000000">Black text</p><p><font color="#111">dim</font></p><p style="color:#336699">Blue ok?</p></body></html>`
	res := renderMail(src, renderOpts{bg: darkBG, fg: darkFG})
	for _, p := range texts(res) {
		if c := contrast(colourOf(t, p[1]), darkBG); c < minContrast {
			t.Errorf("%q: contrast %.2f on dark bg (colour %s)", p[0], c, p[1])
		}
	}
}

func TestLightModeFixesWhiteText(t *testing.T) {
	src := `<p style="color:white">White on white</p><p style="color:rgb(0,0,0)">Black</p>`
	for _, p := range texts(renderMail(src, renderOpts{bg: lightBG, fg: lightFG})) {
		if contrast(colourOf(t, p[1]), lightBG) < minContrast {
			t.Errorf("%q unreadable on light: %s", p[0], p[1])
		}
	}
}

func TestReadableColourKept(t *testing.T) {
	// Mid-blue is readable on both; it must not be rewritten.
	c := color.NRGBA{0x3b, 0x82, 0xf6, 0xff}
	if got := readable(c, darkBG, darkFG); got != color.Color(c) {
		t.Errorf("changed readable colour: %v", got)
	}
	// Navy on dark flips to a light blue rather than collapsing to grey-white.
	got := color.NRGBAModel.Convert(readable(color.NRGBA{0, 0, 0x80, 0xff}, darkBG, darkFG)).(color.NRGBA)
	if got.B <= got.R || got.B <= got.G {
		t.Errorf("hue lost: %v", got)
	}
}

func TestStructure(t *testing.T) {
	src := `<head><title>x</title><style>p{color:red}</style></head><body>
	<div style="display:none">preheader junk</div>
	<h1>Hi <b>Ben</b></h1><p>See <a href="https://example.com/a">this link</a> now.</p>
	<a href="javascript:alert(1)">bad</a><ul><li>one</li><li>two</li></ul>
	<img src="https://t.example/p.gif" width="1" height="1"><img src="https://x.example/i.png" alt="Logo">
	<blockquote>quoted</blockquote></body>`
	res := renderMail(src, renderOpts{bg: darkBG, fg: darkFG})
	var all []string
	links := 0
	for _, p := range texts(res) {
		all = append(all, p[0])
		if strings.HasPrefix(p[1], "link:") {
			links++
		}
	}
	joined := strings.Join(all, "|")
	for _, bad := range []string{"preheader", "color:red", "alert"} {
		if strings.Contains(joined, bad) && bad != "alert" {
			t.Errorf("leaked %q in %q", bad, joined)
		}
	}
	if links != 1 { // javascript: link must be rendered as plain text, not a link
		t.Errorf("want 1 link, got %d (%s)", links, joined)
	}
	if res.blocked != 1 {
		t.Errorf("blocked images = %d, want 1 (tracking pixel ignored)", res.blocked)
	}
	for _, want := range []string{"Hi ", "Ben", "this link", "one", "two", "quoted", "[Logo]"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	loaded := renderMail(src, renderOpts{bg: darkBG, fg: darkFG, images: true})
	if loaded.blocked != 0 {
		t.Error("images=true should not block")
	}
}

func TestPlainTextLinkified(t *testing.T) {
	res := renderMail("See https://example.com/x, ok?\n\nSecond para", renderOpts{bg: darkBG, fg: darkFG})
	links := 0
	for _, p := range texts(res) {
		if p[1] == "link:https://example.com/x" {
			links++
		}
	}
	if links != 1 {
		t.Fatalf("links=%d %v", links, texts(res))
	}
}

func renderedAll(res renderResult) string {
	var b strings.Builder
	var links []string
	for _, t := range texts(res) {
		b.WriteString(t[0])
		if strings.HasPrefix(t[1], "link:") {
			links = append(links, t[1])
		}
	}
	return b.String() + " " + strings.Join(links, " ") + " " + imageSources(res.segments)
}

func imageSources(segs []widget.RichTextSegment) (s string) {
	for _, g := range segs {
		if im, ok := g.(*widget.ImageSegment); ok {
			s += " img:" + im.Source.String()
		}
	}
	return
}

func TestXSSPayloadsNeutralised(t *testing.T) {
	payloads := []string{
		`<script>alert(1)</script>`,
		`<img src=x onerror=alert(1)>`,
		`<a href="javascript:alert(1)">click</a>`,
		`<a href="  JaVaScRiPt:alert(1)">click</a>`,
		`<a href="java&#9;script:alert(1)">click</a>`,
		`<a href="data:text/html,<script>alert(1)</script>">click</a>`,
		`<a href="vbscript:msgbox(1)">click</a>`,
		`<iframe src="https://evil.example/x"></iframe>`,
		`<object data="x.swf"></object><embed src="x.swf">`,
		`<form action="https://evil.example"><input name=a><button>go</button></form>`,
		`<svg onload=alert(1)><script>alert(1)</script></svg>`,
		`<style>body{background:url(javascript:alert(1))}</style>`,
		`<div style="background:url(javascript:alert(1))" onclick="alert(1)">x</div>`,
		`<body onload=alert(1)>x</body>`,
		`<img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=">`,
		`<img src="cid:abc@x">`,
		`<img src="file:///etc/passwd">`,
		`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
		`<base href="javascript:alert(1)//">`,
	}
	for _, p := range payloads {
		for _, images := range []bool{false, true} {
			out := strings.ToLower(renderedAll(renderMail("<html><body>"+p+"</body></html>", renderOpts{bg: lightBG, fg: lightFG, images: images})))
			for _, bad := range []string{"alert", "javascript:", "vbscript:", "data:", "file:", "onerror", "onclick", "evil.example", "msgbox", "cid:"} {
				if strings.Contains(out, bad) {
					t.Errorf("payload %q (images=%v) leaked %q: %s", p, images, bad, out)
				}
			}
		}
	}
}

func TestRemoteImageNotLoadedByDefault(t *testing.T) {
	src := `<img src="https://tracker.example/p.png" alt="logo">`
	res := renderMail(src, renderOpts{bg: lightBG, fg: lightFG})
	if res.blocked != 1 || imageSources(res.segments) != "" || !strings.Contains(renderedAll(res), "[logo]") {
		t.Fatalf("blocked=%d out=%q", res.blocked, renderedAll(res))
	}
	res = renderMail(src, renderOpts{bg: lightBG, fg: lightFG, images: true})
	if !strings.Contains(imageSources(res.segments), "https://tracker.example/p.png") {
		t.Fatal("image should load when allowed")
	}
}

func TestSafeLinkAndFormatting(t *testing.T) {
	src := `<h1>T</h1><p><b>b</b> <i>i</i> <u>u</u> <a href="https://ok.example/x">l</a></p><ul><li>one</li></ul><blockquote>q</blockquote>`
	res := renderMail(src, renderOpts{bg: lightBG, fg: lightFG})
	out := renderedAll(res)
	for _, want := range []string{"T", "one", "q", "link:https://ok.example/x"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	var underlined bool
	for _, g := range res.segments {
		if ts, ok := g.(*widget.TextSegment); ok && ts.Text == "u" && ts.Style.TextStyle.Underline {
			underlined = true
		}
	}
	if !underlined {
		t.Error("<u> not underlined")
	}
}

func plainTexts(res renderResult) string {
	var b strings.Builder
	for _, t := range texts(res) {
		b.WriteString(t[0])
	}
	return b.String()
}

func TestRenderTableKeepsRowsTogether(t *testing.T) {
	src := `<table><tr><th>Item</th><th>Qty</th></tr><tr><td>Apple</td><td>3</td></tr></table>`
	res := renderMail(src, renderOpts{bg: lightBG, fg: lightFG})
	tab, ok := res.segments[0].(*tableSegment)
	if !ok || tab.cols != 2 || len(tab.rows) != 2 || !strings.Contains(tab.Textual(), "Apple 3") {
		t.Errorf("want 2x2 grid, got %#v", res.segments)
	}
}

func TestRenderPlainTextWithAngleBrackets(t *testing.T) {
	src := "Hi Bob,\nsee <https://example.com/x>\n\nFrom: Al <al@example.com>\nthanks"
	if isHTML(src) {
		t.Fatal("plain text with angle brackets detected as HTML")
	}
	got := plainTexts(renderMail(src, renderOpts{bg: lightBG, fg: lightFG}))
	if !strings.Contains(got, "Hi Bob,\nsee <") || !strings.Contains(got, "al@example.com>\nthanks") {
		t.Errorf("line breaks lost: %q", got)
	}
}

func TestQuotedReplyFolded(t *testing.T) {
	html := renderMail(`<div>Thanks!</div><div class="gmail_quote">On Mon, A wrote:<blockquote>old</blockquote></div>`, renderOpts{bg: lightBG, fg: lightFG})
	if len(html.quoted) == 0 || strings.Contains(renderedAll(renderResult{segments: html.segments}), "old") {
		t.Fatalf("html: segs=%q quoted=%d", renderedAll(renderResult{segments: html.segments}), len(html.quoted))
	}
	plain := renderMail("Thanks!\n\nOn Mon, Jan 1, A <a@x> wrote:\n> old", renderOpts{bg: lightBG, fg: lightFG})
	if len(plain.quoted) == 0 || strings.Contains(renderedAll(renderResult{segments: plain.segments}), "old") {
		t.Fatalf("plain: quoted=%d", len(plain.quoted))
	}
}

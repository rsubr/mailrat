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

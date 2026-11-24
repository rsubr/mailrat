package richclip

import (
	"testing"
)

// The offsets count bytes: "é" is 2 and "世界" is 6 in UTF-8.
func TestBuildCFHTMLGolden(t *testing.T) {
	got := string(buildCFHTML("<b>hé 世界</b>"))
	want := "Version:0.9\r\nStartHTML:0000000105\r\nEndHTML:0000000190\r\nStartFragment:0000000139\r\nEndFragment:0000000156\r\n" +
		"<html><body>\r\n<!--StartFragment--><b>hé 世界</b><!--EndFragment-->\r\n</body></html>"
	if got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
	if len(got) != 190 || got[139:156] != "<b>hé 世界</b>" {
		t.Fatalf("offsets do not match the bytes: len %d frag %q", len(got), got[139:156])
	}
}

func TestParseCFHTML(t *testing.T) {
	frag := "<p>naïve — 日本語</p>"
	nul := append(buildCFHTML(frag), 0)
	cases := map[string]struct{ in, want string }{
		"round trip":         {string(buildCFHTML(frag)), frag},
		"trailing NUL":       {string(nul), frag},
		"empty":              {string(buildCFHTML("")), ""},
		"bad offsets, marks": {"Version:0.9\r\nStartFragment:9999999999\r\nEndFragment:1\r\n<!--StartFragment-->x<!--EndFragment-->", "x"},
		"html offsets":       {"Version:0.9\r\nStartHTML:0000000055\r\nEndHTML:0000000064\r\n<p>hi</p>", "<p>hi</p>"},
		"no header":          {"<p>raw</p>", "<p>raw</p>"},
		"start mark only":    {"Version:0.9\r\n\r\n<!--StartFragment-->tail", "tail"},
	}
	for name, c := range cases {
		if got := parseCFHTML([]byte(c.in)); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}

func TestDecodeHTML(t *testing.T) {
	u16le := []byte{'<', 0, 'p', 0, '>', 0, 0xe9, 0, 0x16, 0x4e, '<', 0, '/', 0, 'p', 0, '>', 0}
	cases := map[string]struct {
		in   []byte
		want string
	}{
		"utf8":      {[]byte("<p>é世</p>"), "<p>é世</p>"},
		"utf8 bom":  {[]byte("\xef\xbb\xbf<p>x</p>"), "<p>x</p>"},
		"utf16 bom": {append([]byte{0xff, 0xfe}, u16le...), "<p>é世</p>"},
		"utf16 raw": {u16le, "<p>é世</p>"},
		"empty":     {nil, ""},
	}
	for name, c := range cases {
		if got := decodeHTML(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}

func TestFakeRoundTrip(t *testing.T) {
	f := &Fake{}
	_ = f.Set("<b>x</b>", "x")
	if h, p, err := f.ReadHTML(); h != "<b>x</b>" || p != "x" || err != nil {
		t.Fatal(h, p, err)
	}
}

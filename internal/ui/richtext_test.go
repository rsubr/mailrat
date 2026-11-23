package ui

import "testing"

func TestMarkupToHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hi", "<div>hi</div>"},
		{"a < b & c", "<div>a &lt; b &amp; c</div>"},
		{"a\n\nb", "<div>a</div><div><br></div><div>b</div>"},
		{"[b]x[/b] [i]y[/i] [u]z[/u]", "<div><b>x</b> <i>y</i> <u>z</u></div>"},
		{"[font=Georgia]g[/font]", `<div><span style="font-family:'Georgia'">g</span></div>`},
		{"[size=huge]h[/size]", `<div><span style="font-size:32px">h</span></div>`},
		{"[color=#cc0000]r[/color]", `<div><span style="color:#cc0000">r</span></div>`},
		{"[align=center]c[/align]", `<div style="text-align:center"><div>c</div></div>`},
		{"[url=https://x.org/?a=1&b=2]l[/url]", `<div><a href="https://x.org/?a=1&amp;b=2">l</a></div>`},
		{"- a\n- b\nc", "<ul><li>a</li><li>b</li></ul><div>c</div>"},
		{"1. a\n2. b", "<ol><li>a</li><li>b</li></ol>"},
		{"> q1\n> q2\nafter", `<blockquote style="` + quoteStyle + `"><div>q1</div><div>q2</div></blockquote><div>after</div>`},
		// formatting survives line breaks
		{"[b]a\nb[/b]", "<div><b>a</b></div><div><b>b</b></div>"},
		// misnested tags are repaired
		{"[b][i]x[/b]y[/i]", "<div><b><i>x</i></b><i>y</i></div>"},
		// unsafe or unknown tags stay literal
		{"[url=javascript:alert(1)]x[/url]", "<div>[url=javascript:alert(1)]x[/url]</div>"},
		{"[color=red;x]x[/color]", "<div>[color=red;x]x[/color]</div>"},
		{"[font=a'b]x", "<div>[font=a&#39;b]x</div>"},
		{"[nope]x[/b]", "<div>[nope]x[/b]</div>"},
	}
	for _, c := range cases {
		if got := markupToHTML(c.in); got != c.want {
			t.Errorf("markupToHTML(%q)\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}

package mail

import (
	"strings"
	"testing"
)

func TestSanitizeHTMLBlocks(t *testing.T) {
	cases := map[string][]string{
		`<script>alert(1)</script>x`:                                             {"script", "alert"},
		`<p onclick="x()">a</p>`:                                                 {"onclick"},
		`<a href="javascript:alert(1)">a</a>`:                                    {"javascript"},
		`<a href=" jav&#x09;ascript:alert(1)">a</a>`:                             {"script:"},
		`<a href="data:text/html,x">a</a>`:                                       {"data:"},
		`<img src="javascript:x">`:                                               {"<img"},
		`<img src="http://t.com/p.png">`:                                         {"<img"},
		`<img src="data:image/svg+xml,<svg/>">`:                                  {"<img"},
		`<iframe src="https://x"></iframe><object></object>`:                     {"iframe", "object"},
		`<form action="x"><input></form>`:                                        {"form", "input"},
		`<style>p{}</style>y`:                                                    {"style"},
		`<p style="color:red;background:url(javascript:x);position:fixed">a</p>`: {"url(", "position", "background:"},
		`<svg><script>x</script></svg>`:                                          {"svg", "script"},
		`<p class="evil">a</p>`:                                                  {"class"},
	}
	for in, bad := range cases {
		out := SanitizeHTML(in)
		for _, b := range bad {
			if strings.Contains(out, b) {
				t.Errorf("%q -> %q still contains %q", in, out, b)
			}
		}
	}
}

func TestSanitizeHTMLKeeps(t *testing.T) {
	cases := map[string]string{
		`<a href="https://e.com/?a=1&amp;b=2">x</a>`:               `href="https://e.com/?a=1&amp;b=2"`,
		`<a href="mailto:a@b.com">x</a>`:                           `mailto:a@b.com`,
		`<img src="cid:abc" alt="z">`:                              `src="cid:abc"`,
		`<img src="https://e.com/p.png">`:                          `https://e.com/p.png`,
		`<img src="data:image/png;base64,AAAA">`:                   `data:image/png`,
		`<p style="color: red; font-size: 12px">a</p>`:             `color: red; font-size: 12px`,
		`<blockquote class="gmail_quote" dir="ltr">q</blockquote>`: `class="gmail_quote"`,
		`<table><tbody><tr><td>c</td></tr></tbody></table>`:        `<td>c</td>`,
		`<font color="red"><b>bold</b></font>`:                     `<b>bold</b>`,
	}
	for in, want := range cases {
		if out := SanitizeHTML(in); !strings.Contains(out, want) {
			t.Errorf("%q -> %q missing %q", in, out, want)
		}
	}
}

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

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

func TestStripMarkup(t *testing.T) {
	if got := stripMarkup("[b]a[/b] [url=https://x]b[/url] [size=large]c[/size] [x]"); got != "a b c [x]" {
		t.Errorf("got %q", got)
	}
}

func TestMentionAt(t *testing.T) {
	cases := []struct {
		text   string
		cur    int
		q      string
		wantOK bool
	}{
		{"hi @an", 6, "an", true},
		{"@", 1, "", true},
		{"+bo", 3, "bo", true},
		{"mail me@x.com", 13, "", false}, // @ inside an address is not a mention
		{"@an done", 8, "", false},       // whitespace ended it
		{"plain", 5, "", false},
		{"@an|rest", 3, "an", true}, // only text before the cursor counts
	}
	for _, c := range cases {
		text := strings.ReplaceAll(c.text, "|", "")
		q, ok := mentionAt([]rune(text), c.cur)
		if q != c.q || ok != c.wantOK {
			t.Errorf("mentionAt(%q,%d) = %q,%v want %q,%v", c.text, c.cur, q, ok, c.q, c.wantOK)
		}
	}
}

func TestAddRecipient(t *testing.T) {
	ann := desktop.ContactItem{Name: "Ann", Email: "Ann@x.com"}
	if to, added := addRecipient("", "", ann); !added || to != "Ann <Ann@x.com>" {
		t.Errorf("empty To: %q %v", to, added)
	}
	if to, added := addRecipient("bob@x.com, ", "", ann); !added || to != "bob@x.com, Ann <Ann@x.com>" {
		t.Errorf("append: %q %v", to, added)
	}
	if _, added := addRecipient("Ann K <ann@x.com>", "", ann); added {
		t.Error("already in To")
	}
	if _, added := addRecipient("bob@x.com", "x, ann@x.com", ann); added {
		t.Error("already in Cc")
	}
}

func TestMentionMarkup(t *testing.T) {
	if got := mentionMarkup(desktop.ContactItem{Name: "Ann [B]", Email: "a@x.com"}); got != "[url=mailto:a@x.com]@Ann B[/url] " {
		t.Errorf("got %q", got)
	}
	if got := mentionMarkup(desktop.ContactItem{Email: "a@x.com"}); got != "[url=mailto:a@x.com]@a[/url] " {
		t.Errorf("got %q", got)
	}
}

func TestBodyEntryFormatting(t *testing.T) {
	test.NewTempApp(t)
	e := newBodyEntry(&suggester{})
	e.SetText("hello")
	e.CursorColumn = 5
	e.wrap("[b]", "[/b]") // no selection: empty pair, cursor between tags
	e.insert("X")
	if e.Text != "hello[b]X[/b]" {
		t.Fatalf("got %q", e.Text)
	}
	e.SetText("")
	e.prefixLines("- ")
	e.insert("one")
	e.prefixLines("- ")
	e.insert("two")
	if e.Text != "- one\n- two" {
		t.Fatalf("lists: %q", e.Text)
	}
}

func TestMentionPickInsertsLinkAndRecipient(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("t")
	u := &UI{inline: true, be: &fakeBackend{}}
	sg := &suggester{canvas: w.Canvas()}
	body := newBodyEntry(sg)
	to := u.recipientEntry(w, func() string { return "" }, "To")
	cc := u.recipientEntry(w, func() string { return "" }, "Cc")
	u.richEditor(w, body, to, cc, func() string { return "" })

	body.SetText("hi @an")
	body.CursorColumn = 6
	c := desktop.ContactItem{Name: "Ann", Email: "ann@x.com"}
	sg.pick(c)
	if body.Text != "hi [url=mailto:ann@x.com]@Ann[/url] " {
		t.Fatalf("body %q", body.Text)
	}
	if to.Text != "Ann <ann@x.com>" {
		t.Fatalf("to %q", to.Text)
	}
	if !strings.Contains(markupToHTML(body.Text), `<a href="mailto:ann@x.com">@Ann</a>`) {
		t.Fatal("mention is not a mailto link in the HTML")
	}
}

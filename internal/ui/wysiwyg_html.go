package ui

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/mailrat/mailrat/internal/mail"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// HTML import/export for the WYSIWYG document model. Input is parsed with x/net/html
// and reduced to the model, so anything outside the allowlist (p, br, div, span, b,
// strong, i, em, u, s, a, ul, ol, li, blockquote, font and style colour/size) is
// dropped while its text is kept.

var (
	hexColorRe = regexp.MustCompile(`^#([0-9a-f]{3}|[0-9a-f]{6})$`)
	rgbColorRe = regexp.MustCompile(`^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*[\d.]+\s*)?\)$`)
	hostPortRe = regexp.MustCompile(`^(?:localhost|[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+):\d{1,5}(?:[/?#].*)?$`)
	pxSizeRe   = regexp.MustCompile(`^(\d+(?:\.\d+)?)(px|pt)$`)
)

var wyNamedColors = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000",
	"blue": "#0000ff", "yellow": "#ffff00", "orange": "#ffa500", "purple": "#800080",
	"gray": "#808080", "grey": "#808080",
}

// parseColor normalises a CSS colour to "#rrggbb"; unknown or invalid input gives "".
func parseColor(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if n, ok := wyNamedColors[c]; ok {
		return n
	}
	if hexColorRe.MatchString(c) {
		if len(c) == 4 {
			return "#" + string([]byte{c[1], c[1], c[2], c[2], c[3], c[3]})
		}
		return c
	}
	if m := rgbColorRe.FindStringSubmatch(c); m != nil {
		var v [3]int
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+1])
			if v[i] > 255 {
				return ""
			}
		}
		return fmt.Sprintf("#%02x%02x%02x", v[0], v[1], v[2])
	}
	return ""
}

// cleanLink validates a user-entered link target. "" is valid and means no link.
// Only http, https and mailto survive; bare "name@host" becomes mailto: and bare
// "host.tld/..." becomes https://.
func cleanLink(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	if strings.IndexFunc(raw, func(r rune) bool { return r <= ' ' || r == '"' || r == '<' || r == '>' || r == 0x7f }) >= 0 {
		return "", false
	}
	l := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(l, "http://") && len(raw) > 7,
		strings.HasPrefix(l, "https://") && len(raw) > 8,
		strings.HasPrefix(l, "mailto:") && len(raw) > 7:
		return raw, true
	case hostPortRe.MatchString(raw):
		return "https://" + raw, true
	case strings.Contains(raw, ":"):
		return "", false
	case strings.Contains(raw, "@") && strings.Contains(raw, "/"):
		return "", false // "user@host/path" would become a userinfo URL
	case strings.Contains(raw, "@"):
		return "mailto:" + raw, true
	case strings.Contains(raw, "."):
		return "https://" + raw, true
	}
	return "", false
}

// --- parsing ---------------------------------------------------------------

type wyParser struct {
	doc     wyDoc
	cur     *wyParagraph
	lists   []wyKind // open ul/ol, innermost last
	quote   int
	started bool
	fold    bool // a gmail_quote block opened: its first paragraph starts the folded quote
	folded  bool // the doc already has its fold; later quote blocks are plain content
	sig     bool // a gmail_signature block opened: its first paragraph is marked
	sigSeen bool
	pre     bool // inside <pre>: line breaks and spaces are kept
}

func (p *wyParser) kind() wyKind {
	if n := len(p.lists); n > 0 {
		return p.lists[n-1]
	}
	if p.quote > 0 {
		return wyQuote
	}
	return wyPara
}

// push finishes the current paragraph, if any.
func (p *wyParser) push() {
	if p.cur == nil {
		return
	}
	runs := p.cur.Runs
	if n := len(runs); n > 0 {
		runs[n-1].Text = strings.TrimRight(runs[n-1].Text, " ")
	}
	for i := range runs {
		runs[i].Text = strings.ReplaceAll(runs[i].Text, " ", " ")
	}
	p.cur.Runs = normRuns(runs)
	if p.fold {
		if len(p.doc.Paras) == 0 { // the editor keeps a visible paragraph above the quote
			p.doc.Paras = append(p.doc.Paras, wyParagraph{})
		}
		p.cur.Fold, p.fold, p.folded = true, false, true
	}
	if p.sig {
		p.cur.Sig, p.sig = true, false
		if p.cur.text() == strings.TrimSpace(sigMarker) { // the separator keeps its trailing space
			p.cur.Runs = []wyRun{{Text: sigMarker}}
		}
	}
	p.doc.Paras = append(p.doc.Paras, *p.cur)
	p.cur = nil
}

func (p *wyParser) open() *wyParagraph {
	if p.cur == nil {
		p.cur = &wyParagraph{Kind: p.kind()}
	}
	return p.cur
}

func (p *wyParser) text(s string, st wyStyle) {
	if p.pre {
		for i, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"), "\n") {
			if i > 0 {
				p.open()
				p.push()
			}
			if line != "" {
				cur := p.open()
				cur.Runs = append(cur.Runs, wyRun{Text: strings.ReplaceAll(line, "\t", "    "), wyStyle: st})
			}
		}
		return
	}
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			return ' '
		}
		return r
	}, s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if p.cur == nil || len(p.cur.Runs) == 0 {
		s = strings.TrimLeft(s, " ")
		if s == "" {
			return
		}
	} else if last := p.cur.Runs[len(p.cur.Runs)-1].Text; strings.HasSuffix(last, " ") {
		s = strings.TrimLeft(s, " ")
	}
	if s == "" {
		return
	}
	cur := p.open()
	cur.Runs = append(cur.Runs, wyRun{Text: s, wyStyle: st})
}

func styleDecls(v string) map[string]string {
	m := map[string]string{}
	for _, d := range strings.Split(v, ";") {
		if k, val, ok := strings.Cut(d, ":"); ok {
			m[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(val))
		}
	}
	return m
}

func wyAttr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// fontSizeFor maps CSS px/pt and the legacy <font size=1..7> to px.
func fontSizeFor(v string) int {
	if m := pxSizeRe.FindStringSubmatch(v); m != nil {
		f, _ := strconv.ParseFloat(m[1], 64)
		if m[2] == "pt" {
			f = f * 4 / 3
		}
		return max(8, min(int(f+0.5), 96))
	}
	return 0
}

var legacySizes = map[string]int{"1": 10, "2": 13, "3": 16, "4": 18, "5": 24, "6": 32, "7": 48}

// inline applies the formatting carried by element n to st.
func inline(n *xhtml.Node, st wyStyle) wyStyle {
	switch n.DataAtom {
	case atom.B, atom.Strong:
		st.Bold = true
	case atom.I, atom.Em:
		st.Italic = true
	case atom.U:
		st.Underline = true
	case atom.S, atom.Strike, atom.Del:
		st.Strike = true
	case atom.A:
		if u, ok := cleanLink(wyAttr(n, "href")); ok && u != "" {
			st.Link = u
		}
	case atom.Font:
		if c := parseColor(wyAttr(n, "color")); c != "" {
			st.Color = c
		}
		if px := legacySizes[wyAttr(n, "size")]; px != 0 {
			st.Size = px
		}
	}
	if s := wyAttr(n, "style"); s != "" {
		d := styleDecls(s)
		if c := parseColor(d["color"]); c != "" {
			st.Color = c
		}
		if px := fontSizeFor(d["font-size"]); px != 0 {
			st.Size = px
		}
		if w := d["font-weight"]; w == "bold" || w == "700" || w == "800" || w == "900" {
			st.Bold = true
		}
		if d["font-style"] == "italic" {
			st.Italic = true
		}
		if td := d["text-decoration"]; strings.Contains(td, "underline") {
			st.Underline = true
		} else if strings.Contains(td, "line-through") {
			st.Strike = true
		}
	}
	return st
}

func (p *wyParser) walk(n *xhtml.Node, st wyStyle) {
	switch n.Type {
	case xhtml.TextNode:
		p.text(n.Data, st)
	case xhtml.ElementNode:
		switch n.DataAtom {
		case atom.Script, atom.Style, atom.Head, atom.Title, atom.Template, atom.Object, atom.Iframe:
			return
		case atom.Br:
			p.open()
			p.push()
			return
		case atom.Ul, atom.Ol:
			p.push()
			k := wyBullet
			if n.DataAtom == atom.Ol {
				k = wyNumber
			}
			p.lists = append(p.lists, k)
			p.children(n, st)
			p.push()
			p.lists = p.lists[:len(p.lists)-1]
			return
		case atom.Blockquote:
			p.push()
			p.quote++
			p.children(n, st)
			p.push()
			p.quote--
			return
		case atom.P, atom.Li:
			p.push()
			p.open()
			p.children(n, inline(n, st))
			p.push()
			return
		case atom.Div:
			p.push()
			opens := !p.fold && !p.folded && wyAttr(n, "class") == "gmail_quote"
			p.fold = p.fold || opens
			sig := !p.sigSeen && wyAttr(n, "class") == "gmail_signature"
			p.sig, p.sigSeen = p.sig || sig, p.sigSeen || sig
			p.children(n, inline(n, st))
			p.push()
			if opens { // an empty quote block folds nothing
				p.fold = false
			}
			if sig {
				p.sig = false
			}
			return
		case atom.Pre:
			p.push()
			was := p.pre
			p.pre = true
			p.children(n, inline(n, st))
			p.push()
			p.pre = was
			return
		case atom.Td, atom.Th:
			p.children(n, inline(n, st))
			p.text(" ", st) // cells would otherwise run together
			return
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Table, atom.Tr:
			p.push()
			p.children(n, inline(n, st))
			p.push()
			return
		}
		p.children(n, inline(n, st))
	default:
		p.children(n, st)
	}
}

func (p *wyParser) children(n *xhtml.Node, st wyStyle) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.walk(c, st)
	}
}

// parseHTML converts src to a document; it never fails and always has one paragraph.
func parseHTML(src string) wyDoc {
	ctx := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(src), ctx)
	if err != nil {
		return newWyDoc()
	}
	p := &wyParser{}
	for _, n := range nodes {
		p.walk(n, wyStyle{})
	}
	p.push()
	if len(p.doc.Paras) == 0 {
		return newWyDoc()
	}
	return p.doc
}

// --- serialising -----------------------------------------------------------

func runHTML(r wyRun, text string) string {
	open, closing := "", ""
	add := func(o, c string) { open += o; closing = c + closing }
	if r.Link != "" {
		add(`<a href="`+html.EscapeString(r.Link)+`">`, "</a>")
	}
	if r.Bold {
		add("<b>", "</b>")
	}
	if r.Italic {
		add("<i>", "</i>")
	}
	if r.Underline {
		add("<u>", "</u>")
	}
	if r.Strike {
		add("<s>", "</s>")
	}
	var css []string
	if r.Color != "" {
		css = append(css, "color: "+r.Color)
	}
	if r.Size != 0 {
		css = append(css, "font-size: "+strconv.Itoa(r.Size)+"px")
	}
	if len(css) > 0 {
		add(`<span style="`+strings.Join(css, "; ")+`">`, "</span>")
	}
	return open + text + closing
}

// paraInner renders the runs of p; spaces that HTML would collapse become &nbsp;.
func paraInner(p wyParagraph) string {
	if p.text() == sigMarker { // keep the separator's trailing space, as Gmail does
		return sigMarker
	}
	total := p.length()
	if total == 0 {
		return "<br>"
	}
	var b strings.Builder
	idx, prevSpace := 0, true // a leading space must be kept
	for _, r := range p.Runs {
		var t strings.Builder
		for _, c := range r.Text {
			switch {
			case c == ' ' && (prevSpace || idx == total-1):
				t.WriteString("&nbsp;")
			case c == ' ':
				t.WriteByte(' ')
			default:
				t.WriteString(html.EscapeString(string(c)))
			}
			prevSpace = c == ' '
			idx++
		}
		b.WriteString(runHTML(r, t.String()))
	}
	return b.String()
}

// toHTML serialises d as clean HTML; the result also passes mail.SanitizeHTML.
func (d wyDoc) toHTML() string {
	f := d.foldAt()
	if f < 0 {
		return mail.SanitizeHTML(sigHTML(d.Paras))
	}
	// The first quoted paragraph is the "On ... wrote:" line: Gmail wants it as gmail_attr.
	q := d.Paras[f:]
	attr := ""
	if q[0].Kind == wyPara {
		attr, q = `<div class="gmail_attr">`+paraInner(q[0])+"</div>", q[1:]
	}
	return mail.SanitizeHTML(sigHTML(d.Paras[:f]) + quoteOpen + attr + parasHTML(q) + "</div>")
}

// sigHTML renders paras, wrapping the signature (from its "-- " line) in gmail_signature.
func sigHTML(paras []wyParagraph) string {
	for i, p := range paras {
		if p.Sig {
			return parasHTML(paras[:i]) + `<div class="gmail_signature">` + parasHTML(paras[i:]) + "</div>"
		}
	}
	return parasHTML(paras)
}

// parasHTML renders consecutive paragraphs, grouping lists and quotes.
func parasHTML(paras []wyParagraph) string {
	d := wyDoc{Paras: paras}
	var b strings.Builder
	for i := 0; i < len(d.Paras); {
		k := d.Paras[i].Kind
		j := i
		for j < len(d.Paras) && d.Paras[j].Kind == k {
			j++
		}
		switch k {
		case wyBullet, wyNumber:
			tag := map[wyKind]string{wyBullet: "ul", wyNumber: "ol"}[k]
			b.WriteString("<" + tag + ">")
			for _, p := range d.Paras[i:j] {
				b.WriteString("<li>" + paraInner(p) + "</li>")
			}
			b.WriteString("</" + tag + ">")
		case wyQuote:
			b.WriteString("<blockquote>")
			for _, p := range d.Paras[i:j] {
				b.WriteString("<p>" + paraInner(p) + "</p>")
			}
			b.WriteString("</blockquote>")
		default:
			for _, p := range d.Paras[i:j] {
				b.WriteString("<p>" + paraInner(p) + "</p>")
			}
		}
		i = j
	}
	return b.String()
}

// plainText renders d with "- ", "1. " and "> " line prefixes for lists and quotes.
func (d wyDoc) plainText() string {
	lines := make([]string, len(d.Paras))
	n := 0
	for i, p := range d.Paras {
		if p.Kind == wyNumber {
			n++
		} else {
			n = 0
		}
		switch p.Kind {
		case wyBullet:
			lines[i] = "- " + p.text()
		case wyNumber:
			lines[i] = strconv.Itoa(n) + ". " + p.text()
		case wyQuote:
			lines[i] = "> " + p.text()
		default:
			lines[i] = p.text()
		}
	}
	return strings.Join(lines, "\n")
}

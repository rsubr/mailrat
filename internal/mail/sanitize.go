package mail

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var allowedTags = map[atom.Atom]bool{
	atom.P: true, atom.Br: true, atom.Div: true, atom.Span: true,
	atom.B: true, atom.Strong: true, atom.I: true, atom.Em: true, atom.U: true,
	atom.S: true, atom.Strike: true, atom.Ul: true, atom.Ol: true, atom.Li: true,
	atom.A: true, atom.Blockquote: true, atom.Pre: true, atom.Code: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Hr: true, atom.Img: true,
	atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tfoot: true,
	atom.Tr: true, atom.Td: true, atom.Th: true, atom.Caption: true,
	atom.Colgroup: true, atom.Col: true,
}

// dropWithContent tags are removed along with everything inside them.
var dropWithContent = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Iframe: true, atom.Object: true,
	atom.Embed: true, atom.Form: true, atom.Head: true, atom.Title: true,
	atom.Noscript: true, atom.Template: true, atom.Applet: true, atom.Svg: true,
	atom.Math: true, atom.Textarea: true, atom.Select: true, atom.Button: true,
	atom.Frame: true, atom.Frameset: true, atom.Xmp: true, atom.Plaintext: true,
}

var allowedStyleProps = map[string]bool{
	"color": true, "background-color": true, "font-size": true, "font-family": true,
	"font-weight": true, "font-style": true, "text-decoration": true, "text-align": true,
}

// SanitizeHTML returns html reduced to a strict allowlist of tags, attributes,
// URL schemes and CSS properties, suitable for outgoing mail.
func SanitizeHTML(src string) string {
	ctx := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(src), ctx)
	if err != nil {
		return ""
	}
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	for _, n := range nodes {
		sanitizeInto(root, n)
	}
	var b strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}

func sanitizeInto(parent, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
	case html.ElementNode:
		if dropWithContent[n.DataAtom] {
			return
		}
		target := parent
		if allowedTags[n.DataAtom] {
			attrs := sanitizeAttrs(n)
			if n.DataAtom == atom.Img && !hasAttr(attrs, "src") {
				return
			}
			el := &html.Node{Type: html.ElementNode, DataAtom: n.DataAtom, Data: n.Data, Attr: attrs}
			parent.AppendChild(el)
			target = el
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			sanitizeInto(target, c)
		}
	}
}

func hasAttr(attrs []html.Attribute, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

func sanitizeAttrs(n *html.Node) []html.Attribute {
	var out []html.Attribute
	for _, a := range n.Attr {
		if a.Namespace != "" {
			continue
		}
		key := strings.ToLower(a.Key)
		val := a.Val
		switch key {
		case "href":
			if n.DataAtom == atom.A && safeURL(val, "http:", "https:", "mailto:") {
				out = append(out, html.Attribute{Key: key, Val: val})
			}
		case "src":
			if n.DataAtom == atom.Img && safeURL(val, "https:", "cid:", "data:image/") {
				out = append(out, html.Attribute{Key: key, Val: val})
			}
		case "alt":
			if n.DataAtom == atom.Img {
				out = append(out, html.Attribute{Key: key, Val: val})
			}
		case "dir":
			if v := strings.ToLower(val); v == "ltr" || v == "rtl" || v == "auto" {
				out = append(out, html.Attribute{Key: key, Val: v})
			}
		case "class":
			if val == "gmail_quote" || val == "gmail_attr" || val == "gmail_signature" {
				out = append(out, html.Attribute{Key: key, Val: val})
			}
		case "style":
			if s := sanitizeStyle(val); s != "" {
				out = append(out, html.Attribute{Key: key, Val: s})
			}
		}
	}
	return out
}

// safeURL reports whether v starts with one of the allowed prefixes after
// stripping whitespace/control characters browsers ignore inside schemes.
func safeURL(v string, prefixes ...string) bool {
	var b strings.Builder
	for _, r := range v {
		if r <= ' ' || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.ToLower(b.String())
	if strings.HasPrefix(s, "data:image/svg") {
		return false
	}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func sanitizeStyle(v string) string {
	var parts []string
	for _, decl := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		val = strings.TrimSpace(val)
		lv := strings.ToLower(val)
		if !allowedStyleProps[k] || val == "" ||
			strings.ContainsAny(val, "\\<>{}@\"") ||
			strings.Contains(lv, "url(") || strings.Contains(lv, "expression") ||
			strings.Contains(lv, "javascript") || strings.Contains(val, "/*") {
			continue
		}
		parts = append(parts, k+": "+val)
	}
	return strings.Join(parts, "; ")
}

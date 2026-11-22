package ui

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var blankLines = regexp.MustCompile(`\n{3,}`)

// htmlToText renders an HTML (or plain-text) mail body as readable plain text.
func htmlToText(src string) string {
	if !strings.Contains(src, "<") {
		return strings.TrimSpace(src)
	}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return strings.TrimSpace(tagRe.ReplaceAllString(src, ""))
	}
	var b strings.Builder
	var walk func(n *html.Node, quote int)
	nl := func() {
		s := b.String()
		if s != "" && !strings.HasSuffix(s, "\n") {
			b.WriteByte('\n')
		}
	}
	walk = func(n *html.Node, quote int) {
		switch n.Type {
		case html.TextNode:
			t := strings.Join(strings.Fields(n.Data), " ")
			if t == "" {
				if strings.ContainsAny(n.Data, " \t\n") && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteByte(' ')
				}
				return
			}
			if strings.HasPrefix(n.Data, " ") && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") && !strings.HasSuffix(b.String(), " ") {
				b.WriteByte(' ')
			}
			if s := b.String(); strings.HasSuffix(s, "\n") && quote > 0 {
				b.WriteString(strings.Repeat("> ", quote))
			}
			b.WriteString(t)
			if strings.HasSuffix(n.Data, " ") {
				b.WriteByte(' ')
			}
			return
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "head", "title":
				return
			case "br":
				b.WriteByte('\n')
				return
			case "blockquote":
				quote++
			}
		}
		elem := n.Type == html.ElementNode
		block := elem && isBlock(n.Data)
		para := elem && isParagraph(n.Data)
		if block {
			nl()
			if para && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
				b.WriteByte('\n')
			}
		}
		if elem && n.Data == "li" {
			b.WriteString("• ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, quote)
		}
		if block {
			nl()
			if para {
				b.WriteByte('\n')
			}
		}
	}
	walk(doc, 0)
	return strings.TrimSpace(blankLines.ReplaceAllString(b.String(), "\n\n"))
}

func isBlock(tag string) bool {
	switch tag {
	case "p", "div", "tr", "table", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "hr", "section", "article", "header", "footer":
		return true
	}
	return false
}

func isParagraph(tag string) bool {
	switch tag {
	case "p", "ul", "ol", "table", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6", "pre":
		return true
	}
	return false
}

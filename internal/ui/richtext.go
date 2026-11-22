package ui

import (
	"html"
	"regexp"
	"strings"
)

// The compose body is edited as plain text with light BBCode-style markup, because
// Fyne has no editable rich-text widget. markupToHTML turns it into the HTML sent
// as the text/html part:
//
//	[b] [i] [u]            bold, italic, underline
//	[font=Name] [size=S]   font family; size small|normal|large|huge
//	[color=#rrggbb]        text colour
//	[align=center]         left|center|right|justify
//	[indent] [quote]       indented block, quoted block
//	[url=https://...]      link (http, https, mailto only)
//	"- " / "1. " / "> "    bullet item, numbered item, quoted line (line prefixes)
//
// Anything else, including unknown or malformed tags, is sent as literal text.

var markupTagRe = regexp.MustCompile(`^\[(/?)(b|i|u|font|size|color|align|indent|quote|url)(?:=([^\]]*))?\]`)

var (
	fontValueRe  = regexp.MustCompile(`^[A-Za-z0-9 -]{1,40}$`)
	colorValueRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6})$`)
	numItemRe    = regexp.MustCompile(`^\d+\. `)
	// stripMarkupRe matches every tag markupToHTML understands, for "remove formatting".
	stripMarkupRe = regexp.MustCompile(`\[/?(b|i|u|font|size|color|align|indent|quote|url)(=[^\]]*)?\]`)
)

var sizePx = map[string]string{"small": "10px", "normal": "13px", "large": "18px", "huge": "32px"}

const quoteStyle = `margin:0 0 0 .8ex;border-left:1px solid #ccc;padding-left:1ex`

type mtag struct {
	name  string
	open  string
	close string
	block bool
}

// tagHTML returns the HTML for an opening tag, or false when the value is not allowed.
func tagHTML(name, val string) (mtag, bool) {
	t := mtag{name: name}
	switch name {
	case "b", "i", "u":
		t.open, t.close = "<"+name+">", "</"+name+">"
	case "font":
		if !fontValueRe.MatchString(val) {
			return t, false
		}
		t.open, t.close = `<span style="font-family:'`+val+`'">`, "</span>"
	case "size":
		px, ok := sizePx[val]
		if !ok {
			return t, false
		}
		t.open, t.close = `<span style="font-size:`+px+`">`, "</span>"
	case "color":
		if !colorValueRe.MatchString(val) {
			return t, false
		}
		t.open, t.close = `<span style="color:`+val+`">`, "</span>"
	case "url":
		l := strings.ToLower(val)
		if !(strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:")) {
			return t, false
		}
		t.open, t.close = `<a href="`+html.EscapeString(val)+`">`, "</a>"
	case "align":
		switch val {
		case "left", "center", "right", "justify":
		default:
			return t, false
		}
		t.open, t.close, t.block = `<div style="text-align:`+val+`">`, "</div>", true
	case "indent":
		t.open, t.close, t.block = `<div style="margin-left:40px">`, "</div>", true
	case "quote":
		t.open, t.close, t.block = `<blockquote style="`+quoteStyle+`">`, "</blockquote>", true
	}
	return t, true
}

// markupToHTML converts compose markup to an HTML fragment (one <div> per line).
func markupToHTML(src string) string {
	var out strings.Builder
	var stack []mtag // tags still open; they carry over line breaks
	list := ""       // "ul", "ol" or "" while inside a list
	inQuote := false
	closeRun := func() {
		if list != "" {
			out.WriteString("</" + list + ">")
			list = ""
		}
		if inQuote {
			out.WriteString("</blockquote>")
			inQuote = false
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		kind, quoted := "", false
		switch {
		case strings.HasPrefix(line, "- "):
			kind, line = "ul", line[2:]
		case numItemRe.MatchString(line):
			kind, line = "ol", line[strings.Index(line, " ")+1:]
		case strings.HasPrefix(line, "> "):
			quoted, line = true, line[2:]
		}
		if kind != list || (quoted != inQuote) {
			closeRun()
			if kind != "" {
				out.WriteString("<" + kind + ">")
				list = kind
			}
			if quoted {
				out.WriteString(`<blockquote style="` + quoteStyle + `">`)
				inQuote = true
			}
		}

		var inline strings.Builder
		var blocks []mtag // block wrappers active at any point of this line
		for _, t := range stack {
			if t.block {
				blocks = append(blocks, t)
			} else {
				inline.WriteString(t.open)
			}
		}
		var text strings.Builder
		flush := func() {
			inline.WriteString(html.EscapeString(text.String()))
			text.Reset()
		}
		for i := 0; i < len(line); {
			if line[i] == '[' {
				if m := markupTagRe.FindStringSubmatch(line[i:]); m != nil {
					if m[1] == "" {
						if t, ok := tagHTML(m[2], m[3]); ok {
							flush()
							stack = append(stack, t)
							if t.block {
								blocks = append(blocks, t)
							} else {
								inline.WriteString(t.open)
							}
							i += len(m[0])
							continue
						}
					} else if j := lastOpen(stack, m[2]); j >= 0 {
						flush()
						// Close everything above the match, drop it, reopen the rest.
						for k := len(stack) - 1; k >= j; k-- {
							if !stack[k].block {
								inline.WriteString(stack[k].close)
							}
						}
						stack = append(stack[:j], stack[j+1:]...)
						for _, t := range stack[j:] {
							if !t.block {
								inline.WriteString(t.open)
							}
						}
						i += len(m[0])
						continue
					}
				}
			}
			text.WriteByte(line[i])
			i++
		}
		flush()
		for k := len(stack) - 1; k >= 0; k-- {
			if !stack[k].block {
				inline.WriteString(stack[k].close)
			}
		}
		body := inline.String()
		if kind != "" {
			body = "<li>" + body + "</li>"
		} else if body == "" {
			body = "<div><br></div>"
		} else {
			body = "<div>" + body + "</div>"
		}
		for _, t := range blocks {
			out.WriteString(t.open)
		}
		out.WriteString(body)
		for k := len(blocks) - 1; k >= 0; k-- {
			out.WriteString(blocks[k].close)
		}
	}
	closeRun()
	return out.String()
}

func lastOpen(stack []mtag, name string) int {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].name == name {
			return i
		}
	}
	return -1
}

// stripMarkup removes all formatting tags, leaving the text.
func stripMarkup(s string) string { return stripMarkupRe.ReplaceAllString(s, "") }

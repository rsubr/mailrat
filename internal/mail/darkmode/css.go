package darkmode

import (
	"sort"
	"strings"

	"golang.org/x/net/html"
)

type decl struct {
	prop, val string
	imp       bool
}

// splitDecls parses a declaration list ("a:b;c:d !important"). Separators
// inside parentheses or quotes are respected.
func splitDecls(s string) []decl {
	var out []decl
	depth, start := 0, 0
	var q byte
	flush := func(end int) {
		part := s[start:end]
		start = end + 1
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			return
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		if k == "" || v == "" || !validProp(k) {
			return
		}
		if strings.Contains(v, ";") {
			return // quoted/parenthesised ';' is ambiguous for naive consumers
		}
		d := decl{prop: k, val: v}
		if lv := strings.ToLower(v); strings.HasSuffix(lv, "!important") {
			d.imp = true
			d.val = strings.TrimSpace(v[:len(v)-len("!important")])
		}
		out = append(out, d)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case c == ';' && depth == 0:
			flush(i)
		}
	}
	flush(len(s))
	return out
}

func validProp(k string) bool {
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

type simpleSel struct {
	tag, id string
	classes []string
	spec    int
}

type rule struct {
	sel   simpleSel
	decls []decl
	order int
}

type sheet struct {
	byID, byClass, byTag map[string][]*rule
	universal            []*rule
	n                    int
}

const maxRules = 5000

func newSheet() *sheet {
	return &sheet{byID: map[string][]*rule{}, byClass: map[string][]*rule{}, byTag: map[string][]*rule{}}
}

// parseSimple accepts tag, .class, #id and combinations (tag.a.b#id, *).
// Pseudo-classes of interactive state are reported as ok=false, silent=true.
func parseSimple(s string) (sel simpleSel, ok, silent bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " >+~[]()\t\n\r,") {
		return sel, false, false
	}
	if strings.Contains(s, ":") {
		return sel, false, true
	}
	cur, kind := "", byte('t')
	flush := func() bool {
		switch kind {
		case 't':
			if cur != "*" {
				sel.tag = cur
				if cur != "" {
					sel.spec++
				}
			}
		case '.':
			if cur == "" {
				return false
			}
			sel.classes = append(sel.classes, cur)
			sel.spec += 10
		case '#':
			if cur == "" {
				return false
			}
			sel.id = cur
			sel.spec += 100
		}
		return true
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '.' || c == '#' {
			if !flush() {
				return sel, false, false
			}
			cur, kind = "", c
		} else {
			cur += string(c)
		}
	}
	if !flush() {
		return sel, false, false
	}
	sel.tag = strings.ToLower(sel.tag)
	return sel, true, false
}

// add parses one <style> block into the sheet. unsupported collects
// constructs whose colours cannot be resolved.
func (sh *sheet) add(src string, unsupported func(string)) {
	// strip comments and legacy comment wrappers
	for {
		i := strings.Index(src, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(src[i+2:], "*/")
		if j < 0 {
			src = src[:i]
			break
		}
		src = src[:i] + " " + src[i+2+j+2:]
	}
	src = strings.NewReplacer("<!--", " ", "-->", " ").Replace(src)
	for len(src) > 0 {
		src = strings.TrimSpace(src)
		if src == "" {
			return
		}
		ob, sc := strings.IndexByte(src, '{'), strings.IndexByte(src, ';')
		if strings.HasPrefix(src, "@") && sc >= 0 && (ob < 0 || sc < ob) {
			if strings.HasPrefix(src, "@import") {
				unsupported("@import")
			}
			src = src[sc+1:]
			continue
		}
		if ob < 0 {
			return
		}
		prelude := strings.TrimSpace(src[:ob])
		depth, end := 1, -1
		for i := ob + 1; i < len(src); i++ {
			if src[i] == '{' {
				depth++
			} else if src[i] == '}' {
				if depth--; depth == 0 {
					end = i
					break
				}
			}
		}
		body := ""
		if end < 0 {
			body, src = src[ob+1:], ""
		} else {
			body, src = src[ob+1:end], src[end+1:]
		}
		if strings.HasPrefix(prelude, "@") {
			if strings.HasPrefix(prelude, "@media") || strings.HasPrefix(prelude, "@supports") {
				unsupported(strings.Fields(prelude)[0])
			}
			continue
		}
		ds := splitDecls(body)
		for _, s := range strings.Split(prelude, ",") {
			sel, ok, silent := parseSimple(s)
			if !ok {
				if !silent && strings.TrimSpace(s) != "" {
					unsupported("selector " + strings.TrimSpace(s))
				}
				continue
			}
			if sh.n >= maxRules {
				unsupported("too many rules")
				return
			}
			r := &rule{sel: sel, decls: ds, order: sh.n}
			sh.n++
			switch {
			case sel.id != "":
				sh.byID[sel.id] = append(sh.byID[sel.id], r)
			case len(sel.classes) > 0:
				sh.byClass[sel.classes[0]] = append(sh.byClass[sel.classes[0]], r)
			case sel.tag != "":
				sh.byTag[sel.tag] = append(sh.byTag[sel.tag], r)
			default:
				sh.universal = append(sh.universal, r)
			}
		}
	}
}

func (r *rule) matches(n *html.Node, classes map[string]bool) bool {
	if r.sel.tag != "" && r.sel.tag != n.Data {
		return false
	}
	if r.sel.id != "" && r.sel.id != attrVal(n, "id") {
		return false
	}
	for _, c := range r.sel.classes {
		if !classes[c] {
			return false
		}
	}
	return true
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// cascade returns the element's winning declarations: matching <style> rules
// by (specificity, order), then the inline style; !important beats the rest.
// Declarations keep first-seen property order for stable output.
func (sh *sheet) cascade(n *html.Node, inline string) []decl {
	var cands []*rule
	if sh != nil && sh.n > 0 {
		classes := map[string]bool{}
		for _, c := range strings.Fields(attrVal(n, "class")) {
			classes[c] = true
		}
		cands = append(cands, sh.universal...)
		cands = append(cands, sh.byTag[n.Data]...)
		for c := range classes {
			cands = append(cands, sh.byClass[c]...)
		}
		if id := attrVal(n, "id"); id != "" {
			cands = append(cands, sh.byID[id]...)
		}
		out := cands[:0]
		for _, r := range cands {
			if r.matches(n, classes) {
				out = append(out, r)
			}
		}
		cands = out
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].sel.spec != cands[j].sel.spec {
				return cands[i].sel.spec < cands[j].sel.spec
			}
			return cands[i].order < cands[j].order
		})
	}
	var out []decl
	idx := map[string]int{}
	put := func(d decl) {
		if i, ok := idx[d.prop]; ok {
			if out[i].imp && !d.imp {
				return
			}
			out[i] = d
			return
		}
		idx[d.prop] = len(out)
		out = append(out, d)
	}
	for _, r := range cands {
		for _, d := range r.decls {
			put(d)
		}
	}
	for _, d := range splitDecls(inline) {
		put(d)
	}
	return out
}

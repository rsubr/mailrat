package ui

import (
	"reflect"
	"strings"
)

// sigMarker is the Gmail-style separator line that starts a signature block.
const sigMarker = "-- "

// quoteOpen starts every reply/forward quote block built by quoteHTML/forwardHTML;
// compose uses it to tell a fresh reply/forward seed from a reopened draft body.
const quoteOpen = `<div class="gmail_quote">`

// sigBlock is the signature as inserted into the editor: "-- " line, then sig.
func sigBlock(sig string) string {
	if strings.TrimSpace(sig) == "" {
		return ""
	}
	return `<div class="gmail_signature"><p>` + sigMarker + `</p>` + sig + `</div>`
}

// composeHTML builds the initial compose document: an empty line for the user's
// text, then the signature (if any), then the quoted original (if any).
func composeHTML(sig, quote string) string {
	return "<p></p>" + sigBlock(sig) + quote
}

// sigParas is the paragraph run sigBlock(sig) becomes inside the editor.
func sigParas(sig string) []wyParagraph {
	if sigBlock(sig) == "" {
		return nil
	}
	paras := parseHTML(sigBlock(sig)).Paras
	// The HTML parser trims trailing blanks; the "-- " separator must keep its space.
	paras[0].Runs = []wyRun{{Text: sigMarker}}
	return paras
}

// swapSignature replaces the signature block oldSig with newSig. tail is the
// number of trailing paragraphs that belong to the quote: with no old block the
// new one goes just above them. It reports false (doc unchanged) when oldSig is
// set but no longer found intact, e.g. because the user edited it.
func swapSignature(d wyDoc, oldSig, newSig string, tail int) (wyDoc, bool) {
	oldP, newP := sigParas(oldSig), sigParas(newSig)
	at := len(d.Paras) - tail
	if at < 0 {
		at = 0
	}
	end := at
	if len(oldP) > 0 {
		at = -1
		for i := 0; i+len(oldP) <= len(d.Paras); i++ {
			if reflect.DeepEqual(d.Paras[i:i+len(oldP)], oldP) {
				at = i
				break
			}
		}
		if at < 0 {
			return d, false
		}
		end = at + len(oldP)
	}
	paras := append([]wyParagraph(nil), d.Paras[:at]...)
	paras = append(paras, newP...)
	paras = append(paras, d.Paras[end:]...)
	return wyDoc{Paras: paras}, true
}

// quoteParas counts the paragraphs the quote block becomes in the editor.
func quoteParas(quote string) int {
	if quote == "" {
		return 0
	}
	return len(parseHTML(quote).Paras)
}

// replaceSignature swaps the signature block in the editor, keeping the caret
// (clamped) and the content around it. It reports whether the swap happened.
func (w *Wysiwyg) replaceSignature(oldSig, newSig string, tail int) bool {
	d, ok := swapSignature(w.ed.doc, oldSig, newSig, tail)
	if !ok {
		return false
	}
	clamp := func(p wyPos) wyPos {
		if p.P >= len(d.Paras) {
			p = wyPos{P: len(d.Paras) - 1}
			p.Off = d.Paras[p.P].length()
		}
		return p
	}
	a, c := clamp(w.ed.anchor), clamp(w.ed.caret)
	w.ed.setDoc(d)
	w.ed.anchor, w.ed.caret = a, c
	w.rebuild()
	w.notifyCursor()
	return true
}

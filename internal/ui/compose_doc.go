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
	out, _, _, ok := swapSig(d, oldSig, newSig, tail)
	return out, ok
}

// swapSig is swapSignature that also reports where the block sits (at) and how
// many paragraphs the document grew by, so positions below it can follow.
func swapSig(d wyDoc, oldSig, newSig string, tail int) (out wyDoc, at, delta int, ok bool) {
	oldP, newP := sigParas(oldSig), sigParas(newSig)
	at = max(len(d.Paras)-tail, 0)
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
			return d, 0, 0, false
		}
		end = at + len(oldP)
	}
	paras := append([]wyParagraph(nil), d.Paras[:at]...)
	paras = append(paras, newP...)
	paras = append(paras, d.Paras[end:]...)
	return wyDoc{Paras: paras}, at, len(newP) - len(oldP), true
}

// quoteParas counts the paragraphs the quote block becomes in the editor.
func quoteParas(quote string) int {
	if quote == "" {
		return 0
	}
	return len(parseHTML(composeHTML("", quote)).Paras) - 1 // minus the user's own line
}

// replaceSignature swaps the signature block in the editor. It arrives
// asynchronously, so it must not cost the user what they already did: the caret
// follows its paragraph and the undo history is kept (updated to match). It
// reports whether the swap happened.
func (w *Wysiwyg) replaceSignature(oldSig, newSig string, tail int) bool {
	d, at, delta, ok := swapSig(w.ed.doc, oldSig, newSig, tail)
	if !ok {
		return false
	}
	oldLen := len(sigParas(oldSig))
	shift := func(p wyPos, at, delta int) wyPos {
		switch {
		case p.P >= at+oldLen:
			p.P += delta
		case p.P >= at && oldLen > 0:
			p = wyPos{at, 0}
		}
		return p
	}
	e := w.ed
	e.anchor, e.caret = shift(e.anchor, at, delta), shift(e.caret, at, delta)
	e.doc = d
	for _, stack := range [][]wySnap{e.undo, e.redo} {
		for i, sn := range stack {
			if sd, a, dl, ok := swapSig(sn.doc, oldSig, newSig, tail); ok {
				stack[i] = wySnap{sd, shift(sn.anchor, a, dl), shift(sn.caret, a, dl)}
			}
		}
	}
	e.anchor, e.caret = e.clamp(e.anchor), e.clamp(e.caret)
	w.rebuild()
	w.notifyCursor()
	return true
}

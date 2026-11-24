package ui

import (
	"fyne.io/fyne/v2"

	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/richclip"
)

// richReader supplies the clipboard's HTML for paste; tests swap in a fake.
var richReader richclip.Reader = richclip.SystemReader()

// wyMaxPasteHTML is the largest clipboard HTML pasted as rich text; bigger pastes as plain text.
const wyMaxPasteHTML = 2 << 20

// pasteDoc turns clipboard HTML into pasteable paragraphs. The HTML goes through the
// mail sanitizer first, then the editor's importer keeps what the model can show
// (paragraphs, lists, quotes, bold, italic, underline, strike, links) and drops the
// rest, keeping its text. Colours and sizes are dropped so pasted text takes on the
// message's own look. ok is false when nothing visible is left.
func pasteDoc(html string) (d wyDoc, ok bool) {
	if html == "" || len(html) > wyMaxPasteHTML {
		return d, false
	}
	d = parseHTML(mail.SanitizeHTML(html))
	if len(d.Paras) > 1 && d.Paras[0].length() == 0 && d.Paras[1].Fold {
		d.Paras = d.Paras[1:] // the importer's placeholder above a quote block
	}
	for i := range d.Paras {
		p := &d.Paras[i]
		p.Fold, p.Sig = false, false
		for j := range p.Runs {
			p.Runs[j].Color, p.Runs[j].Size = "", 0
		}
		p.Runs = normRuns(p.Runs)
	}
	return d, d.plainText() != ""
}

// insertDoc inserts d at the caret, replacing the selection, as one undo step. The
// first paragraph continues the caret's paragraph and the last one is followed by
// whatever came after the caret.
func (e *wyEditor) insertDoc(d wyDoc) {
	if len(d.Paras) == 0 {
		return
	}
	e.begin("paste")
	e.deleteSel()
	at := e.caret
	cur := e.doc.Paras[at.P]
	head, tail := cutRuns(cur.Runs, 0, at.Off), cutRuns(cur.Runs, at.Off, cur.length())
	paras := d.clone().Paras
	paras[0].Fold, paras[0].Sig = cur.Fold, cur.Sig
	if len(head) > 0 || cur.length() > 0 || len(paras) == 1 {
		paras[0].Kind = cur.Kind
	}
	paras[0].Runs = append(head, paras[0].Runs...)
	last := &paras[len(paras)-1]
	end := last.length() // the caret ends after the pasted text, before the old tail
	last.Runs = normRuns(append(last.Runs, tail...))
	paras[0].Runs = normRuns(paras[0].Runs)
	rest := append(paras, e.doc.Paras[at.P+1:]...)
	e.doc.Paras = append(e.doc.Paras[:at.P], rest...)
	e.caret = wyPos{at.P + len(paras) - 1, end}
	e.anchor = e.caret
	e.pending = nil
}

// paste inserts the clipboard at the caret: rich when it holds HTML and plain is
// false, otherwise (and whenever the HTML yields nothing) as plain text.
func (w *Wysiwyg) paste(clip fyne.Clipboard, plain bool) {
	if !plain {
		if h, _, err := richReader.ReadHTML(); err == nil {
			if d, ok := pasteDoc(h); ok {
				w.ed.insertDoc(d)
				w.after(true)
				return
			}
		}
	}
	w.ed.insertText(clip.Content())
	w.after(true)
}

package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/mailrat/mailrat/internal/desktop"
)

func TestComposeHTMLOrder(t *testing.T) {
	m := desktop.MessageItem{FromAddr: "Ann <ann@x.com>", Body: "<p>original</p>", Date: 1700000000000}
	d, _ := swapSignature(parseHTML(composeHTML("", quoteHTML(m))), "", "<p>SIGTEXT</p>", quoteParas(quoteHTML(m)))
	got := d.plainText()
	user, sig, quote := strings.Index(got, "-- "), strings.Index(got, "SIGTEXT"), strings.Index(got, "original")
	if !strings.HasPrefix(got, "\n") || user < 0 || !(user < sig && sig < quote) {
		t.Fatalf("order wrong: %q", got)
	}
	if !strings.Contains(got, "wrote:") || !strings.Contains(got, "Ann <ann@x.com>") {
		t.Fatalf("attribution missing: %q", got)
	}
}

func TestComposeHTMLNoSignature(t *testing.T) {
	if got := composeHTML("  ", ""); got != "<p></p>" {
		t.Fatalf("got %q", got)
	}
	if got := composeHTML("", quoteOpen+"q</div>"); strings.Contains(got, "gmail_signature") {
		t.Fatalf("unexpected signature: %q", got)
	}
}

func TestForwardHTML(t *testing.T) {
	m := desktop.MessageItem{FromAddr: "Ann <a@x.com>", ToAddr: "me@x.com", Subject: "S & T", Body: "line1\nline2", Date: 1700000000000}
	h := forwardHTML(m)
	for _, want := range []string{"---------- Forwarded message ---------", "From: Ann &lt;a@x.com&gt;", "Date: ", "Subject: S &amp; T", "To: me@x.com", "line1<br>line2"} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in %s", want, h)
		}
	}
	if !strings.HasPrefix(h, quoteOpen) {
		t.Errorf("not a quote block: %s", h)
	}
}

func TestQuoteHTMLSanitizesOriginal(t *testing.T) {
	m := desktop.MessageItem{FromAddr: "a@x.com", Body: `<p>hi</p><script>evil()</script>`}
	if h := quoteHTML(m); strings.Contains(h, "script") || strings.Contains(h, "evil") {
		t.Fatalf("not sanitized: %s", h)
	}
}

func TestSwapSignature(t *testing.T) {
	quote := quoteOpen + "<blockquote>q</blockquote></div>"
	tail := quoteParas(quote)
	d := parseHTML(composeHTML("", quote))
	d, ok := swapSignature(d, "", "<p>A</p>", tail)
	if !ok {
		t.Fatal("insert failed")
	}
	if got := d.plainText(); !strings.Contains(got, "-- \nA\n") || strings.Index(got, "A") > strings.Index(got, "q") {
		t.Fatalf("insert: %q", got)
	}
	d, ok = swapSignature(d, "<p>A</p>", "<p>B</p>", tail)
	if got := d.plainText(); !ok || strings.Contains(got, "A") || !strings.Contains(got, "B") || strings.Count(got, "-- ") != 1 {
		t.Fatalf("swap: %q", got)
	}
	d, ok = swapSignature(d, "<p>B</p>", "", tail)
	if got := d.plainText(); !ok || strings.Contains(got, "B") || strings.Contains(got, "-- ") || !strings.Contains(got, "q") {
		t.Fatalf("remove: %q", got)
	}
	// An edited signature is left alone.
	if _, ok := swapSignature(d, "<p>gone</p>", "<p>C</p>", tail); ok {
		t.Fatal("swapped a signature that is not there")
	}
}

// composeEditor finds the Wysiwyg in the newest compose window.
func composeEditor(t *testing.T, u *UI) *Wysiwyg {
	t.Helper()
	ws := u.app.Driver().AllWindows()
	var find func(o fyne.CanvasObject) *Wysiwyg
	find = func(o fyne.CanvasObject) *Wysiwyg {
		if w, ok := o.(*Wysiwyg); ok {
			return w
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, ch := range c.Objects {
				if w := find(ch); w != nil {
					return w
				}
			}
		}
		return nil
	}
	if w := find(ws[len(ws)-1].Content()); w != nil {
		return w
	}
	t.Fatal("no editor in compose window")
	return nil
}

func closeComposeWindows(t *testing.T, u *UI) {
	t.Cleanup(func() {
		fyne.DoAndWait(func() {
			for _, w := range append([]fyne.Window(nil), u.app.Driver().AllWindows()...) {
				if w != u.win {
					w.Close()
				}
			}
		})
	})
}

func TestComposeReplyInsertsSignatureAboveQuote(t *testing.T) {
	old := draftInterval
	draftInterval = 20 * time.Millisecond
	t.Cleanup(func() { draftInterval = old })
	u, fb, _ := testUI(t)
	fb.sig = "<p>Best, Me</p>"
	u.accounts = []desktop.AccountItem{{ID: "a1", Email: "a@x.com"}}
	closeComposeWindows(t, u)
	u.compose(replyDraft(desktop.MessageItem{AccountID: "a1", FromAddr: "Ann <ann@x.com>", Subject: "Hi", Body: "<p>original</p>"}, false))
	ed := composeEditor(t, u)
	got := ed.PlainText()
	if !strings.HasPrefix(got, "\n-- \nBest, Me\n") || strings.Index(got, "Best") > strings.Index(got, "original") {
		t.Fatalf("document: %q", got)
	}
	if !strings.Contains(ed.HTML(), "<p>--\u00a0</p>") {
		t.Fatalf("separator lost in HTML: %q", ed.HTML())
	}
	if ed.ed.caret != (wyPos{}) {
		t.Fatalf("caret not at top: %+v", ed.ed.caret)
	}
	time.Sleep(150 * time.Millisecond)
	if len(fb.drafts) != 0 {
		t.Fatalf("signature insertion triggered %d autosaves", len(fb.drafts))
	}
}

func TestComposeNewWithoutSignatureAndLegacyDraft(t *testing.T) {
	u, _, _ := testUI(t)
	u.accounts = []desktop.AccountItem{{ID: "a1", Email: "a@x.com"}}
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1"})
	if got := composeEditor(t, u).PlainText(); strings.Contains(got, "-- ") {
		t.Fatalf("no signature expected: %q", got)
	}
	u.compose(desktop.OutboundMsg{AccountID: "a1", Body: "[b]old[/b] draft"})
	if got := composeEditor(t, u).HTML(); !strings.Contains(got, "<b>old</b> draft") {
		t.Fatalf("legacy draft: %q", got)
	}
}

func TestEditDraftOpensPrefilledAndUpdatesSameDraft(t *testing.T) {
	old := draftInterval
	draftInterval = 20 * time.Millisecond
	t.Cleanup(func() { draftInterval = old })
	u, fb, _ := testUI(t)
	fb.sig = "<p>SIG</p>"
	closeComposeWindows(t, u)
	fb.draftMsg = desktop.OutboundMsg{AccountID: "a2", To: "x@y.com", Cc: "c@y.com", Subject: "Draft S",
		Body: "<p>hello draft</p>", InReplyTo: "<irt@y>", ThreadID: "t9", DraftID: "local-7"}
	u.selected = &desktop.ThreadItem{ID: "t9", AccountID: "a2", LabelIDs: []string{"DRAFT"}}
	u.messages = []desktop.MessageItem{{ID: "m9", AccountID: "a2"}}
	u.editDraft()
	if len(fb.opened) != 1 || fb.opened[0] != "m9" {
		t.Fatalf("opened %v", fb.opened)
	}
	ed := composeEditor(t, u)
	if got := ed.HTML(); !strings.Contains(got, "hello draft") || strings.Contains(got, "SIG") {
		t.Fatalf("body %q", got)
	}
	ed.SetHTML("<p>edited</p>")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(fb.savedIDs) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(fb.savedIDs) == 0 || fb.savedIDs[0] != "local-7" || fb.drafts[0].InReplyTo != "<irt@y>" {
		t.Fatalf("autosave ids %v drafts %+v", fb.savedIDs, fb.drafts)
	}
}

func TestReplyBarOffersEditDraft(t *testing.T) {
	u, _, _ := testUI(t)
	u.selected = &desktop.ThreadItem{LabelIDs: []string{"DRAFT"}}
	if got := buttonLabels(u.replyBar()); len(got) != 1 || got[0] != "Edit draft" {
		t.Fatalf("got %v", got)
	}
}

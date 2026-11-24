package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

func qcMsg() desktop.MessageItem {
	return desktop.MessageItem{ID: "m1", ThreadID: "t1", AccountID: "a1", FromAddr: "Ann <ann@x.com>", ToAddr: "me@x.com",
		Subject: "Hi", Date: msAt(3), Body: "<p>original text</p>"}
}

// qcOpen opens a reply (or forward) with a signature and returns its editor.
func qcOpen(t *testing.T, forward bool) (*UI, *fakeBackend, fyne.Window, *Wysiwyg) {
	t.Helper()
	freezeNow(t, goldenNow)
	u, fb := newTestUI(t, withDark())
	fb.sig = "<p>Best, Neil</p>"
	closeComposeWindows(t, u)
	if forward {
		u.compose(forwardDraft(qcMsg()))
	} else {
		u.compose(replyDraft(qcMsg(), false))
	}
	return u, fb, compflowWin(u), composeEditor(t, u)
}

// qcHidden reports whether no laid-out line belongs to the quote.
func qcHidden(ed *Wysiwyg) bool {
	f := ed.ed.doc.foldAt()
	for _, l := range ed.lines {
		if l.p >= f {
			return false
		}
	}
	return f > 0
}

func TestQuoteCollapsedByDefault(t *testing.T) {
	for _, fwd := range []bool{false, true} {
		_, _, _, ed := qcOpen(t, fwd)
		if !ed.QuoteCollapsed() || !qcHidden(ed) || !ed.more.Visible() || ed.more.Text != "…" || ed.more.tip != "Show trimmed content" {
			t.Fatalf("forward=%v: not collapsed (collapsed=%v hidden=%v)", fwd, ed.QuoteCollapsed(), qcHidden(ed))
		}
		if !strings.Contains(ed.HTML(), "original text") || !strings.Contains(ed.HTML(), `class="gmail_quote"`) {
			t.Fatalf("forward=%v: collapsed quote missing from HTML: %s", fwd, ed.HTML())
		}
	}
}

func TestQuoteToggleExpandsAndCollapses(t *testing.T) {
	_, _, _, ed := qcOpen(t, false)
	before := ed.HTML()
	ed.more.OnTapped()
	if ed.QuoteCollapsed() || qcHidden(ed) || ed.more.tip != "Hide trimmed content" {
		t.Fatal("tap should expand")
	}
	if ed.HTML() != before {
		t.Fatal("expanding must not change the HTML")
	}
	ed.more.OnTapped()
	if !ed.QuoteCollapsed() || !qcHidden(ed) || ed.HTML() != before {
		t.Fatal("second tap should collapse again")
	}
}

func TestQuoteToggleKeyboard(t *testing.T) {
	u, _, w, ed := qcOpen(t, false)
	w.Canvas().Focus(ed.more)
	if w.Canvas().Focused() != ed.more {
		t.Fatal("ellipsis must be focusable")
	}
	ed.more.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	if ed.QuoteCollapsed() {
		t.Fatal("Space should expand")
	}
	ed.more.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !ed.QuoteCollapsed() {
		t.Fatal("Enter should collapse")
	}
	_ = u
}

func TestQuoteSentWhileCollapsed(t *testing.T) {
	_, fb, w, ed := qcOpen(t, false)
	ed.TypedRune('h')
	if !ed.QuoteCollapsed() {
		t.Fatal("typing must not expand")
	}
	compflowButton(t, w.Content(), "Send").OnTapped()
	if len(fb.sent) != 1 {
		t.Fatalf("sent %d", len(fb.sent))
	}
	b := fb.sent[0].Body
	if !strings.Contains(b, "original text") || !strings.Contains(b, "wrote:") || strings.Index(b, "Best, Neil") > strings.Index(b, "original text") {
		t.Fatalf("quote missing or above signature: %s", b)
	}
}

func TestQuoteInAutosavedDraftAndReopenCollapsed(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _, ed := qcOpen(t, true)
	ed.TypedRune('x')
	tick()
	compflowUntil(t, "autosave", func() bool { return fb.draftCount() == 1 })
	drafts, _ := fb.savedDrafts()
	if !strings.Contains(drafts[0].Body, "Forwarded message") || !strings.Contains(drafts[0].Body, "original text") {
		t.Fatalf("draft lost the quote: %s", drafts[0].Body)
	}
	closeComposeWindows(t, u)
	d := drafts[0]
	d.DraftID = "d1"
	u.compose(d)
	re := composeEditor(t, u)
	if !re.QuoteCollapsed() || !qcHidden(re) || re.HTML() != d.Body {
		t.Fatalf("reopened draft not collapsed/identical: collapsed=%v\n%s\n%s", re.QuoteCollapsed(), re.HTML(), d.Body)
	}
}

func TestQuoteSignatureAboveEllipsis(t *testing.T) {
	_, _, _, ed := qcOpen(t, false)
	last := ed.lines[len(ed.lines)-1]
	if got := ed.ed.doc.Paras[last.p].text(); got != "Best, Neil" {
		t.Fatalf("last visible line %q", got)
	}
	if ed.moreY < last.y+last.h {
		t.Fatalf("ellipsis (y=%v) must sit below the signature (ends %v)", ed.moreY, last.y+last.h)
	}
}

func TestQuoteSelectAllAndEditingStayAboveQuote(t *testing.T) {
	_, _, _, ed := qcOpen(t, false)
	ed.TypedShortcut(&fyne.ShortcutSelectAll{})
	if got := ed.ed.selectedText(); strings.Contains(got, "original") || !strings.Contains(got, "Best, Neil") {
		t.Fatalf("collapsed select-all must cover only the visible text: %q", got)
	}
	// Delete at the end of the visible text must not eat into the hidden quote.
	ed.ed.moveTo(ed.ed.docEnd(), false)
	ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
	ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if ed.ed.caret.P >= ed.ed.doc.foldAt() || !strings.Contains(ed.HTML(), "original text") {
		t.Fatal("caret or edit reached the hidden quote")
	}
	// Cut everything visible: the quote stays.
	ed.TypedShortcut(&fyne.ShortcutSelectAll{})
	ed.TypedShortcut(&fyne.ShortcutCut{Clipboard: test.NewClipboard()})
	if !strings.Contains(ed.HTML(), "original text") || !ed.QuoteCollapsed() {
		t.Fatal("cutting the selection must keep the quote")
	}
}

func TestQuoteUndoAcrossToggle(t *testing.T) {
	_, _, _, ed := qcOpen(t, false)
	ed.TypedRune('a')
	ed.more.OnTapped()
	ed.more.OnTapped()
	ed.Undo()
	if strings.Contains(ed.PlainText(), "a\n") && strings.HasPrefix(ed.PlainText(), "a") {
		t.Fatalf("undo did not remove typed text: %q", ed.PlainText())
	}
	if !strings.Contains(ed.HTML(), "original text") || !ed.QuoteCollapsed() {
		t.Fatal("undo lost the quote or its state")
	}
	ed.Redo()
	if !strings.HasPrefix(ed.PlainText(), "a") || !strings.Contains(ed.HTML(), "original text") {
		t.Fatalf("redo broken: %q", ed.PlainText())
	}
}

func TestQuoteCaretInExpandedQuoteMovesOutOnCollapse(t *testing.T) {
	_, _, _, ed := qcOpen(t, false)
	ed.more.OnTapped()
	ed.ed.moveTo(ed.ed.docEnd(), false)
	ed.more.OnTapped()
	if ed.ed.caret.P >= ed.ed.doc.foldAt() {
		t.Fatalf("caret left inside hidden quote: %+v", ed.ed.caret)
	}
}

func TestQuoteParseRoundTrip(t *testing.T) {
	d := parseHTML("<p>hi</p>" + quoteHTML(qcMsg()))
	if d.foldAt() != 1 {
		t.Fatalf("fold at %d", d.foldAt())
	}
	if again := parseHTML(d.toHTML()); again.foldAt() != 1 || again.toHTML() != d.toHTML() {
		t.Fatalf("round trip changed: %s", again.toHTML())
	}
	// Only the first quote block folds; a nested one in the original is content.
	nested := parseHTML("<p>hi</p>" + quoteOpen + "<p>a</p>" + quoteOpen + "<p>b</p></div></div>")
	if nested.foldAt() != 1 {
		t.Fatalf("nested fold at %d", nested.foldAt())
	}
	if n := strings.Count(nested.toHTML(), "gmail_quote"); n != 1 {
		t.Fatalf("expected one quote wrapper, got %d", n)
	}
	// A quote with nothing above it still gets an editable line first.
	if q := parseHTML(quoteHTML(qcMsg())); q.foldAt() != 1 || q.Paras[0].length() != 0 {
		t.Fatalf("leading quote: %+v", q.Paras[0])
	}
}

func TestQuoteGoldens(t *testing.T) {
	for _, c := range []struct {
		name   string
		fwd    bool
		expand bool
	}{{"compose_collapsed_reply", false, false}, {"compose_collapsed_forward", true, false}, {"compose_collapsed_expanded", false, true}} {
		_, _, w, ed := qcOpen(t, c.fwd)
		if c.expand {
			ed.more.OnTapped()
		}
		golden(t, c.name, w.Canvas().Capture())
	}
}

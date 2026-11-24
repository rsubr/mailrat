package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// compflowWalk visits o and everything below it, through containers and widget renderers.
func compflowWalk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(o)
	if c, ok := o.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			compflowWalk(ch, fn)
		}
	} else if w, ok := o.(fyne.Widget); ok {
		for _, ch := range test.WidgetRenderer(w).Objects() {
			compflowWalk(ch, fn)
		}
	}
}

// compflowWin is the newest compose window.
func compflowWin(u *UI) fyne.Window {
	ws := u.app.Driver().AllWindows()
	return ws[len(ws)-1]
}

func compflowButton(t *testing.T, root fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	var found *widget.Button
	compflowWalk(root, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && found == nil && b.Text == text {
			found = b
		}
	})
	if found == nil {
		t.Fatalf("no button %q", text)
	}
	return found
}

func compflowRecipient(t *testing.T, w fyne.Window, name string) *suggestEntry {
	t.Helper()
	var found *suggestEntry
	compflowWalk(w.Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*suggestEntry); ok && e.PlaceHolder == name {
			found = e
		}
	})
	if found == nil {
		t.Fatalf("no recipient entry %q", name)
	}
	return found
}

func compflowSubject(t *testing.T, w fyne.Window) *widget.Entry {
	t.Helper()
	var found *widget.Entry
	compflowWalk(w.Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok && found == nil && e.PlaceHolder == "" {
			found = e
		}
	})
	if found == nil {
		t.Fatal("no subject entry")
	}
	return found
}

func compflowFrom(t *testing.T, w fyne.Window) *widget.Select {
	t.Helper()
	var found *widget.Select
	compflowWalk(w.Content(), func(o fyne.CanvasObject) {
		if s, ok := o.(*widget.Select); ok && found == nil {
			found = s // From comes first; the editor toolbar has more selects
		}
	})
	if found == nil {
		t.Fatal("no From select")
	}
	return found
}

// compflowOverlayTexts is the lower-cased text (fyne capitalises error messages) of labels in the top overlay (dialog), nil when none.
func compflowOverlayTexts(w fyne.Window) (out []string) {
	top := w.Canvas().Overlays().Top()
	if top == nil {
		return nil
	}
	compflowWalk(top, func(o fyne.CanvasObject) {
		if l, ok := o.(*widget.Label); ok {
			out = append(out, strings.ToLower(l.Text))
		}
	})
	return out
}

// compflowUntil polls cond (evaluated on the UI goroutine) until it holds.
func compflowUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var ok bool
		fyne.DoAndWait(func() { ok = cond() })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// compflowTicks replaces the autosave ticker with a manual one and returns the
// tick func. The fyne test driver runs fyne.Do inline on the caller goroutine, so
// a free-running ticker would read editor state while the test edits it. tick
// delivers one tick; the pass runs on the autosave goroutine, so wait on a
// fakeBackend effect (compflowUntil) before editing again. A second tick() is
// only accepted once the first pass finished, which makes "nothing was saved"
// assertions meaningful.
func compflowTicks(t *testing.T) (tick func()) {
	t.Helper()
	ch := make(chan time.Time)
	old := newDraftTicker
	newDraftTicker = func(time.Duration) (<-chan time.Time, func()) { return ch, func() {} }
	t.Cleanup(func() { newDraftTicker = old })
	return func() {
		t.Helper()
		select {
		case ch <- time.Now():
		case <-time.After(3 * time.Second):
			t.Fatal("autosave goroutine not receiving ticks")
		}
	}
}

func compflowWindows(u *UI) int { return len(u.app.Driver().AllWindows()) }

func TestComposeFlowWindowLayout(t *testing.T) {
	freezeNow(t, goldenNow)
	u, _ := newTestUI(t)
	closeComposeWindows(t, u)
	before := compflowWindows(u)
	u.compose(desktop.OutboundMsg{})
	w := compflowWin(u)
	if compflowWindows(u) != before+1 || w.Title() != "New Message" {
		t.Fatalf("windows %d->%d title %q", before, compflowWindows(u), w.Title())
	}
	from := compflowFrom(t, w)
	if from.Selected != "neil@personal.com" || len(from.Options) != 2 {
		t.Fatalf("from %q %v", from.Selected, from.Options)
	}
	for _, n := range []string{"To", "Cc", "Bcc"} {
		compflowRecipient(t, w, n)
	}
	compflowButton(t, w.Content(), "Send")
	compflowButton(t, w.Content(), "Discard")
	if w.Canvas().Focused() != compflowRecipient(t, w, "To") {
		t.Fatal("To should be focused for a new message")
	}
	golden(t, "compose_new", w.Canvas().Capture())
}

func TestComposeFlowReplyPrefillAndSignature(t *testing.T) {
	freezeNow(t, goldenNow)
	u, fb := newTestUI(t, withDark())
	fb.sig = "<p>Best, Neil</p>"
	closeComposeWindows(t, u)
	u.compose(replyDraft(desktop.MessageItem{ID: "m1", ThreadID: "t1", AccountID: "a1", FromAddr: "Ann <ann@x.com>", Subject: "Hi", Date: msAt(3), Body: "<p>original text</p>"}, false))
	w := compflowWin(u)
	if got := compflowRecipient(t, w, "To").Text; got != "Ann <ann@x.com>" {
		t.Fatalf("To %q", got)
	}
	if got := compflowSubject(t, w).Text; got != "Re: Hi" {
		t.Fatalf("subject %q", got)
	}
	got := composeEditor(t, u).PlainText()
	if sig, q := strings.Index(got, "Best, Neil"), strings.Index(got, "original text"); sig < 0 || q < 0 || sig > q {
		t.Fatalf("signature must sit above the quote: %q", got)
	}
	golden(t, "compose_reply_signature", w.Canvas().Capture())

	// Switching From re-fetches the signature and replaces the old one in place.
	fb.sig = "<p>Cheers, Work</p>"
	compflowFrom(t, w).SetSelected("neil@work.com")
	got = composeEditor(t, u).PlainText()
	if strings.Contains(got, "Best, Neil") || strings.Count(got, "Cheers, Work") != 1 ||
		strings.Index(got, "Cheers, Work") > strings.Index(got, "original text") {
		t.Fatalf("signature not swapped above quote: %q", got)
	}

	// An account without a signature drops it, leaving the quote.
	fb.sig = ""
	compflowFrom(t, w).SetSelected("neil@personal.com")
	got = composeEditor(t, u).PlainText()
	if strings.Contains(got, "Cheers") || strings.Contains(got, "-- ") || !strings.Contains(got, "original text") {
		t.Fatalf("signature not removed: %q", got)
	}
}

func TestComposeFlowForwardSignatureAboveForwarded(t *testing.T) {
	u, fb, _ := testUI(t)
	fb.sig = "<p>SIGLINE</p>"
	closeComposeWindows(t, u)
	u.compose(forwardDraft(desktop.MessageItem{AccountID: "a1", FromAddr: "Ann <a@x.com>", ToAddr: "n@x.com", Subject: "Plan", Body: "<p>forwarded body</p>", Date: msAt(2)}))
	got := composeEditor(t, u).PlainText()
	sig, fwd, body := strings.Index(got, "SIGLINE"), strings.Index(got, "Forwarded message"), strings.Index(got, "forwarded body")
	if sig < 0 || fwd < 0 || body < 0 || !(sig < fwd && fwd < body) {
		t.Fatalf("order wrong: %q", got)
	}
	if got := compflowSubject(t, compflowWin(u)).Text; got != "Fwd: Plan" {
		t.Fatalf("subject %q", got)
	}
}

func TestComposeFlowAutocompletePicksContact(t *testing.T) {
	u, fb, _ := testUI(t)
	fb.found = []desktop.ContactItem{{Name: "Ann Lee", Email: "ann@x.com"}, {Email: "anders@x.com"}}
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{})
	w := compflowWin(u)
	to := compflowRecipient(t, w, "To")
	test.Type(to, "an")
	if got := buttonLabels(w.Content()); !compflowHas(got, "Ann Lee <ann@x.com>", "anders@x.com") {
		t.Fatalf("suggestions not shown: %v", got)
	}
	compflowButton(t, w.Content(), "Ann Lee <ann@x.com>").OnTapped()
	if to.Text != "Ann Lee <ann@x.com>, " {
		t.Fatalf("To %q", to.Text)
	}
	if got := buttonLabels(w.Content()); compflowHas(got, "anders@x.com") {
		t.Fatalf("suggestions still shown: %v", got)
	}

	// A second address keeps the finished head and replaces only the typed token.
	test.Type(to, "an")
	to.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	to.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if to.Text != "Ann Lee <ann@x.com>, anders@x.com, " {
		t.Fatalf("To %q", to.Text)
	}
}

func TestComposeFlowAutocompleteNoMatchShowsNothing(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{})
	w := compflowWin(u)
	test.Type(compflowRecipient(t, w, "To"), "zz")
	for _, l := range buttonLabels(w.Content()) {
		if strings.Contains(l, "@") {
			t.Fatalf("unexpected suggestion %q", l)
		}
	}
	if fb.calls[len(fb.calls)-1] != "SearchContacts" {
		t.Fatalf("contacts not searched: %v", fb.calls)
	}
}

func TestComposeFlowSendSuccess(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a2", InReplyTo: "m1", ThreadID: "t1", DraftID: "local-3", Body: "<p>start</p>"})
	w := compflowWin(u)
	before := compflowWindows(u)
	compflowRecipient(t, w, "To").SetText("bob@x.com")
	compflowRecipient(t, w, "Cc").SetText("cc@x.com")
	compflowSubject(t, w).SetText("Hello")
	composeEditor(t, u).SetHTML("<p>the body</p>")
	compflowButton(t, w.Content(), "Send").OnTapped()
	if len(fb.sent) != 1 {
		t.Fatalf("sent %d", len(fb.sent))
	}
	m := fb.sent[0]
	if m.AccountID != "a2" || m.To != "bob@x.com" || m.Cc != "cc@x.com" || m.Subject != "Hello" ||
		m.InReplyTo != "m1" || m.ThreadID != "t1" || !strings.Contains(m.Body, "the body") {
		t.Fatalf("payload %+v", m)
	}
	if compflowWindows(u) != before-1 {
		t.Fatal("compose window should close after sending")
	}
	if len(fb.discarded) != 1 || fb.discarded[0] != "local-3" {
		t.Fatalf("stale draft not discarded: %v", fb.discarded)
	}
}

func TestComposeFlowSendShortcut(t *testing.T) {
	t.Skip("fyne test canvas cannot deliver canvas shortcuts (no TypedShortcut), so Cmd+Enter cannot be driven headless")
}

func TestComposeFlowSendWithoutRecipient(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1"})
	w := compflowWin(u)
	before := compflowWindows(u)
	compflowButton(t, w.Content(), "Send").OnTapped()
	if len(fb.sent) != 0 || compflowWindows(u) != before {
		t.Fatal("must not send or close without a recipient")
	}
	if got := compflowOverlayTexts(w); !compflowHas(got, strings.ToLower(errNoRecipient.Error())) {
		t.Fatalf("error not shown: %v", got)
	}
	for _, c := range fb.calls {
		if c == "SendMessage" {
			t.Fatal("SendMessage called")
		}
	}
}

func TestComposeFlowSendFailureKeepsWindow(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com", DraftID: "local-1"})
	w := compflowWin(u)
	before := compflowWindows(u)
	fb.errs = map[string]error{"SendMessage": errors.New("smtp exploded")}
	send := compflowButton(t, w.Content(), "Send")
	send.OnTapped()
	if compflowWindows(u) != before {
		t.Fatal("window must stay open after a failed send")
	}
	if send.Disabled() {
		t.Fatal("Send must be re-enabled to retry")
	}
	if got := compflowOverlayTexts(w); !compflowHas(got, "smtp exploded") {
		t.Fatalf("error not shown: %v", got)
	}
	if len(fb.sent) != 0 || len(fb.discarded) != 0 {
		t.Fatalf("sent %v discarded %v", fb.sent, fb.discarded)
	}
	// Retry succeeds once the backend recovers.
	delete(fb.errs, "SendMessage")
	send.OnTapped()
	if len(fb.sent) != 1 || compflowWindows(u) != before-1 {
		t.Fatalf("retry: sent %d windows %d", len(fb.sent), compflowWindows(u))
	}
}

func TestComposeFlowAutosavesDraftAndReusesID(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	subject := compflowSubject(t, w)
	subject.SetText("first")
	tick()
	compflowUntil(t, "first autosave", func() bool { return fb.draftCount() == 1 })
	drafts, ids := fb.savedDrafts()
	if ids[0] != "" || drafts[0].Subject != "first" || drafts[0].To != "bob@x.com" {
		t.Fatalf("first save id %q %+v", ids[0], drafts[0])
	}
	subject.SetText("second")
	tick()
	compflowUntil(t, "second autosave", func() bool { return fb.draftCount() == 2 })
	drafts, ids = fb.savedDrafts()
	if ids[1] != "d1" || drafts[1].Subject != "second" {
		t.Fatalf("second save must update draft d1: %v %+v", ids, drafts[1])
	}
}

func TestComposeFlowAutosaveRetriesAfterFailure(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	fb.errs = map[string]error{"SaveDraft": errors.New("offline")}
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	subj := compflowSubject(t, compflowWin(u))
	subj.SetText("keep me")
	tick()
	compflowUntil(t, "failed save attempted", func() bool { return compflowCalls(fb, "SaveDraft") >= 1 })
	delete(fb.errs, "SaveDraft")
	tick()
	compflowUntil(t, "retried save", func() bool { return fb.draftCount() == 1 })
	if drafts, _ := fb.savedDrafts(); drafts[0].Subject != "keep me" {
		t.Fatalf("draft %+v", drafts[0])
	}
}

func TestComposeFlowDiscard(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	before := compflowWindows(u)
	subj := compflowSubject(t, w)
	subj.SetText("to be dropped")
	tick()
	compflowUntil(t, "autosave", func() bool { return fb.draftCount() == 1 })

	// Declining keeps window and draft.
	compflowButton(t, w.Content(), "Discard").OnTapped()
	if got := compflowOverlayTexts(w); !compflowHas(got, "this draft will be deleted.") {
		t.Fatalf("no confirmation: %v", got)
	}
	compflowButton(t, w.Canvas().Overlays().Top(), "No").OnTapped()
	if compflowWindows(u) != before || len(fb.discarded) != 0 {
		t.Fatalf("cancel closed window or discarded: %v", fb.discarded)
	}

	compflowButton(t, w.Content(), "Discard").OnTapped()
	compflowButton(t, w.Canvas().Overlays().Top(), "Yes").OnTapped()
	if compflowWindows(u) != before-1 {
		t.Fatal("window should close after discard")
	}
	if len(fb.discarded) != 1 || fb.discarded[0] != "d1" {
		t.Fatalf("discarded %v", fb.discarded)
	}
}

func TestComposeFlowDiscardUnsavedDeletesNothing(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1"})
	w := compflowWin(u)
	before := compflowWindows(u)
	compflowButton(t, w.Content(), "Discard").OnTapped()
	compflowButton(t, w.Canvas().Overlays().Top(), "Yes").OnTapped()
	if compflowWindows(u) != before-1 || len(fb.discarded) != 0 {
		t.Fatalf("windows %d discarded %v", compflowWindows(u), fb.discarded)
	}
}

func TestComposeFlowCloseKeepsSavedDraft(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	subj := compflowSubject(t, w)
	subj.SetText("unfinished")
	tick()
	compflowUntil(t, "autosave", func() bool { return fb.draftCount() == 1 })
	w.Close()
	if len(fb.discarded) != 0 {
		t.Fatalf("closing the window must keep the draft, discarded %v", fb.discarded)
	}
}

func TestComposeFlowNoAccountsShowsError(t *testing.T) {
	u, _ := newTestUI(t, withNoAccounts())
	before := compflowWindows(u)
	u.compose(desktop.OutboundMsg{})
	if compflowWindows(u) != before {
		t.Fatal("compose must not open without accounts")
	}
	if got := compflowOverlayTexts(u.win); !compflowHas(got, strings.ToLower(errNoAccounts.Error())) {
		t.Fatalf("error not shown: %v", got)
	}
}

func compflowHas(have []string, want ...string) bool {
	for _, w := range want {
		ok := false
		for _, h := range have {
			if h == w {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func compflowCalls(fb *fakeBackend, name string) (n int) {
	for _, c := range fb.callLog() {
		if c == name {
			n++
		}
	}
	return
}

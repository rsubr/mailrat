package ui

import (
	"errors"
	"fmt"
	"net/mail"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

func f3Msg(from, to, cc string) desktop.MessageItem {
	return desktop.MessageItem{ID: "g1", MessageID: "<rfc1@x.com>", ThreadID: "t1", AccountID: "a1", FromAddr: from, ToAddr: to, CcAddr: cc, Subject: "Hi"}
}

func TestF3ReplyRecipients(t *testing.T) {
	me := []string{"neil@personal.com", "alias@personal.com"}
	tests := []struct {
		name   string
		m      desktop.MessageItem
		all    bool
		to, cc string
	}{
		{"reply", f3Msg("Ann <ann@x.com>", "neil@personal.com", ""), false, "Ann <ann@x.com>", ""},
		{"reply-all drops own from Cc, keeps Cc", f3Msg("Ann <ann@x.com>", "neil@personal.com, Bob <bob@x.com>", "Cy <cy@x.com>"), true, "Ann <ann@x.com>", "Bob <bob@x.com>, Cy <cy@x.com>"},
		{"alias is own", f3Msg("ann@x.com", "ALIAS@personal.com, bob@x.com", ""), true, "ann@x.com", "bob@x.com"},
		{"sender not repeated in Cc", f3Msg("ann@x.com", "neil@personal.com, ann@x.com", ""), true, "ann@x.com", ""},
		{"reply to own message goes to recipients", f3Msg("Neil <neil@personal.com>", "Ann <ann@x.com>", "cy@x.com"), false, "Ann <ann@x.com>", ""},
		{"reply-all to own message", f3Msg("neil@personal.com", "Ann <ann@x.com>", "cy@x.com"), true, "Ann <ann@x.com>", "cy@x.com"},
		{"message to myself", f3Msg("neil@personal.com", "neil@personal.com", ""), false, "neil@personal.com", ""},
		{"comma in quoted name", f3Msg(`"Doe, Jane" <j@x.com>`, "neil@personal.com", ""), false, `"Doe, Jane" <j@x.com>`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := replyDraft(tc.m, tc.all, me...)
			if d.To != tc.to || d.Cc != tc.cc {
				t.Fatalf("To %q Cc %q, want %q %q", d.To, d.Cc, tc.to, tc.cc)
			}
			if d.InReplyTo != "<rfc1@x.com>" {
				t.Fatalf("InReplyTo %q is not the RFC Message-ID", d.InReplyTo)
			}
		})
	}
	m := f3Msg("a@x.com", "n@x.com", "")
	m.MessageID = ""
	if d := replyDraft(m, false); d.InReplyTo != "" {
		t.Fatalf("no Message-ID must not fall back to the Gmail id: %q", d.InReplyTo)
	}
}

func TestF3ContactInsertParses(t *testing.T) {
	tests := []struct {
		c    desktop.ContactItem
		want string
	}{
		{desktop.ContactItem{Name: "Ann Lee", Email: "ann@x.com"}, "Ann Lee <ann@x.com>"},
		{desktop.ContactItem{Name: "Doe, Jane", Email: "j@x.com"}, `"Doe, Jane" <j@x.com>`},
		{desktop.ContactItem{Name: `Q "Z" \ <x>`, Email: "q@x.com"}, `"Q \"Z\" \\ <x>" <q@x.com>`},
		{desktop.ContactItem{Name: "Zoë", Email: "z@x.com"}, "Zoë <z@x.com>"},
		{desktop.ContactItem{Email: "bare@x.com"}, "bare@x.com"},
	}
	for _, tc := range tests {
		got := contactInsert(tc.c)
		if got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
		a, err := mail.ParseAddress(got)
		if err != nil || a.Address != tc.c.Email || (tc.c.Name != "" && a.Name != tc.c.Name) {
			t.Errorf("%q parses to %+v, %v", got, a, err)
		}
	}
}

func TestF3SplitRecipientQuoted(t *testing.T) {
	tests := []struct{ in, head, token string }{
		{"ann", "", "ann"},
		{"a@x.com, bo", "a@x.com, ", "bo"},
		{`"Doe, Jane" <j@x.com>, bo`, `"Doe, Jane" <j@x.com>, `, "bo"},
		{`"Doe, Ja`, "", `"Doe, Ja`},
		{`a@x.com, "Doe, Ja`, "a@x.com, ", `"Doe, Ja`},
	}
	for _, tc := range tests {
		if h, k := splitRecipient(tc.in); h != tc.head || k != tc.token {
			t.Errorf("%q -> %q, %q", tc.in, h, k)
		}
	}
}

func TestF3PickCommaContact(t *testing.T) {
	u, fb, _ := testUI(t)
	fb.found = []desktop.ContactItem{{Name: "Doe, Jane", Email: "j@x.com"}}
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{})
	w := compflowWin(u)
	to := compflowRecipient(t, w, "To")
	test.Type(to, "do")
	compflowButton(t, w.Content(), "Doe, Jane <j@x.com>").OnTapped()
	if _, err := mail.ParseAddressList(to.Text); err != nil || to.Text != `"Doe, Jane" <j@x.com>, ` {
		t.Fatalf("To %q: %v", to.Text, err)
	}
}

func TestF3CloseFlushesUnsavedEdits(t *testing.T) {
	compflowTicks(t) // no tick ever fires: only the close can save
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	compflowSubject(t, w).SetText("typed just now")
	w.Close()
	compflowUntil(t, "flush on close", func() bool { return fb.draftCount() == 1 })
	if d, _ := fb.savedDrafts(); d[0].Subject != "typed just now" {
		t.Fatalf("saved %+v", d[0])
	}
	if len(fb.discarded) != 0 {
		t.Fatalf("discarded %v", fb.discarded)
	}
}

func TestF3CloseUntouchedSavesNothing(t *testing.T) {
	compflowTicks(t)
	u, fb, _ := testUI(t)
	fb.sig = "<p>sig</p>"
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{})
	compflowWin(u).Close()
	if fb.draftCount() != 0 {
		t.Fatal("an untouched message must not leave a draft")
	}
}

func TestF3CloseAfterTickOnlySavesNewEdits(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	subj := compflowSubject(t, w)
	subj.SetText("one")
	tick()
	compflowUntil(t, "tick save", func() bool { return fb.draftCount() == 1 })
	w.Close()
	if fb.draftCount() != 1 {
		t.Fatalf("nothing changed since the tick, saves %d", fb.draftCount())
	}
}

func TestF3FromChangeMovesDraft(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	compflowSubject(t, w).SetText("s")
	tick()
	compflowUntil(t, "first save", func() bool { return fb.draftCount() == 1 })
	compflowFrom(t, w).SetSelected("neil@work.com")
	if len(fb.discardedAcc) != 1 || fb.discardedAcc[0] != "a1/d1" {
		t.Fatalf("old account's draft not dropped: %v", fb.discardedAcc)
	}
	tick()
	compflowUntil(t, "save under new account", func() bool { return fb.draftCount() == 2 })
	d, ids := fb.savedDrafts()
	if d[1].AccountID != "a2" || ids[1] != "" {
		t.Fatalf("second save %+v id %q: must be a fresh draft under a2", d[1], ids[1])
	}
}

func TestF3SendOnlyCc(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1"})
	w := compflowWin(u)
	compflowRecipient(t, w, "Cc").SetText("c@x.com")
	compflowButton(t, w.Content(), "Send").OnTapped()
	if len(fb.sent) != 1 {
		t.Fatalf("sent %d", len(fb.sent))
	}
}

func TestF3CloseMidSendSuccessDiscardsDraft(t *testing.T) {
	compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com", DraftID: "local-9"})
	w := compflowWin(u)
	compflowSubject(t, w).SetText("edited")
	fb.onSend = func() { w.Close() } // user closes the window while the send is in flight
	compflowButton(t, w.Content(), "Send").OnTapped()
	compflowUntil(t, "draft discarded after send", func() bool { return len(fb.discarded) == 1 })
	if len(fb.sent) != 1 {
		t.Fatalf("sent %d", len(fb.sent))
	}
}

func TestF3CloseMidSendFailureKeepsDraftAndReports(t *testing.T) {
	compflowTicks(t)
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	compflowSubject(t, w).SetText("important")
	fb.errs = map[string]error{"SendMessage": errors.New("net down")}
	fb.onSend = func() { w.Close() }
	compflowButton(t, w.Content(), "Send").OnTapped()
	compflowUntil(t, "edits kept", func() bool { return fb.draftCount() == 1 })
	if len(fb.discarded) != 0 {
		t.Fatalf("draft must survive a failed send: %v", fb.discarded)
	}
	if got := compflowOverlayTexts(u.win); !compflowHas(got, "net down (the message was kept in drafts)") {
		t.Fatalf("failure not reported: %v", got)
	}
}

func TestF3DiscardDisabledWhileSending(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "bob@x.com"})
	w := compflowWin(u)
	discard := compflowButton(t, w.Content(), "Discard")
	var during bool
	fb.onSend = func() { during = discard.Disabled() }
	compflowButton(t, w.Content(), "Send").OnTapped()
	if !during {
		t.Fatal("Discard must be disabled while a send is in flight")
	}
}

func TestF3SendRejectsPartialRecipientFromBackend(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	fb.errs = map[string]error{"SendMessage": errors.New(`invalid recipient "dee"`)}
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "dee"})
	w := compflowWin(u)
	compflowButton(t, w.Content(), "Send").OnTapped()
	if got := compflowOverlayTexts(w); !compflowHas(got, `invalid recipient "dee"`) {
		t.Fatalf("error not shown: %v", got)
	}
	if compflowRecipient(t, w, "To").Text != "dee" {
		t.Fatal("typed text must stay")
	}
}

func TestF3TabOrder(t *testing.T) {
	u, _, _ := testUI(t)
	closeComposeWindows(t, u)
	u.compose(desktop.OutboundMsg{AccountID: "a1"})
	w := compflowWin(u)
	c := w.Canvas()
	c.Focus(compflowRecipient(t, w, "To"))
	var seq []string
	for range 24 {
		c.(interface{ FocusNext() }).FocusNext()
		switch f := c.Focused().(type) {
		case *suggestEntry:
			seq = append(seq, f.PlaceHolder)
		case *Wysiwyg:
			seq = append(seq, "body")
		case *widget.Button:
			seq = append(seq, f.Text)
		case *widget.Entry:
			seq = append(seq, "subject")
		default:
			seq = append(seq, fmt.Sprintf("%T", f))
		}
	}
	t.Logf("order after To: %v", seq)
	idx := func(s string) int {
		for i, v := range seq {
			if v == s {
				return i
			}
		}
		return 99
	}
	if !(idx("Cc") < idx("Bcc") && idx("Bcc") < idx("body") && idx("body") < idx("Discard") && idx("Discard") < idx("Send")) {
		t.Fatalf("tab order %v", seq)
	}
}

package ui

import (
	"strings"
	"testing"

	"github.com/mailrat/mailrat/internal/desktop"
)

func rtMsg(from, to, cc, replyTo string) desktop.MessageItem {
	m := f3Msg(from, to, cc)
	m.ReplyTo = replyTo
	return m
}

func TestReplyToRecipientMatrix(t *testing.T) {
	me := []string{"neil@personal.com", "alias@personal.com"}
	tests := []struct {
		name   string
		m      desktop.MessageItem
		all    bool
		to, cc string
	}{
		{"reply uses Reply-To", rtMsg("Ann <ann@x.com>", "neil@personal.com", "", "Support <help@x.com>"), false, "Support <help@x.com>", ""},
		{"no Reply-To uses From", rtMsg("Ann <ann@x.com>", "neil@personal.com", "", ""), false, "Ann <ann@x.com>", ""},
		{"Reply-To list", rtMsg("ann@x.com", "neil@personal.com", "", "a@x.com, B <b@x.com>"), false, "a@x.com, B <b@x.com>", ""},
		{"Reply-To group and encoded word", rtMsg("ann@x.com", "neil@personal.com", "", "Team: =?UTF-8?B?w4lsw6g=?= <e@x.com>;"), false, "Élè <e@x.com>", ""},
		{"malformed Reply-To falls back to From", rtMsg("Ann <ann@x.com>", "neil@personal.com", "", "<<junk"), false, "Ann <ann@x.com>", ""},
		{"Reply-To only me falls back to From", rtMsg("Ann <ann@x.com>", "neil@personal.com", "", "Alias@personal.com"), false, "Ann <ann@x.com>", ""},
		{"Reply-To with me and other drops me", rtMsg("Ann <ann@x.com>", "neil@personal.com", "", "alias@personal.com, o@x.com"), false, "o@x.com", ""},
		{"own message ignores its Reply-To", rtMsg("neil@personal.com", "Ann <ann@x.com>", "", "other@x.com"), false, "Ann <ann@x.com>", ""},
		{"reply-all: To is Reply-To, Cc the rest minus own", rtMsg("Ann <ann@x.com>", "neil@personal.com, Bob <bob@x.com>", "cy@x.com", "list@x.com"), true, "list@x.com", "Bob <bob@x.com>, cy@x.com"},
		{"reply-all: Reply-To not repeated in Cc, case-insensitive", rtMsg("ann@x.com", "neil@personal.com, LIST@x.com", "", "list@x.com"), true, "list@x.com", ""},
		{"reply-all: duplicate Cc dropped", rtMsg("ann@x.com", "neil@personal.com, bob@x.com", "BOB@x.com", ""), true, "ann@x.com", "bob@x.com"},
		{"reply-all own message", rtMsg("neil@personal.com", "Ann <ann@x.com>", "cy@x.com", "other@x.com"), true, "Ann <ann@x.com>", "cy@x.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := replyDraft(tc.m, tc.all, me...)
			if d.To != tc.to || d.Cc != tc.cc {
				t.Fatalf("To %q Cc %q, want %q %q", d.To, d.Cc, tc.to, tc.cc)
			}
		})
	}
}

func TestReplyReferencesChain(t *testing.T) {
	m := f3Msg("a@x.com", "n@x.com", "")
	m.MessageID = "<p@x>"
	m.References, m.InReplyTo = "<a@x>\r\n <b@x>", "<b@x>"
	d := replyDraft(m, false)
	if d.References != "<a@x> <b@x> <p@x>" || d.InReplyTo != "<p@x>" {
		t.Fatalf("%q %q", d.References, d.InReplyTo)
	}
	m.References = ""
	if d := replyDraft(m, true); d.References != "<b@x> <p@x>" {
		t.Fatalf("In-Reply-To fallback: %q", d.References)
	}
	m.InReplyTo = ""
	if d := replyDraft(m, false); d.References != "<p@x>" {
		t.Fatalf("root parent: %q", d.References)
	}
	m.References = strings.Repeat("<x@x> ", 40)
	if d := replyDraft(m, false); len(strings.Fields(d.References)) != 21 {
		t.Fatalf("not trimmed: %q", d.References)
	}
	if d := forwardDraft(m); d.InReplyTo != "" || d.References != "" {
		t.Fatalf("forward must start a new thread: %+v", d)
	}
}

func TestReplyUsesReplyToEndToEnd(t *testing.T) {
	u, fb, _ := testUI(t)
	closeComposeWindows(t, u)
	m := rtMsg("Ann <ann@x.com>", "n@x.com", "", "Replies <replies@x.com>")
	m.MessageID, m.References = "<p@x>", "<a@x>"
	u.messages = []desktop.MessageItem{m}
	u.reply(false)
	w := compflowWin(u)
	if got := compflowRecipient(t, w, "To").Text; !strings.Contains(got, "replies@x.com") || strings.Contains(got, "ann@x.com") {
		t.Fatalf("To %q", got)
	}
	compflowButton(t, w.Content(), "Send").OnTapped()
	if len(fb.sent) != 1 {
		t.Fatalf("sent %d", len(fb.sent))
	}
	s := fb.sent[0]
	if s.InReplyTo != "<p@x>" || s.References != "<a@x> <p@x>" || s.ThreadID != "t1" || !strings.Contains(s.To, "replies@x.com") {
		t.Fatalf("%+v", s)
	}
}

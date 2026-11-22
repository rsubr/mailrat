package ui

import (
	"context"
	"fyne.io/fyne/v2/theme"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

// fakeBackend serves canned Mimestream-style data and records mutations.
type fakeBackend struct {
	archived, trashed []string
	sent              []desktop.OutboundMsg
}

func ms(h int) int64 { return time.Now().Add(-time.Duration(h) * time.Hour).UnixMilli() }

func (f *fakeBackend) SetEventHandler(desktop.EventHandler) {}
func (f *fakeBackend) Start(context.Context)                {}
func (f *fakeBackend) Shutdown()                            {}
func (f *fakeBackend) GetAccounts() ([]desktop.AccountItem, error) {
	return []desktop.AccountItem{
		{ID: "a1", Email: "neil@personal.com", DisplayName: "Personal"},
		{ID: "a2", Email: "neil@work.com", DisplayName: "Work"},
	}, nil
}
func (f *fakeBackend) StartAccountAdd() error     { return nil }
func (f *fakeBackend) RemoveAccount(string) error { return nil }
func (f *fakeBackend) GetAccountSettings(id string) (desktop.AccountSettingsItem, error) {
	return desktop.AccountSettingsItem{AccountID: id, SyncPeriod: "90d"}, nil
}
func (f *fakeBackend) SetAccountSettings(desktop.AccountSettingsItem) error { return nil }
func (f *fakeBackend) GetGlobalSettings() (desktop.GlobalSettingsItem, error) {
	return desktop.GlobalSettingsItem{SyncFrequency: "15m"}, nil
}
func (f *fakeBackend) SetGlobalSettings(desktop.GlobalSettingsItem) error { return nil }
func (f *fakeBackend) GetInbox(string, int, int) ([]desktop.ThreadItem, error) {
	return []desktop.ThreadItem{
		{ID: "t1", AccountID: "a1", Subject: "Positions at Mimestream?", Participants: "Dan Smith <dan@x.com>", Snippet: "Hey Neil, I hope all is well. I'm…", Date: ms(1), MessageCount: 5, IsUnread: true, LabelIDs: []string{"L1"}},
		{ID: "t2", AccountID: "a1", Subject: "Lake Tahoe Trip", Participants: "Keith <k@x.com>, Bruce <b@x.com>, Neil <n@x.com>", Snippet: "I might also come in on the evening…", Date: ms(2), MessageCount: 5, HasAttachments: true, LabelIDs: []string{"L2"}},
		{ID: "t3", AccountID: "a1", Subject: "Lunch this Friday?", Participants: "Ben Johnson <ben@x.com>, Neil <n@x.com>", Snippet: "12PM at Delarosa sounds great to me! See you on Friday!", Date: ms(5), MessageCount: 3},
		{ID: "t4", AccountID: "a1", Subject: "Interesting Article", Participants: "Samantha Schultz <s@x.com>", Snippet: "I thought you might like reading this article.", Date: ms(30)},
		{ID: "t5", AccountID: "a1", Subject: "Referral?", Participants: "Gregory Gardner <g@x.com>", Snippet: "Hey Neil! I was interested in a job at your…", Date: ms(80)},
	}, nil
}
func (f *fakeBackend) GetLabelThreads(string, string, int, int) ([]desktop.ThreadItem, error) {
	return nil, nil
}
func (f *fakeBackend) SearchInbox(string, int) ([]desktop.ThreadItem, error) { return nil, nil }
func (f *fakeBackend) GetThread(_, id string) ([]desktop.MessageItem, error) {
	return []desktop.MessageItem{
		{ID: "m1", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Ben Johnson <ben@x.com>", ToAddr: "Neil Jhaveri <n@x.com>", Snippet: "Hey Neil, It's been a while since we've caught up! I'm going to be in…", Date: ms(20), Body: "<div>Hey Neil, It's been a while since we've caught up! I'm going to be in town.</div>"},
		{ID: "m2", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Neil Jhaveri <n@x.com>", ToAddr: "Ben Johnson <ben@x.com>", Snippet: "Hey Ben, Yes! Lunch on Friday sounds great to me!", Date: ms(14), Body: "Hey Ben, Yes! Lunch on Friday sounds great to me!"},
		{ID: "m3", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Ben Johnson <ben@x.com>", ToAddr: "Neil Jhaveri <n@x.com>", Snippet: "12PM at Delarosa", Date: ms(5), Body: "<p>12PM at Delarosa sounds great to me!</p><p>See you on Friday!</p><p>– Ben</p>"},
	}, nil
}
func (f *fakeBackend) GetMessage(_, id string) (desktop.MessageItem, error) {
	return desktop.MessageItem{ID: id}, nil
}
func (f *fakeBackend) GetAttachmentData(_, _, _ string) (desktop.AttachmentDataItem, error) {
	return desktop.AttachmentDataItem{}, nil
}
func (f *fakeBackend) SendMessage(m desktop.OutboundMsg) error {
	f.sent = append(f.sent, m)
	return nil
}
func (f *fakeBackend) MarkMessageRead(_, _ string) error   { return nil }
func (f *fakeBackend) MarkMessageUnread(_, _ string) error { return nil }
func (f *fakeBackend) TrashThread(_, id string) error      { f.trashed = append(f.trashed, id); return nil }
func (f *fakeBackend) ArchiveThread(_, id string) error {
	f.archived = append(f.archived, id)
	return nil
}
func (f *fakeBackend) MarkThreadSpam(_, _ string) error    { return nil }
func (f *fakeBackend) MarkThreadNotSpam(_, _ string) error { return nil }
func (f *fakeBackend) Sync(string) error                   { return nil }
func (f *fakeBackend) DrainPendingOps()                    {}
func (f *fakeBackend) SearchContacts(_, _ string, _ int) ([]desktop.ContactItem, error) {
	return nil, nil
}
func (f *fakeBackend) GetLabels(string) ([]desktop.LabelItem, error) {
	return []desktop.LabelItem{
		{ID: "INBOX", Name: "Inbox", Type: "system"},
		{ID: "STARRED", Name: "Starred", Type: "system"},
		{ID: "DRAFT", Name: "Drafts", Type: "system"},
		{ID: "SENT", Name: "Sent", Type: "system"},
		{ID: "SPAM", Name: "Spam", Type: "system"},
		{ID: "TRASH", Name: "Trash", Type: "system"},
		{ID: "L1", Name: "Inquiries", Type: "user", BackgroundColor: "#a4c2f4", TextColor: "#0b3a8c"},
		{ID: "L2", Name: "Travel", Type: "user", BackgroundColor: "#d5a6bd", TextColor: "#5c1a4d"},
	}, nil
}
func (f *fakeBackend) GetLabelUnreadCounts(string) (map[string]int, error) {
	return map[string]int{"INBOX": 12, "DRAFT": 2}, nil
}
func (f *fakeBackend) GetAPIErrors() []desktop.APIError { return nil }
func (f *fakeBackend) ClearAPIErrors()                  {}

func testUI(t *testing.T) (*UI, *fakeBackend, fyne.Window) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(mimeTheme{})
	if os.Getenv("MAILRAT_DARK") != "" {
		a.Settings().SetTheme(darkTheme{})
	}
	fb := &fakeBackend{}
	u := newUI(a, fb)
	u.inline = true
	u.win = test.NewWindow(nil)
	u.win.SetContent(u.build())
	u.win.Resize(fyne.NewSize(1280, 800))
	u.reloadAll()
	return u, fb, u.win
}

func TestThreadFlowAndActions(t *testing.T) {
	u, fb, _ := testUI(t)
	if len(u.threads) != 5 || len(u.accounts) != 2 || u.unread["INBOX"] != 12 {
		t.Fatalf("initial load: %d threads %d accounts", len(u.threads), len(u.accounts))
	}
	u.list.Select(2)
	if u.selected == nil || u.selected.ID != "t3" || len(u.messages) != 3 {
		t.Fatalf("open thread: %+v", u.selected)
	}
	u.archive()
	if len(fb.archived) != 1 || fb.archived[0] != "t3" || len(u.threads) != 4 || u.selected != nil {
		t.Fatalf("archive: %v threads=%d", fb.archived, len(u.threads))
	}
	u.list.Select(0)
	u.trash()
	if len(fb.trashed) != 1 || len(u.threads) != 3 {
		t.Fatalf("trash: %v", fb.trashed)
	}
}

func TestReplyDraft(t *testing.T) {
	u, _, _ := testUI(t)
	u.list.Select(2)
	last, _ := u.lastMessage()
	d := replyDraft(last, true)
	if d.Subject != "Re: Lunch this Friday?" || d.InReplyTo != "m3" || d.To != "Ben Johnson <ben@x.com>" {
		t.Fatalf("%+v", d)
	}
	if want := "> 12PM at Delarosa sounds great to me!"; !contains(d.Body, want) {
		t.Fatalf("quote missing in %q", d.Body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf2(s, sub) >= 0 })()
}
func indexOf2(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestScreenshot renders the window to a PNG when MAILRAT_SHOT is set.
func TestScreenshot(t *testing.T) {
	path := os.Getenv("MAILRAT_SHOT")
	if path == "" {
		t.Skip("set MAILRAT_SHOT=/path/out.png")
	}
	u, _, w := testUI(t)
	u.list.Select(2)
	w.Resize(fyne.NewSize(1281, 800))
	w.Resize(fyne.NewSize(1280, 800))
	img := w.Canvas().Capture()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// darkTheme forces the dark variant; the test app reports no variant preference.
type darkTheme struct{ mimeTheme }

func (d darkTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return d.mimeTheme.Color(n, theme.VariantDark)
}

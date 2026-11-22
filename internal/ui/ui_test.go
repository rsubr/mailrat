package ui

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2/theme"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// fakeBackend serves canned Mimestream-style data and records mutations.
type fakeBackend struct {
	archived, trashed []string
	spammed, labelled []string
	syncs             int
	sent              []desktop.OutboundMsg
	noCreds           bool
	imported          string
	loginStarted      int
	cancelled         int
	submitted         string
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
func (f *fakeBackend) HasCredentials() bool { return !f.noCreds }
func (f *fakeBackend) ImportCredentials(p string) error {
	f.imported = p
	f.noCreds = false
	return nil
}
func (f *fakeBackend) BeginLogin() (string, error) {
	f.loginStarted++
	return "https://accounts.example/auth?x=1", nil
}
func (f *fakeBackend) OpenLoginBrowser() bool           { return true }
func (f *fakeBackend) SubmitLoginCode(raw string) error { f.submitted = raw; return nil }
func (f *fakeBackend) CancelLogin()                     { f.cancelled++ }
func (f *fakeBackend) RemoveAccount(string) error       { return nil }
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
		{ID: "m3", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Ben Johnson <ben@x.com>", ToAddr: "Neil Jhaveri <n@x.com>", Snippet: "12PM at Delarosa", Date: ms(5), Body: `<html><body style="background:#ffffff;color:#000000"><p style="color:#000">12PM at Delarosa sounds great to me!</p><p><font color="#222222">See you on Friday!</font> Menu: <a href="https://example.com/menu" style="color:#1a0dab">delarosa.com/menu</a></p><ul><li style="color:#000">Bring <b>cash</b></li><li>Parking is <i>tight</i></li></ul><img src="https://x.example/logo.png" alt="Delarosa"><blockquote>On Tue, Neil wrote: sounds good</blockquote><p>– Ben</p></body></html>`},
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
func (f *fakeBackend) MarkThreadSpam(_, id string) error {
	f.spammed = append(f.spammed, id)
	return nil
}
func (f *fakeBackend) MarkThreadNotSpam(_, _ string) error { return nil }
func (f *fakeBackend) SetThreadLabel(_, id, label string, on bool) error {
	f.labelled = append(f.labelled, fmt.Sprintf("%s:%s:%t", id, label, on))
	return nil
}
func (f *fakeBackend) Sync(string) error { f.syncs++; return nil }
func (f *fakeBackend) DrainPendingOps()  {}
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
		{ID: "L3", Name: "Home", Type: "user"},
		{ID: "L4", Name: "Home/Remodel", Type: "user"},
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

func TestAddAccountFlow(t *testing.T) {
	u, fb, _ := testUI(t)
	u.addAccount()
	if fb.loginStarted != 1 || u.loginDlg == nil {
		t.Fatalf("sign-in not started: started=%d dlg=%v", fb.loginStarted, u.loginDlg)
	}
	// Backend reports success: dialog closes without cancelling the (finished) login.
	u.handleEvent("accounts:updated", nil)
	if u.loginDlg != nil || fb.cancelled != 0 {
		t.Fatalf("dialog should close cleanly: dlg=%v cancelled=%d", u.loginDlg, fb.cancelled)
	}
}

func TestAddAccountWithoutCredentialsAsksForThem(t *testing.T) {
	u, fb, _ := testUI(t)
	fb.noCreds = true
	u.addAccount()
	if fb.loginStarted != 0 {
		t.Fatal("must not start sign-in without credentials")
	}
}

func TestLoginErrorShown(t *testing.T) {
	u, _, _ := testUI(t)
	u.showLogin("https://x")
	u.handleEvent("account:add:error", "") // cancellation: silent
	if u.loginDlg != nil {
		t.Fatal("dialog should close on cancel event")
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

func typeKeys(u *UI, s string) {
	for _, r := range s {
		u.typedRune(r)
	}
}

func TestGmailShortcuts(t *testing.T) {
	u, fb, _ := testUI(t)
	u.installShortcuts()

	typeKeys(u, "j") // first conversation
	if u.selected == nil || u.selected.ID != "t1" {
		t.Fatalf("j should open first thread: %+v", u.selected)
	}
	typeKeys(u, "jj")
	if u.selected.ID != "t3" {
		t.Fatalf("jj: want t3, got %s", u.selected.ID)
	}
	typeKeys(u, "k")
	if u.selected.ID != "t2" {
		t.Fatalf("k: want t2, got %s", u.selected.ID)
	}

	typeKeys(u, "s")
	if len(fb.labelled) != 1 || fb.labelled[0] != "t2:STARRED:true" || !u.selected.IsStarred {
		t.Fatalf("star: %v", fb.labelled)
	}
	typeKeys(u, "s")
	if fb.labelled[1] != "t2:STARRED:false" {
		t.Fatalf("unstar: %v", fb.labelled)
	}
	typeKeys(u, "+")
	if fb.labelled[2] != "t2:IMPORTANT:true" {
		t.Fatalf("important: %v", fb.labelled)
	}

	typeKeys(u, "e") // archive t2, cursor lands on next row
	if len(fb.archived) != 1 || fb.archived[0] != "t2" || u.selected != nil {
		t.Fatalf("e: %v", fb.archived)
	}
	typeKeys(u, "j")
	if u.selected == nil || u.selected.ID != "t3" {
		t.Fatalf("j after archive should open the next row, got %+v", u.selected)
	}
	typeKeys(u, "#")
	if len(fb.trashed) != 1 || fb.trashed[0] != "t3" {
		t.Fatalf("#: %v", fb.trashed)
	}
	typeKeys(u, "j!")
	if len(fb.spammed) != 1 {
		t.Fatalf("!: %v", fb.spammed)
	}

	typeKeys(u, "N")
	if fb.syncs == 0 {
		t.Fatal("N should sync")
	}
}

func TestGmailGoToAndMessageKeys(t *testing.T) {
	u, _, _ := testUI(t)
	u.installShortcuts()

	typeKeys(u, "gs")
	if u.activeLabel != "STARRED" {
		t.Fatalf("g s: %s", u.activeLabel)
	}
	typeKeys(u, "gi")
	if u.activeLabel != "INBOX" || u.pendingKey != 0 {
		t.Fatalf("g i: %s", u.activeLabel)
	}
	typeKeys(u, "gz") // unknown pair: ignored, prefix cleared
	if u.activeLabel != "INBOX" || u.pendingKey != 0 {
		t.Fatal("bad sequence should reset")
	}
	typeKeys(u, "g")
	u.pendingAt = time.Now().Add(-time.Minute) // stale prefix: second key acts alone
	typeKeys(u, "j")
	if u.selected == nil {
		t.Fatal("stale g must not swallow j")
	}

	// Thread of three messages: cursor starts on the newest.
	u.list.Select(2)
	if u.msgCursor != 2 {
		t.Fatalf("cursor: %d", u.msgCursor)
	}
	typeKeys(u, "p")
	if u.msgCursor != 1 || !u.expanded(u.messages[1], false) {
		t.Fatalf("p should open message 1: cursor=%d", u.msgCursor)
	}
	typeKeys(u, "pp")
	if u.msgCursor != 0 {
		t.Fatalf("clamp at first: %d", u.msgCursor)
	}
	typeKeys(u, "nn")
	if u.msgCursor != 2 {
		t.Fatalf("n: %d", u.msgCursor)
	}
	before := u.expanded(u.messages[2], true)
	typeKeys(u, "o")
	if u.expanded(u.messages[2], true) == before {
		t.Fatal("o should toggle the cursor message")
	}

	typeKeys(u, "u")
	if u.selected != nil {
		t.Fatal("u should return to the list")
	}
	typeKeys(u, "?") // help dialog builds without panic
	if u.win.Canvas().Overlays().Top() == nil {
		t.Fatal("? should show help")
	}
	typeKeys(u, "e") // overlay open: keys ignored
}

func TestGmailSearchKey(t *testing.T) {
	u, _, _ := testUI(t)
	u.installShortcuts()
	typeKeys(u, "/")
	if u.win.Canvas().Focused() != fyne.Focusable(u.search) {
		t.Fatal("/ should focus search")
	}
}

func TestLabelsSectionCollapsePersists(t *testing.T) {
	u, _, _ := testUI(t)
	if u.labelsHidden {
		t.Fatal("labels should start expanded")
	}
	u.toggleLabels()
	if !u.labelsHidden || !u.app.Preferences().Bool(prefLabelsHidden) {
		t.Fatal("collapse not stored")
	}
	if u2 := newUI(u.app, &fakeBackend{}); !u2.labelsHidden {
		t.Fatal("state not restored from preferences")
	}
	u.toggleLabels()
	if u.labelsHidden || u.app.Preferences().Bool(prefLabelsHidden) {
		t.Fatal("expand not stored")
	}
}

func TestSidebarMinimalToggleAndRowShape(t *testing.T) {
	u, _, _ := testUI(t)
	if u.sidebarMini {
		t.Fatal("sidebar should start expanded")
	}
	th := u.threads[0]
	two := newThreadRow(false)
	u.setRow(two, th)

	u.toggleSidebar()
	if !u.sidebarMini || !u.app.Preferences().Bool(prefSidebarMini) {
		t.Fatal("minimal state not stored")
	}
	if u2 := newUI(u.app, &fakeBackend{}); !u2.sidebarMini {
		t.Fatal("state not restored from preferences")
	}
	one := newThreadRow(true)
	u.setRow(one, th)
	if one.MinSize().Height >= two.MinSize().Height {
		t.Fatalf("single-line row (%v) should be shorter than two-line (%v)", one.MinSize().Height, two.MinSize().Height)
	}

	// Unread rows are bold, read rows are not.
	th.IsUnread = true
	u.setRow(one, th)
	if !one.when.TextStyle.Bold || !one.head.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold {
		t.Fatal("unread row not bold")
	}
	th.IsUnread = false
	u.setRow(one, th)
	if one.when.TextStyle.Bold {
		t.Fatal("read row should not be bold")
	}

	u.toggleSidebar()
	if u.sidebarMini || u.app.Preferences().Bool(prefSidebarMini) {
		t.Fatal("expand not stored")
	}
}

package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// fakeBackend serves canned Mimestream-style data and records mutations.
type fakeBackend struct {
	mu sync.Mutex // guards calls, drafts, savedIDs and the recorded settings (autosave runs off the UI goroutine)

	archived, trashed []string
	untrashed         []string
	spammed, labelled []string
	syncs             int
	sent              []desktop.OutboundMsg
	drafts            []desktop.OutboundMsg
	discarded         []string
	discardedAcc      []string // "account/id" for each DiscardDraft
	onSend            func()   // runs inside SendMessage, as the user acting mid-send
	opened            []string
	savedIDs          []string
	draftMsg          desktop.OutboundMsg // OpenDraft result
	noCreds           bool
	sig               string
	imported          string
	loginStarted      int
	cancelled         int
	submitted         string
	found, book       []desktop.ContactItem // SearchContacts / ListContacts results
	frequent          []desktop.ContactItem
	savedContacts     []desktop.ContactItem
	deletedContacts   []string
	renamed           []string

	// Test controls (zero values keep the canned data).
	handler         desktop.EventHandler   // captured by SetEventHandler; see fire
	errs            map[string]error       // method name -> error returned by that method
	calls           []string               // every error-returning method called, in order
	accounts        *[]desktop.AccountItem // non-nil overrides GetAccounts (pointer to empty slice = no accounts)
	threadsOverride *[]desktop.ThreadItem  // non-nil overrides GetInbox (pointer to empty slice = empty inbox)
	labelThreads    []desktop.ThreadItem   // GetLabelThreads results
	searchThreads   []desktop.ThreadItem   // SearchInbox results
	labelsOverride  []desktop.LabelItem    // non-nil overrides GetLabels
	apiErrs         []desktop.APIError     // GetAPIErrors result

	savedAccount []desktop.AccountSettingsItem // SetAccountSettings arguments
	savedGlobal  []desktop.GlobalSettingsItem  // SetGlobalSettings arguments
	savedSync    []desktop.SyncConfigItem      // SetSyncConfig arguments
	syncConfigs  []desktop.SyncConfigItem      // non-nil overrides GetSyncConfigs
}

// savedDrafts returns copies of the drafts and ids recorded by SaveDraft.
func (f *fakeBackend) savedDrafts() ([]desktop.OutboundMsg, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.drafts), slices.Clone(f.savedIDs)
}

// syncSaves returns a copy of the SetSyncConfig arguments (safe while a debounce timer may fire).
func (f *fakeBackend) syncSaves() []desktop.SyncConfigItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.savedSync)
}

// draftCount returns how many drafts SaveDraft has recorded.
func (f *fakeBackend) draftCount() int {
	d, _ := f.savedDrafts()
	return len(d)
}

// callLog returns a copy of the recorded calls.
func (f *fakeBackend) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// fail records a call to method and returns the error injected for it, if any:
// fb.errs = map[string]error{"GetInbox": errors.New("boom")}.
func (f *fakeBackend) fail(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method)
	return f.errs[method]
}

// fire delivers a backend event ("sync:progress", "inbox:updated", ...) to the UI.
func (f *fakeBackend) fire(name string, data any) {
	if f.handler != nil {
		f.handler(name, data)
	}
}

func ms(h int) int64 { return time.Now().Add(-time.Duration(h) * time.Hour).UnixMilli() }

func (f *fakeBackend) RefetchAccount(string) error {
	if err := f.fail("RefetchAccount"); err != nil {
		return err
	}
	return nil
}
func (f *fakeBackend) SetEventHandler(h desktop.EventHandler) { f.handler = h }
func (f *fakeBackend) Start(context.Context)                  {}
func (f *fakeBackend) Shutdown()                              {}
func (f *fakeBackend) GetAccounts() ([]desktop.AccountItem, error) {
	if err := f.fail("GetAccounts"); err != nil {
		return nil, err
	}
	if f.accounts != nil {
		return *f.accounts, nil
	}
	return []desktop.AccountItem{
		{ID: "a1", Email: "neil@personal.com", DisplayName: "Personal"},
		{ID: "a2", Email: "neil@work.com", DisplayName: "Work"},
	}, nil
}
func (f *fakeBackend) HasCredentials() bool { return !f.noCreds }
func (f *fakeBackend) ImportCredentials(p string) error {
	if err := f.fail("ImportCredentials"); err != nil {
		return err
	}
	f.imported = p
	f.noCreds = false
	return nil
}
func (f *fakeBackend) BeginLogin() (string, error) {
	if err := f.fail("BeginLogin"); err != nil {
		return "", err
	}
	f.loginStarted++
	return "https://accounts.example/auth?x=1", nil
}
func (f *fakeBackend) OpenLoginBrowser() bool { return true }
func (f *fakeBackend) SubmitLoginCode(raw string) error {
	if err := f.fail("SubmitLoginCode"); err != nil {
		return err
	}
	f.submitted = raw
	return nil
}
func (f *fakeBackend) CancelLogin() { f.cancelled++ }
func (f *fakeBackend) RemoveAccount(string) error {
	if err := f.fail("RemoveAccount"); err != nil {
		return err
	}
	return nil
}
func (f *fakeBackend) RenameAccount(id, n string) error {
	if err := f.fail("RenameAccount"); err != nil {
		return err
	}
	f.renamed = append(f.renamed, id+"="+n)
	return nil
}
func (f *fakeBackend) GetAccountSettings(id string) (desktop.AccountSettingsItem, error) {
	if err := f.fail("GetAccountSettings"); err != nil {
		return desktop.AccountSettingsItem{}, err
	}
	return desktop.AccountSettingsItem{AccountID: id, SyncPeriod: "90d"}, nil
}
func (f *fakeBackend) SetAccountSettings(st desktop.AccountSettingsItem) error {
	if err := f.fail("SetAccountSettings"); err != nil {
		return err
	}
	f.mu.Lock()
	f.savedAccount = append(f.savedAccount, st)
	f.mu.Unlock()
	return nil
}
func (f *fakeBackend) GetGlobalSettings() (desktop.GlobalSettingsItem, error) {
	if err := f.fail("GetGlobalSettings"); err != nil {
		return desktop.GlobalSettingsItem{}, err
	}
	return desktop.GlobalSettingsItem{SyncFrequency: "15m"}, nil
}
func (f *fakeBackend) SetGlobalSettings(gs desktop.GlobalSettingsItem) error {
	if err := f.fail("SetGlobalSettings"); err != nil {
		return err
	}
	f.mu.Lock()
	f.savedGlobal = append(f.savedGlobal, gs)
	f.mu.Unlock()
	return nil
}
func (f *fakeBackend) GetInbox(string, int, int) ([]desktop.ThreadItem, error) {
	if err := f.fail("GetInbox"); err != nil {
		return nil, err
	}
	if f.threadsOverride != nil {
		return *f.threadsOverride, nil
	}
	return []desktop.ThreadItem{
		{ID: "t1", AccountID: "a1", Subject: "Positions at Mimestream?", Participants: "Dan Smith <dan@x.com>", Snippet: "Hey Neil, I hope all is well. I'm…", Date: ms(1), MessageCount: 5, IsUnread: true, LabelIDs: []string{"L1"}},
		{ID: "t2", AccountID: "a1", Subject: "Lake Tahoe Trip", Participants: "Keith <k@x.com>, Bruce <b@x.com>, Neil <n@x.com>", Snippet: "I might also come in on the evening…", Date: ms(2), MessageCount: 5, HasAttachments: true, LabelIDs: []string{"L2"}},
		{ID: "t3", AccountID: "a1", Subject: "Lunch this Friday?", Participants: "Ben Johnson <ben@x.com>, Neil <n@x.com>", Snippet: "12PM at Delarosa sounds great to me! See you on Friday!", Date: ms(5), MessageCount: 3},
		{ID: "t4", AccountID: "a1", Subject: "Interesting Article", Participants: "Samantha Schultz <s@x.com>", Snippet: "I thought you might like reading this article.", Date: ms(30)},
		{ID: "t5", AccountID: "a1", Subject: "Referral?", Participants: "Gregory Gardner <g@x.com>", Snippet: "Hey Neil! I was interested in a job at your…", Date: ms(80)},
	}, nil
}
func (f *fakeBackend) GetLabelThreads(string, string, int, int) ([]desktop.ThreadItem, error) {
	if err := f.fail("GetLabelThreads"); err != nil {
		return nil, err
	}
	return f.labelThreads, nil
}
func (f *fakeBackend) SearchInbox(string, int) ([]desktop.ThreadItem, error) {
	if err := f.fail("SearchInbox"); err != nil {
		return nil, err
	}
	return f.searchThreads, nil
}
func (f *fakeBackend) GetThread(_, id string) ([]desktop.MessageItem, error) {
	if err := f.fail("GetThread"); err != nil {
		return nil, err
	}
	return []desktop.MessageItem{
		{ID: "m1", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Ben Johnson <ben@x.com>", ToAddr: "Neil Jhaveri <n@x.com>", Snippet: "Hey Neil, It's been a while since we've caught up! I'm going to be in…", Date: ms(20), Body: "<div>Hey Neil, It's been a while since we've caught up! I'm going to be in town.</div>"},
		{ID: "m2", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Neil Jhaveri <n@x.com>", ToAddr: "Ben Johnson <ben@x.com>", Snippet: "Hey Ben, Yes! Lunch on Friday sounds great to me!", Date: ms(14), Body: "Hey Ben, Yes! Lunch on Friday sounds great to me!"},
		{ID: "m3", MessageID: "<m3@x.com>", ThreadID: id, AccountID: "a1", Subject: "Lunch this Friday?", FromAddr: "Ben Johnson <ben@x.com>", ToAddr: "Neil Jhaveri <n@x.com>", Snippet: "12PM at Delarosa", Date: ms(5), Body: `<html><body style="background:#ffffff;color:#000000"><p style="color:#000">12PM at Delarosa sounds great to me!</p><p><font color="#222222">See you on Friday!</font> Menu: <a href="https://example.com/menu" style="color:#1a0dab">delarosa.com/menu</a></p><ul><li style="color:#000">Bring <b>cash</b></li><li>Parking is <i>tight</i></li></ul><img src="https://x.example/logo.png" alt="Delarosa"><blockquote>On Tue, Neil wrote: sounds good</blockquote><p>– Ben</p></body></html>`},
	}, nil
}
func (f *fakeBackend) GetMessage(_, id string) (desktop.MessageItem, error) {
	if err := f.fail("GetMessage"); err != nil {
		return desktop.MessageItem{}, err
	}
	return desktop.MessageItem{ID: id}, nil
}
func (f *fakeBackend) GetAttachmentData(_, _, _ string) (desktop.AttachmentDataItem, error) {
	if err := f.fail("GetAttachmentData"); err != nil {
		return desktop.AttachmentDataItem{}, err
	}
	return desktop.AttachmentDataItem{}, nil
}
func (f *fakeBackend) SendMessage(m desktop.OutboundMsg) error {
	if f.onSend != nil {
		f.onSend()
	}
	if err := f.fail("SendMessage"); err != nil {
		return err
	}
	f.sent = append(f.sent, m)
	return nil
}
func (f *fakeBackend) SaveDraft(id string, m desktop.OutboundMsg) (string, error) {
	if err := f.fail("SaveDraft"); err != nil {
		return "", err
	}
	f.mu.Lock()
	f.drafts = append(f.drafts, m)
	f.savedIDs = append(f.savedIDs, id)
	f.mu.Unlock()
	return "d1", nil
}
func (f *fakeBackend) OpenDraft(_, id string) (desktop.OutboundMsg, error) {
	if err := f.fail("OpenDraft"); err != nil {
		return desktop.OutboundMsg{}, err
	}
	f.opened = append(f.opened, id)
	return f.draftMsg, nil
}
func (f *fakeBackend) DiscardDraft(acc, id string) error {
	if err := f.fail("DiscardDraft"); err != nil {
		return err
	}
	f.discardedAcc = append(f.discardedAcc, acc+"/"+id)
	f.discarded = append(f.discarded, id)
	return nil
}
func (f *fakeBackend) MarkMessageRead(_, _ string) error {
	if err := f.fail("MarkMessageRead"); err != nil {
		return err
	}
	return nil
}
func (f *fakeBackend) MarkMessageUnread(_, _ string) error {
	if err := f.fail("MarkMessageUnread"); err != nil {
		return err
	}
	return nil
}
func (f *fakeBackend) TrashThread(_, id string) error {
	if err := f.fail("TrashThread"); err != nil {
		return err
	}
	f.trashed = append(f.trashed, id)
	return nil
}
func (f *fakeBackend) UndoTrashThread(_, id string) error {
	if err := f.fail("UndoTrashThread"); err != nil {
		return err
	}
	f.untrashed = append(f.untrashed, id)
	return nil
}
func (f *fakeBackend) ArchiveThread(_, id string) error {
	if err := f.fail("ArchiveThread"); err != nil {
		return err
	}
	f.archived = append(f.archived, id)
	return nil
}
func (f *fakeBackend) MarkThreadSpam(_, id string) error {
	if err := f.fail("MarkThreadSpam"); err != nil {
		return err
	}
	f.spammed = append(f.spammed, id)
	return nil
}
func (f *fakeBackend) MarkThreadNotSpam(_, _ string) error {
	if err := f.fail("MarkThreadNotSpam"); err != nil {
		return err
	}
	return nil
}
func (f *fakeBackend) SetThreadLabel(_, id, label string, on bool) error {
	if err := f.fail("SetThreadLabel"); err != nil {
		return err
	}
	f.labelled = append(f.labelled, fmt.Sprintf("%s:%s:%t", id, label, on))
	return nil
}
func (f *fakeBackend) Sync(string) error {
	if err := f.fail("Sync"); err != nil {
		return err
	}
	f.syncs++
	return nil
}
func (f *fakeBackend) DrainPendingOps() {}
func (f *fakeBackend) GetSyncConfigs(string) ([]desktop.SyncConfigItem, error) {
	if err := f.fail("GetSyncConfigs"); err != nil {
		return nil, err
	}
	if f.syncConfigs != nil {
		return f.syncConfigs, nil
	}
	return []desktop.SyncConfigItem{{LabelID: "INBOX", LabelName: "Inbox", Mode: desktop.SyncModeAll}}, nil
}
func (f *fakeBackend) SetSyncConfig(_ string, it desktop.SyncConfigItem) error {
	if err := f.fail("SetSyncConfig"); err != nil {
		return err
	}
	f.mu.Lock()
	f.savedSync = append(f.savedSync, it)
	f.mu.Unlock()
	return nil
}
func (f *fakeBackend) LoadOlder(string, string) (int, error) {
	if err := f.fail("LoadOlder"); err != nil {
		return 0, err
	}
	return 0, nil
}
func (f *fakeBackend) Signature(_, _ string) string { return f.sig }
func (f *fakeBackend) SearchContacts(_, _ string, _ int) ([]desktop.ContactItem, error) {
	if err := f.fail("SearchContacts"); err != nil {
		return nil, err
	}
	return f.found, nil
}
func (f *fakeBackend) ListContacts(string) ([]desktop.ContactItem, error) {
	if err := f.fail("ListContacts"); err != nil {
		return nil, err
	}
	return append([]desktop.ContactItem(nil), f.book...), nil
}
func (f *fakeBackend) SaveContact(c desktop.ContactItem) (desktop.ContactItem, error) {
	if err := f.fail("SaveContact"); err != nil {
		return desktop.ContactItem{}, err
	}
	if c.ID == "" {
		c.ID = "new:" + c.Email
	}
	f.savedContacts = append(f.savedContacts, c)
	return c, nil
}
func (f *fakeBackend) DeleteContact(_, id string) error {
	if err := f.fail("DeleteContact"); err != nil {
		return err
	}
	f.deletedContacts = append(f.deletedContacts, id)
	return nil
}
func (f *fakeBackend) FrequentAddresses(string, int) ([]desktop.ContactItem, error) {
	if err := f.fail("FrequentAddresses"); err != nil {
		return nil, err
	}
	return f.frequent, nil
}
func (f *fakeBackend) CreateLabel(_, name string) (desktop.LabelItem, error) {
	if err := f.fail("CreateLabel"); err != nil {
		return desktop.LabelItem{}, err
	}
	return desktop.LabelItem{ID: "new-" + name, Name: name, Type: "user"}, nil
}
func (f *fakeBackend) GetLabels(string) ([]desktop.LabelItem, error) {
	if err := f.fail("GetLabels"); err != nil {
		return nil, err
	}
	if f.labelsOverride != nil {
		return f.labelsOverride, nil
	}
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
	if err := f.fail("GetLabelUnreadCounts"); err != nil {
		return nil, err
	}
	return map[string]int{"INBOX": 12, "DRAFT": 2}, nil
}
func (f *fakeBackend) GetAPIErrors() []desktop.APIError { return f.apiErrs }
func (f *fakeBackend) ClearAPIErrors()                  {}

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

// buttonLabels collects the text of every labelled button under o.
func buttonLabels(o fyne.CanvasObject) (out []string) {
	if b, ok := o.(*widget.Button); ok && b.Text != "" {
		out = append(out, b.Text)
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			out = append(out, buttonLabels(ch)...)
		}
	}
	return
}

func findTapRow(o fyne.CanvasObject) *tapRow {
	if r, ok := o.(*tapRow); ok {
		return r
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			if r := findTapRow(ch); r != nil {
				return r
			}
		}
	}
	if th, ok := o.(*container.ThemeOverride); ok { // the mail view's palette wrapper
		return findTapRow(th.Content)
	}
	return nil
}

// Display target: MacBook Air M4 13" = 1470x956 logical, usable window 1470x918.
// UI tests run at this one size only (docs/ux-tests/headless.md).
const (
	uxWindowW float32 = 1470
	uxWindowH float32 = 918
)

// testOpt tweaks the UI built by newTestUI.
type testOpt func(*testConfig)

type testConfig struct {
	dark bool
	size fyne.Size
	fb   *fakeBackend
}

// withDark forces the dark theme variant (the test app reports no variant).
func withDark() testOpt { return func(c *testConfig) { c.dark = true } }

// withBackend preconfigures the fake (errors, overrides, creds) BEFORE the first load.
func withBackend(fb *fakeBackend) testOpt { return func(c *testConfig) { c.fb = fb } }

// withEmpty makes the inbox empty.
func withEmpty() testOpt {
	return func(c *testConfig) {
		c.fb.threadsOverride = &[]desktop.ThreadItem{}
	}
}

// withNoAccounts makes GetAccounts return no accounts (welcome screen).
func withNoAccounts() testOpt {
	return func(c *testConfig) { c.fb.accounts = &[]desktop.AccountItem{} }
}

// withNoCreds makes HasCredentials return false.
func withNoCreds() testOpt { return func(c *testConfig) { c.fb.noCreds = true } }

// newTestUI builds the UI on a fake backend with options applied before the
// first load. It also freezes nothing: call freezeNow(t, ...) for goldens.
func newTestUI(t *testing.T, opts ...testOpt) (*UI, *fakeBackend) {
	t.Helper()
	cfg := &testConfig{size: fyne.NewSize(uxWindowW, uxWindowH), fb: &fakeBackend{}}
	for _, o := range opts {
		o(cfg)
	}
	a := test.NewTempApp(t)
	a.Settings().SetTheme(mimeTheme{})
	if cfg.dark || os.Getenv("MAILRAT_DARK") != "" {
		a.Settings().SetTheme(darkTheme{})
	}
	u := newUI(a, cfg.fb)
	u.inline = true
	u.win = test.NewTempWindow(t, nil)
	u.win.SetContent(u.build())
	u.win.Resize(cfg.size)
	u.reloadAll()
	t.Cleanup(func() { // a pending undo expiry must not fire during later tests
		if u.undoTimer != nil {
			u.undoTimer.Stop()
		}
	})
	return u, cfg.fb
}

func testUI(t *testing.T) (*UI, *fakeBackend, fyne.Window) {
	t.Helper()
	u, fb := newTestUI(t)
	return u, fb, u.win
}

// freezeNow pins the UI clock (package var now) to at until the test ends.
func freezeNow(t *testing.T, at time.Time) {
	t.Helper()
	old := now
	now = func() time.Time { return at }
	oldLoc := time.Local
	time.Local = time.UTC // goldens must not depend on the host timezone
	t.Cleanup(func() { now = old; time.Local = oldLoc })
}

// goldenNow is a fixed clock for goldens; relative dates render as "n hours ago" style.
var goldenNow = time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

// msAt returns a unix-ms timestamp h hours before goldenNow (use with freezeNow(t, goldenNow)).
func msAt(h int) int64 { return goldenNow.Add(-time.Duration(h) * time.Hour).UnixMilli() }

// newWindow returns a registered temp window of the given size holding c.
func newWindow(t *testing.T, c fyne.CanvasObject, w, h float32) fyne.Window {
	t.Helper()
	win := test.NewTempWindow(t, c)
	win.Resize(fyne.NewSize(w, h))
	return win
}

// golden compares img with testdata/<name>.png. Run with UPDATE_GOLDEN=1 to
// (re)write the master instead (review the diff before committing). On mismatch
// fyne writes the actual image to testdata/failed/<name>.png.
func golden(t *testing.T, name string, img image.Image) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join("testdata", name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	test.AssertImageMatches(t, name+".png", img)
}

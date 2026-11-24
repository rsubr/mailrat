package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/storage"
	"google.golang.org/api/googleapi"
)

func appWithStore(t *testing.T) *App {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &App{store: s}
}

func addAccount(t *testing.T, a *App, id string) {
	t.Helper()
	if err := a.store.UpsertAccount(storage.Account{ID: id, Email: id + "@x.test"}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncIntervalFollowsSetting(t *testing.T) {
	a := appWithStore(t)
	for freq, want := range map[string]time.Duration{
		"15m": 15 * time.Minute, "1h": time.Hour, "manual": 0, "": 15 * time.Minute,
	} {
		if err := a.SetGlobalSettings(GlobalSettingsItem{SyncFrequency: freq}); err != nil {
			t.Fatal(err)
		}
		if got := a.syncInterval(); got != want {
			t.Errorf("%q: interval %v, want %v", freq, got, want)
		}
	}
}

func TestRemoveAccountClearsDefault(t *testing.T) {
	a := appWithStore(t)
	addAccount(t, a, "a1")
	addAccount(t, a, "a2")
	if err := a.SetGlobalSettings(GlobalSettingsItem{SyncFrequency: "1h", DefaultAccountID: "a1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveAccount("a2"); err != nil {
		t.Fatal(err)
	}
	if gs, _ := a.GetGlobalSettings(); gs.DefaultAccountID != "a1" {
		t.Fatalf("removing another account changed the default: %+v", gs)
	}
	if err := a.RemoveAccount("a1"); err != nil {
		t.Fatal(err)
	}
	if gs, _ := a.GetGlobalSettings(); gs.DefaultAccountID != "" || gs.SyncFrequency != "1h" {
		t.Fatalf("default not cleared (or frequency lost): %+v", gs)
	}
}

func TestGetGlobalSettingsIgnoresDeadDefault(t *testing.T) {
	a := appWithStore(t)
	if err := a.SetGlobalSettings(GlobalSettingsItem{SyncFrequency: "15m", DefaultAccountID: "gone"}); err != nil {
		t.Fatal(err)
	}
	if gs, _ := a.GetGlobalSettings(); gs.DefaultAccountID != "" {
		t.Fatalf("dead default returned: %+v", gs)
	}
}

func TestSetSyncConfigValidates(t *testing.T) {
	a := appWithStore(t)
	addAccount(t, a, "a1")
	bad := []SyncConfigItem{
		{LabelID: "INBOX", Mode: "weekly", Value: 3},
		{Mode: SyncModeDays, Value: 3},
	}
	for _, it := range bad {
		if err := a.SetSyncConfig("a1", it); err == nil {
			t.Errorf("accepted %+v", it)
		}
	}
	if err := a.SetSyncConfig("a1", SyncConfigItem{LabelID: "INBOX", LabelName: "Inbox", Mode: SyncModeDays, Value: 1 << 40}); err != nil {
		t.Fatal(err)
	}
	cfgs, _ := a.store.ListSyncConfigs("a1")
	if len(cfgs) != 1 || cfgs[0].DaysToSync != maxSyncValue {
		t.Fatalf("value not capped: %+v", cfgs)
	}
}

func TestSearchContactsIncludesOtherContacts(t *testing.T) {
	a := appWithStore(t)
	addAccount(t, a, "a1")
	if err := a.store.UpsertContact(storage.Contact{ID: "c1", AccountID: "a1", Name: "Dee Saved", Email: "dee@saved.test"}); err != nil {
		t.Fatal(err)
	}
	for i, m := range []storage.Message{
		{ID: "m1", ThreadID: "t1", AccountID: "a1", FromAddr: "Deepak S <deepak@other.test>", ToAddr: "a1@x.test", InternalDate: 2},
		{ID: "m2", ThreadID: "t1", AccountID: "a1", FromAddr: "Dee Saved <dee@saved.test>", ToAddr: "a1@x.test", InternalDate: 1},
	} {
		if err := a.store.UpsertMessageMeta(m, nil, false); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	got := emails(mustSearch(t, a, "a1", "dee"))
	if len(got) != 2 || got[0] != "dee@saved.test" || got[1] != "deepak@other.test" {
		t.Fatalf("got %v, want the saved contact then the other contact (no duplicate)", got)
	}
	if got := mustSearch(t, a, "unified", "zzz"); len(got) != 0 {
		t.Fatalf("unexpected %v", got)
	}
}

func mustSearch(t *testing.T, a *App, acct, q string) []ContactItem {
	t.Helper()
	got, err := a.SearchContacts(acct, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestStartupRestrictsDatabasePermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAILRAT_DATA_DIR", dir)
	db := filepath.Join(dir, "mailrat.db")
	if err := os.WriteFile(db, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := NewApp()
	a.startup(ctx)
	defer func() { cancel(); a.Shutdown() }()
	st, err := os.Stat(db)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("db mode %v", st.Mode().Perm())
	}
}

// loginApp is an App with a fake OAuth client whose token endpoint always
// rejects, a data dir, and an event recorder.
type loginApp struct {
	*App
	dir    string
	mu     sync.Mutex
	events []string
}

func (l *loginApp) seen(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		if e == name {
			return true
		}
	}
	return false
}

func newLoginApp(t *testing.T) *loginApp {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	t.Cleanup(ts.Close)
	a := appWithStore(t)
	dir := t.TempDir()
	t.Setenv("MAILRAT_DATA_DIR", dir)
	t.Setenv("MAILRAT_OAUTH_HEADLESS", "1")
	creds := filepath.Join(dir, "credentials.json")
	body := fmt.Sprintf(`{"installed":{"client_id":"i","client_secret":"s","auth_uri":"%s/auth","token_uri":"%s/token","redirect_uris":["http://localhost"]}}`, ts.URL, ts.URL)
	if err := os.WriteFile(creds, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := accounts.NewManager(a.store, creds)
	if err != nil {
		t.Fatal(err)
	}
	a.setAccountMgr(mgr)
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	l := &loginApp{App: a, dir: dir}
	a.SetEventHandler(func(name string, _ any) {
		l.mu.Lock()
		l.events = append(l.events, name)
		l.mu.Unlock()
	})
	t.Cleanup(func() { a.CancelLogin(); cancel(); time.Sleep(50 * time.Millisecond) })
	return l
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(6 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestHeadlessRedirectFile(t *testing.T) {
	l := newLoginApp(t)
	redirect := filepath.Join(l.dir, "oauth-redirect.txt")
	if err := os.WriteFile(redirect, []byte("http://127.0.0.1:1/callback?code=old&state=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authURL, err := l.BeginLogin()
	if err != nil {
		t.Fatal(err)
	}
	if exists(redirect) {
		t.Fatal("stale redirect file from an earlier run was kept")
	}
	state := mustParse(t, authURL).Query().Get("state")

	// A paste with someone else's state is rejected and the attempt survives.
	if err := os.WriteFile(redirect, []byte("http://127.0.0.1:1/callback?code=evil&state=other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if l.seen("account:add:retry") || l.seen("account:add:error") {
		t.Fatalf("foreign-state paste was exchanged or aborted: %v", l.events)
	}

	// A correct paste (with stray whitespace) is consumed; the token endpoint
	// rejects it, which is reported without ending the attempt.
	if err := os.WriteFile(redirect, []byte("  http://127.0.0.1:1/callback?code=good&state="+state+" \n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "redirect file consumed", func() bool { return !exists(redirect) })
	waitFor(t, "retry event", func() bool { return l.seen("account:add:retry") })
	if l.seen("account:add:error") {
		t.Fatalf("failed exchange aborted the login: %v", l.events)
	}
	if !exists(filepath.Join(l.dir, "oauth-url.txt")) {
		t.Fatal("oauth-url.txt removed while sign-in still running")
	}

	l.CancelLogin()
	waitFor(t, "url file removed after the attempt ends", func() bool { return !exists(filepath.Join(l.dir, "oauth-url.txt")) })
	waitFor(t, "cancel event", func() bool { return l.seen("account:add:error") })
}

func TestSupersededLoginDoesNotReportCancel(t *testing.T) {
	l := newLoginApp(t)
	if _, err := l.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if l.seen("account:add:error") {
		t.Fatalf("superseded attempt reported cancellation (would close the new dialog): %v", l.events)
	}
	l.CancelLogin()
	waitFor(t, "cancel of the current attempt", func() bool { return l.seen("account:add:error") })
}

func mustParse(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSaveContactRejectsMalformedEmail(t *testing.T) {
	a := appWithStore(t)
	for _, e := range []string{"foo", "Ann <ann@x.com>", "a@"} {
		if _, err := a.SaveContact(ContactItem{AccountID: "a1", Email: e}); err == nil {
			t.Errorf("accepted %q", e)
		}
	}
}

// Opening a thread must not wait on the network backfill of reply headers.
func TestGetThreadDoesNotBlockOnReplyHeaderFetch(t *testing.T) {
	a := appWithStore(t)
	addAccount(t, a, "acc")
	if err := a.store.UpsertMessage(storage.Message{ID: "m1", ThreadID: "t1", AccountID: "acc"}); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	a.fillHeaders = func(m storage.Message) storage.Message { <-release; return m }
	done := make(chan struct{})
	go func() { _, _ = a.GetThread("acc", "t1"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetThread blocked on reply header fetch")
	}
}

// A queued op that 404s (target deleted on Gmail) purges the stale local
// message or thread quietly instead of logging an API error.
func TestRollbackOpNotFoundPurgesLocal(t *testing.T) {
	a := appWithStore(t)
	addAccount(t, a, "a")
	for _, id := range []string{"m1", "m2"} {
		if err := a.store.UpsertMessageMeta(storage.Message{ID: id, ThreadID: "t", AccountID: "a", CacheState: storage.CacheMetadataOnly}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	nf := fmt.Errorf("modify: %w", &googleapi.Error{Code: 404})
	a.rollbackOp(storage.PendingOp{AccountID: "a", MessageID: "m1", OpType: storage.OpMarkRead}, nf)
	if a.store.HasMessage("a", "m1") || !a.store.HasMessage("a", "m2") {
		t.Fatal("message op should delete only m1")
	}
	a.rollbackOp(storage.PendingOp{AccountID: "a", MessageID: "t", OpType: storage.OpArchive}, nf)
	if a.store.HasMessage("a", "m2") {
		t.Fatal("thread op should delete the whole thread")
	}
	if len(a.GetAPIErrors()) != 0 {
		t.Fatalf("404 must not be logged: %v", a.GetAPIErrors())
	}
}

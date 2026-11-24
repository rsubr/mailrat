package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/contacts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
	mailsync "github.com/mailrat/mailrat/internal/sync"
)

// EventHandler receives backend events such as "inbox:updated" or "sync:complete".
// It may be called from any goroutine.
type EventHandler func(name string, data any)

// App is the backend facade used by the native UI.
type App struct {
	ctx     context.Context
	onEvent EventHandler
	store   *storage.Store
	// accounts is the OAuth manager; nil until credentials.json exists. It is
	// replaced at runtime by ImportCredentials, so access only via accountMgr and
	// setAccountMgr. Lock order: accountsMu is a leaf (never held while calling
	// out, doing I/O, or taking another lock).
	accountsMu sync.RWMutex
	accounts   *accounts.Manager

	errMu  sync.Mutex
	errLog []APIError

	attachments *mail.AttachmentCache

	// fillHeaders backfills reply headers; nil means fillReplyHeaders. Tests swap it.
	fillHeaders func(storage.Message) storage.Message

	// Background attachment caching of synced mail, one run per account. It has
	// its own cancel scope, separate from the thread-open prefetch above.
	syncPrefetchMu     sync.Mutex
	syncPrefetchCancel map[string]context.CancelFunc

	queue  *mail.Queue   // optimistic mutations awaiting Gmail
	trash  *mail.Delayed // trashes still inside their undo window
	syncMu sync.Mutex

	contactsMu     sync.Mutex
	contactsSynced map[string]time.Time

	loginMu     sync.Mutex
	login       *accounts.Login
	loginCancel context.CancelFunc
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// SetEventHandler registers the callback that receives backend events.
func (a *App) SetEventHandler(h EventHandler) { a.onEvent = h }

// Start opens the database and begins background sync.
func (a *App) Start(ctx context.Context) {
	a.startup(ctx)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	dir, err := dataDir()
	if err != nil {
		a.logError(fmt.Sprintf("data dir: %v", err))
		return
	}

	store, err := storage.Open(filepath.Join(dir, "mailrat.db"))
	if err != nil {
		a.logError(fmt.Sprintf("open db: %v", err))
		return
	}
	a.store = store
	for _, suffix := range []string{"", "-wal", "-shm"} { // holds refresh tokens and all mail
		_ = os.Chmod(filepath.Join(dir, "mailrat.db"+suffix), 0600)
	}
	a.attachments = mail.NewAttachmentCache(store)
	// Only the last 14 days of mail get attachments cached in the background;
	// older ones download on open.
	a.attachments.Window = time.Duration(mailsync.AllMailDefaultDays) * 24 * time.Hour
	a.queue = mail.NewQueue(store, a.applyPendingOp, gmail.IsPermanent, a.rollbackOp)
	a.trash = mail.NewDelayed(mail.UndoWindow)
	go a.queue.Run(ctx)

	credPath := filepath.Join(dir, "credentials.json")
	mgr, err := accounts.NewManager(store, credPath)
	if err != nil {
		a.logWarning(fmt.Sprintf("no credentials: %v", err))
		return
	}
	a.setAccountMgr(mgr)

	go a.syncLoop(ctx)
}

// syncInterval is how often to sync in the background, from the "Check for new
// mail" setting. Zero means manual: never sync on a timer or on wake.
func (a *App) syncInterval() time.Duration {
	gs, _ := a.GetGlobalSettings()
	switch gs.SyncFrequency {
	case "manual":
		return 0
	case "1h":
		return time.Hour
	}
	return 15 * time.Minute
}

// syncLoop syncs on the configured interval and right after the machine wakes from sleep.
// Wake is detected as a wall-clock jump between ticks (the monotonic clock may
// not advance while suspended); idle connections are dropped first because
// they went stale during the sleep.
func (a *App) syncLoop(ctx context.Context) {
	const tick = 10 * time.Second
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	last, lastSync := time.Now().Round(0), time.Now().Round(0)
	for {
		select {
		case <-ticker.C:
			now := time.Now().Round(0)
			woke := now.Sub(last) > 3*tick
			last = now
			if woke {
				accounts.CloseIdleConnections()
			}
			if every := a.syncInterval(); every > 0 && (woke || now.Sub(lastSync) >= every) {
				lastSync = now
				go func() { _ = a.Sync("unified") }() // never stall the loop on a slow sync
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) emitEvent(name string, data any) {
	if a.onEvent != nil {
		a.onEvent(name, data)
	}
}

func (a *App) logError(msg string) {
	fmt.Fprintln(os.Stderr, "ERROR:", msg)
}

func (a *App) logWarning(msg string) {
	fmt.Fprintln(os.Stderr, "WARN:", msg)
}

// maxErrLog bounds the in-memory API error log.
const maxErrLog = 200

func (a *App) addAPIError(msg string) {
	a.logWarning(msg)
	entry := APIError{Timestamp: time.Now().UnixMilli(), Message: msg}
	a.errMu.Lock()
	a.errLog = append(a.errLog, entry)
	if len(a.errLog) > maxErrLog {
		a.errLog = append([]APIError(nil), a.errLog[len(a.errLog)-maxErrLog:]...) // keep the newest
	}
	a.errMu.Unlock()
	a.emitEvent("api:error", entry)
}

// GetAPIErrors returns the accumulated API error log.
func (a *App) GetAPIErrors() []APIError {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	out := make([]APIError, len(a.errLog))
	copy(out, a.errLog)
	return out
}

// ClearAPIErrors empties the error log.
func (a *App) ClearAPIErrors() {
	a.errMu.Lock()
	a.errLog = nil
	a.errMu.Unlock()
}

// Shutdown closes the database when the app exits.
func (a *App) Shutdown() {
	if a.trash != nil {
		a.trash.Flush() // queue pending deletes so they survive the restart
	}
	if a.store != nil {
		a.store.Close() //nolint:errcheck
	}
}

// GetAccounts returns all connected Gmail accounts.
func (a *App) GetAccounts() ([]AccountItem, error) {
	if a.store == nil {
		return nil, nil
	}
	accs, err := a.store.ListAccounts()
	if err != nil {
		return nil, err
	}
	return accountsToItems(accs), nil
}

// HasCredentials reports whether a Google OAuth client (credentials.json) is configured.
func (a *App) HasCredentials() bool { return a.accountMgr() != nil }

// ImportCredentials validates a Google Cloud OAuth client file and installs it as
// ~/.mailrat/credentials.json so accounts can be added from the UI.
func (a *App) ImportCredentials(path string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialised")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := accounts.ValidateCredentials(data); err != nil {
		return err
	}
	dir, err := dataDir()
	if err != nil {
		return err
	}
	credPath := filepath.Join(dir, "credentials.json")
	tmp := credPath + ".tmp" // write aside so a failure cannot leave a half-written client file
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, credPath); err != nil {
		return err
	}
	mgr, err := accounts.NewManager(a.store, credPath)
	if err != nil {
		return err
	}
	first := a.setAccountMgr(mgr)
	if first {
		go a.syncLoop(a.ctx)
	}
	return nil
}

// accountMgr returns the current OAuth manager (nil if unconfigured).
func (a *App) accountMgr() *accounts.Manager {
	a.accountsMu.RLock()
	defer a.accountsMu.RUnlock()
	return a.accounts
}

// setAccountMgr installs m and reports whether there was no manager before.
func (a *App) setAccountMgr(m *accounts.Manager) (first bool) {
	a.accountsMu.Lock()
	defer a.accountsMu.Unlock()
	first = a.accounts == nil
	a.accounts = m
	return first
}

// BeginLogin starts a Google sign-in: it opens the system browser and waits for
// the redirect in the background. The returned URL lets the UI offer a copy
// link if the browser did not open. Success emits "accounts:updated"; failure or
// cancellation emits "account:add:error" (cancellation carries no message).
func (a *App) BeginLogin() (string, error) {
	if a.accountMgr() == nil {
		return "", fmt.Errorf("no Google credentials configured — import credentials.json first")
	}
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.loginCancel != nil {
		a.loginCancel() // supersede a stale attempt
		if a.login != nil {
			select { // let it release its (possibly pinned) port before listening again
			case <-a.login.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
	login, err := a.accountMgr().BeginLogin()
	if err != nil {
		return "", err
	}
	login.OnError = func(err error) { a.emitEvent("account:add:retry", err.Error()) }
	ctx, cancel := context.WithCancel(a.ctx)
	a.login, a.loginCancel = login, cancel
	if !login.OpenBrowser() && os.Getenv("MAILRAT_OAUTH_HEADLESS") == "1" {
		a.handoffHeadlessLogin(ctx, login)
	}
	go func() {
		_, err := login.Wait(ctx)
		cancel()
		a.loginMu.Lock()
		current := a.login == login
		if current {
			a.login, a.loginCancel = nil, nil
		}
		a.loginMu.Unlock()
		switch {
		case err == nil:
			a.emitEvent("accounts:updated", nil)
		case !current:
			// Superseded by a newer attempt, whose dialog must stay open.
		case ctx.Err() != nil:
			a.emitEvent("account:add:error", "")
		default:
			a.emitEvent("account:add:error", err.Error())
		}
	}()
	return login.AuthURL(), nil
}

// handoffHeadlessLogin (MAILRAT_OAUTH_HEADLESS=1) publishes the consent URL to
// stdout and $DATA_DIR/oauth-url.txt, then completes the sign-in from a redirect
// URL pasted into $DATA_DIR/oauth-redirect.txt.
func (a *App) handoffHeadlessLogin(ctx context.Context, login *accounts.Login) {
	dir, err := dataDir()
	if err != nil {
		return
	}
	fmt.Println("OAUTH_URL:", login.AuthURL())
	urlFile := filepath.Join(dir, "oauth-url.txt")
	_ = os.WriteFile(urlFile, []byte(login.AuthURL()+"\n"), 0600)
	redirect := filepath.Join(dir, "oauth-redirect.txt")
	_ = os.Remove(redirect) // a paste left over from an earlier run is not for this attempt
	go func() {
		defer os.Remove(urlFile)
		lastBad := ""
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			b, err := os.ReadFile(redirect)
			raw := strings.TrimSpace(string(b))
			if err != nil || raw == "" {
				continue
			}
			// A file still being written fails to parse; leave it and look again.
			if err := login.Submit(raw); err != nil {
				if raw != lastBad {
					lastBad = raw
					a.logWarning(fmt.Sprintf("oauth-redirect.txt: %v", err))
				}
				continue
			}
			_ = os.Remove(redirect)
		}
	}()
}

// OpenLoginBrowser re-opens the consent page for the sign-in in progress.
func (a *App) OpenLoginBrowser() bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	return a.login != nil && a.login.OpenBrowser()
}

// SubmitLoginCode completes the sign-in in progress from a pasted redirect URL,
// for when the browser cannot reach the local callback (remote machine, firewall).
func (a *App) SubmitLoginCode(raw string) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.login == nil {
		return fmt.Errorf("no sign-in in progress")
	}
	return a.login.Submit(raw)
}

// CancelLogin abandons the sign-in in progress.
func (a *App) CancelLogin() {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.loginCancel != nil {
		a.loginCancel()
	}
}

// RemoveAccount deletes an account and all its local data via FK cascade.
func (a *App) RemoveAccount(accountID string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	// Stop everything that could still write for this account, as RefetchAccount
	// does: held trashes are dropped and the queue paused, and the delete waits
	// for any sync in flight.
	if a.trash != nil {
		a.trash.DropPrefix(accountID + "/")
	}
	a.stopSyncPrefetch(accountID)
	if a.queue != nil {
		defer a.queue.Pause(accountID)()
	}
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if err := a.store.DeleteAccount(accountID); err != nil {
		return err
	}
	// The default sender must not outlive its account.
	if gs, err := a.store.GetGlobalSettings(); err == nil && gs.DefaultAccountID == accountID {
		gs.DefaultAccountID = ""
		return a.store.SetGlobalSettings(gs)
	}
	return nil
}

// RefetchAccount discards an account's local mail and syncs it again from
// scratch through the first-sync path. Unsent local changes are lost.
func (a *App) RefetchAccount(accountID string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	// Stop everything that could write this account's changes after the reset:
	// held trashes are dropped (their local state is wiped anyway) and the
	// queue is paused until the reset is done.
	if a.trash != nil {
		a.trash.DropPrefix(accountID + "/")
	}
	a.stopSyncPrefetch(accountID)
	// The reset waits on syncMu (held for a whole sync, possibly another
	// account's refetch) and on the queue drain, so it must not run on the
	// caller's (UI) goroutine or the window freezes.
	go func() {
		resume := func() {}
		if a.queue != nil {
			resume = a.queue.Pause(accountID)
		}
		a.syncMu.Lock()
		err := a.store.ResetAccountMail(accountID)
		a.syncMu.Unlock()
		resume()
		if err != nil {
			a.addAPIError(fmt.Sprintf("refetch %s: %v", accountID, err))
			return
		}
		a.emitEvent("inbox:updated", nil)
		_ = a.Sync(accountID)
	}()
	return nil
}

// RenameAccount sets the display name shown for an account.
func (a *App) RenameAccount(accountID, name string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	return a.store.UpdateAccountDisplayName(accountID, strings.TrimSpace(name))
}

// GetAccountSettings returns sync settings for an account.
func (a *App) GetAccountSettings(accountID string) (AccountSettingsItem, error) {
	if a.store == nil {
		return AccountSettingsItem{}, fmt.Errorf("store not initialized")
	}
	st, err := a.store.GetAccountSettings(accountID)
	if err != nil {
		return AccountSettingsItem{}, err
	}
	return AccountSettingsItem{
		AccountID:  st.AccountID,
		SyncPeriod: st.SyncPeriod,
	}, nil
}

// SetAccountSettings persists sync settings for an account.
func (a *App) SetAccountSettings(settings AccountSettingsItem) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	return a.store.UpsertAccountSettings(storage.AccountSettings{
		AccountID:  settings.AccountID,
		SyncPeriod: settings.SyncPeriod,
	})
}

// GetGlobalSettings returns app-wide settings.
func (a *App) GetGlobalSettings() (GlobalSettingsItem, error) {
	if a.store == nil {
		return GlobalSettingsItem{SyncFrequency: "15m"}, nil
	}
	gs, err := a.store.GetGlobalSettings()
	if err != nil {
		return GlobalSettingsItem{SyncFrequency: "15m"}, nil
	}
	if gs.DefaultAccountID != "" {
		if _, err := a.store.GetAccount(gs.DefaultAccountID); err != nil {
			gs.DefaultAccountID = "" // account was removed
		}
	}
	return GlobalSettingsItem{
		SyncFrequency:    gs.SyncFrequency,
		DefaultAccountID: gs.DefaultAccountID,
	}, nil
}

// SetGlobalSettings persists app-wide settings.
func (a *App) SetGlobalSettings(settings GlobalSettingsItem) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	return a.store.SetGlobalSettings(storage.GlobalSettings{
		SyncFrequency:    settings.SyncFrequency,
		DefaultAccountID: settings.DefaultAccountID,
	})
}

// GetInbox returns thread summaries for the inbox.
// Pass accountID="unified" or "" to get the unified inbox across all accounts.
func (a *App) GetInbox(accountID string, limit, offset int) ([]ThreadItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	if limit <= 0 {
		limit = 50
	}

	var summaries []storage.ThreadSummary
	var err error
	if accountID == "" || accountID == "unified" {
		summaries, err = a.store.ListAllThreadSummaries(limit, offset)
	} else {
		summaries, err = a.store.ListThreadSummaries(accountID, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	return threadSummariesToItems(summaries), nil
}

// GetLabelThreads returns thread summaries for a given Gmail label.
// Pass accountID="unified" or "" for all accounts.
func (a *App) GetLabelThreads(accountID, labelID string, limit, offset int) ([]ThreadItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	if limit <= 0 {
		limit = 60
	}
	var summaries []storage.ThreadSummary
	var err error
	if accountID == "" || accountID == "unified" {
		summaries, err = a.store.ListAllThreadSummariesByLabel(labelID, limit, offset)
	} else {
		summaries, err = a.store.ListThreadSummariesByLabel(accountID, labelID, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	return threadSummariesToItems(summaries), nil
}

// FilterThreads is GetInbox/GetLabelThreads/SearchInbox narrowed to threads that
// are unread, starred or have attachments (filter "unread", "starred" or
// "attachments"). A non-empty query searches instead of listing labelID; paging
// applies to the filtered result, so pages stay full.
func (a *App) FilterThreads(filter, accountID, labelID, query string, limit, offset int) ([]ThreadItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	if limit <= 0 {
		limit = 60
	}
	var summaries []storage.ThreadSummary
	var err error
	if query != "" {
		summaries, err = a.store.LocalSearchThreadSummariesFiltered(query, storage.ThreadFilter(filter), limit)
	} else {
		if accountID == "unified" {
			accountID = ""
		}
		summaries, err = a.store.ListThreadSummariesFiltered(accountID, labelID, storage.ThreadFilter(filter), limit, offset)
	}
	if err != nil {
		return nil, err
	}
	return threadSummariesToItems(summaries), nil
}

// SearchInbox searches threads by subject, snippet, or sender across all accounts.
func (a *App) SearchInbox(query string, limit int) ([]ThreadItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	if limit <= 0 {
		limit = 50
	}
	summaries, err := a.store.LocalSearchThreadSummaries(query, limit)
	if err != nil {
		return nil, err
	}
	return threadSummariesToItems(summaries), nil
}

// GetThread returns all messages in a thread, oldest first.
func (a *App) GetThread(accountID, threadID string) ([]MessageItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	msgs, err := a.store.ListMessagesByThread(accountID, threadID)
	if err != nil {
		return nil, err
	}
	if n := len(msgs); n > 0 && !msgs[n-1].HeadersKnown {
		// A Gmail round trip (up to 5s): never block opening the thread on it.
		fill := a.fillReplyHeaders
		if a.fillHeaders != nil {
			fill = a.fillHeaders
		}
		go fill(msgs[n-1])
	}
	items := make([]MessageItem, len(msgs))
	for i, m := range msgs {
		item := messageToItem(m)
		if m.HasAttachments {
			if atts, err := a.store.GetAttachmentsMeta(accountID, m.ID); err == nil {
				item.Attachments = attachmentsToItems(atts)
			}
		}
		items[i] = item
	}
	return items, nil
}

// fillReplyHeaders fetches the reply headers of a message synced before they
// were stored, so replying to it still threads and honours Reply-To. It is
// fail-soft: on any error the message is returned unchanged.
func (a *App) fillReplyHeaders(m storage.Message) storage.Message {
	if a.accountMgr() == nil {
		return m
	}
	client, err := a.gmailClient(m.AccountID)
	if err != nil {
		return m
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	meta, err := client.GetMessageMeta(ctx, m.ID)
	if err != nil {
		return m
	}
	m.ReplyTo, m.References, m.InReplyTo, m.HeadersKnown = meta.ReplyTo, meta.References, meta.InReplyTo, true
	_ = a.store.UpsertMessage(m)
	return m
}

// GetMessage returns a single message, lazily fetching the body from Gmail if not cached.
func (a *App) GetMessage(accountID, messageID string) (MessageItem, error) {
	if a.store == nil {
		return MessageItem{}, fmt.Errorf("store not initialised")
	}

	if a.accountMgr() != nil {
		client, err := a.gmailClient(accountID)
		if err == nil {
			svc := mail.NewInboxService(a.store, client)
			msg, err := svc.GetMessage(a.ctx, accountID, messageID)
			if err == nil {
				item := messageToItem(msg)
				if msg.HasAttachments {
					if atts, err := a.store.GetAttachmentsMeta(accountID, messageID); err == nil {
						item.Attachments = attachmentsToItems(atts)
					}
				}
				return item, nil
			}
		}
	}

	msg, err := a.store.GetMessage(accountID, messageID)
	if err != nil {
		return MessageItem{}, err
	}
	item := messageToItem(msg)
	if msg.HasAttachments {
		if atts, err := a.store.GetAttachmentsMeta(accountID, messageID); err == nil {
			item.Attachments = attachmentsToItems(atts)
		}
	}
	return item, nil
}

// GetAttachmentData returns the binary content of an attachment.
// Serves from SQLite cache when available; falls back to the Gmail API and
// writes the result back to cache so subsequent requests are served locally.
func (a *App) GetAttachmentData(accountID, messageID, attachmentID string) (AttachmentDataItem, error) {
	if a.store == nil {
		return AttachmentDataItem{}, fmt.Errorf("store not initialised")
	}
	var src mail.AttachmentSource
	if a.accountMgr() != nil && !strings.HasPrefix(attachmentID, "name:") {
		// A client error only matters on a cache miss; Get ignores src on a hit.
		if client, err := a.gmailClient(accountID); err == nil {
			src = client
		}
	}
	if src == nil {
		src = noSource{}
	}
	data, mimeType, err := a.attachments.Get(a.ctx, src, accountID, messageID, attachmentID)
	if err != nil {
		return AttachmentDataItem{}, err
	}
	return AttachmentDataItem{Data: data, MimeType: mimeType}, nil
}

// noSource stands in when no Gmail client is available.
type noSource struct{}

func (noSource) GetAttachmentData(context.Context, string, string) ([]byte, error) {
	return nil, fmt.Errorf("attachment not cached and no account credentials available")
}

// SendMessage sends an outbound email.
func (a *App) SendMessage(msg OutboundMsg) error {
	if a.accountMgr() == nil {
		return fmt.Errorf("no accounts configured")
	}
	acc, err := a.store.GetAccount(msg.AccountID)
	if err != nil {
		return err
	}
	client, err := a.gmailClient(msg.AccountID)
	if err != nil {
		return err
	}
	svc := mail.NewSendService(client)
	_, err = svc.Send(a.ctx, mail.OutboundMessage{
		From:       acc.Email,
		To:         msg.To,
		Cc:         msg.Cc,
		Bcc:        msg.Bcc,
		Subject:    msg.Subject,
		Body:       msg.Body,
		InReplyTo:  msg.InReplyTo,
		References: msg.References,
		ThreadID:   msg.ThreadID,

		Attachments: toMailAttachments(msg.Attachments),
	})
	return err
}

// SaveDraft autosaves a compose window to the local Drafts store (no network);
// it syncs to Gmail on the next queue drain. localID is empty on the first
// save; the returned ID identifies the same draft on later saves.
func (a *App) SaveDraft(localID string, msg OutboundMsg) (string, error) {
	acc, err := a.store.GetAccount(msg.AccountID)
	if err != nil {
		return "", err
	}
	if localID == "" {
		localID = fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	return localID, mail.SaveLocalDraft(a.store, localID, msg.AccountID, mail.OutboundMessage{
		From: acc.Email, To: msg.To, Cc: msg.Cc, Bcc: msg.Bcc, Subject: msg.Subject,
		Body: msg.Body, InReplyTo: msg.InReplyTo, References: msg.References, ThreadID: msg.ThreadID,
		Attachments: toMailAttachments(msg.Attachments),
	})
}

// OpenDraft prepares a synced Gmail draft message for editing. It finds the
// Gmail draft behind the message and links it to a local draft, so later
// autosaves update that same draft instead of creating a new one.
func (a *App) OpenDraft(accountID, messageID string) (OutboundMsg, error) {
	m, err := a.GetMessage(accountID, messageID)
	if err != nil {
		return OutboundMsg{}, err
	}
	acc, err := a.store.GetAccount(accountID)
	if err != nil {
		return OutboundMsg{}, err
	}
	client, err := a.gmailClient(accountID)
	if err != nil {
		return OutboundMsg{}, err
	}
	remoteID, inReplyTo, references, err := client.FindDraft(a.ctx, messageID)
	if err != nil {
		return OutboundMsg{}, err
	}
	out := draftOutbound(m, "", inReplyTo)
	out.References = references
	for _, at := range m.Attachments {
		d, err := a.GetAttachmentData(accountID, messageID, at.AttachmentID)
		if err != nil {
			a.addAPIError(fmt.Sprintf("draft attachment %s: %v", at.Name, err))
			continue
		}
		out.Attachments = append(out.Attachments, OutAttachment{Filename: at.Name, MIMEType: at.MimeType, Size: int64(len(d.Data)), Data: d.Data})
	}
	localID, err := mail.LinkRemoteDraft(a.store, accountID, remoteID, mail.OutboundMessage{
		From: acc.Email, To: m.ToAddr, Cc: m.CcAddr, Bcc: m.BccAddr, Subject: m.Subject,
		Body: m.Body, InReplyTo: inReplyTo, References: references, ThreadID: m.ThreadID,
		Attachments: toMailAttachments(out.Attachments),
	})
	if err != nil {
		return OutboundMsg{}, err
	}
	out.DraftID = localID
	return out, nil
}

// DiscardDraft deletes an autosaved draft (after sending or discarding).
func (a *App) DiscardDraft(accountID, localID string) error {
	return mail.DiscardLocalDraft(a.store, accountID, localID)
}

// MarkMessageRead removes the UNREAD label from a message. The change is
// applied locally at once and sent to Gmail in the background.
func (a *App) MarkMessageRead(accountID, messageID string) error {
	return a.enqueue(storage.OpMarkRead, accountID, messageID)
}

// MarkMessageUnread adds the UNREAD label to a message (see MarkMessageRead).
func (a *App) MarkMessageUnread(accountID, messageID string) error {
	return a.enqueue(storage.OpMarkUnread, accountID, messageID)
}

// TrashThread hides a thread immediately (local TRASH label) and queues the
// Gmail deletion once the undo window has passed; UndoTrashThread cancels it.
func (a *App) TrashThread(accountID, threadID string) error {
	if a.queue == nil || a.trash == nil {
		return fmt.Errorf("store not initialised")
	}
	inbox, err := a.store.MessageIDsWithLabel(accountID, threadID, mail.LabelInbox)
	if err != nil {
		return err
	}
	if err := a.store.ModifyLabels(accountID, threadID, true, []string{mail.LabelTrash}, []string{mail.LabelInbox}); err != nil {
		return err
	}
	a.trash.Hold(trashKey(accountID, threadID),
		func() { _ = a.enqueue(storage.OpTrashThread, accountID, threadID) },
		func() {
			_ = a.store.ModifyLabels(accountID, threadID, true, nil, []string{mail.LabelTrash})
			for _, id := range inbox {
				_ = a.store.ModifyLabels(accountID, id, false, []string{mail.LabelInbox}, nil)
			}
			a.emitEvent("inbox:updated", nil)
		})
	a.emitEvent("inbox:updated", nil)
	return nil
}

func trashKey(accountID, threadID string) string { return accountID + "/" + threadID }

// trashHeld reports whether a thread's trash is still inside its undo window.
func (a *App) trashHeld(accountID, threadID string) bool {
	return a.trash != nil && a.trash.IsHeld(trashKey(accountID, threadID))
}

// UndoTrashThread restores a trashed thread while its undo window is open.
func (a *App) UndoTrashThread(accountID, threadID string) error {
	if a.trash == nil || !a.trash.Undo(trashKey(accountID, threadID)) {
		return fmt.Errorf("too late to undo")
	}
	return nil
}

// ArchiveThread removes the Inbox label from all messages in a thread.
func (a *App) ArchiveThread(accountID, threadID string) error {
	return a.enqueue(storage.OpArchive, accountID, threadID)
}

// MarkThreadSpam moves all messages in a thread to Spam and removes them from Inbox.
func (a *App) MarkThreadSpam(accountID, threadID string) error {
	return a.enqueue(storage.OpMarkSpam, accountID, threadID)
}

// MarkThreadNotSpam moves all messages in a thread from Spam to Inbox.
func (a *App) MarkThreadNotSpam(accountID, threadID string) error {
	return a.enqueue(storage.OpMarkNotSpam, accountID, threadID)
}

// SetThreadLabel adds (on) or removes a label such as STARRED or IMPORTANT on
// every message in a thread.
func (a *App) SetThreadLabel(accountID, threadID, labelID string, on bool) error {
	op := storage.OpRemoveLabel
	if on {
		op = storage.OpAddLabel
	}
	return a.enqueue(op+labelID, accountID, threadID)
}

// enqueue applies a mutation locally and queues it for Gmail; it never waits on the network.
func (a *App) enqueue(opType, accountID, targetID string) error {
	if a.queue == nil {
		return fmt.Errorf("store not initialised")
	}
	if err := a.queue.Enqueue(opType, accountID, targetID); err != nil {
		return err
	}
	a.emitEvent("inbox:updated", nil)
	return nil
}

// applyPendingOp sends one queued operation to Gmail.
func (a *App) applyPendingOp(ctx context.Context, op storage.PendingOp) error {
	if a.accountMgr() == nil {
		return fmt.Errorf("no accounts configured")
	}
	client, err := a.gmailClient(op.AccountID)
	if err != nil {
		return err
	}
	if mail.IsDraftOp(op.OpType) {
		return mail.ApplyDraftOp(ctx, a.store, client, op)
	}
	return mail.ApplyOpRemote(ctx, client, op)
}

// rollbackOp reconciles local state with Gmail after an operation was abandoned.
func (a *App) rollbackOp(op storage.PendingOp, cause error) {
	a.addAPIError(fmt.Sprintf("%s failed and was undone: %v", op.OpType, cause))
	if mail.IsDraftOp(op.OpType) {
		return
	}
	go func() {
		client, err := a.gmailClient(op.AccountID)
		if err != nil {
			return
		}
		mgr := mailsync.NewManager(a.store, client)
		if mail.OpIsThread(op.OpType) {
			err = mgr.RefreshThread(a.ctx, op.AccountID, op.MessageID)
		} else {
			err = mgr.RefreshMessage(a.ctx, op.AccountID, op.MessageID)
		}
		if err == nil {
			a.emitEvent("inbox:updated", nil)
		}
	}()
}

// DrainPendingOps replays queued operations against Gmail now.
func (a *App) DrainPendingOps() {
	if a.queue != nil {
		a.queue.Drain(a.ctx)
	}
}

// Sync runs an incremental sync. Pass accountID="unified" to sync all accounts.
// Syncs are serialised, and so are the accounts within one sync (bounds peak memory).
func (a *App) Sync(accountID string) error {
	if a.accountMgr() == nil {
		a.emitEvent("sync:complete", nil) // clear any "Syncing…" a client already shows
		return fmt.Errorf("no accounts configured")
	}
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	// Flush local changes first so a sync cannot overwrite them with stale state.
	a.DrainPendingOps()
	accs, err := a.store.ListAccounts()
	if err != nil {
		a.emitEvent("sync:complete", nil)
		return err
	}
	for _, acc := range accs {
		if accountID != "" && accountID != "unified" && acc.ID != accountID {
			continue
		}
		a.syncAccount(acc)
	}
	a.emitEvent("sync:complete", nil)
	a.emitEvent("inbox:updated", nil)
	defer debug.FreeOSMemory()
	for _, acc := range accs {
		if accountID == "" || accountID == "unified" || acc.ID == accountID {
			a.prefetchSyncedAttachments(acc.ID)
			go a.syncContacts(acc.ID, acc.Email)
		}
	}
	return nil
}

func (a *App) syncAccount(acc storage.Account) {
	client, err := a.gmailClient(acc.ID)
	if err != nil {
		a.addAPIError(fmt.Sprintf("sync client %s: %v", acc.Email, err))
		return
	}
	a.emitEvent("sync:progress", map[string]string{"account": acc.Email})
	go a.refreshSendAs(acc, client)
	mgr := mailsync.NewManager(a.store, client)
	mgr.SetHeld(a.trashHeld)
	stored, _ := a.store.GetAccount(acc.ID)
	if stored.HistoryID == "" {
		a.initialSync(mgr, acc)
		return
	}
	err = mgr.IncrementalSync(a.ctx, acc.ID)
	if errors.Is(err, gmail.ErrHistoryExpired) {
		_ = a.store.UpdateHistoryID(acc.ID, "")
		a.initialSync(mgr, acc)
		return
	}
	if err != nil {
		a.addAPIError(fmt.Sprintf("sync %s: %v", acc.Email, err))
		return
	}
	// Labels whose sync scope changed (or never ran) are filled in now.
	configs, _ := a.store.ListSyncConfigs(acc.ID)
	if err := mgr.BackfillFromConfigs(a.ctx, acc.ID, configs); err != nil {
		a.addAPIError(fmt.Sprintf("backfill %s: %v", acc.Email, err))
	}
	// Fetch bodies + attachment binaries for messages added by the incremental sync.
	if err := mgr.FetchBodies(a.ctx, acc.ID, mailsync.BodyFetchLimit); err != nil {
		a.addAPIError(fmt.Sprintf("fetch bodies %s: %v", acc.Email, err))
	}
}

// prefetchSyncedAttachments caches, in the background, the attachments still
// missing for mail stored by sync (any label). It covers new messages
// and backfills older ones left uncached. It returns at once; a run already in
// progress for the account is left alone and the next sync picks up the rest.
func (a *App) prefetchSyncedAttachments(accountID string) {
	ctx, cancel := context.WithCancel(a.ctx)
	a.syncPrefetchMu.Lock()
	if _, running := a.syncPrefetchCancel[accountID]; running {
		a.syncPrefetchMu.Unlock()
		cancel()
		return
	}
	if a.syncPrefetchCancel == nil {
		a.syncPrefetchCancel = map[string]context.CancelFunc{}
	}
	a.syncPrefetchCancel[accountID] = cancel
	a.syncPrefetchMu.Unlock()

	go func() {
		defer func() {
			cancel()
			a.syncPrefetchMu.Lock()
			delete(a.syncPrefetchCancel, accountID)
			a.syncPrefetchMu.Unlock()
		}()
		client, err := a.gmailClient(accountID)
		if err != nil {
			a.logWarning(fmt.Sprintf("attachment sync prefetch: %v", err))
			return
		}
		_ = a.attachments.PrefetchSynced(ctx, client, accountID, mailsync.BodyFetchLimit, func(err error) {
			a.logWarning(fmt.Sprintf("attachment sync prefetch: %v", err))
		})
	}()
}

// stopSyncPrefetch cancels an account's background attachment caching.
func (a *App) stopSyncPrefetch(accountID string) {
	a.syncPrefetchMu.Lock()
	if cancel := a.syncPrefetchCancel[accountID]; cancel != nil {
		cancel()
	}
	a.syncPrefetchMu.Unlock()
}

func (a *App) initialSync(mgr *mailsync.Manager, acc storage.Account) {
	st, _ := a.store.GetAccountSettings(acc.ID)
	configs, _ := a.store.ListSyncConfigs(acc.ID)
	if err := mgr.InitialSync(a.ctx, acc.ID, syncPeriodToDays(st.SyncPeriod), configs); err != nil {
		a.addAPIError(fmt.Sprintf("initial sync %s: %v", acc.Email, err))
	}
}

// olderBatch is how many older messages one scroll-to-load-more request fetches.
const olderBatch = mailsync.OlderBatch

// LoadOlder fetches mail older than what is stored locally for a label and
// returns how many messages were added. Pass accountID="unified" or "" for all accounts.
func (a *App) LoadOlder(accountID, labelID string) (int, error) {
	if a.store == nil || a.accountMgr() == nil {
		return 0, nil
	}
	accs, err := a.store.ListAccounts()
	if err != nil {
		return 0, err
	}
	total := 0
	var firstErr error
	for _, acc := range accs {
		if accountID != "" && accountID != "unified" && acc.ID != accountID {
			continue
		}
		client, err := a.gmailClient(acc.ID)
		if err == nil {
			var n int
			mgr := mailsync.NewManager(a.store, client)
			mgr.SetHeld(a.trashHeld)
			n, err = mgr.LoadOlder(a.ctx, acc.ID, labelID, olderBatch)
			total += n
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return total, firstErr
}

// GetSyncConfigs returns the effective per-label sync scope for an account:
// stored choices, or the defaults (Inbox and Starred sync everything; other
// labels use the account's sync period).
func (a *App) GetSyncConfigs(accountID string) ([]SyncConfigItem, error) {
	if a.store == nil {
		return nil, nil
	}
	labels, err := a.store.ListLabels(accountID)
	if err != nil {
		return nil, err
	}
	configs, err := a.store.ListSyncConfigs(accountID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]storage.SyncConfig, len(configs))
	for _, c := range configs {
		byID[c.LabelID] = c
	}
	st, _ := a.store.GetAccountSettings(accountID)
	defaultDays := syncPeriodToDays(st.SyncPeriod)
	var out []SyncConfigItem
	for _, l := range labels {
		if !syncableLabel(l) {
			continue
		}
		c, ok := byID[l.ID]
		if !ok {
			c = storage.SyncConfig{DaysToSync: defaultDays}
			if l.ID == "INBOX" || l.ID == "STARRED" || l.ID == storage.AllMailLabelID {
				c.DaysToSync = mailsync.AllMailDefaultDays
			}
		}
		item := SyncConfigItem{LabelID: l.ID, LabelName: l.Name, Mode: SyncModeAll}
		switch {
		case c.MaxEmails > 0:
			item.Mode, item.Value = SyncModeCount, c.MaxEmails
		case c.DaysToSync > 0:
			item.Mode, item.Value = SyncModeDays, c.DaysToSync
		}
		out = append(out, item)
	}
	return out, nil
}

// syncableLabel limits the settings list to labels users browse.
func syncableLabel(l storage.Label) bool {
	if l.LabelType == "user" {
		return true
	}
	switch l.ID {
	case "INBOX", "STARRED", "SENT", "IMPORTANT", "DRAFT", "SPAM", "TRASH", storage.AllMailLabelID:
		return true
	}
	return false
}

// maxSyncValue caps a label's days / message count.
const maxSyncValue = 1_000_000

// SetSyncConfig persists one label's sync scope and starts a sync to honour it.
func (a *App) SetSyncConfig(accountID string, item SyncConfigItem) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	if item.LabelID == "" {
		return fmt.Errorf("label is required")
	}
	cfg := storage.SyncConfig{
		LabelID:   item.LabelID,
		AccountID: accountID,
		LabelName: item.LabelName,
	}
	switch item.Mode {
	case SyncModeAll:
	case SyncModeDays:
		cfg.DaysToSync = min(max(item.Value, 1), maxSyncValue)
	case SyncModeCount:
		cfg.MaxEmails = min(max(item.Value, 1), maxSyncValue)
	default:
		return fmt.Errorf("unknown sync mode %q", item.Mode)
	}
	if err := a.store.UpsertSyncConfig(cfg); err != nil {
		return err
	}
	if a.accountMgr() != nil {
		go func() { _ = a.Sync(accountID) }()
	}
	return nil
}

// contactsSyncEvery bounds how often the full People API list is re-fetched.
const contactsSyncEvery = time.Hour

// syncContacts refreshes an account's Google contacts for compose autocomplete.
// Failures (for example a token granted without the contacts scope) are logged as
// warnings only: mail works without contacts.
func (a *App) syncContacts(accountID, email string) {
	a.contactsMu.Lock()
	if a.contactsSynced == nil {
		a.contactsSynced = map[string]time.Time{}
	}
	if time.Since(a.contactsSynced[accountID]) < contactsSyncEvery {
		a.contactsMu.Unlock()
		return
	}
	a.contactsSynced[accountID] = time.Now()
	a.contactsMu.Unlock()
	httpClient, err := a.accountMgr().GetClient(a.ctx, accountID)
	if err == nil {
		var svc *contacts.Service
		if svc, err = contacts.NewService(a.ctx, a.store, accountID, httpClient); err == nil {
			err = svc.Sync(a.ctx)
		}
	}
	if err != nil {
		a.logWarning(fmt.Sprintf("contacts sync %s: %v", email, err))
	}
}

// SearchContacts returns contacts matching the query string, prefix matches first
// (see rankContacts). Pass accountID="unified" or "" to search across all accounts.
func (a *App) SearchContacts(accountID, query string, limit int) ([]ContactItem, error) {
	if a.store == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 8
	}
	var contacts []storage.Contact
	var err error
	if accountID == "" || accountID == "unified" {
		contacts, err = a.store.SearchAllContacts(query, searchPool)
	} else {
		contacts, err = a.store.SearchContacts(accountID, query, searchPool)
	}
	if err != nil {
		return nil, err
	}
	items := contactsToItems(contacts)
	return rankContacts(append(items, a.otherContactMatches(accountID, query, items)...), query, limit), nil
}

// otherContactMatches are the addresses seen in recent mail that match query
// but are not saved contacts (Google's "Other contacts"), most frequent first.
func (a *App) otherContactMatches(accountID, query string, saved []ContactItem) []ContactItem {
	headers, err := a.store.RecentAddressHeaders(accountID, 3000)
	if err != nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	have := map[string]bool{}
	for _, c := range saved {
		have[strings.ToLower(c.Email)] = true
	}
	var out []ContactItem
	for _, c := range countAddresses(headers, 0) {
		if !have[strings.ToLower(c.Email)] && strings.Contains(strings.ToLower(c.Name+"\n"+c.Email), q) {
			out = append(out, c)
		}
	}
	return out
}

// GetLabels returns all labels for an account.
func (a *App) GetLabels(accountID string) ([]LabelItem, error) {
	if a.store == nil {
		return nil, nil
	}
	labels, err := a.store.ListLabels(accountID)
	if err != nil {
		return nil, err
	}
	return labelsToItems(labels), nil
}

// CreateLabel creates a user label in Gmail and stores it locally.
func (a *App) CreateLabel(accountID, name string) (LabelItem, error) {
	if a.accountMgr() == nil || a.store == nil {
		return LabelItem{}, fmt.Errorf("no accounts configured")
	}
	client, err := a.gmailClient(accountID)
	if err != nil {
		return LabelItem{}, err
	}
	l, err := client.CreateLabel(a.ctx, name)
	if err != nil {
		return LabelItem{}, err
	}
	if err := a.store.UpsertLabel(storage.Label{ID: l.ID, AccountID: accountID, Name: l.Name, LabelType: "user"}); err != nil {
		return LabelItem{}, err
	}
	return LabelItem{ID: l.ID, Name: l.Name, Type: "user"}, nil
}

// GetLabelUnreadCounts returns unread thread counts per label for an account.
// Pass accountID "unified" or "" to aggregate across all accounts.
func (a *App) GetLabelUnreadCounts(accountID string) (map[string]int, error) {
	if a.store == nil {
		return nil, nil
	}
	if accountID == "" || accountID == "unified" {
		accounts, err := a.store.ListAccounts()
		if err != nil {
			return nil, err
		}
		total := map[string]int{}
		for _, acc := range accounts {
			counts, err := a.store.GetLabelUnreadCounts(acc.ID)
			if err != nil {
				continue
			}
			for k, v := range counts {
				total[k] += v
			}
		}
		return total, nil
	}
	return a.store.GetLabelUnreadCounts(accountID)
}

// gmailClient returns an authenticated gmail.Client for the given account ID.
func (a *App) gmailClient(accountID string) (*gmail.Client, error) {
	httpClient, err := a.accountMgr().GetClient(a.ctx, accountID)
	if err != nil {
		return nil, err
	}
	return gmail.NewClient(a.ctx, httpClient, accountID)
}

func syncPeriodToDays(period string) int {
	switch period {
	case "1y":
		return 365
	case "90d":
		return 90
	case "all":
		return 0
	default:
		return 30
	}
}

// dataDir returns (and creates if needed) $MAILRAT_DATA_DIR, else ~/.mailrat
func dataDir() (string, error) {
	dir := os.Getenv("MAILRAT_DATA_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".mailrat")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

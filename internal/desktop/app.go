package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
	mailsync "github.com/mailrat/mailrat/internal/sync"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application struct. All exported methods are bound to the frontend.
type App struct {
	ctx      context.Context
	store    *storage.Store
	accounts *accounts.Manager
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// OnStartup initialises storage and the account manager when the app starts.
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	dir, err := dataDir()
	if err != nil {
		runtime.LogError(ctx, "data dir: "+err.Error())
		return
	}

	store, err := storage.Open(filepath.Join(dir, "mailrat.db"))
	if err != nil {
		runtime.LogError(ctx, "open db: "+err.Error())
		return
	}
	a.store = store

	credPath := filepath.Join(dir, "credentials.json")
	mgr, err := accounts.NewManager(store, credPath)
	if err != nil {
		runtime.LogWarning(ctx, "no credentials: "+err.Error())
		return
	}
	a.accounts = mgr
}

// OnShutdown closes the database when the app exits.
func (a *App) OnShutdown(_ context.Context) {
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
	out := make([]AccountItem, len(accs))
	for i, acc := range accs {
		out[i] = AccountItem{ID: acc.ID, Email: acc.Email}
	}
	return out, nil
}

// GetInbox returns thread summaries for the inbox.
// Pass accountID="unified" or "" to get the unified inbox across all accounts.
func (a *App) GetInbox(accountID string, limit int) ([]ThreadItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	if limit <= 0 {
		limit = 50
	}

	var summaries []storage.ThreadSummary
	var err error
	if accountID == "" || accountID == "unified" {
		summaries, err = a.store.ListAllThreadSummaries(limit)
	} else {
		summaries, err = a.store.ListThreadSummaries(accountID, limit)
	}
	if err != nil {
		return nil, err
	}

	out := make([]ThreadItem, len(summaries))
	for i, s := range summaries {
		out[i] = threadSummaryToItem(s)
	}
	return out, nil
}

// SearchInbox searches threads by subject, snippet, or sender across all accounts.
func (a *App) SearchInbox(query string, limit int) ([]ThreadItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialised")
	}
	if limit <= 0 {
		limit = 50
	}
	summaries, err := a.store.SearchAllThreadSummaries(query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ThreadItem, len(summaries))
	for i, s := range summaries {
		out[i] = threadSummaryToItem(s)
	}
	return out, nil
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
	out := make([]MessageItem, len(msgs))
	for i, m := range msgs {
		out[i] = messageToItem(m)
	}
	return out, nil
}

// GetMessage returns a single message, lazily fetching the body from Gmail if not cached.
func (a *App) GetMessage(accountID, messageID string) (MessageItem, error) {
	if a.store == nil {
		return MessageItem{}, fmt.Errorf("store not initialised")
	}

	if a.accounts != nil {
		client, err := a.gmailClient(accountID)
		if err == nil {
			svc := mail.NewInboxService(a.store, client)
			msg, err := svc.GetMessage(a.ctx, accountID, messageID)
			if err == nil {
				return messageToItem(msg), nil
			}
		}
	}

	msg, err := a.store.GetMessage(accountID, messageID)
	if err != nil {
		return MessageItem{}, err
	}
	return messageToItem(msg), nil
}

// SendMessage sends an outbound email.
func (a *App) SendMessage(msg OutboundMsg) error {
	if a.accounts == nil {
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
		From:      acc.Email,
		To:        msg.To,
		Subject:   msg.Subject,
		Body:      msg.Body,
		InReplyTo: msg.InReplyTo,
		ThreadID:  msg.ThreadID,
	})
	return err
}

// Sync runs an incremental sync. Pass accountID="unified" to sync all accounts.
func (a *App) Sync(accountID string) error {
	if a.accounts == nil {
		return fmt.Errorf("no accounts configured")
	}
	accs, err := a.store.ListAccounts()
	if err != nil {
		return err
	}
	for _, acc := range accs {
		if accountID != "" && accountID != "unified" && acc.ID != accountID {
			continue
		}
		client, err := a.gmailClient(acc.ID)
		if err != nil {
			runtime.LogWarning(a.ctx, "sync client "+acc.Email+": "+err.Error())
			continue
		}
		runtime.EventsEmit(a.ctx, "sync:progress", map[string]string{"account": acc.Email})
		mgr := mailsync.NewManager(a.store, client)
		if err := mgr.IncrementalSync(a.ctx, acc.ID); err != nil {
			runtime.LogWarning(a.ctx, "sync "+acc.Email+": "+err.Error())
		}
	}
	runtime.EventsEmit(a.ctx, "sync:complete", nil)
	runtime.EventsEmit(a.ctx, "inbox:updated", nil)
	return nil
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
	out := make([]LabelItem, len(labels))
	for i, l := range labels {
		out[i] = LabelItem{ID: l.ID, Name: l.Name, Type: l.LabelType}
	}
	return out, nil
}

// gmailClient returns an authenticated gmail.Client for the given account ID.
func (a *App) gmailClient(accountID string) (*gmail.Client, error) {
	httpClient, err := a.accounts.GetClient(a.ctx, accountID)
	if err != nil {
		return nil, err
	}
	return gmail.NewClient(a.ctx, httpClient, accountID)
}

// dataDir returns (and creates if needed) ~/.mailrat
func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mailrat")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

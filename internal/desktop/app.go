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
	isWails  bool
	store    *storage.Store
	accounts *accounts.Manager
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// OnStartup is called by Wails on desktop startup.
func (a *App) OnStartup(ctx context.Context) {
	a.isWails = true
	a.startup(ctx)
}

// StartWeb initialises the app for use in the HTTP web server (no Wails runtime).
func (a *App) StartWeb(ctx context.Context) {
	a.isWails = false
	a.startup(ctx)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	dir, err := dataDir()
	if err != nil {
		a.logError("data dir: " + err.Error())
		return
	}

	store, err := storage.Open(filepath.Join(dir, "mailrat.db"))
	if err != nil {
		a.logError("open db: " + err.Error())
		return
	}
	a.store = store

	credPath := filepath.Join(dir, "credentials.json")
	mgr, err := accounts.NewManager(store, credPath)
	if err != nil {
		a.logWarning("no credentials: " + err.Error())
		return
	}
	a.accounts = mgr
}

// emitEvent sends a Wails runtime event only when running inside the desktop app.
func (a *App) emitEvent(name string, data any) {
	if a.isWails {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

func (a *App) logError(msg string) {
	if a.isWails {
		runtime.LogError(a.ctx, msg)
	} else {
		fmt.Fprintln(os.Stderr, "ERROR:", msg)
	}
}

func (a *App) logWarning(msg string) {
	if a.isWails {
		runtime.LogWarning(a.ctx, msg)
	} else {
		fmt.Fprintln(os.Stderr, "WARN:", msg)
	}
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
	return accountsToItems(accs), nil
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
	return messagesToItems(msgs), nil
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
		Cc:        msg.Cc,
		Bcc:       msg.Bcc,
		Subject:   msg.Subject,
		Body:      msg.Body,
		InReplyTo: msg.InReplyTo,
		ThreadID:  msg.ThreadID,
	})
	return err
}

// MarkMessageRead removes the UNREAD label from a message.
// Optimistically updates SQLite first; queues the Gmail API call if offline.
func (a *App) MarkMessageRead(accountID, messageID string) error {
	if a.store != nil {
		if msg, err := a.store.GetMessage(accountID, messageID); err == nil {
			msg.IsUnread = false
			_ = a.store.UpsertMessage(msg)
		}
	}
	client, err := a.gmailClient(accountID)
	if err != nil {
		_ = a.store.AddPendingOp(storage.OpMarkRead, accountID, messageID)
		return nil
	}
	if err := client.ModifyLabels(a.ctx, messageID, nil, []string{mail.LabelUnread}); err != nil {
		_ = a.store.AddPendingOp(storage.OpMarkRead, accountID, messageID)
	}
	return nil
}

// MarkMessageUnread adds the UNREAD label to a message.
// Optimistically updates SQLite first; queues the Gmail API call if offline.
func (a *App) MarkMessageUnread(accountID, messageID string) error {
	if a.store != nil {
		if msg, err := a.store.GetMessage(accountID, messageID); err == nil {
			msg.IsUnread = true
			_ = a.store.UpsertMessage(msg)
		}
	}
	client, err := a.gmailClient(accountID)
	if err != nil {
		_ = a.store.AddPendingOp(storage.OpMarkUnread, accountID, messageID)
		return nil
	}
	if err := client.ModifyLabels(a.ctx, messageID, []string{mail.LabelUnread}, nil); err != nil {
		_ = a.store.AddPendingOp(storage.OpMarkUnread, accountID, messageID)
	}
	return nil
}

// TrashThread moves all messages in a thread to the Trash.
// If offline, queues the operation for replay on the next sync.
func (a *App) TrashThread(accountID, threadID string) error {
	client, err := a.gmailClient(accountID)
	if err != nil {
		_ = a.store.AddPendingOp(storage.OpTrashThread, accountID, threadID)
		return nil
	}
	msgs, err := a.store.ListMessagesByThread(accountID, threadID)
	if err != nil {
		return err
	}
	svc := mail.NewLabelService(a.store, client)
	for _, msg := range msgs {
		if err := svc.Trash(a.ctx, accountID, msg.ID); err != nil {
			_ = a.store.AddPendingOp(storage.OpTrashThread, accountID, threadID)
			return nil
		}
	}
	a.emitEvent("inbox:updated", nil)
	return nil
}

// MarkThreadSpam moves all messages in a thread to Spam and removes from Inbox.
// If offline, queues the operation for replay on the next sync.
func (a *App) MarkThreadSpam(accountID, threadID string) error {
	client, err := a.gmailClient(accountID)
	if err != nil {
		_ = a.store.AddPendingOp(storage.OpMarkSpam, accountID, threadID)
		return nil
	}
	msgs, err := a.store.ListMessagesByThread(accountID, threadID)
	if err != nil {
		return err
	}
	svc := mail.NewLabelService(a.store, client)
	for _, msg := range msgs {
		if err := svc.MarkAsSpam(a.ctx, accountID, msg.ID); err != nil {
			_ = a.store.AddPendingOp(storage.OpMarkSpam, accountID, threadID)
			return nil
		}
	}
	a.emitEvent("inbox:updated", nil)
	return nil
}

// MarkThreadNotSpam moves all messages in a thread from Spam to Inbox.
// If offline, queues the operation for replay on the next sync.
func (a *App) MarkThreadNotSpam(accountID, threadID string) error {
	client, err := a.gmailClient(accountID)
	if err != nil {
		_ = a.store.AddPendingOp(storage.OpMarkNotSpam, accountID, threadID)
		return nil
	}
	msgs, err := a.store.ListMessagesByThread(accountID, threadID)
	if err != nil {
		return err
	}
	svc := mail.NewLabelService(a.store, client)
	for _, msg := range msgs {
		if err := svc.MarkAsNotSpam(a.ctx, accountID, msg.ID); err != nil {
			_ = a.store.AddPendingOp(storage.OpMarkNotSpam, accountID, threadID)
			return nil
		}
	}
	a.emitEvent("inbox:updated", nil)
	return nil
}

// applyPendingOp applies a single pending operation via the Gmail API.
func (a *App) applyPendingOp(ctx context.Context, op storage.PendingOp, client *gmail.Client) error {
	switch op.OpType {
	case storage.OpMarkRead:
		return client.ModifyLabels(ctx, op.MessageID, nil, []string{mail.LabelUnread})
	case storage.OpMarkUnread:
		return client.ModifyLabels(ctx, op.MessageID, []string{mail.LabelUnread}, nil)
	case storage.OpTrashThread:
		msgs, err := a.store.ListMessagesByThread(op.AccountID, op.MessageID)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := client.TrashMessage(ctx, msg.ID); err != nil {
				return err
			}
		}
	case storage.OpMarkSpam:
		msgs, err := a.store.ListMessagesByThread(op.AccountID, op.MessageID)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := client.ModifyLabels(ctx, msg.ID, []string{mail.LabelSpam}, []string{mail.LabelInbox}); err != nil {
				return err
			}
		}
	case storage.OpMarkNotSpam:
		msgs, err := a.store.ListMessagesByThread(op.AccountID, op.MessageID)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := client.ModifyLabels(ctx, msg.ID, []string{mail.LabelInbox}, []string{mail.LabelSpam}); err != nil {
				return err
			}
		}
	}
	return nil
}

// DrainPendingOps replays queued offline operations against the Gmail API.
// Called after a successful sync when connectivity is confirmed.
func (a *App) DrainPendingOps() {
	if a.store == nil || a.accounts == nil {
		return
	}
	ops, err := a.store.ListPendingOps()
	if err != nil || len(ops) == 0 {
		return
	}
	ctx := a.ctx
	clients := make(map[string]*gmail.Client)
	for _, op := range ops {
		client, ok := clients[op.AccountID]
		if !ok {
			client, err = a.gmailClient(op.AccountID)
			if err != nil {
				continue
			}
			clients[op.AccountID] = client
		}
		if err := a.applyPendingOp(ctx, op, client); err == nil {
			_ = a.store.DeletePendingOp(op.ID)
		}
	}
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
			a.logWarning("sync client " + acc.Email + ": " + err.Error())
			continue
		}
		a.emitEvent("sync:progress", map[string]string{"account": acc.Email})
		mgr := mailsync.NewManager(a.store, client)
		if err := mgr.IncrementalSync(a.ctx, acc.ID); err != nil {
			a.logWarning("sync " + acc.Email + ": " + err.Error())
		}
	}
	a.emitEvent("sync:complete", nil)
	a.emitEvent("inbox:updated", nil)
	return nil
}

// SearchContacts returns contacts matching the query string (name or email prefix).
// Pass accountID="unified" or "" to search across all accounts.
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
		contacts, err = a.store.SearchAllContacts(query, limit)
	} else {
		contacts, err = a.store.SearchContacts(accountID, query, limit)
	}
	if err != nil {
		return nil, err
	}
	return contactsToItems(contacts), nil
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

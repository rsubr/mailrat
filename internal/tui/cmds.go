package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
	mailsync "github.com/mailrat/mailrat/internal/sync"
)

// tea.Msg types

type msgAccountsLoaded struct {
	accounts []storage.Account
}

type msgInboxLoaded struct {
	threads []storage.ThreadSummary
	err     error
}

type msgThreadLoaded struct {
	msgs []storage.Message
	err  error
}

type msgMessageLoaded struct {
	msg         storage.Message
	attachments []storage.Attachment
	err         error
}

type msgSyncDone struct {
	err error
}

type msgSendDone struct {
	err error
}

type msgConfigLoaded struct {
	rows []configRow
	err  error
}

type msgConfigSaved struct {
	err error
}

type msgMarkReadDone struct {
	acctID    string
	messageID string
	queued    bool // true when offline and op was deferred
}

type msgDrainDone struct {
	applied int // number of pending ops successfully flushed
}

// Commands

func loadAccountsCmd(store *storage.Store) tea.Cmd {
	return func() tea.Msg {
		accs, err := store.ListAccounts()
		if err != nil {
			return msgAccountsLoaded{}
		}
		return msgAccountsLoaded{accounts: accs}
	}
}

func loadInboxCmd(store *storage.Store, acctID string) tea.Cmd {
	return func() tea.Msg {
		var threads []storage.ThreadSummary
		var err error
		if acctID == "" {
			threads, err = store.ListAllThreadSummaries(100)
		} else {
			threads, err = store.ListThreadSummaries(acctID, 100)
		}
		return msgInboxLoaded{threads: threads, err: err}
	}
}

func loadThreadCmd(store *storage.Store, acctID, threadID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := store.ListMessagesByThread(acctID, threadID)
		return msgThreadLoaded{msgs: msgs, err: err}
	}
}

func loadMessageCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msg, err := store.GetMessage(acctID, msgID)
		if err != nil {
			return msgMessageLoaded{err: fmt.Errorf("get message: %w", err)}
		}
		// If body not cached, fetch from Gmail API (also saves attachments to store).
		if msg.CacheState == storage.CacheMetadataOnly {
			httpClient, err := accMgr.GetClient(ctx, acctID)
			if err != nil {
				return msgMessageLoaded{msg: msg}
			}
			gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
			if err != nil {
				return msgMessageLoaded{msg: msg}
			}
			inboxSvc := mail.NewInboxService(store, gmailClient)
			fetched, err := inboxSvc.GetMessage(ctx, acctID, msgID)
			if err == nil {
				msg = fetched
			}
		}
		atts, _ := store.GetAttachments(acctID, msgID)
		return msgMessageLoaded{msg: msg, attachments: atts}
	}
}

func syncAllCmd(store *storage.Store, accMgr *accounts.Manager, accts []storage.Account) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		for _, acct := range accts {
			httpClient, err := accMgr.GetClient(ctx, acct.ID)
			if err != nil {
				return msgSyncDone{err: fmt.Errorf("auth %s: %w", acct.Email, err)}
			}
			gmailClient, err := gmail.NewClient(ctx, httpClient, acct.ID)
			if err != nil {
				return msgSyncDone{err: fmt.Errorf("gmail client: %w", err)}
			}
			mgr := mailsync.NewManager(store, gmailClient)
			if err := mgr.IncrementalSync(ctx, acct.ID); err != nil {
				return msgSyncDone{err: fmt.Errorf("sync %s: %w", acct.Email, err)}
			}
		}
		return msgSyncDone{}
	}
}

// markReadCmd optimistically updates local storage, then tries the Gmail API.
// If offline, the operation is queued in pending_ops for later.
func markReadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Optimistic local update — do this regardless of network state.
		if msg, err := store.GetMessage(acctID, msgID); err == nil && msg.IsUnread {
			msg.IsUnread = false
			_ = store.UpsertMessage(msg)
		}

		// Attempt the Gmail API call.
		httpClient, err := accMgr.GetClient(ctx, acctID)
		if err == nil {
			if gc, err := gmail.NewClient(ctx, httpClient, acctID); err == nil {
				if err := gc.ModifyLabels(ctx, msgID, nil, []string{"UNREAD"}); err == nil {
					return msgMarkReadDone{acctID: acctID, messageID: msgID, queued: false}
				}
			}
		}

		// Offline or API error — queue for the next successful sync.
		_ = store.AddPendingOp(storage.OpMarkRead, acctID, msgID)
		return msgMarkReadDone{acctID: acctID, messageID: msgID, queued: true}
	}
}

// drainPendingOpsCmd runs after a successful sync and replays any queued operations.
func drainPendingOpsCmd(store *storage.Store, accMgr *accounts.Manager) tea.Cmd {
	return func() tea.Msg {
		ops, err := store.ListPendingOps()
		if err != nil || len(ops) == 0 {
			return msgDrainDone{}
		}

		ctx := context.Background()
		clients := make(map[string]*gmail.Client)
		applied := 0

		for _, op := range ops {
			if _, ok := clients[op.AccountID]; !ok {
				httpClient, err := accMgr.GetClient(ctx, op.AccountID)
				if err != nil {
					continue
				}
				gc, err := gmail.NewClient(ctx, httpClient, op.AccountID)
				if err != nil {
					continue
				}
				clients[op.AccountID] = gc
			}
			gc := clients[op.AccountID]

			switch op.OpType {
			case storage.OpMarkRead:
				if err := gc.ModifyLabels(ctx, op.MessageID, nil, []string{"UNREAD"}); err == nil {
					_ = store.DeletePendingOp(op.ID)
					applied++
				}
			}
		}

		return msgDrainDone{applied: applied}
	}
}

// loadConfigCmd loads labels and existing sync configs for the given account,
// merging them with smart defaults (INBOX/STARRED: all+attachments, rest: 30d).
func loadConfigCmd(store *storage.Store, acctID string) tea.Cmd {
	return func() tea.Msg {
		labels, err := store.ListLabels(acctID)
		if err != nil {
			return msgConfigLoaded{err: err}
		}

		existing, _ := store.ListSyncConfigs(acctID)
		cfgMap := make(map[string]storage.SyncConfig, len(existing))
		for _, c := range existing {
			cfgMap[c.LabelID] = c
		}

		rows := make([]configRow, 0, len(labels))
		for _, l := range labels {
			row := configRow{
				LabelID:   l.ID,
				LabelName: l.Name,
				// defaults
				DaysToSync:       30,
				FetchAttachments: false,
			}
			if c, ok := cfgMap[l.ID]; ok {
				row.DaysToSync = c.DaysToSync
				row.FetchAttachments = c.FetchAttachments
			} else if l.ID == "INBOX" || l.ID == "STARRED" {
				row.DaysToSync = 0    // all mail
				row.FetchAttachments = true
			}
			rows = append(rows, row)
		}
		return msgConfigLoaded{rows: rows}
	}
}

// saveConfigCmd persists all config rows to the database.
func saveConfigCmd(store *storage.Store, acctID string, rows []configRow) tea.Cmd {
	return func() tea.Msg {
		for _, row := range rows {
			if err := store.UpsertSyncConfig(storage.SyncConfig{
				LabelID:          row.LabelID,
				AccountID:        acctID,
				LabelName:        row.LabelName,
				DaysToSync:       row.DaysToSync,
				FetchAttachments: row.FetchAttachments,
			}); err != nil {
				return msgConfigSaved{err: err}
			}
		}
		return msgConfigSaved{}
	}
}

func sendEmailCmd(accMgr *accounts.Manager, acctID string, outMsg mail.OutboundMessage) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		httpClient, err := accMgr.GetClient(ctx, acctID)
		if err != nil {
			return msgSendDone{err: fmt.Errorf("get client: %w", err)}
		}
		gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
		if err != nil {
			return msgSendDone{err: fmt.Errorf("gmail client: %w", err)}
		}
		svc := mail.NewSendService(gmailClient)
		_, err = svc.Send(ctx, outMsg)
		return msgSendDone{err: err}
	}
}

package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
	mailsync "github.com/mailrat/mailrat/internal/sync"
)

func syncAllCmd(store *storage.Store, accMgr *accounts.Manager, accts []storage.Account) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		for _, acct := range accts {
			if err := syncAccount(ctx, store, accMgr, acct); err != nil {
				return msgSyncDone{err: err}
			}
		}
		return msgSyncDone{}
	}
}

func syncAccount(ctx context.Context, store *storage.Store, accMgr *accounts.Manager, acct storage.Account) error {
	httpClient, err := accMgr.GetClient(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("auth %s: %w", acct.Email, err)
	}

	gmailClient, err := gmail.NewClient(ctx, httpClient, acct.ID)
	if err != nil {
		return fmt.Errorf("gmail client: %w", err)
	}

	mgr := mailsync.NewManager(store, gmailClient)
	if err := mgr.IncrementalSync(ctx, acct.ID); err != nil {
		return fmt.Errorf("sync %s: %w", acct.Email, err)
	}

	configs, _ := store.ListSyncConfigs(acct.ID)
	if err := mgr.BackfillFromConfigs(ctx, acct.ID, configs); err != nil {
		return fmt.Errorf("backfill %s: %w", acct.Email, err)
	}

	return nil
}

// markReadCmd optimistically updates local storage, then tries the Gmail API.
// If offline, the operation is queued in pending_ops for later.
func markReadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		markMessageReadLocally(store, acctID, msgID)

		if applyMarkRead(ctx, accMgr, acctID, msgID) == nil {
			return msgMarkReadDone{acctID: acctID, messageID: msgID, queued: false}
		}

		_ = store.AddPendingOp(storage.OpMarkRead, acctID, msgID)
		return msgMarkReadDone{acctID: acctID, messageID: msgID, queued: true}
	}
}

func markMessageReadLocally(store *storage.Store, acctID, msgID string) {
	msg, err := store.GetMessage(acctID, msgID)
	if err != nil || !msg.IsUnread {
		return
	}

	msg.IsUnread = false
	_ = store.UpsertMessage(msg)
}

func applyMarkRead(ctx context.Context, accMgr *accounts.Manager, acctID, msgID string) error {
	httpClient, err := accMgr.GetClient(ctx, acctID)
	if err != nil {
		return err
	}

	gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
	if err != nil {
		return err
	}

	return gmailClient.ModifyLabels(ctx, msgID, nil, []string{"UNREAD"})
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
			gmailClient := pendingOpsClient(ctx, accMgr, clients, op.AccountID)
			if gmailClient == nil {
				continue
			}

			var applyErr error
			switch op.OpType {
			case storage.OpMarkRead:
				applyErr = gmailClient.ModifyLabels(ctx, op.MessageID, nil, []string{"UNREAD"})
			case storage.OpMarkUnread:
				applyErr = gmailClient.ModifyLabels(ctx, op.MessageID, []string{"UNREAD"}, nil)
			case storage.OpTrashThread:
				msgs, err := store.ListMessagesByThread(op.AccountID, op.MessageID)
				if err != nil {
					continue
				}
				for _, msg := range msgs {
					if applyErr = gmailClient.TrashMessage(ctx, msg.ID); applyErr != nil {
						break
					}
				}
			}
			if applyErr == nil {
				_ = store.DeletePendingOp(op.ID)
				applied++
			}
		}

		return msgDrainDone{applied: applied}
	}
}

func pendingOpsClient(ctx context.Context, accMgr *accounts.Manager, clients map[string]*gmail.Client, acctID string) *gmail.Client {
	if client, ok := clients[acctID]; ok {
		return client
	}

	httpClient, err := accMgr.GetClient(ctx, acctID)
	if err != nil {
		return nil
	}

	gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
	if err != nil {
		return nil
	}

	clients[acctID] = gmailClient
	return gmailClient
}

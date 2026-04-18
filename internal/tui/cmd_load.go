package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
)

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
		var (
			threads []storage.ThreadSummary
			err     error
		)

		if acctID == "" {
			threads, err = store.ListAllThreadSummaries(100, 0)
		} else {
			threads, err = store.ListThreadSummaries(acctID, 100, 0)
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

		if msg.CacheState == storage.CacheMetadataOnly {
			msg = hydrateMessageBody(ctx, store, accMgr, acctID, msgID, msg)
		}

		atts, _ := store.GetAttachments(acctID, msgID)
		return msgMessageLoaded{msg: msg, attachments: atts}
	}
}

func hydrateMessageBody(ctx context.Context, store *storage.Store, accMgr *accounts.Manager, acctID, msgID string, msg storage.Message) storage.Message {
	httpClient, err := accMgr.GetClient(ctx, acctID)
	if err != nil {
		return msg
	}

	gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
	if err != nil {
		return msg
	}

	inboxSvc := mail.NewInboxService(store, gmailClient)
	fetched, err := inboxSvc.GetMessage(ctx, acctID, msgID)
	if err != nil {
		return msg
	}

	return fetched
}

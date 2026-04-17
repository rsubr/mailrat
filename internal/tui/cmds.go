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
	msgs []storage.Message
	err  error
}

type msgThreadLoaded struct {
	msgs []storage.Message
	err  error
}

type msgMessageLoaded struct {
	msg storage.Message
	err error
}

type msgSyncDone struct {
	err error
}

type msgSendDone struct {
	err error
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
		var msgs []storage.Message
		var err error
		if acctID == "" {
			msgs, err = store.ListAllMessages(100)
		} else {
			msgs, err = store.ListMessages(acctID, 100)
		}
		return msgInboxLoaded{msgs: msgs, err: err}
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
		// If body not cached, fetch from Gmail API
		if msg.CacheState == storage.CacheMetadataOnly {
			httpClient, err := accMgr.GetClient(ctx, acctID)
			if err != nil {
				return msgMessageLoaded{msg: msg} // show snippet we have
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
		return msgMessageLoaded{msg: msg}
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

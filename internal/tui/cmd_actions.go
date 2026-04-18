package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

type msgActionDone struct {
	status     string
	err        error
	cmd        tea.Cmd
	accountID  string
	messageID  string
	markUnread bool
}

func archiveThreadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, threadID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := store.ListMessagesByThread(acctID, threadID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("load thread: %w", err)}
		}
		if len(msgs) == 0 {
			return msgActionDone{status: "Nothing to archive"}
		}
		if err := modifyMessages(accMgr, acctID, msgs, nil, []string{"INBOX"}); err != nil {
			return msgActionDone{err: fmt.Errorf("archive: %w", err)}
		}
		for _, msg := range msgs {
			_ = removeLocalLabel(store, acctID, msg.ID, "INBOX")
		}
		return msgActionDone{
			status: "Conversation archived",
			cmd:    loadInboxCmd(store, ""),
		}
	}
}

func markUnreadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		httpClient, err := accMgr.GetClient(ctx, acctID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("auth: %w", err)}
		}
		gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("gmail client: %w", err)}
		}
		if err := gmailClient.ModifyLabels(ctx, msgID, []string{"UNREAD"}, nil); err != nil {
			return msgActionDone{err: fmt.Errorf("mark unread: %w", err)}
		}
		if err := addLocalLabel(store, acctID, msgID, "UNREAD"); err != nil {
			return msgActionDone{err: err}
		}
		msg, err := store.GetMessage(acctID, msgID)
		if err == nil {
			msg.IsUnread = true
			_ = store.UpsertMessage(msg)
		}
		return msgActionDone{
			status:     "Marked unread",
			accountID:  acctID,
			messageID:  msgID,
			markUnread: true,
		}
	}
}

func modifyMessages(accMgr *accounts.Manager, acctID string, msgs []storage.Message, add, remove []string) error {
	ctx := context.Background()
	httpClient, err := accMgr.GetClient(ctx, acctID)
	if err != nil {
		return err
	}
	gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		if err := gmailClient.ModifyLabels(ctx, msg.ID, add, remove); err != nil {
			return err
		}
	}
	return nil
}

func addLocalLabel(store *storage.Store, acctID, msgID, labelID string) error {
	ids, err := store.GetMessageLabelIDs(acctID, msgID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == labelID {
			return nil
		}
	}
	ids = append(ids, labelID)
	return store.SetMessageLabels(acctID, msgID, ids)
}

func removeLocalLabel(store *storage.Store, acctID, msgID, labelID string) error {
	ids, err := store.GetMessageLabelIDs(acctID, msgID)
	if err != nil {
		return err
	}
	updated := ids[:0]
	for _, id := range ids {
		if id != labelID {
			updated = append(updated, id)
		}
	}
	return store.SetMessageLabels(acctID, msgID, updated)
}

func searchThreadsCmd(store *storage.Store, query string) tea.Cmd {
	return func() tea.Msg {
		results, err := store.LocalSearchThreadSummaries(query, 200)
		return msgSearchLoaded{results: results, err: err}
	}
}

func quoteBody(msg storage.Message) string {
	attr := msg.FromAddr + " wrote:"
	if msg.InternalDate > 0 {
		t := time.UnixMilli(msg.InternalDate).Format("Mon, Jan 2, 2006 at 3:04 PM")
		attr = "On " + t + ", " + msg.FromAddr + " wrote:"
	}
	body := strings.TrimSpace(stripHTML(msg.Body))
	if body == "" {
		body = strings.TrimSpace(msg.Snippet)
	}
	if body == "" {
		body = "(no body)"
	}
	var quoted []string
	for _, line := range strings.Split(body, "\n") {
		quoted = append(quoted, "> "+line)
	}
	return fmt.Sprintf("\n\n%s\n%s", attr, strings.Join(quoted, "\n"))
}

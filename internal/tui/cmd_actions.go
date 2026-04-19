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
	isStarred  bool
	setStarred bool // true if we are updating star status
}

func getGmailClient(accMgr *accounts.Manager, acctID string) (*gmail.Client, context.Context, error) {
	ctx := context.Background()
	httpClient, err := accMgr.GetClient(ctx, acctID)
	if err != nil {
		return nil, nil, err
	}
	gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
	if err != nil {
		return nil, nil, err
	}
	return gmailClient, ctx, nil
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

func markSpamCmd(store *storage.Store, accMgr *accounts.Manager, acctID, threadID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := store.ListMessagesByThread(acctID, threadID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("load thread: %w", err)}
		}
		if len(msgs) == 0 {
			return msgActionDone{status: "Nothing to mark as spam"}
		}
		if err := modifyMessages(accMgr, acctID, msgs, []string{"SPAM"}, []string{"INBOX"}); err != nil {
			return msgActionDone{err: fmt.Errorf("mark spam: %w", err)}
		}
		for _, msg := range msgs {
			_ = removeLocalLabel(store, acctID, msg.ID, "INBOX")
			_ = addLocalLabel(store, acctID, msg.ID, "SPAM")
		}
		return msgActionDone{
			status: "Conversation marked as spam",
			cmd:    loadInboxCmd(store, ""),
		}
	}
}

func markUnreadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		if err := modifyMessages(accMgr, acctID, []storage.Message{{ID: msgID}}, []string{"UNREAD"}, nil); err != nil {
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

func markReadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		markMessageReadLocally(store, acctID, msgID)

		if applyMarkRead(accMgr, acctID, msgID) == nil {
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

func applyMarkRead(accMgr *accounts.Manager, acctID, msgID string) error {
	gmailClient, ctx, err := getGmailClient(accMgr, acctID)
	if err != nil {
		return err
	}

	return gmailClient.ModifyLabels(ctx, msgID, nil, []string{"UNREAD"})
}

func toggleStarCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		msg, err := store.GetMessage(acctID, msgID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("load message: %w", err)}
		}

		if msg.IsStarred {
			return unstarCmd(store, accMgr, acctID, msgID)()
		}
		return starCmd(store, accMgr, acctID, msgID)()
	}
}

func starCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		if err := modifyMessages(accMgr, acctID, []storage.Message{{ID: msgID}}, []string{"STARRED"}, nil); err != nil {
			return msgActionDone{err: fmt.Errorf("star: %w", err)}
		}
		if err := addLocalLabel(store, acctID, msgID, "STARRED"); err != nil {
			return msgActionDone{err: err}
		}
		return msgActionDone{
			status:     "Starred",
			accountID:  acctID,
			messageID:  msgID,
			isStarred:  true,
			setStarred: true,
		}
	}
}

func unstarCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		if err := modifyMessages(accMgr, acctID, []storage.Message{{ID: msgID}}, nil, []string{"STARRED"}); err != nil {
			return msgActionDone{err: fmt.Errorf("unstar: %w", err)}
		}
		if err := removeLocalLabel(store, acctID, msgID, "STARRED"); err != nil {
			return msgActionDone{err: err}
		}
		return msgActionDone{
			status:     "Unstarred",
			accountID:  acctID,
			messageID:  msgID,
			isStarred:  false,
			setStarred: true,
		}
	}
}

func toggleThreadStarCmd(store *storage.Store, accMgr *accounts.Manager, acctID, threadID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := store.ListMessagesByThread(acctID, threadID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("load thread: %w", err)}
		}
		if len(msgs) == 0 {
			return msgActionDone{status: "Nothing to star"}
		}

		// Use the first message to determine the state of the thread star.
		isStarred := false
		for _, m := range msgs {
			if m.IsStarred {
				isStarred = true
				break
			}
		}

		var add, remove []string
		if isStarred {
			remove = []string{"STARRED"}
		} else {
			add = []string{"STARRED"}
		}

		if err := modifyMessages(accMgr, acctID, msgs, add, remove); err != nil {
			return msgActionDone{err: fmt.Errorf("star: %w", err)}
		}

		for _, msg := range msgs {
			if isStarred {
				_ = removeLocalLabel(store, acctID, msg.ID, "STARRED")
			} else {
				_ = addLocalLabel(store, acctID, msg.ID, "STARRED")
			}
		}

		status := "Conversation starred"
		if isStarred {
			status = "Conversation unstarred"
		}

		return msgActionDone{
			status:     status,
			accountID:  acctID,
			messageID:  msgs[0].ID, // Just use the first one for the msgActionDone logic
			isStarred:  !isStarred,
			setStarred: true,
			cmd:        loadInboxCmd(store, ""),
		}
	}
}

func trashThreadCmd(store *storage.Store, accMgr *accounts.Manager, acctID, threadID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := store.ListMessagesByThread(acctID, threadID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("load thread: %w", err)}
		}
		if len(msgs) == 0 {
			return msgActionDone{status: "Nothing to trash"}
		}

		gmailClient, ctx, err := getGmailClient(accMgr, acctID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("auth: %w", err)}
		}

		for _, msg := range msgs {
			if err := gmailClient.TrashMessage(ctx, msg.ID); err != nil {
				return msgActionDone{err: fmt.Errorf("trash: %w", err)}
			}
		}

		return msgActionDone{
			status: "Conversation moved to trash",
			cmd:    loadInboxCmd(store, ""),
		}
	}
}

func trashMessageCmd(store *storage.Store, accMgr *accounts.Manager, acctID, msgID string) tea.Cmd {
	return func() tea.Msg {
		gmailClient, ctx, err := getGmailClient(accMgr, acctID)
		if err != nil {
			return msgActionDone{err: fmt.Errorf("auth: %w", err)}
		}
		if err := gmailClient.TrashMessage(ctx, msgID); err != nil {
			return msgActionDone{err: fmt.Errorf("trash: %w", err)}
		}
		// In a real app, we'd remove it from the local INBOX view immediately.
		return msgActionDone{
			status: "Message moved to trash",
			cmd:    loadInboxCmd(store, ""),
		}
	}
}

func modifyMessages(accMgr *accounts.Manager, acctID string, msgs []storage.Message, add, remove []string) error {
	gmailClient, ctx, err := getGmailClient(accMgr, acctID)
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

func searchThreadsCmd(store *storage.Store, acctID, query string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(query) == "" {
			var (
				threads []storage.ThreadSummary
				err     error
			)
			if acctID == "" {
				threads, err = store.ListAllThreadSummaries(200, 0)
			} else {
				threads, err = store.ListThreadSummaries(acctID, 200, 0)
			}
			return msgSearchLoaded{results: threads, err: err}
		}
		results, err := store.LocalSearchThreadSummaries(query, 200)
		return msgSearchLoaded{results: results, err: err}
	}
}

func searchDebounceCmd(token int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(300 * time.Millisecond)
		return msgSearchDebounce{token: token}
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

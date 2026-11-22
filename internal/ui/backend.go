package ui

import (
	"context"

	"github.com/mailrat/mailrat/internal/desktop"
)

// Backend is the subset of desktop.App the UI depends on.
type Backend interface {
	SetEventHandler(h desktop.EventHandler)
	GetAccounts() ([]desktop.AccountItem, error)
	StartAccountAdd() error
	RemoveAccount(accountID string) error
	GetAccountSettings(accountID string) (desktop.AccountSettingsItem, error)
	SetAccountSettings(s desktop.AccountSettingsItem) error
	GetGlobalSettings() (desktop.GlobalSettingsItem, error)
	SetGlobalSettings(s desktop.GlobalSettingsItem) error
	GetInbox(accountID string, limit, offset int) ([]desktop.ThreadItem, error)
	GetLabelThreads(accountID, labelID string, limit, offset int) ([]desktop.ThreadItem, error)
	SearchInbox(query string, limit int) ([]desktop.ThreadItem, error)
	GetThread(accountID, threadID string) ([]desktop.MessageItem, error)
	GetMessage(accountID, messageID string) (desktop.MessageItem, error)
	GetAttachmentData(accountID, messageID, attachmentID string) (desktop.AttachmentDataItem, error)
	SendMessage(msg desktop.OutboundMsg) error
	MarkMessageRead(accountID, messageID string) error
	MarkMessageUnread(accountID, messageID string) error
	TrashThread(accountID, threadID string) error
	ArchiveThread(accountID, threadID string) error
	MarkThreadSpam(accountID, threadID string) error
	MarkThreadNotSpam(accountID, threadID string) error
	Sync(accountID string) error
	DrainPendingOps()
	SearchContacts(accountID, query string, limit int) ([]desktop.ContactItem, error)
	GetLabels(accountID string) ([]desktop.LabelItem, error)
	GetLabelUnreadCounts(accountID string) (map[string]int, error)
	GetAPIErrors() []desktop.APIError
	ClearAPIErrors()
	Start(ctx context.Context)
	Shutdown()
}

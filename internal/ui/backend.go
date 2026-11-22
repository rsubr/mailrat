package ui

import (
	"context"

	"github.com/mailrat/mailrat/internal/desktop"
)

// Backend is the subset of desktop.App the UI depends on.
type Backend interface {
	SetEventHandler(h desktop.EventHandler)
	GetAccounts() ([]desktop.AccountItem, error)
	HasCredentials() bool
	ImportCredentials(path string) error
	BeginLogin() (authURL string, err error)
	OpenLoginBrowser() bool
	SubmitLoginCode(raw string) error
	CancelLogin()
	RemoveAccount(accountID string) error
	RenameAccount(accountID, name string) error
	RefetchAccount(accountID string) error
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
	UndoTrashThread(accountID, threadID string) error
	ArchiveThread(accountID, threadID string) error
	MarkThreadSpam(accountID, threadID string) error
	MarkThreadNotSpam(accountID, threadID string) error
	SetThreadLabel(accountID, threadID, labelID string, on bool) error
	GetSyncConfigs(accountID string) ([]desktop.SyncConfigItem, error)
	SetSyncConfig(accountID string, item desktop.SyncConfigItem) error
	LoadOlder(accountID, labelID string) (int, error)
	Sync(accountID string) error
	DrainPendingOps()
	SearchContacts(accountID, query string, limit int) ([]desktop.ContactItem, error)
	ListContacts(accountID string) ([]desktop.ContactItem, error)
	SaveContact(c desktop.ContactItem) (desktop.ContactItem, error)
	DeleteContact(accountID, id string) error
	FrequentAddresses(accountID string, limit int) ([]desktop.ContactItem, error)
	GetLabels(accountID string) ([]desktop.LabelItem, error)
	GetLabelUnreadCounts(accountID string) (map[string]int, error)
	GetAPIErrors() []desktop.APIError
	ClearAPIErrors()
	Start(ctx context.Context)
	Shutdown()
}

// *desktop.App must keep satisfying Backend; cmd/mailrat passes it to Run.
var _ Backend = (*desktop.App)(nil)

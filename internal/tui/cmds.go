package tui

import "github.com/mailrat/mailrat/internal/storage"

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

type msgSearchLoaded struct {
	results []storage.ThreadSummary
	err     error
}

type msgSearchDebounce struct{ token int }

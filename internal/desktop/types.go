package desktop

import (
	"strings"

	"github.com/mailrat/mailrat/internal/storage"
)

// AccountItem is the frontend representation of a connected account.
type AccountItem struct {
	ID          string
	Email       string
	PhotoURL    string
	DisplayName string
}

// AccountSettingsItem is the frontend representation of per-account sync settings.
type AccountSettingsItem struct {
	AccountID        string
	SyncPeriod       string // "all", "1y", "90d", "30d"
	FetchAttachments bool
}

// GlobalSettingsItem is the frontend representation of app-wide settings.
type GlobalSettingsItem struct {
	SyncFrequency    string // "15m", "1h", "manual"
	DefaultAccountID string
}

// Sync scope modes for SyncConfigItem.
const (
	SyncModeAll   = "all"   // every message with the label
	SyncModeDays  = "days"  // messages from the last Value days
	SyncModeCount = "count" // the newest Value messages
)

// SyncConfigItem is how much mail of one label is kept in sync.
type SyncConfigItem struct {
	LabelID   string
	LabelName string
	Mode      string
	Value     int // days or message count; unused for SyncModeAll
}

// ThreadItem is the frontend representation of an inbox thread.
type ThreadItem struct {
	ID             string
	AccountID      string
	Subject        string
	Participants   string
	Snippet        string
	Date           int64
	IsUnread       bool
	IsStarred      bool
	MessageCount   int
	HasAttachments bool
	LabelIDs       []string
}

// AttachmentItem is the frontend representation of a single message attachment.
type AttachmentItem struct {
	Name         string
	MimeType     string
	Size         int64
	AttachmentID string
}

// MessageItem is the frontend representation of a single message.
type MessageItem struct {
	ID             string
	ThreadID       string
	MessageID      string // RFC 2822 Message-ID header value
	AccountID      string
	Subject        string
	FromAddr       string
	ToAddr         string
	CcAddr         string
	BccAddr        string
	Body           string
	Snippet        string
	Date           int64
	IsUnread       bool
	HasAttachments bool
	Attachments    []AttachmentItem
}

// LabelItem is the frontend representation of a Gmail label.
type LabelItem struct {
	ID              string
	Name            string
	Type            string
	TextColor       string
	BackgroundColor string
}

// ContactItem is the frontend representation of an address book contact.
type ContactItem struct {
	ID        string // storage id; empty for an address that is not (yet) a saved contact
	AccountID string
	Name      string
	Email     string
	Phone     string
	Count     int // messages exchanged, set only by FrequentAddresses
}

// AttachmentDataItem carries raw attachment bytes for the frontend.
type AttachmentDataItem struct {
	Data     []byte
	MimeType string
}

// APIError is a timestamped error entry shown in the error log modal.
type APIError struct {
	Timestamp int64
	Message   string
}

// OutboundMsg is the frontend payload for sending an email.
type OutboundMsg struct {
	AccountID string
	To        string
	Cc        string
	Bcc       string
	Subject   string
	Body      string
	InReplyTo string
	ThreadID  string
}

func threadSummaryToItem(t storage.ThreadSummary) ThreadItem {
	labelIDs := t.LabelIDs
	if labelIDs == nil {
		labelIDs = []string{}
	}
	return ThreadItem{
		ID:             t.ThreadID,
		AccountID:      t.AccountID,
		Subject:        t.Subject,
		Participants:   t.Participants,
		Snippet:        t.Snippet,
		Date:           t.LastDate,
		IsUnread:       t.UnreadCount > 0,
		IsStarred:      t.IsStarred,
		MessageCount:   t.MessageCount,
		HasAttachments: t.HasAttachments,
		LabelIDs:       labelIDs,
	}
}

func messageToItem(m storage.Message) MessageItem {
	return MessageItem{
		ID:             m.ID,
		ThreadID:       m.ThreadID,
		MessageID:      m.MessageID,
		AccountID:      m.AccountID,
		Subject:        m.Subject,
		FromAddr:       m.FromAddr,
		ToAddr:         m.ToAddr,
		CcAddr:         m.CcAddr,
		BccAddr:        m.BccAddr,
		Body:           m.Body,
		Snippet:        m.Snippet,
		Date:           m.InternalDate,
		IsUnread:       m.IsUnread,
		HasAttachments: m.HasAttachments,
		Attachments:    []AttachmentItem{},
	}
}

func attachmentsToItems(atts []storage.Attachment) []AttachmentItem {
	out := make([]AttachmentItem, len(atts))
	for i, a := range atts {
		out[i] = AttachmentItem{
			Name:         a.Name,
			MimeType:     a.MimeType,
			Size:         a.Size,
			AttachmentID: a.AttachmentID,
		}
	}
	return out
}

func threadSummariesToItems(summaries []storage.ThreadSummary) []ThreadItem {
	out := make([]ThreadItem, len(summaries))
	for i, s := range summaries {
		out[i] = threadSummaryToItem(s)
	}
	return out
}

func accountsToItems(accs []storage.Account) []AccountItem {
	out := make([]AccountItem, len(accs))
	for i, acc := range accs {
		out[i] = AccountItem{
			ID:          acc.ID,
			Email:       acc.Email,
			PhotoURL:    acc.PhotoURL,
			DisplayName: acc.DisplayName,
		}
	}
	return out
}

func contactsToItems(contacts []storage.Contact) []ContactItem {
	out := make([]ContactItem, len(contacts))
	for i, c := range contacts {
		out[i] = ContactItem{ID: c.ID, AccountID: c.AccountID, Name: c.Name, Email: c.Email, Phone: c.Phone}
	}
	return out
}

// formatSystemLabelName converts a system label ID like "ALL_MAIL" to "All Mail".
// It splits on spaces and underscores, title-cases each word, and joins with spaces.
func formatSystemLabelName(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == ' '
	})
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

func labelsToItems(labels []storage.Label) []LabelItem {
	out := make([]LabelItem, len(labels))
	for i, l := range labels {
		name := l.Name
		if l.LabelType == "system" {
			name = formatSystemLabelName(l.Name)
		}
		out[i] = LabelItem{ID: l.ID, Name: name, Type: l.LabelType, TextColor: l.TextColor, BackgroundColor: l.BackgroundColor}
	}
	return out
}

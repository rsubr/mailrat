package desktop

import (
	"bytes"
	"html"
	"mime"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mailrat/mailrat/internal/mail"
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
	AccountID  string
	SyncPeriod string // "all", "1y", "90d", "30d"
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
	ID        string
	ThreadID  string
	MessageID string // RFC 2822 Message-ID header value
	AccountID string
	Subject   string
	FromAddr  string
	ToAddr    string
	CcAddr    string
	BccAddr   string
	// ReplyTo, References and InReplyTo are the raw reply headers (unfolded).
	// They are empty when absent or not yet fetched.
	ReplyTo        string
	References     string
	InReplyTo      string
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
	AccountID  string
	To         string
	Cc         string
	Bcc        string
	Subject    string
	Body       string
	InReplyTo  string
	References string // space-separated Message-ID chain; empty means just InReplyTo
	ThreadID   string
	DraftID    string // local draft to keep updating; set when a saved draft is reopened
	// Attachments are the files to send. Files from disk are referenced by Path
	// and read only when sending or saving a draft.
	Attachments []OutAttachment
}

// Equal reports whether m and o hold the same content. OutboundMsg holds a
// slice, so it can no longer be compared with ==.
func (m OutboundMsg) Equal(o OutboundMsg) bool {
	return slices.EqualFunc(m.Attachments, o.Attachments, func(a, b OutAttachment) bool {
		return a.Filename == b.Filename && a.MIMEType == b.MIMEType && a.Size == b.Size && a.Path == b.Path && bytes.Equal(a.Data, b.Data)
	}) && m.AccountID == o.AccountID && m.To == o.To && m.Cc == o.Cc && m.Bcc == o.Bcc && m.Subject == o.Subject &&
		m.Body == o.Body && m.InReplyTo == o.InReplyTo && m.References == o.References && m.ThreadID == o.ThreadID && m.DraftID == o.DraftID
}

// OutAttachment is a file attached to a message being composed. Set Path for a
// file on disk, or Data for content already in memory (such as the attachments
// of a reopened draft); Size is informational (see StatAttachment).
type OutAttachment struct {
	Filename string
	MIMEType string
	Size     int64
	Path     string
	Data     []byte
}

// MaxAttachmentBytes is Gmail's 25 MB message limit, which the base64-encoded
// attachments and body must fit in.
const MaxAttachmentBytes = mail.MaxMessageBytes

// StatAttachment describes the file at path for display in the compose window.
// It fails for missing, unreadable, non-regular or oversized files.
func StatAttachment(path string) (OutAttachment, error) {
	a := mail.Attachment{Path: path}
	n, err := mail.AttachmentSize(a)
	if err != nil {
		return OutAttachment{}, err
	}
	if err := mail.CheckAttachmentsSize([]mail.Attachment{a}, 0); err != nil {
		return OutAttachment{}, err
	}
	name := filepath.Base(path)
	return OutAttachment{Filename: name, MIMEType: mime.TypeByExtension(strings.ToLower(filepath.Ext(name))), Size: n, Path: path}, nil
}

// CheckAttachments reports whether atts plus a body of bodyLen bytes fit in a
// single Gmail message, so the compose window can refuse a file when it is added.
func CheckAttachments(atts []OutAttachment, bodyLen int) error {
	return mail.CheckAttachmentsSize(toMailAttachments(atts), bodyLen)
}

func toMailAttachments(in []OutAttachment) []mail.Attachment {
	out := make([]mail.Attachment, len(in))
	for i, a := range in {
		out[i] = mail.Attachment{Filename: a.Filename, MIMEType: a.MIMEType, Path: a.Path, Data: a.Data}
	}
	return out
}

// draftOutbound builds the compose prefill for a saved draft message. The body is
// kept as stored: it already holds the signature and any quote.
func draftOutbound(m MessageItem, draftID, inReplyTo string) OutboundMsg {
	return OutboundMsg{
		AccountID: m.AccountID, To: m.ToAddr, Cc: m.CcAddr, Bcc: m.BccAddr,
		Subject: m.Subject, Body: m.Body, InReplyTo: inReplyTo, ThreadID: m.ThreadID, DraftID: draftID,
	}
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
		Snippet:        html.UnescapeString(t.Snippet), // Gmail sends snippets HTML-escaped
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
		ReplyTo:        m.ReplyTo,
		References:     m.References,
		InReplyTo:      m.InReplyTo,
		Body:           mail.RepairMojibake(m.Body),
		Snippet:        html.UnescapeString(m.Snippet),
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

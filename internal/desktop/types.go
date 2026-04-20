package desktop

import (
	"strings"

	"github.com/mailrat/mailrat/internal/storage"
)

// AccountItem is the frontend representation of a connected account.
type AccountItem struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// ThreadItem is the frontend representation of an inbox thread.
type ThreadItem struct {
	ID             string   `json:"id"`
	AccountID      string   `json:"accountId"`
	Subject        string   `json:"subject"`
	Participants   string   `json:"participants"`
	Snippet        string   `json:"snippet"`
	Date           int64    `json:"date"`
	IsUnread       bool     `json:"isUnread"`
	IsStarred      bool     `json:"isStarred"`
	MessageCount   int      `json:"messageCount"`
	HasAttachments bool     `json:"hasAttachments"`
	LabelIDs       []string `json:"labelIds"`
}

// MessageItem is the frontend representation of a single message.
type MessageItem struct {
	ID             string `json:"id"`
	ThreadID       string `json:"threadId"`
	MessageID      string `json:"messageId"` // RFC 2822 Message-ID header value
	AccountID      string `json:"accountId"`
	Subject        string `json:"subject"`
	FromAddr       string `json:"from"`
	ToAddr         string `json:"to"`
	CcAddr         string `json:"cc"`
	BccAddr        string `json:"bcc"`
	Body           string `json:"body"`
	Snippet        string `json:"snippet"`
	Date           int64  `json:"date"`
	IsUnread       bool   `json:"isUnread"`
	HasAttachments bool   `json:"hasAttachments"`
}

// LabelItem is the frontend representation of a Gmail label.
type LabelItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	TextColor       string `json:"textColor"`
	BackgroundColor string `json:"backgroundColor"`
}

// ContactItem is the frontend representation of an address book contact.
type ContactItem struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// OutboundMsg is the frontend payload for sending an email.
type OutboundMsg struct {
	AccountID string `json:"accountId"`
	To        string `json:"to"`
	Cc        string `json:"cc"`
	Bcc       string `json:"bcc"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	InReplyTo string `json:"inReplyTo"`
	ThreadID  string `json:"threadId"`
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
	}
}

func threadSummariesToItems(summaries []storage.ThreadSummary) []ThreadItem {
	out := make([]ThreadItem, len(summaries))
	for i, s := range summaries {
		out[i] = threadSummaryToItem(s)
	}
	return out
}

func messagesToItems(msgs []storage.Message) []MessageItem {
	out := make([]MessageItem, len(msgs))
	for i, m := range msgs {
		out[i] = messageToItem(m)
	}
	return out
}

func accountsToItems(accs []storage.Account) []AccountItem {
	out := make([]AccountItem, len(accs))
	for i, acc := range accs {
		out[i] = AccountItem{ID: acc.ID, Email: acc.Email}
	}
	return out
}

func contactsToItems(contacts []storage.Contact) []ContactItem {
	out := make([]ContactItem, len(contacts))
	for i, c := range contacts {
		out[i] = ContactItem{Name: c.Name, Email: c.Email}
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

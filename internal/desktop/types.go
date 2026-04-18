package desktop

import "github.com/mailrat/mailrat/internal/storage"

// AccountItem is the frontend representation of a connected account.
type AccountItem struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// ThreadItem is the frontend representation of an inbox thread.
type ThreadItem struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	Subject        string `json:"subject"`
	Participants   string `json:"participants"`
	Snippet        string `json:"snippet"`
	Date           int64  `json:"date"`
	IsUnread       bool   `json:"isUnread"`
	MessageCount   int    `json:"messageCount"`
	HasAttachments bool   `json:"hasAttachments"`
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
	Body           string `json:"body"`
	Snippet        string `json:"snippet"`
	Date           int64  `json:"date"`
	IsUnread       bool   `json:"isUnread"`
	HasAttachments bool   `json:"hasAttachments"`
}

// LabelItem is the frontend representation of a Gmail label.
type LabelItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// OutboundMsg is the frontend payload for sending an email.
type OutboundMsg struct {
	AccountID string `json:"accountId"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	InReplyTo string `json:"inReplyTo"`
	ThreadID  string `json:"threadId"`
}

func threadSummaryToItem(t storage.ThreadSummary) ThreadItem {
	return ThreadItem{
		ID:             t.ThreadID,
		AccountID:      t.AccountID,
		Subject:        t.Subject,
		Participants:   t.Participants,
		Snippet:        t.Snippet,
		Date:           t.LastDate,
		IsUnread:       t.UnreadCount > 0,
		MessageCount:   t.MessageCount,
		HasAttachments: t.HasAttachments,
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
		Body:           m.Body,
		Snippet:        m.Snippet,
		Date:           m.InternalDate,
		IsUnread:       m.IsUnread,
		HasAttachments: m.HasAttachments,
	}
}

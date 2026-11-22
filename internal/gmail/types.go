package gmail

// MessageMeta is a lightweight message with headers only.
type MessageMeta struct {
	ID           string
	ThreadID     string
	MessageID    string // RFC 2822 Message-ID header value, e.g. "<abc@mail.gmail.com>"
	Subject      string
	From         string
	To           string
	Cc           string
	Bcc          string
	Snippet      string
	InternalDate int64
	LabelIDs     []string
	IsUnread     bool
}

// Message is a full message including body.
type Message struct {
	MessageMeta
	Body        string       // plain text or HTML body
	Attachments []Attachment // populated on format=full fetch
}

// Attachment holds metadata for a MIME attachment part.
type Attachment struct {
	Name         string
	MimeType     string
	Size         int64
	AttachmentID string // Gmail part attachment ID; empty for small inline attachments
	InlineData   []byte // decoded bytes for small attachments embedded in the message payload
}

// Thread is a Gmail thread.
type Thread struct {
	ID       string
	Messages []MessageMeta
}

// Label is a Gmail label.
type Label struct {
	ID              string
	Name            string
	LabelType       string // "system" or "user"
	TextColor       string // hex color for label text, e.g. "#ffffff"
	BackgroundColor string // hex color for label background, e.g. "#4a86e8"
}

// HistoryEvent represents a change from the history list.
type HistoryEvent struct {
	LabelsAdded   *LabelChange
	LabelsRemoved *LabelChange
}

// LabelChange records label additions/removals for a message.
type LabelChange struct {
	MessageID string
	LabelIDs  []string
}

// HistoryResponse is the result of a history.list call.
type HistoryResponse struct {
	AddedIDs  []string // new messages; fetch their metadata separately
	Events    []HistoryEvent
	HistoryID uint64
}

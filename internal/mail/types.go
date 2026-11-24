package mail

// OutboundMessage is an email to be sent or saved as draft.
type OutboundMessage struct {
	From    string
	To      string
	Cc      string
	Bcc     string
	Subject string
	Body    string
	// InReplyTo is the Message-ID header value when replying.
	InReplyTo string
	// References is the space-separated Message-ID chain of the thread; it
	// defaults to InReplyTo.
	References string
	// ThreadID is set when replying so Gmail keeps the thread.
	ThreadID string
	// Attachments are sent as multipart/mixed parts after the body.
	Attachments []Attachment
}

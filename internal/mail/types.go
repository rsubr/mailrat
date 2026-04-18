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
	// ThreadID is set when replying so Gmail keeps the thread.
	ThreadID string
}

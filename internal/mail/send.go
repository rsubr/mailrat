package mail

import (
	"context"
	"fmt"

	"github.com/mailrat/mailrat/internal/gmail"
)

// SendService sends and replies to emails.
type SendService struct {
	client *gmail.Client
}

// NewSendService creates a SendService.
func NewSendService(client *gmail.Client) *SendService {
	return &SendService{client: client}
}

// Send sends an outbound message. Returns the new message ID.
func (s *SendService) Send(ctx context.Context, msg OutboundMessage) (string, error) {
	raw := buildRaw(msg)
	id, err := s.client.SendMessage(ctx, gmail.EncodeRaw(raw), msg.ThreadID)
	if err != nil {
		return "", err
	}
	return id, nil
}

// buildRaw constructs a minimal RFC 2822 message string.
func buildRaw(msg OutboundMessage) string {
	var headers string
	headers += fmt.Sprintf("From: %s\r\n", msg.From)
	headers += fmt.Sprintf("To: %s\r\n", msg.To)
	headers += fmt.Sprintf("Subject: %s\r\n", msg.Subject)
	if msg.InReplyTo != "" {
		headers += fmt.Sprintf("In-Reply-To: %s\r\n", msg.InReplyTo)
		headers += fmt.Sprintf("References: %s\r\n", msg.InReplyTo)
	}
	headers += "MIME-Version: 1.0\r\n"
	headers += "Content-Type: text/plain; charset=\"UTF-8\"\r\n"
	headers += "\r\n"
	return headers + msg.Body
}

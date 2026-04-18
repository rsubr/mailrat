package mail

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// buildRaw constructs an RFC 2822 message. HTML bodies get multipart/alternative.
func buildRaw(msg OutboundMessage) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", msg.From))
	b.WriteString(fmt.Sprintf("To: %s\r\n", msg.To))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	if msg.InReplyTo != "" {
		b.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", msg.InReplyTo))
		b.WriteString(fmt.Sprintf("References: %s\r\n", msg.InReplyTo))
	}
	b.WriteString("MIME-Version: 1.0\r\n")

	if strings.HasPrefix(strings.TrimSpace(msg.Body), "<") {
		boundary := fmt.Sprintf("mailrat_%x", time.Now().UnixNano())
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary))
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		b.WriteString(htmlToText(msg.Body))
		b.WriteString("\r\n")
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
		b.WriteString(msg.Body)
		b.WriteString("\r\n")
		b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		b.WriteString(msg.Body)
	}
	return b.String()
}

// htmlToText converts HTML to plain text for the text/plain MIME part.
func htmlToText(s string) string {
	for _, tag := range []string{"</p>", "</div>", "</li>", "<br>", "<br/>", "<br />"} {
		s = strings.ReplaceAll(s, tag, "\n")
	}
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(r)
		}
	}
	s = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
		"&mdash;", "\u2014", "&ndash;", "\u2013",
	).Replace(buf.String())
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

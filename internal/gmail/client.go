package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Client wraps the Gmail v1 API.
type Client struct {
	svc       *gmailv1.Service
	accountID string
}

// NewClient creates a Gmail client from an authenticated HTTP client.
func NewClient(ctx context.Context, httpClient *http.Client, accountID string) (*Client, error) {
	svc, err := gmailv1.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}
	return &Client{svc: svc, accountID: accountID}, nil
}

// ListMessages returns message metadata matching the given query.
func (c *Client) ListMessages(ctx context.Context, query string, maxResults int64) ([]MessageMeta, error) {
	var out []MessageMeta
	pageToken := ""

	for {
		call := c.svc.Users.Messages.List("me").Q(query).MaxResults(maxResults)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := withRetry(ctx, func() (*gmailv1.ListMessagesResponse, error) {
			return call.Context(ctx).Do()
		})
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}

		for _, m := range resp.Messages {
			meta, err := c.GetMessageMeta(ctx, m.Id)
			if err != nil {
				return nil, err
			}
			out = append(out, meta)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

// GetMessageMeta fetches headers + metadata for a single message.
func (c *Client) GetMessageMeta(ctx context.Context, id string) (MessageMeta, error) {
	msg, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		return c.svc.Users.Messages.Get("me", id).
			Format("metadata").
			MetadataHeaders("Subject", "From", "To").
			Context(ctx).Do()
	})
	if err != nil {
		return MessageMeta{}, fmt.Errorf("get message %s: %w", id, err)
	}
	return parseMessageMeta(msg), nil
}

// GetMessage fetches the full message including body.
func (c *Client) GetMessage(ctx context.Context, id string) (Message, error) {
	msg, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		return c.svc.Users.Messages.Get("me", id).Format("full").Context(ctx).Do()
	})
	if err != nil {
		return Message{}, fmt.Errorf("get full message %s: %w", id, err)
	}

	meta := parseMessageMeta(msg)
	body := extractBody(msg.Payload)
	return Message{MessageMeta: meta, Body: body}, nil
}

// GetThread fetches a full thread.
func (c *Client) GetThread(ctx context.Context, threadID string) (Thread, error) {
	thread, err := withRetry(ctx, func() (*gmailv1.Thread, error) {
		return c.svc.Users.Threads.Get("me", threadID).Context(ctx).Do()
	})
	if err != nil {
		return Thread{}, fmt.Errorf("get thread %s: %w", threadID, err)
	}

	t := Thread{ID: thread.Id}
	for _, m := range thread.Messages {
		t.Messages = append(t.Messages, parseMessageMeta(m))
	}
	return t, nil
}

// ListLabels returns all labels for the account.
func (c *Client) ListLabels(ctx context.Context) ([]Label, error) {
	resp, err := withRetry(ctx, func() (*gmailv1.ListLabelsResponse, error) {
		return c.svc.Users.Labels.List("me").Context(ctx).Do()
	})
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}

	var out []Label
	for _, l := range resp.Labels {
		out = append(out, Label{
			ID:        l.Id,
			Name:      l.Name,
			LabelType: strings.ToLower(l.Type),
		})
	}
	return out, nil
}

// ModifyLabels adds and removes labels on a message.
func (c *Client) ModifyLabels(ctx context.Context, msgID string, add, remove []string) error {
	_, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		return c.svc.Users.Messages.Modify("me", msgID, &gmailv1.ModifyMessageRequest{
			AddLabelIds:    add,
			RemoveLabelIds: remove,
		}).Context(ctx).Do()
	})
	if err != nil {
		return fmt.Errorf("modify labels on %s: %w", msgID, err)
	}
	return nil
}

// SendMessage sends a raw RFC 2822 message (base64url encoded).
func (c *Client) SendMessage(ctx context.Context, rawBase64 string) (string, error) {
	msg, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		return c.svc.Users.Messages.Send("me", &gmailv1.Message{
			Raw: rawBase64,
		}).Context(ctx).Do()
	})
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return msg.Id, nil
}

// CreateDraft creates a draft and returns its ID.
func (c *Client) CreateDraft(ctx context.Context, rawBase64 string) (string, error) {
	draft, err := withRetry(ctx, func() (*gmailv1.Draft, error) {
		return c.svc.Users.Drafts.Create("me", &gmailv1.Draft{
			Message: &gmailv1.Message{Raw: rawBase64},
		}).Context(ctx).Do()
	})
	if err != nil {
		return "", fmt.Errorf("create draft: %w", err)
	}
	return draft.Id, nil
}

// GetHistoryList returns history events since startHistoryID.
func (c *Client) GetHistoryList(ctx context.Context, startHistoryID uint64) (*HistoryResponse, error) {
	var events []HistoryEvent
	var latestHistoryID uint64
	pageToken := ""

	for {
		call := c.svc.Users.History.List("me").StartHistoryId(startHistoryID)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := withRetry(ctx, func() (*gmailv1.ListHistoryResponse, error) {
			return call.Context(ctx).Do()
		})
		if err != nil {
			return nil, fmt.Errorf("history list: %w", err)
		}

		if resp.HistoryId > latestHistoryID {
			latestHistoryID = resp.HistoryId
		}

		for _, h := range resp.History {
			for _, ma := range h.MessagesAdded {
				meta := parseMessageMeta(ma.Message)
				e := meta // copy
				events = append(events, HistoryEvent{MessageAdded: &e})
			}
			for _, la := range h.LabelsAdded {
				events = append(events, HistoryEvent{LabelsAdded: &LabelChange{
					MessageID: la.Message.Id,
					LabelIDs:  la.LabelIds,
				}})
			}
			for _, lr := range h.LabelsRemoved {
				events = append(events, HistoryEvent{LabelsRemoved: &LabelChange{
					MessageID: lr.Message.Id,
					LabelIDs:  lr.LabelIds,
				}})
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return &HistoryResponse{Events: events, HistoryID: latestHistoryID}, nil
}

// GetProfile returns the current historyId from the Gmail profile.
func (c *Client) GetProfile(ctx context.Context) (uint64, error) {
	profile, err := withRetry(ctx, func() (*gmailv1.Profile, error) {
		return c.svc.Users.GetProfile("me").Context(ctx).Do()
	})
	if err != nil {
		return 0, fmt.Errorf("get profile: %w", err)
	}
	return uint64(profile.HistoryId), nil
}

// EncodeRaw base64url-encodes a raw RFC 2822 message.
func EncodeRaw(raw string) string {
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// --- helpers ---

func parseMessageMeta(m *gmailv1.Message) MessageMeta {
	meta := MessageMeta{
		ID:           m.Id,
		ThreadID:     m.ThreadId,
		Snippet:      m.Snippet,
		InternalDate: m.InternalDate,
		LabelIDs:     m.LabelIds,
	}

	for _, l := range m.LabelIds {
		if l == "UNREAD" {
			meta.IsUnread = true
		}
	}

	if m.Payload != nil {
		for _, h := range m.Payload.Headers {
			switch h.Name {
			case "Subject":
				meta.Subject = h.Value
			case "From":
				meta.From = h.Value
			case "To":
				meta.To = h.Value
			}
		}
	}
	return meta
}

func extractBody(payload *gmailv1.MessagePart) string {
	if payload == nil {
		return ""
	}

	// Prefer text/plain; fall back to text/html.
	plain := extractPart(payload, "text/plain")
	if plain != "" {
		return plain
	}
	return extractPart(payload, "text/html")
}

func extractPart(part *gmailv1.MessagePart, mimeType string) string {
	if part == nil {
		return ""
	}
	if part.MimeType == mimeType && part.Body != nil && part.Body.Data != "" {
		b, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return string(b)
		}
	}
	for _, p := range part.Parts {
		if s := extractPart(p, mimeType); s != "" {
			return s
		}
	}
	return ""
}

// withRetry calls fn with exponential backoff on 429/500 errors.
func withRetry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if !isRetryable(err) {
			return zero, err
		}
		wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}
	var result T
	return result, fmt.Errorf("max retries exceeded")
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "429") ||
		strings.Contains(s, "500") ||
		strings.Contains(s, "503") ||
		strings.Contains(s, "rateLimitExceeded") ||
		strings.Contains(s, "userRateLimitExceeded")
}

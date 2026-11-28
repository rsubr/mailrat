package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	mimeTextPlain = "text/plain"
	mimeTextHTML  = "text/html"

	headerSubject   = "Subject"
	headerFrom      = "From"
	headerTo        = "To"
	headerCC        = "Cc"
	headerBCC       = "Bcc"
	headerMessageID = "Message-ID"
	headerReplyTo   = "Reply-To"
	headerRefs      = "References"
	headerInReplyTo = "In-Reply-To"

	retryTooManyRequests = "429"
	retryInternalError   = "500"
	retryServiceUnavail  = "503"
	retryRateLimited     = "rateLimitExceeded"
	retryUserRateLimited = "userRateLimitExceeded"
)

// Client wraps the Gmail v1 API.
type Client struct {
	svc *gmailv1.Service
}

// NewClient creates a Gmail client from an authenticated HTTP client.
func NewClient(ctx context.Context, httpClient *http.Client, accountID string) (*Client, error) {
	svc, err := gmailv1.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// metaWorkers bounds concurrent per-message requests (Gmail allows ~50 get/s per user).
const metaWorkers = 4

// ListMessageIDs returns the IDs of messages matching query, newest first.
// limit caps the total returned; 0 means no cap.
func (c *Client) ListMessageIDs(ctx context.Context, query string, limit int64) ([]string, error) {
	var out []string
	pageToken := ""
	for {
		page := int64(500)
		if limit > 0 && limit-int64(len(out)) < page {
			page = limit - int64(len(out))
		}
		call := c.svc.Users.Messages.List("me").Q(query).MaxResults(page)
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
			out = append(out, m.Id)
		}
		if resp.NextPageToken == "" || (limit > 0 && int64(len(out)) >= limit) {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

// GetMessageMetas fetches metadata for many messages concurrently. It returns
// every message it could fetch plus the first error, so callers can keep
// partial progress.
func (c *Client) GetMessageMetas(ctx context.Context, ids []string) ([]MessageMeta, error) {
	metas := make([]MessageMeta, len(ids))
	ok := make([]bool, len(ids))
	var firstErr error
	var mu sync.Mutex
	sem := make(chan struct{}, metaWorkers)
	var wg sync.WaitGroup
	for i, id := range ids {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			meta, err := c.GetMessageMeta(ctx, id)
			if IsNotFound(err) { // deleted since it was listed: skip it
				return
			}
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			metas[i], ok[i] = meta, true
		}()
	}
	wg.Wait()
	out := metas[:0]
	for i := range metas {
		if ok[i] {
			out = append(out, metas[i])
		}
	}
	return out, firstErr
}

// GetMessageMeta fetches headers + metadata for a single message.
func (c *Client) GetMessageMeta(ctx context.Context, id string) (MessageMeta, error) {
	msg, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		return c.svc.Users.Messages.Get("me", id).
			Format("metadata").
			MetadataHeaders(headerSubject, headerFrom, headerTo, headerCC, headerBCC, headerMessageID, headerReplyTo, headerRefs, headerInReplyTo).
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
	if err := fillLargeBodyParts(msg.Payload, func(attID string) ([]byte, error) {
		return c.GetAttachmentData(ctx, id, attID)
	}); err != nil {
		return Message{}, fmt.Errorf("get full message %s: %w", id, err)
	}
	body := extractBody(msg.Payload)
	attachments := extractAttachments(msg.Payload)
	return Message{MessageMeta: meta, Body: body, Attachments: attachments}, nil
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
		lbl := Label{
			ID:        l.Id,
			Name:      l.Name,
			LabelType: strings.ToLower(l.Type),
		}
		if l.Color != nil {
			lbl.TextColor = l.Color.TextColor
			lbl.BackgroundColor = l.Color.BackgroundColor
		}
		out = append(out, lbl)
	}
	return out, nil
}

// SendAs is a send-as alias with its signature.
type SendAs struct {
	Email         string
	DisplayName   string
	SignatureHTML string
	IsPrimary     bool
	IsDefault     bool
}

// ListSendAs returns the account's send-as aliases and their signatures.
// Needs gmail.readonly (or modify/settings.basic), all already granted.
func (c *Client) ListSendAs(ctx context.Context) ([]SendAs, error) {
	resp, err := withRetry(ctx, func() (*gmailv1.ListSendAsResponse, error) {
		return c.svc.Users.Settings.SendAs.List("me").Context(ctx).Do()
	})
	if err != nil {
		return nil, fmt.Errorf("list send-as: %w", err)
	}
	return mapSendAs(resp.SendAs), nil
}

func mapSendAs(in []*gmailv1.SendAs) []SendAs {
	var out []SendAs
	for _, s := range in {
		out = append(out, SendAs{
			Email:         s.SendAsEmail,
			DisplayName:   s.DisplayName,
			SignatureHTML: s.Signature,
			IsPrimary:     s.IsPrimary,
			IsDefault:     s.IsDefault,
		})
	}
	return out
}

// CreateLabel creates a user label and returns it.
func (c *Client) CreateLabel(ctx context.Context, name string) (Label, error) {
	l, err := withRetry(ctx, func() (*gmailv1.Label, error) {
		return c.svc.Users.Labels.Create("me", &gmailv1.Label{
			Name:                  name,
			LabelListVisibility:   "labelShow",
			MessageListVisibility: "show",
		}).Context(ctx).Do()
	})
	if err != nil {
		return Label{}, fmt.Errorf("create label: %w", err)
	}
	return Label{ID: l.Id, Name: l.Name, LabelType: "user"}, nil
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

// ModifyThreadLabels adds and removes labels on every message of a thread in one call.
func (c *Client) ModifyThreadLabels(ctx context.Context, threadID string, add, remove []string) error {
	_, err := withRetry(ctx, func() (*gmailv1.Thread, error) {
		return c.svc.Users.Threads.Modify("me", threadID, &gmailv1.ModifyThreadRequest{
			AddLabelIds:    add,
			RemoveLabelIds: remove,
		}).Context(ctx).Do()
	})
	if err != nil {
		return fmt.Errorf("modify labels on thread %s: %w", threadID, err)
	}
	return nil
}

// TrashThread moves a whole thread to the Trash in one call.
func (c *Client) TrashThread(ctx context.Context, threadID string) error {
	_, err := withRetry(ctx, func() (*gmailv1.Thread, error) {
		return c.svc.Users.Threads.Trash("me", threadID).Context(ctx).Do()
	})
	if err != nil {
		return fmt.Errorf("trash thread %s: %w", threadID, err)
	}
	return nil
}

// SendMessage sends a raw RFC 2822 message (base64url encoded).
// threadID is optional; when non-empty it tells Gmail which thread to attach the message to.
// Messages over mediaThreshold are sent through the media upload endpoint.
func (c *Client) SendMessage(ctx context.Context, rawBase64, threadID string) (string, error) {
	msg, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		m := &gmailv1.Message{Raw: rawBase64, ThreadId: threadID}
		call := c.svc.Users.Messages.Send("me", m).Context(ctx)
		if r := mediaBody(m); r != nil {
			call.Media(r, googleapi.ContentType(rfc822))
		}
		return call.Do()
	})
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return msg.Id, nil
}

// CreateDraft creates a draft and returns its ID.
func (c *Client) CreateDraft(ctx context.Context, rawBase64 string) (string, error) {
	draft, err := withRetry(ctx, func() (*gmailv1.Draft, error) {
		d := &gmailv1.Draft{Message: &gmailv1.Message{Raw: rawBase64}}
		call := c.svc.Users.Drafts.Create("me", d).Context(ctx)
		if r := mediaBody(d.Message); r != nil {
			call.Media(r, googleapi.ContentType(rfc822))
		}
		return call.Do()
	})
	if err != nil {
		return "", fmt.Errorf("create draft: %w", err)
	}
	return draft.Id, nil
}

// FindDraft returns the Gmail draft ID behind a draft message, plus the
// message's In-Reply-To and References headers.
func (c *Client) FindDraft(ctx context.Context, messageID string) (draftID, inReplyTo, references string, err error) {
	page := ""
	for draftID == "" {
		resp, e := withRetry(ctx, func() (*gmailv1.ListDraftsResponse, error) {
			return c.svc.Users.Drafts.List("me").PageToken(page).Context(ctx).Do()
		})
		if e != nil {
			return "", "", "", fmt.Errorf("list drafts: %w", e)
		}
		draftID = matchDraft(resp.Drafts, messageID)
		if page = resp.NextPageToken; page == "" {
			break
		}
	}
	if draftID == "" {
		return "", "", "", fmt.Errorf("no draft for message %s", messageID)
	}
	msg, err := withRetry(ctx, func() (*gmailv1.Message, error) {
		return c.svc.Users.Messages.Get("me", messageID).Format("metadata").
			MetadataHeaders(headerInReplyTo, headerRefs).Context(ctx).Do()
	})
	if err != nil {
		return "", "", "", fmt.Errorf("get draft message %s: %w", messageID, err)
	}
	return draftID, headerValue(msg, headerInReplyTo), firstValue("", headerValue(msg, headerRefs)), nil
}

func matchDraft(drafts []*gmailv1.Draft, messageID string) string {
	for _, d := range drafts {
		if d.Message != nil && d.Message.Id == messageID {
			return d.Id
		}
	}
	return ""
}

func headerValue(m *gmailv1.Message, name string) string {
	if m.Payload != nil {
		for _, h := range m.Payload.Headers {
			if strings.EqualFold(h.Name, name) {
				return h.Value
			}
		}
	}
	return ""
}

// UpdateDraft updates an existing draft.
func (c *Client) UpdateDraft(ctx context.Context, draftID, rawBase64 string) error {
	_, err := withRetry(ctx, func() (*gmailv1.Draft, error) {
		d := &gmailv1.Draft{Message: &gmailv1.Message{Raw: rawBase64}}
		call := c.svc.Users.Drafts.Update("me", draftID, d).Context(ctx)
		if r := mediaBody(d.Message); r != nil {
			call.Media(r, googleapi.ContentType(rfc822))
		}
		return call.Do()
	})
	if err != nil {
		return fmt.Errorf("update draft %s: %w", draftID, err)
	}
	return nil
}

// DeleteDraft deletes a draft from Gmail.
func (c *Client) DeleteDraft(ctx context.Context, draftID string) error {
	_, err := withRetry(ctx, func() (struct{}, error) {
		err := c.svc.Users.Drafts.Delete("me", draftID).Context(ctx).Do()
		return struct{}{}, err
	})
	if err != nil {
		return fmt.Errorf("delete draft %s: %w", draftID, err)
	}
	return nil
}

// ErrHistoryExpired means startHistoryId is too old; a full sync is required.
var ErrHistoryExpired = errors.New("history id expired")

// GetHistoryList returns the changes since startHistoryID. New messages are
// reported as IDs only (see HistoryResponse.AddedIDs) so the caller can fetch
// them concurrently and skip ones it already has.
func (c *Client) GetHistoryList(ctx context.Context, startHistoryID uint64) (*HistoryResponse, error) {
	resp := &HistoryResponse{}
	added := map[string]struct{}{}
	deleted := map[string]struct{}{}
	pageToken := ""

	for {
		call := c.svc.Users.History.List("me").StartHistoryId(startHistoryID)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		page, err := withRetry(ctx, func() (*gmailv1.ListHistoryResponse, error) {
			return call.Context(ctx).Do()
		})
		if err != nil {
			var ge *googleapi.Error
			if errors.As(err, &ge) && ge.Code == http.StatusNotFound {
				return nil, ErrHistoryExpired
			}
			return nil, fmt.Errorf("history list: %w", err)
		}
		if page.HistoryId > resp.HistoryID {
			resp.HistoryID = page.HistoryId
		}

		for _, h := range page.History {
			for _, ma := range h.MessagesAdded {
				if _, dup := added[ma.Message.Id]; !dup {
					added[ma.Message.Id] = struct{}{}
					resp.AddedIDs = append(resp.AddedIDs, ma.Message.Id)
				}
			}
			for _, md := range h.MessagesDeleted {
				deleted[md.Message.Id] = struct{}{}
			}
			for _, la := range h.LabelsAdded {
				resp.Events = append(resp.Events, HistoryEvent{LabelsAdded: &LabelChange{
					MessageID: la.Message.Id,
					LabelIDs:  la.LabelIds,
				}})
			}
			for _, lr := range h.LabelsRemoved {
				resp.Events = append(resp.Events, HistoryEvent{LabelsRemoved: &LabelChange{
					MessageID: lr.Message.Id,
					LabelIDs:  lr.LabelIds,
				}})
			}
		}

		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}

	// A message added and deleted within the window no longer exists.
	kept := resp.AddedIDs[:0]
	for _, id := range resp.AddedIDs {
		if _, gone := deleted[id]; !gone {
			kept = append(kept, id)
		} else {
			delete(deleted, id)
		}
	}
	resp.AddedIDs = kept
	for id := range deleted {
		resp.DeletedIDs = append(resp.DeletedIDs, id)
	}
	sort.Strings(resp.DeletedIDs)
	return resp, nil
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

// mediaThreshold is the encoded size above which a message is uploaded as
// message/rfc822 media (multipart, or resumable in chunks) instead of as a
// base64 JSON field, which Gmail only accepts for small requests.
const mediaThreshold = 5 << 20

const rfc822 = "message/rfc822"

// mediaBody moves a large message's Raw into an upload body (clearing Raw on m)
// and returns it, or returns nil when m is small enough to send as JSON.
func mediaBody(m *gmailv1.Message) io.Reader {
	if len(m.Raw) <= mediaThreshold {
		return nil
	}
	raw, err := base64.URLEncoding.DecodeString(m.Raw)
	if err != nil {
		return nil // let Gmail report the bad encoding
	}
	m.Raw = ""
	return bytes.NewReader(raw)
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
			case headerSubject:
				meta.Subject = h.Value
			case headerFrom:
				meta.From = h.Value
			case headerTo:
				meta.To = h.Value
			case headerCC:
				meta.Cc = h.Value
			case headerBCC:
				meta.Bcc = h.Value
			case headerMessageID:
				meta.MessageID = h.Value
			default:
				// Header names are case-insensitive and Gmail echoes them as sent.
				// A repeated header keeps its first value.
				switch {
				case strings.EqualFold(h.Name, headerReplyTo):
					meta.ReplyTo = firstValue(meta.ReplyTo, h.Value)
				case strings.EqualFold(h.Name, headerRefs):
					meta.References = firstValue(meta.References, h.Value)
				case strings.EqualFold(h.Name, headerInReplyTo):
					meta.InReplyTo = firstValue(meta.InReplyTo, h.Value)
				}
			}
		}
	}
	return meta
}

// firstValue keeps cur when set, else the unfolded v.
func firstValue(cur, v string) string {
	if cur != "" {
		return cur
	}
	return strings.Join(strings.Fields(v), " ")
}

func extractBody(payload *gmailv1.MessagePart) string {
	if payload == nil {
		return ""
	}

	// Prefer text/html (the renderer handles it and it carries the author's
	// layout); fall back to text/plain.
	if h := extractPart(payload, mimeTextHTML); strings.TrimSpace(h) != "" {
		return h
	}
	return extractPart(payload, mimeTextPlain)
}

// fillLargeBodyParts loads text/html and text/plain body parts that Gmail
// returns with body.attachmentId and no inline data, so extractBody sees the
// complete text. Any fetch failure is returned: an empty body must never be
// cached as if it were the message.
func fillLargeBodyParts(part *gmailv1.MessagePart, fetch func(attachmentID string) ([]byte, error)) error {
	if part == nil {
		return nil
	}
	if part.Filename == "" && part.Body != nil && part.Body.AttachmentId != "" && part.Body.Data == "" &&
		(strings.EqualFold(part.MimeType, mimeTextHTML) || strings.EqualFold(part.MimeType, mimeTextPlain)) {
		b, err := fetch(part.Body.AttachmentId)
		if err != nil {
			return err
		}
		part.Body.Data = base64.RawURLEncoding.EncodeToString(b)
	}
	for _, p := range part.Parts {
		if err := fillLargeBodyParts(p, fetch); err != nil {
			return err
		}
	}
	return nil
}

func extractPart(part *gmailv1.MessagePart, mimeType string) string {
	if part == nil {
		return ""
	}
	if strings.EqualFold(part.MimeType, mimeType) && part.Body != nil && part.Body.Data != "" {
		if s, ok := decodePartBody(part); ok {
			return s
		}
	}
	for _, p := range part.Parts {
		if s := extractPart(p, mimeType); s != "" {
			return s
		}
	}
	return ""
}

// decodePartBody base64url-decodes a part body and converts it to UTF-8 using
// the charset in its Content-Type header, else the HTML meta declaration.
func decodePartBody(part *gmailv1.MessagePart) (string, bool) {
	data := strings.TrimRight(part.Body.Data, "=")
	b, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return "", false
	}
	ctype := part.MimeType
	for _, h := range part.Headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			ctype = h.Value
			break
		}
	}
	return toUTF8(b, ctype), true
}

// toUTF8 converts b to UTF-8. Bytes that already are valid UTF-8 are kept
// whatever the declared legacy charset says: senders routinely label UTF-8 as
// windows-1252 or us-ascii, and decoding it again yields "â€"" for a dash.
func toUTF8(b []byte, ctype string) string {
	label := ""
	if _, params, err := mime.ParseMediaType(ctype); err == nil {
		label = strings.ToLower(params["charset"])
	}
	wide := strings.HasPrefix(label, "utf-16") || strings.HasPrefix(label, "utf-32")
	if utf8.Valid(b) && !wide {
		return string(b)
	}
	if r, err := charset.NewReader(bytes.NewReader(b), ctype); err == nil {
		if conv, err := io.ReadAll(r); err == nil {
			return string(conv)
		}
	}
	return string(b)
}

// extractAttachments walks the MIME part tree and returns attachment metadata.
// Binary content is not fetched here — that requires a separate attachments.get call.
func extractAttachments(payload *gmailv1.MessagePart) []Attachment {
	if payload == nil {
		return nil
	}
	var out []Attachment
	collectAttachments(payload, &out)
	return out
}

func collectAttachments(part *gmailv1.MessagePart, out *[]Attachment) {
	if part == nil {
		return
	}
	// A part is an attachment if it has a filename.
	if part.Filename != "" {
		size := int64(0)
		attachmentID := ""
		var inlineData []byte
		if part.Body != nil {
			size = part.Body.Size
			attachmentID = part.Body.AttachmentId
			if attachmentID == "" && part.Body.Data != "" {
				// Small attachment: binary is embedded inline as base64url.
				decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
				if err != nil {
					decoded, _ = base64.StdEncoding.DecodeString(part.Body.Data)
				}
				inlineData = decoded
				// Use a stable synthetic ID so callers can reference this attachment.
				attachmentID = "name:" + part.Filename
			}
		}
		*out = append(*out, Attachment{
			Name:         part.Filename,
			MimeType:     part.MimeType,
			Size:         size,
			AttachmentID: attachmentID,
			InlineData:   inlineData,
		})
	}
	for _, p := range part.Parts {
		collectAttachments(p, out)
	}
}

// GetAttachmentData fetches the binary content of an attachment from Gmail.
// Returns the raw decoded bytes.
func (c *Client) GetAttachmentData(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	att, err := withRetry(ctx, func() (*gmailv1.MessagePartBody, error) {
		return c.svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	})
	if err != nil {
		return nil, fmt.Errorf("get attachment %s: %w", attachmentID, err)
	}
	data, err := base64.URLEncoding.DecodeString(att.Data)
	if err != nil {
		// Gmail may use standard or URL encoding
		data, err = base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			return nil, fmt.Errorf("decode attachment data: %w", err)
		}
	}
	return data, nil
}

// IsNotFound reports whether err is Gmail's 404: the message or thread no
// longer exists on the server.
func IsNotFound(err error) bool {
	var ge *googleapi.Error
	return errors.As(err, &ge) && ge.Code == http.StatusNotFound
}

// IsPermanent reports whether err is a Gmail client error (4xx other than rate
// limiting) that retrying cannot fix, such as a message that no longer exists.
func IsPermanent(err error) bool {
	var ge *googleapi.Error
	if !errors.As(err, &ge) {
		return false
	}
	return ge.Code >= 400 && ge.Code < 500 && ge.Code != http.StatusTooManyRequests && ge.Code != http.StatusRequestTimeout
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
	return strings.Contains(s, retryTooManyRequests) ||
		strings.Contains(s, retryInternalError) ||
		strings.Contains(s, retryServiceUnavail) ||
		strings.Contains(s, retryRateLimited) ||
		strings.Contains(s, retryUserRateLimited)
}

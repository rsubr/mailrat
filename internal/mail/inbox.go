package mail

import (
	"context"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// InboxService provides inbox-level operations.
type InboxService struct {
	store  *storage.Store
	client *gmail.Client // used to lazy-fetch bodies on demand
}

// NewInboxService creates an InboxService.
func NewInboxService(store *storage.Store, client *gmail.Client) *InboxService {
	return &InboxService{store: store, client: client}
}

// GetMessage returns a single message, fetching the body from Gmail if not cached.
func (s *InboxService) GetMessage(ctx context.Context, accountID, messageID string) (storage.Message, error) {
	msg, err := s.store.GetMessage(accountID, messageID)
	if err != nil {
		return storage.Message{}, err
	}

	if msg.CacheState == storage.CacheMetadataOnly && s.client != nil {
		full, err := s.client.GetMessage(ctx, messageID)
		if err == nil {
			msg.Body = full.Body
			msg.CacheState = storage.CacheBodyCached
			msg.HasAttachments = len(full.Attachments) > 0
			if full.MessageID != "" {
				msg.MessageID = full.MessageID
			}
			if full.Cc != "" {
				msg.CcAddr = full.Cc
			}
			if full.Bcc != "" {
				msg.BccAddr = full.Bcc
			}
			_ = s.store.UpsertMessage(msg)
			if msg.HasAttachments {
				atts := make([]storage.Attachment, len(full.Attachments))
				for i, a := range full.Attachments {
					// Bytes of large attachments are not downloaded here: that
					// would block opening the thread. They are fetched and
					// cached when the attachment is saved.
					atts[i] = storage.Attachment{
						Name:         a.Name,
						MimeType:     a.MimeType,
						Size:         a.Size,
						AttachmentID: a.AttachmentID,
						Data:         a.InlineData,
					}
				}
				_ = s.store.UpsertAttachments(accountID, messageID, atts)
			}
		}
	}

	return msg, nil
}

package mail

import (
	"context"
	"sort"

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

// GetUnifiedInbox returns messages across all accounts, sorted newest first.
func (s *InboxService) GetUnifiedInbox(limit int) ([]storage.Message, error) {
	return s.store.ListAllMessages(limit)
}

// GetInbox returns messages for a single account.
func (s *InboxService) GetInbox(accountID string, limit int) ([]storage.Message, error) {
	return s.store.ListMessages(accountID, limit)
}

// GetThread returns all messages in a thread, oldest first.
func (s *InboxService) GetThread(accountID, threadID string) ([]storage.Message, error) {
	return s.store.ListMessagesByThread(accountID, threadID)
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
					atts[i] = storage.Attachment{Name: a.Name, MimeType: a.MimeType, Size: a.Size}
				}
				_ = s.store.UpsertAttachments(accountID, messageID, atts)
			}
		}
	}

	return msg, nil
}

// GetThreadsForInbox returns threads for the unified inbox sorted by last message date.
func (s *InboxService) GetThreadsForInbox(accountIDs []string, limit int) ([]storage.Thread, error) {
	var all []storage.Thread
	for _, id := range accountIDs {
		threads, err := s.store.ListThreads(id, limit)
		if err != nil {
			return nil, err
		}
		all = append(all, threads...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].LastMessageDate > all[j].LastMessageDate
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

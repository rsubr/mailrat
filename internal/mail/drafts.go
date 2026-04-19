package mail

import (
	"context"
	"time"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// DraftService manages email drafts.
type DraftService struct {
	store  *storage.Store
	client *gmail.Client
}

// NewDraftService creates a DraftService.
func NewDraftService(store *storage.Store, client *gmail.Client) *DraftService {
	return &DraftService{store: store, client: client}
}

// saveDraftToCache persists a draft to local storage.
func (s *DraftService) saveDraftToCache(draftID, accountID, raw string) error {
	draft := storage.Draft{
		ID:        draftID,
		AccountID: accountID,
		Content:   raw,
		UpdatedAt: time.Now().UnixMilli(),
	}
	return s.store.UpsertDraft(draft)
}

// SaveDraft creates a draft in Gmail and caches it locally. Returns the draft ID.
func (s *DraftService) SaveDraft(ctx context.Context, accountID string, msg OutboundMessage) (string, error) {
	raw := buildRaw(msg)
	id, err := s.client.CreateDraft(ctx, gmail.EncodeRaw(raw))
	if err != nil {
		return "", err
	}
	if err := s.saveDraftToCache(id, accountID, raw); err != nil {
		return "", err
	}
	return id, nil
}

// GetDraft returns a draft from local storage.
func (s *DraftService) GetDraft(accountID, draftID string) (storage.Draft, error) {
	return s.store.GetDraft(accountID, draftID)
}

// UpdateDraft updates a draft in Gmail and the local cache.
func (s *DraftService) UpdateDraft(ctx context.Context, accountID, draftID string, msg OutboundMessage) error {
	raw := buildRaw(msg)
	if err := s.client.UpdateDraft(ctx, draftID, gmail.EncodeRaw(raw)); err != nil {
		return err
	}
	return s.saveDraftToCache(draftID, accountID, raw)
}

// DeleteDraft deletes a draft from Gmail and local storage.
func (s *DraftService) DeleteDraft(ctx context.Context, accountID, draftID string) error {
	if err := s.client.DeleteDraft(ctx, draftID); err != nil {
		return err
	}
	return s.store.DeleteDraft(accountID, draftID)
}

package mail

import (
	"context"
	"errors"
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

// SaveLocalDraft stores a draft locally and queues it for Gmail. It never
// touches the network; the queued op runs on the next drain/sync and updates
// the same Gmail draft (never a duplicate) once one exists.
func SaveLocalDraft(store *storage.Store, localID, accountID string, msg OutboundMessage) error {
	if err := store.UpsertDraft(storage.Draft{ID: localID, AccountID: accountID, Content: buildRaw(msg), UpdatedAt: time.Now().UnixMilli()}); err != nil {
		return err
	}
	return store.AddPendingOp(storage.OpSyncDraft, accountID, localID)
}

// DiscardLocalDraft removes a local draft, and its Gmail copy if it was synced.
func DiscardLocalDraft(store *storage.Store, accountID, localID string) error {
	d, err := store.GetDraft(accountID, localID)
	if errors.Is(err, storage.ErrDraftNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if d.RemoteID == "" {
		return store.DeleteDraft(accountID, localID) // a queued sync op becomes a no-op
	}
	return store.AddPendingOp(storage.OpDeleteDraft, accountID, localID)
}

// IsDraftOp reports whether op is a draft sync/delete operation.
func IsDraftOp(opType string) bool {
	return opType == storage.OpSyncDraft || opType == storage.OpDeleteDraft
}

// ApplyDraftOp performs a queued draft operation against Gmail.
func ApplyDraftOp(ctx context.Context, store *storage.Store, client *gmail.Client, op storage.PendingOp) error {
	d, err := store.GetDraft(op.AccountID, op.MessageID)
	if errors.Is(err, storage.ErrDraftNotFound) {
		return nil // discarded before it ever synced
	}
	if err != nil {
		return err
	}
	if op.OpType == storage.OpDeleteDraft {
		if d.RemoteID != "" {
			if err := client.DeleteDraft(ctx, d.RemoteID); err != nil && !gmail.IsPermanent(err) {
				return err
			}
		}
		return store.DeleteDraft(op.AccountID, op.MessageID)
	}
	raw := gmail.EncodeRaw(d.Content)
	if d.RemoteID != "" {
		return client.UpdateDraft(ctx, d.RemoteID, raw)
	}
	id, err := client.CreateDraft(ctx, raw)
	if err != nil {
		return err
	}
	return store.SetDraftRemoteID(op.AccountID, op.MessageID, id)
}

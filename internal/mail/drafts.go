package mail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// SaveLocalDraft stores a draft locally and queues it for Gmail. It never
// touches the network; the queued op runs on the next drain/sync and updates
// the same Gmail draft (never a duplicate) once one exists.
func SaveLocalDraft(store *storage.Store, localID, accountID string, msg OutboundMessage) error {
	if err := store.UpsertDraft(storage.Draft{ID: localID, AccountID: accountID, Content: buildRaw(msg), UpdatedAt: time.Now().UnixMilli()}); err != nil {
		return err
	}
	return store.AddPendingOp(storage.OpSyncDraft, accountID, localID)
}

// LinkRemoteDraft returns the local draft for a Gmail draft, creating one
// (without queueing a sync) when the draft was made elsewhere. Saving it later
// then updates the Gmail draft rather than creating another.
func LinkRemoteDraft(store *storage.Store, accountID, remoteID string, msg OutboundMessage) (string, error) {
	d, err := store.FindDraftByRemoteID(accountID, remoteID)
	if err == nil {
		return d.ID, nil
	}
	if !errors.Is(err, storage.ErrDraftNotFound) {
		return "", err
	}
	id := fmt.Sprintf("local-%d", time.Now().UnixNano())
	if err := store.UpsertDraft(storage.Draft{ID: id, AccountID: accountID, Content: buildRaw(msg), UpdatedAt: time.Now().UnixMilli()}); err != nil {
		return "", err
	}
	return id, store.SetDraftRemoteID(accountID, id, remoteID)
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

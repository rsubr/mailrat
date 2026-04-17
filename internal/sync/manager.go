package sync

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// Manager orchestrates sync operations for an account.
type Manager struct {
	store  *storage.Store
	client *gmail.Client
}

// NewManager creates a SyncManager.
func NewManager(store *storage.Store, client *gmail.Client) *Manager {
	return &Manager{store: store, client: client}
}

// InitialSync fetches messages newer than `days` days for the account and
// stores them as metadata_only.
func (m *Manager) InitialSync(ctx context.Context, accountID string, days int) error {
	query := fmt.Sprintf("newer_than:%dd", days)

	fmt.Printf("Starting initial sync for account %s (last %d days)...\n", accountID, days)

	messages, err := m.client.ListMessages(ctx, query, 500)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	fmt.Printf("Fetched %d message metadata records\n", len(messages))

	// Sync labels first.
	if err := m.syncLabels(ctx, accountID); err != nil {
		return fmt.Errorf("sync labels: %w", err)
	}

	for _, meta := range messages {
		msg := storage.Message{
			ID:           meta.ID,
			ThreadID:     meta.ThreadID,
			AccountID:    accountID,
			Subject:      meta.Subject,
			FromAddr:     meta.From,
			ToAddr:       meta.To,
			Snippet:      meta.Snippet,
			InternalDate: meta.InternalDate,
			IsUnread:     meta.IsUnread,
			CacheState:   storage.CacheMetadataOnly,
		}
		if err := m.store.UpsertMessage(msg); err != nil {
			return fmt.Errorf("upsert message %s: %w", meta.ID, err)
		}

		thread := storage.Thread{
			ID:              meta.ThreadID,
			AccountID:       accountID,
			LastMessageDate: meta.InternalDate,
		}
		if err := m.store.UpsertThread(thread); err != nil {
			return fmt.Errorf("upsert thread %s: %w", meta.ThreadID, err)
		}

		if err := m.store.SetMessageLabels(accountID, meta.ID, meta.LabelIDs); err != nil {
			return fmt.Errorf("set labels on %s: %w", meta.ID, err)
		}
	}

	// Record the latest historyId from the first message (they share history).
	if len(messages) > 0 {
		// Get the actual historyId by fetching the profile.
		histID, err := m.fetchHistoryID(ctx)
		if err == nil {
			if err := m.store.UpdateHistoryID(accountID, strconv.FormatUint(histID, 10)); err != nil {
				return fmt.Errorf("update history id: %w", err)
			}
		}
	}

	fmt.Printf("Initial sync complete: %d messages stored\n", len(messages))
	return nil
}

// IncrementalSync uses the historyId to fetch only changes since last sync.
func (m *Manager) IncrementalSync(ctx context.Context, accountID string) error {
	acc, err := m.store.GetAccount(accountID)
	if err != nil {
		return err
	}

	if acc.HistoryID == "" {
		return fmt.Errorf("no history ID stored; run initial sync first")
	}

	startHistoryID, err := strconv.ParseUint(acc.HistoryID, 10, 64)
	if err != nil {
		return fmt.Errorf("parse history id: %w", err)
	}

	fmt.Printf("Incremental sync from historyId %d...\n", startHistoryID)

	resp, err := m.client.GetHistoryList(ctx, startHistoryID)
	if err != nil {
		return fmt.Errorf("get history: %w", err)
	}

	added := 0
	labelChanges := 0
	for _, event := range resp.Events {
		switch {
		case event.MessageAdded != nil:
			meta := event.MessageAdded
			msg := storage.Message{
				ID:           meta.ID,
				ThreadID:     meta.ThreadID,
				AccountID:    accountID,
				Subject:      meta.Subject,
				FromAddr:     meta.From,
				ToAddr:       meta.To,
				Snippet:      meta.Snippet,
				InternalDate: meta.InternalDate,
				IsUnread:     meta.IsUnread,
				CacheState:   storage.CacheMetadataOnly,
			}
			if err := m.store.UpsertMessage(msg); err != nil {
				return err
			}
			if err := m.store.UpsertThread(storage.Thread{
				ID:              meta.ThreadID,
				AccountID:       accountID,
				LastMessageDate: meta.InternalDate,
			}); err != nil {
				return err
			}
			if err := m.store.SetMessageLabels(accountID, meta.ID, meta.LabelIDs); err != nil {
				return err
			}
			added++

		case event.LabelsAdded != nil:
			lc := event.LabelsAdded
			existing, _ := m.store.GetMessageLabelIDs(accountID, lc.MessageID)
			merged := mergeLabelIDs(existing, lc.LabelIDs)
			_ = m.store.SetMessageLabels(accountID, lc.MessageID, merged)
			labelChanges++

		case event.LabelsRemoved != nil:
			lc := event.LabelsRemoved
			existing, _ := m.store.GetMessageLabelIDs(accountID, lc.MessageID)
			updated := removeLabelIDs(existing, lc.LabelIDs)
			_ = m.store.SetMessageLabels(accountID, lc.MessageID, updated)
			labelChanges++
		}
	}

	if resp.HistoryID > 0 {
		if err := m.store.UpdateHistoryID(accountID, strconv.FormatUint(resp.HistoryID, 10)); err != nil {
			return fmt.Errorf("update history id: %w", err)
		}
	}

	fmt.Printf("Incremental sync complete: %d new messages, %d label changes\n", added, labelChanges)
	return nil
}

// FetchBodies fetches the full body for messages that only have metadata.
func (m *Manager) FetchBodies(ctx context.Context, accountID string, limit int) error {
	msgs, err := m.store.ListUncachedBodies(accountID, limit)
	if err != nil {
		return err
	}

	fmt.Printf("Fetching bodies for %d messages...\n", len(msgs))

	for _, msg := range msgs {
		full, err := m.client.GetMessage(ctx, msg.ID)
		if err != nil {
			fmt.Printf("warning: failed to fetch body for %s: %v\n", msg.ID, err)
			continue
		}
		msg.Body = full.Body
		msg.CacheState = storage.CacheBodyCached
		if err := m.store.UpsertMessage(msg); err != nil {
			return err
		}
	}

	fmt.Printf("Body fetch complete\n")
	return nil
}

func (m *Manager) syncLabels(ctx context.Context, accountID string) error {
	labels, err := m.client.ListLabels(ctx)
	if err != nil {
		return err
	}
	for _, l := range labels {
		if err := m.store.UpsertLabel(storage.Label{
			ID:        l.ID,
			AccountID: accountID,
			Name:      l.Name,
			LabelType: l.LabelType,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) fetchHistoryID(ctx context.Context) (uint64, error) {
	// Get the current historyId from the Gmail profile endpoint.
	profile, err := m.client.GetProfile(ctx)
	if err != nil {
		return 0, err
	}
	return profile, nil
}

func mergeLabelIDs(existing, add []string) []string {
	set := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		set[id] = struct{}{}
	}
	for _, id := range add {
		set[id] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func removeLabelIDs(existing, remove []string) []string {
	rem := make(map[string]struct{}, len(remove))
	for _, id := range remove {
		rem[id] = struct{}{}
	}
	var out []string
	for _, id := range existing {
		if _, skip := rem[id]; !skip {
			out = append(out, id)
		}
	}
	return out
}

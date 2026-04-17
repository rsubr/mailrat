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

// InitialSync fetches historical messages for the account.
// configs controls per-label behaviour (days_to_sync=0 means all mail).
// defaultDays is used for labels without a config entry.
func (m *Manager) InitialSync(ctx context.Context, accountID string, defaultDays int, configs []storage.SyncConfig) error {
	// Sync labels first so label rows exist before we reference them.
	if err := m.syncLabels(ctx, accountID); err != nil {
		return fmt.Errorf("sync labels: %w", err)
	}

	// Partition configs: "all mail" labels vs "limited days" labels.
	allMailLabels := make(map[string]storage.SyncConfig)
	maxDays := defaultDays
	for _, c := range configs {
		if c.DaysToSync == 0 {
			allMailLabels[c.LabelID] = c
		} else if c.DaysToSync > maxDays {
			maxDays = c.DaysToSync
		}
	}

	fmt.Printf("Starting initial sync for account %s (base: last %d days)...\n", accountID, maxDays)

	seen := make(map[string]bool)

	// Base query: everything within the day window (covers all limited labels).
	baseQuery := fmt.Sprintf("newer_than:%dd", maxDays)
	if err := m.fetchAndStore(ctx, accountID, baseQuery, 500, seen); err != nil {
		return fmt.Errorf("base sync: %w", err)
	}

	// Per "all mail" label: unlimited queries.
	for labelID, cfg := range allMailLabels {
		q := labelIDToGmailQuery(labelID, cfg.LabelName)
		if q == "" {
			continue
		}
		fmt.Printf("Fetching all mail for label %q (%s)...\n", cfg.LabelName, q)
		if err := m.fetchAndStore(ctx, accountID, q, 2000, seen); err != nil {
			fmt.Printf("warning: failed to fetch label %q: %v\n", cfg.LabelName, err)
		}
	}

	// Fetch bodies/attachments for labels that request it.
	var attachLabelIDs []string
	for _, c := range configs {
		if c.FetchAttachments {
			attachLabelIDs = append(attachLabelIDs, c.LabelID)
		}
	}
	if len(attachLabelIDs) > 0 {
		fmt.Printf("Fetching bodies/attachments for configured labels...\n")
		if err := m.FetchBodiesForLabels(ctx, accountID, attachLabelIDs, 200); err != nil {
			fmt.Printf("warning: fetch bodies for labels: %v\n", err)
		}
	}

	// Record the latest historyId.
	histID, err := m.fetchHistoryID(ctx)
	if err == nil {
		if err := m.store.UpdateHistoryID(accountID, strconv.FormatUint(histID, 10)); err != nil {
			return fmt.Errorf("update history id: %w", err)
		}
	}

	fmt.Printf("Initial sync complete: %d messages stored\n", len(seen))
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

// BackfillFromConfigs fetches historical messages for any label whose config
// has not yet been fully backfilled. Call this after IncrementalSync.
// A label is considered backfilled once we've run its historical query at least
// once; the result is persisted so subsequent syncs skip it.
func (m *Manager) BackfillFromConfigs(ctx context.Context, accountID string, configs []storage.SyncConfig) error {
	// Seed defaults for INBOX and STARRED when the user hasn't explicitly
	// configured them yet. Without this, BackfillFromConfigs is a no-op on a
	// fresh install and the user only ever sees the initial 30-day window.
	existing := make(map[string]bool, len(configs))
	for _, c := range configs {
		existing[c.LabelID] = true
	}
	defaults := []storage.SyncConfig{
		{LabelID: "INBOX", LabelName: "INBOX", AccountID: accountID, DaysToSync: 0, FetchAttachments: true},
		{LabelID: "STARRED", LabelName: "STARRED", AccountID: accountID, DaysToSync: 0, FetchAttachments: true},
	}
	for _, d := range defaults {
		if !existing[d.LabelID] {
			if err := m.store.UpsertSyncConfig(d); err == nil {
				configs = append(configs, d)
			}
		}
	}

	seen := make(map[string]bool)
	for _, c := range configs {
		if c.Backfilled {
			continue
		}

		q := buildBackfillQuery(c)
		if q == "" {
			// No meaningful query — mark done so we don't retry endlessly.
			_ = m.store.MarkSyncConfigBackfilled(c.LabelID, accountID)
			continue
		}

		fmt.Printf("Backfilling %q (%s)...\n", c.LabelName, q)
		if err := m.fetchAndStore(ctx, accountID, q, 2000, seen); err != nil {
			fmt.Printf("warning: backfill for %q failed: %v\n", c.LabelName, err)
			continue
		}

		if err := m.store.MarkSyncConfigBackfilled(c.LabelID, accountID); err != nil {
			return fmt.Errorf("mark backfilled %s: %w", c.LabelID, err)
		}
		fmt.Printf("Backfill complete for %q\n", c.LabelName)
	}

	// Fetch bodies/attachments for labels that request it (runs after messages land).
	var attachLabelIDs []string
	for _, c := range configs {
		if c.FetchAttachments {
			attachLabelIDs = append(attachLabelIDs, c.LabelID)
		}
	}
	if len(attachLabelIDs) > 0 {
		if err := m.FetchBodiesForLabels(ctx, accountID, attachLabelIDs, 200); err != nil {
			fmt.Printf("warning: fetch bodies for labels: %v\n", err)
		}
	}

	return nil
}

// buildBackfillQuery returns the Gmail search query for a label's backfill.
// Returns "" if no query is needed (label has no meaningful scope).
func buildBackfillQuery(c storage.SyncConfig) string {
	labelQ := labelIDToGmailQuery(c.LabelID, c.LabelName)
	if labelQ == "" {
		return ""
	}
	if c.DaysToSync == 0 {
		return labelQ // all mail for this label
	}
	return fmt.Sprintf("%s newer_than:%dd", labelQ, c.DaysToSync)
}

// FetchBodiesForLabels fetches bodies/attachment metadata for metadata-only
// messages that carry at least one of the given label IDs.
func (m *Manager) FetchBodiesForLabels(ctx context.Context, accountID string, labelIDs []string, limit int) error {
	msgs, err := m.store.ListUncachedBodiesForLabels(accountID, labelIDs, limit)
	if err != nil {
		return err
	}

	fmt.Printf("Fetching bodies for %d messages (label-filtered)...\n", len(msgs))

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
		if len(full.Attachments) > 0 {
			atts := make([]storage.Attachment, len(full.Attachments))
			for i, a := range full.Attachments {
				atts[i] = storage.Attachment{Name: a.Name, MimeType: a.MimeType, Size: a.Size}
			}
			_ = m.store.UpsertAttachments(accountID, msg.ID, atts)
		}
	}
	return nil
}

// fetchAndStore runs a Gmail query and upserts results, skipping IDs in seen.
func (m *Manager) fetchAndStore(ctx context.Context, accountID, query string, maxResults int64, seen map[string]bool) error {
	messages, err := m.client.ListMessages(ctx, query, maxResults)
	if err != nil {
		return fmt.Errorf("list messages (%s): %w", query, err)
	}
	for _, meta := range messages {
		if seen[meta.ID] {
			continue
		}
		seen[meta.ID] = true

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
		if err := m.store.UpsertThread(storage.Thread{
			ID:              meta.ThreadID,
			AccountID:       accountID,
			LastMessageDate: meta.InternalDate,
		}); err != nil {
			return fmt.Errorf("upsert thread %s: %w", meta.ThreadID, err)
		}
		if err := m.store.SetMessageLabels(accountID, meta.ID, meta.LabelIDs); err != nil {
			return fmt.Errorf("set labels on %s: %w", meta.ID, err)
		}
	}
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
	profile, err := m.client.GetProfile(ctx)
	if err != nil {
		return 0, err
	}
	return profile, nil
}

// labelIDToGmailQuery converts a Gmail label ID to a search query term.
func labelIDToGmailQuery(id, name string) string {
	switch id {
	case "INBOX":
		return "in:inbox"
	case "STARRED":
		return "is:starred"
	case "SENT":
		return "in:sent"
	case "DRAFTS":
		return "in:drafts"
	case "SPAM":
		return "in:spam"
	case "TRASH":
		return "in:trash"
	default:
		if name != "" {
			return "label:" + name
		}
		return ""
	}
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

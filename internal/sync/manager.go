package sync

import (
	"context"
	"fmt"
	"strconv"
	stdsync "sync"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// BodyFetchLimit is the max number of uncached messages to fetch bodies for per sync.
const BodyFetchLimit = 200

// Manager orchestrates sync operations for an account.
type Manager struct {
	store    *storage.Store
	client   *gmail.Client
	reporter progressReporter
	writeMu  stdsync.Mutex // serialises store writes made from concurrent fetches
}

// NewManager creates a SyncManager.
func NewManager(store *storage.Store, client *gmail.Client) *Manager {
	return &Manager{
		store:    store,
		client:   client,
		reporter: stdoutReporter{},
	}
}

// InitialSync fetches historical messages for the account.
// configs controls per-label behaviour (days_to_sync=0 means all mail).
// defaultDays is used for labels without a config entry.
func (m *Manager) InitialSync(ctx context.Context, accountID string, defaultDays int, configs []storage.SyncConfig) error {
	if err := m.syncLabels(ctx, accountID); err != nil {
		return fmt.Errorf("sync labels: %w", err)
	}

	configs, err := m.ensureDefaultBackfillConfigs(accountID, configs)
	if err != nil {
		return err
	}
	m.reporter.Printf("Starting initial sync for account %s (base: last %d days)...\n", accountID, defaultDays)

	seen := make(seenMessages)
	// Labels without their own scope get the account-wide default window.
	if q := initialSyncQuery(defaultDays); q != "" {
		if _, err := m.fetchAndStore(ctx, accountID, q, 0, seen); err != nil {
			return fmt.Errorf("base sync: %w", err)
		}
	}

	for _, plan := range buildLabelPlans(configs, false) {
		if plan.query != "" {
			m.reporter.Printf("Fetching label %q (%s)...\n", plan.config.LabelName, plan.query)
			if _, err := m.fetchAndStore(ctx, accountID, plan.query, plan.limit, seen); err != nil {
				m.reporter.Warnf("failed to fetch label %q: %v\n", plan.config.LabelName, err)
				continue
			}
		}
		if err := m.markBackfilled(plan.config); err != nil {
			return err
		}
	}

	if err := m.fetchConfiguredBodies(ctx, accountID, configs); err != nil {
		m.reporter.Warnf("fetch bodies for labels: %v\n", err)
	}

	if err := m.storeLatestHistoryID(ctx, accountID); err != nil {
		return err
	}

	m.reporter.Printf("Initial sync complete: %d messages stored\n", len(seen))
	return nil
}

// IncrementalSync uses the historyId to fetch only changes since last sync.
func (m *Manager) IncrementalSync(ctx context.Context, accountID string) error {
	startHistoryID, err := m.storedHistoryID(accountID)
	if err != nil {
		return err
	}

	m.reporter.Printf("Incremental sync from historyId %d...\n", startHistoryID)

	resp, err := m.client.GetHistoryList(ctx, startHistoryID)
	if err != nil {
		return fmt.Errorf("get history: %w", err)
	}

	// New messages: skip ones already stored, fetch the rest concurrently.
	var fresh []string
	for _, id := range resp.AddedIDs {
		if !m.store.HasMessage(accountID, id) {
			fresh = append(fresh, id)
		}
	}
	fetched, err := m.fetchAndStoreMetas(ctx, accountID, fresh)
	if err != nil {
		return fmt.Errorf("fetch new messages: %w", err)
	}
	skip := make(map[string]struct{}, len(fresh))
	for _, id := range fresh {
		skip[id] = struct{}{}
	}

	stats, err := m.applyHistoryEvents(accountID, resp.Events, skip)
	if err != nil {
		return err
	}
	stats.added = fetched

	if resp.HistoryID > 0 {
		if err := m.updateHistoryID(accountID, resp.HistoryID); err != nil {
			return err
		}
	}

	m.reporter.Printf("Incremental sync complete: %d new messages, %d label changes\n", stats.added, stats.labelChanges)
	return nil
}

// FetchBodies fetches the full body for messages that only have metadata.
func (m *Manager) FetchBodies(ctx context.Context, accountID string, limit int) error {
	msgs, err := m.store.ListUncachedBodies(accountID, limit)
	if err != nil {
		return err
	}

	m.reporter.Printf("Fetching bodies for %d messages...\n", len(msgs))
	if err := m.fetchBodiesIntoStore(ctx, accountID, msgs); err != nil {
		return err
	}
	m.reporter.Printf("Body fetch complete\n")
	return nil
}

// BackfillFromConfigs fetches historical messages for any label whose config
// has not yet been fully backfilled. Call this after IncrementalSync.
// A label is considered backfilled once we've run its historical query at least
// once; the result is persisted so subsequent syncs skip it.
func (m *Manager) BackfillFromConfigs(ctx context.Context, accountID string, configs []storage.SyncConfig) error {
	configs, err := m.ensureDefaultBackfillConfigs(accountID, configs)
	if err != nil {
		return err
	}

	seen := make(seenMessages)
	for _, plan := range buildLabelPlans(configs, true) {
		if err := m.runBackfillPlan(ctx, accountID, plan, seen); err != nil {
			return err
		}
	}

	if err := m.fetchConfiguredBodies(ctx, accountID, configs); err != nil {
		m.reporter.Warnf("fetch bodies for labels: %v\n", err)
	}

	return nil
}

// FetchMissingAttachments downloads binary data for attachment rows that have no cached data,
// filtered to messages carrying labels with fetch_attachments=true in configs.
func (m *Manager) FetchMissingAttachments(ctx context.Context, accountID string, configs []storage.SyncConfig) error {
	labelIDs := attachmentLabelIDs(configs)
	if len(labelIDs) == 0 {
		return nil
	}

	missing, err := m.store.ListMissingAttachmentRows(accountID, labelIDs, BodyFetchLimit)
	if err != nil {
		return fmt.Errorf("list missing attachments: %w", err)
	}
	if len(missing) == 0 {
		return nil
	}

	m.reporter.Printf("Fetching %d missing attachments...\n", len(missing))
	_ = forEachParallel(len(missing), func(i int) error {
		att := missing[i]
		data, err := m.client.GetAttachmentData(ctx, att.MessageID, att.AttachmentID)
		if err != nil {
			m.reporter.Warnf("fetch attachment %q (%s): %v\n", att.Name, att.MessageID, err)
			return nil
		}
		m.writeMu.Lock()
		defer m.writeMu.Unlock()
		if err := m.store.UpdateAttachmentData(accountID, att.MessageID, att.AttachmentID, data); err != nil {
			m.reporter.Warnf("store attachment %q (%s): %v\n", att.Name, att.MessageID, err)
		}
		return nil
	})
	m.reporter.Printf("Missing attachment fetch complete\n")
	return nil
}

// FetchBodiesForLabels fetches bodies/attachment metadata for metadata-only
// messages that carry at least one of the given label IDs.
func (m *Manager) FetchBodiesForLabels(ctx context.Context, accountID string, labelIDs []string, limit int) error {
	msgs, err := m.store.ListUncachedBodiesForLabels(accountID, labelIDs, limit)
	if err != nil {
		return err
	}

	m.reporter.Printf("Fetching bodies for %d messages (label-filtered)...\n", len(msgs))
	return m.fetchBodiesIntoStore(ctx, accountID, msgs)
}

// fetchAndStore lists message IDs for query (limit 0 = all), skips messages
// already stored or seen, and fetches metadata for the rest concurrently.
// It returns how many messages were stored.
func (m *Manager) fetchAndStore(ctx context.Context, accountID, query string, limit int64, seen seenMessages) (int, error) {
	ids, err := m.client.ListMessageIDs(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("list messages (%s): %w", query, err)
	}
	var fresh []string
	for _, id := range ids {
		if seen.Skip(id) || m.store.HasMessage(accountID, id) {
			continue
		}
		fresh = append(fresh, id)
	}
	return m.fetchAndStoreMetas(ctx, accountID, fresh)
}

func (m *Manager) fetchAndStoreMetas(ctx context.Context, accountID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	metas, fetchErr := m.client.GetMessageMetas(ctx, ids)
	for _, meta := range metas {
		if err := m.storeMessageMetadata(accountID, meta); err != nil {
			return 0, err
		}
	}
	return len(metas), fetchErr
}

// LoadOlder fetches up to n messages for a label that are older than anything
// stored locally and returns how many were added. It backs scroll-to-load-more.
func (m *Manager) LoadOlder(ctx context.Context, accountID, labelID string, n int) (int, error) {
	name := ""
	if labels, err := m.store.ListLabels(accountID); err == nil {
		for _, l := range labels {
			if l.ID == labelID {
				name = l.Name
			}
		}
	}
	query := labelIDToGmailQuery(labelID, name)
	if query == "" {
		return 0, nil
	}
	if oldest, err := m.store.OldestMessageDate(accountID, labelID); err == nil && oldest > 0 {
		query += fmt.Sprintf(" before:%d", oldest/1000)
	}
	return m.fetchAndStore(ctx, accountID, query, int64(n), make(seenMessages))
}

// RefreshThread re-reads a thread's messages and labels from Gmail, replacing
// local state. It is how a failed optimistic change is rolled back.
func (m *Manager) RefreshThread(ctx context.Context, accountID, threadID string) error {
	t, err := m.client.GetThread(ctx, threadID)
	if err != nil {
		return err
	}
	for _, meta := range t.Messages {
		if err := m.storeMessageMetadata(accountID, meta); err != nil {
			return err
		}
	}
	return nil
}

// RefreshMessage is RefreshThread for a single message.
func (m *Manager) RefreshMessage(ctx context.Context, accountID, messageID string) error {
	meta, err := m.client.GetMessageMeta(ctx, messageID)
	if err != nil {
		return err
	}
	return m.storeMessageMetadata(accountID, meta)
}

// SyncLabels fetches all labels from the Gmail API and upserts them into the store.
func (m *Manager) SyncLabels(ctx context.Context, accountID string) error {
	return m.syncLabels(ctx, accountID)
}

func (m *Manager) syncLabels(ctx context.Context, accountID string) error {
	labels, err := m.client.ListLabels(ctx)
	if err != nil {
		return err
	}
	for _, l := range labels {
		if err := m.store.UpsertLabel(storage.Label{
			ID:              l.ID,
			AccountID:       accountID,
			Name:            l.Name,
			LabelType:       l.LabelType,
			TextColor:       l.TextColor,
			BackgroundColor: l.BackgroundColor,
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

func (m *Manager) storedHistoryID(accountID string) (uint64, error) {
	acc, err := m.store.GetAccount(accountID)
	if err != nil {
		return 0, err
	}
	if acc.HistoryID == "" {
		return 0, fmt.Errorf("no history ID stored; run initial sync first")
	}

	startHistoryID, err := strconv.ParseUint(acc.HistoryID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse history id: %w", err)
	}
	return startHistoryID, nil
}

func (m *Manager) storeLatestHistoryID(ctx context.Context, accountID string) error {
	historyID, err := m.fetchHistoryID(ctx)
	if err != nil {
		return nil
	}
	return m.updateHistoryID(accountID, historyID)
}

func (m *Manager) updateHistoryID(accountID string, historyID uint64) error {
	if err := m.store.UpdateHistoryID(accountID, strconv.FormatUint(historyID, 10)); err != nil {
		return fmt.Errorf("update history id: %w", err)
	}
	return nil
}

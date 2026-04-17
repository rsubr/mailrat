package sync

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

const bodyFetchLimit = 200

// Manager orchestrates sync operations for an account.
type Manager struct {
	store    *storage.Store
	client   *gmail.Client
	reporter progressReporter
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

	plan := buildInitialSyncPlan(defaultDays, configs)
	m.reporter.Printf("Starting initial sync for account %s (base: last %d days)...\n", accountID, plan.maxDays)

	seen := make(seenMessages)
	if err := m.fetchAndStore(ctx, accountID, initialSyncQuery(plan.maxDays), 500, seen); err != nil {
		return fmt.Errorf("base sync: %w", err)
	}

	for _, labelPlan := range plan.allMailLabels {
		m.reporter.Printf("Fetching all mail for label %q (%s)...\n", labelPlan.config.LabelName, labelPlan.query)
		if err := m.fetchAndStore(ctx, accountID, labelPlan.query, 2000, seen); err != nil {
			m.reporter.Warnf("failed to fetch label %q: %v\n", labelPlan.config.LabelName, err)
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

	stats, err := m.applyHistoryEvents(accountID, resp.Events)
	if err != nil {
		return err
	}

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
	for _, plan := range buildBackfillPlans(configs) {
		if err := m.runBackfillPlan(ctx, accountID, plan, seen); err != nil {
			return err
		}
	}

	if err := m.fetchConfiguredBodies(ctx, accountID, configs); err != nil {
		m.reporter.Warnf("fetch bodies for labels: %v\n", err)
	}

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

func (m *Manager) fetchAndStore(ctx context.Context, accountID, query string, maxResults int64, seen seenMessages) error {
	messages, err := m.client.ListMessages(ctx, query, maxResults)
	if err != nil {
		return fmt.Errorf("list messages (%s): %w", query, err)
	}
	for _, meta := range messages {
		if seen.Skip(meta.ID) {
			continue
		}
		if err := m.storeMessageMetadata(accountID, meta); err != nil {
			return err
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

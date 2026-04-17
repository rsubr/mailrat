package sync

import (
	"context"
	"fmt"

	"github.com/mailrat/mailrat/internal/storage"
)

func (m *Manager) ensureDefaultBackfillConfigs(accountID string, configs []storage.SyncConfig) ([]storage.SyncConfig, error) {
	existing := make(map[string]struct{}, len(configs))
	for _, cfg := range configs {
		existing[cfg.LabelID] = struct{}{}
	}

	for _, cfg := range defaultBackfillConfigs(accountID) {
		if _, found := existing[cfg.LabelID]; found {
			continue
		}
		if err := m.store.UpsertSyncConfig(cfg); err != nil {
			continue
		}
		configs = append(configs, cfg)
	}

	return configs, nil
}

func (m *Manager) runBackfillPlan(ctx context.Context, accountID string, plan backfillPlan, seen seenMessages) error {
	if plan.query == "" {
		return m.markBackfilled(plan.config)
	}

	m.reporter.Printf("Backfilling %q (%s)...\n", plan.config.LabelName, plan.query)
	if err := m.fetchAndStore(ctx, accountID, plan.query, 2000, seen); err != nil {
		m.reporter.Warnf("backfill for %q failed: %v\n", plan.config.LabelName, err)
		return nil
	}

	if err := m.markBackfilled(plan.config); err != nil {
		return err
	}
	m.reporter.Printf("Backfill complete for %q\n", plan.config.LabelName)
	return nil
}

func (m *Manager) markBackfilled(cfg storage.SyncConfig) error {
	if err := m.store.MarkSyncConfigBackfilled(cfg.LabelID, cfg.AccountID); err != nil {
		return fmt.Errorf("mark backfilled %s: %w", cfg.LabelID, err)
	}
	return nil
}

func (m *Manager) fetchConfiguredBodies(ctx context.Context, accountID string, configs []storage.SyncConfig) error {
	labelIDs := attachmentLabelIDs(configs)
	if len(labelIDs) == 0 {
		return nil
	}
	m.reporter.Printf("Fetching bodies/attachments for configured labels...\n")
	return m.FetchBodiesForLabels(ctx, accountID, labelIDs, bodyFetchLimit)
}

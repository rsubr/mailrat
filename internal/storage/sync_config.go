package storage

// SyncConfig holds per-label sync preferences for one account.
type SyncConfig struct {
	LabelID          string
	AccountID        string
	LabelName        string // display name, denormalised for query building
	DaysToSync       int    // 0 = all mail; >0 = last N days
	FetchAttachments bool
	Backfilled       bool // true once the historical backfill for this label completed
}

func (s *Store) UpsertSyncConfig(c SyncConfig) error {
	fetchAtt := 0
	if c.FetchAttachments {
		fetchAtt = 1
	}
	// Preserve existing backfilled value — UpsertSyncConfig is called from the
	// TUI config save and must not reset a completed backfill.
	_, err := s.db.Exec(`
		INSERT INTO sync_config (label_id, account_id, label_name, days_to_sync, fetch_attachments, backfilled)
		VALUES (?, ?, ?, ?, ?, 0)
		ON CONFLICT(label_id, account_id) DO UPDATE SET
			label_name        = excluded.label_name,
			days_to_sync      = excluded.days_to_sync,
			fetch_attachments = excluded.fetch_attachments,
			backfilled        = CASE
				WHEN excluded.days_to_sync != sync_config.days_to_sync THEN 0
				ELSE sync_config.backfilled
			END
	`, c.LabelID, c.AccountID, c.LabelName, c.DaysToSync, fetchAtt)
	return err
}

func (s *Store) ListSyncConfigs(accountID string) ([]SyncConfig, error) {
	rows, err := s.db.Query(`
		SELECT label_id, account_id, label_name, days_to_sync, fetch_attachments, backfilled
		FROM sync_config WHERE account_id = ?
		ORDER BY label_id ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncConfig
	for rows.Next() {
		var c SyncConfig
		var fetchAtt, backfilled int
		if err := rows.Scan(&c.LabelID, &c.AccountID, &c.LabelName, &c.DaysToSync, &fetchAtt, &backfilled); err != nil {
			return nil, err
		}
		c.FetchAttachments = fetchAtt == 1
		c.Backfilled = backfilled == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkSyncConfigBackfilled records that the historical backfill for a label is done.
func (s *Store) MarkSyncConfigBackfilled(labelID, accountID string) error {
	_, err := s.db.Exec(`
		UPDATE sync_config SET backfilled = 1
		WHERE label_id = ? AND account_id = ?
	`, labelID, accountID)
	return err
}

package storage

// SyncConfig holds per-label sync preferences for one account.
type SyncConfig struct {
	LabelID    string
	AccountID  string
	LabelName  string // display name, denormalised for query building
	DaysToSync int    // 0 = all mail; >0 = last N days (ignored when MaxEmails > 0)
	MaxEmails  int    // >0 = only the newest N emails
	Backfilled bool   // true once the historical backfill for this label completed
}

func (s *Store) UpsertSyncConfig(c SyncConfig) error {
	// Preserve existing backfilled value — UpsertSyncConfig is called from the
	// TUI config save and must not reset a completed backfill.
	_, err := s.db.Exec(`
		INSERT INTO sync_config (label_id, account_id, label_name, days_to_sync, backfilled, max_emails)
		VALUES (?, ?, ?, ?, 0, ?)
		ON CONFLICT(label_id, account_id) DO UPDATE SET
			label_name        = excluded.label_name,
			days_to_sync      = excluded.days_to_sync,
			max_emails        = excluded.max_emails,
			backfilled        = CASE
				WHEN excluded.days_to_sync != sync_config.days_to_sync
				  OR excluded.max_emails != sync_config.max_emails THEN 0
				ELSE sync_config.backfilled
			END
	`, c.LabelID, c.AccountID, c.LabelName, c.DaysToSync, c.MaxEmails)
	return err
}

func (s *Store) ListSyncConfigs(accountID string) ([]SyncConfig, error) {
	rows, err := s.db.Query(`
		SELECT label_id, account_id, label_name, days_to_sync, backfilled, max_emails
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
		var backfilled int
		if err := rows.Scan(&c.LabelID, &c.AccountID, &c.LabelName, &c.DaysToSync, &backfilled, &c.MaxEmails); err != nil {
			return nil, err
		}
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

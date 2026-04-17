package storage

// SyncConfig holds per-label sync preferences for one account.
type SyncConfig struct {
	LabelID          string
	AccountID        string
	LabelName        string // display name, denormalised for query building
	DaysToSync       int    // 0 = all mail; >0 = last N days
	FetchAttachments bool
}

func (s *Store) UpsertSyncConfig(c SyncConfig) error {
	fetchAtt := 0
	if c.FetchAttachments {
		fetchAtt = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO sync_config (label_id, account_id, label_name, days_to_sync, fetch_attachments)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(label_id, account_id) DO UPDATE SET
			label_name        = excluded.label_name,
			days_to_sync      = excluded.days_to_sync,
			fetch_attachments = excluded.fetch_attachments
	`, c.LabelID, c.AccountID, c.LabelName, c.DaysToSync, fetchAtt)
	return err
}

func (s *Store) ListSyncConfigs(accountID string) ([]SyncConfig, error) {
	rows, err := s.db.Query(`
		SELECT label_id, account_id, label_name, days_to_sync, fetch_attachments
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
		var fetchAtt int
		if err := rows.Scan(&c.LabelID, &c.AccountID, &c.LabelName, &c.DaysToSync, &fetchAtt); err != nil {
			return nil, err
		}
		c.FetchAttachments = fetchAtt == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

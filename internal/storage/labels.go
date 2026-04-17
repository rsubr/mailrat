package storage

type Label struct {
	ID        string
	AccountID string
	Name      string
	LabelType string
}

func (s *Store) UpsertLabel(l Label) error {
	_, err := s.db.Exec(`
		INSERT INTO labels (id, account_id, name, label_type)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			name       = excluded.name,
			label_type = excluded.label_type
	`, l.ID, l.AccountID, l.Name, l.LabelType)
	return err
}

func (s *Store) ListLabels(accountID string) ([]Label, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, label_type FROM labels WHERE account_id = ?
		ORDER BY name ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Label
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.AccountID, &l.Name, &l.LabelType); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) SetMessageLabels(accountID, messageID string, labelIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		DELETE FROM message_labels WHERE message_id = ? AND account_id = ?
	`, messageID, accountID); err != nil {
		return err
	}

	for _, lid := range labelIDs {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO message_labels (message_id, account_id, label_id)
			VALUES (?, ?, ?)
		`, messageID, accountID, lid); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetMessageLabelIDs(accountID, messageID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT label_id FROM message_labels WHERE message_id = ? AND account_id = ?
	`, messageID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

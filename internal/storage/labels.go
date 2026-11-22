package storage

import "database/sql"

type Label struct {
	ID              string
	AccountID       string
	Name            string
	LabelType       string
	TextColor       string
	BackgroundColor string
}

func (s *Store) UpsertLabel(l Label) error {
	_, err := s.db.Exec(`
		INSERT INTO labels (id, account_id, name, label_type, text_color, background_color)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			name             = excluded.name,
			label_type       = excluded.label_type,
			text_color       = excluded.text_color,
			background_color = excluded.background_color
	`, l.ID, l.AccountID, l.Name, l.LabelType, l.TextColor, l.BackgroundColor)
	return err
}

func (s *Store) ListLabels(accountID string) ([]Label, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, label_type, text_color, background_color
		FROM labels WHERE account_id = ?
		ORDER BY name ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Label
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.AccountID, &l.Name, &l.LabelType, &l.TextColor, &l.BackgroundColor); err != nil {
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

func (s *Store) GetLabelUnreadCounts(accountID string) (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT ml.label_id, COUNT(DISTINCT m.thread_id)
		FROM message_labels ml
		JOIN messages m ON m.id = ml.message_id AND m.account_id = ml.account_id
		WHERE ml.account_id = ? AND m.is_unread = 1
		GROUP BY ml.label_id
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var labelID string
		var count int
		if err := rows.Scan(&labelID, &count); err != nil {
			return nil, err
		}
		out[labelID] = count
	}
	return out, rows.Err()
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

// ModifyLabels adds and removes labels on a message, or on every message of a
// thread when thread is true. Adding or removing UNREAD also updates is_unread.
func (s *Store) ModifyLabels(accountID, targetID string, thread bool, add, remove []string) error {
	col := "id"
	if thread {
		col = "thread_id"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, l := range remove {
		if _, err := tx.Exec(`DELETE FROM message_labels WHERE account_id = ? AND label_id = ?
			AND message_id IN (SELECT id FROM messages WHERE account_id = ? AND `+col+` = ?)`,
			accountID, l, accountID, targetID); err != nil {
			return err
		}
	}
	for _, l := range add {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO message_labels (message_id, account_id, label_id)
			SELECT id, account_id, ? FROM messages WHERE account_id = ? AND `+col+` = ?`,
			l, accountID, targetID); err != nil {
			return err
		}
	}
	for _, l := range add {
		if l == "UNREAD" {
			if err := setUnread(tx, accountID, col, targetID, 1); err != nil {
				return err
			}
		}
	}
	for _, l := range remove {
		if l == "UNREAD" {
			if err := setUnread(tx, accountID, col, targetID, 0); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func setUnread(tx *sql.Tx, accountID, col, targetID string, v int) error {
	_, err := tx.Exec(`UPDATE messages SET is_unread = ? WHERE account_id = ? AND `+col+` = ?`, v, accountID, targetID)
	return err
}

// OldestMessageDate returns the internal date of the oldest stored message that
// carries labelID, or 0 when there is none.
func (s *Store) OldestMessageDate(accountID, labelID string) (int64, error) {
	var d sql.NullInt64
	err := s.db.QueryRow(`SELECT MIN(m.internal_date) FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id AND ml.account_id = m.account_id
		WHERE m.account_id = ? AND ml.label_id = ?`, accountID, labelID).Scan(&d)
	return d.Int64, err
}

// HasMessage reports whether a message is already stored.
func (s *Store) HasMessage(accountID, messageID string) bool {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM messages WHERE id = ? AND account_id = ?`, messageID, accountID).Scan(&n)
	return err == nil
}

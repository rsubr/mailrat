package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

type Thread struct {
	ID              string
	AccountID       string
	LastMessageDate int64
}

func (s *Store) UpsertThread(t Thread) error {
	_, err := s.db.Exec(`
		INSERT INTO threads (id, account_id, last_message_date)
		VALUES (?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			last_message_date = CASE
				WHEN excluded.last_message_date > threads.last_message_date
				THEN excluded.last_message_date
				ELSE threads.last_message_date
			END
	`, t.ID, t.AccountID, t.LastMessageDate)
	return err
}

func (s *Store) GetThread(accountID, threadID string) (Thread, error) {
	var t Thread
	err := s.db.QueryRow(`
		SELECT id, account_id, last_message_date FROM threads WHERE id = ? AND account_id = ?
	`, threadID, accountID).Scan(&t.ID, &t.AccountID, &t.LastMessageDate)
	if errors.Is(err, sql.ErrNoRows) {
		return Thread{}, fmt.Errorf("thread %q not found", threadID)
	}
	return t, err
}

func (s *Store) ListThreads(accountID string, limit int) ([]Thread, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, last_message_date FROM threads
		WHERE account_id = ?
		ORDER BY last_message_date DESC
		LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Thread
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.AccountID, &t.LastMessageDate); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// ThreadSummary is a thread as shown in the inbox: aggregated from its messages.
type ThreadSummary struct {
	ThreadID       string
	AccountID      string
	Subject        string
	Snippet        string // from the most recent message
	Participants   string // comma-separated distinct from_addr values
	MessageCount   int
	UnreadCount    int
	LastDate       int64
	HasAttachments bool
}

const threadSummarySQL = `
	SELECT
		thread_id,
		account_id,
		COUNT(*)                                                   AS message_count,
		SUM(is_unread)                                             AS unread_count,
		MAX(internal_date)                                         AS last_date,
		MAX(has_attachments)                                       AS has_attachments,
		GROUP_CONCAT(DISTINCT from_addr)                           AS participants,
		(SELECT subject FROM messages m2
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		 ORDER BY internal_date DESC LIMIT 1)                      AS subject,
		(SELECT snippet  FROM messages m2
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		 ORDER BY internal_date DESC LIMIT 1)                      AS snippet
	FROM messages m
`

func scanThreadSummaries(rows *sql.Rows) ([]ThreadSummary, error) {
	var out []ThreadSummary
	for rows.Next() {
		var t ThreadSummary
		var hasAttachments int
		if err := rows.Scan(
			&t.ThreadID, &t.AccountID,
			&t.MessageCount, &t.UnreadCount, &t.LastDate, &hasAttachments,
			&t.Participants, &t.Subject, &t.Snippet,
		); err != nil {
			return nil, err
		}
		t.HasAttachments = hasAttachments == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllThreadSummaries returns one row per thread across all accounts, newest first.
func (s *Store) ListAllThreadSummaries(limit int) ([]ThreadSummary, error) {
	rows, err := s.db.Query(
		threadSummarySQL+`GROUP BY thread_id, account_id ORDER BY last_date DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

// ListThreadSummaries returns one row per thread for a single account, newest first.
func (s *Store) ListThreadSummaries(accountID string, limit int) ([]ThreadSummary, error) {
	rows, err := s.db.Query(
		threadSummarySQL+`WHERE account_id = ? GROUP BY thread_id, account_id ORDER BY last_date DESC LIMIT ?`,
		accountID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

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

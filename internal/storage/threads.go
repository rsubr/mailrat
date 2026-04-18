package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	IsStarred      bool
	LabelIDs       []string // distinct label IDs across all messages in thread
}

const threadSummarySQL = `
	SELECT
		m.thread_id,
		m.account_id,
		COUNT(*)                                                   AS message_count,
		SUM(m.is_unread)                                           AS unread_count,
		MAX(m.internal_date)                                       AS last_date,
		MAX(m.has_attachments)                                     AS has_attachments,
		GROUP_CONCAT(DISTINCT m.from_addr)                         AS participants,
		(SELECT subject FROM messages m2
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		 ORDER BY m2.internal_date DESC LIMIT 1)                   AS subject,
		(SELECT snippet  FROM messages m2
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		 ORDER BY m2.internal_date DESC LIMIT 1)                   AS snippet,
		(SELECT GROUP_CONCAT(DISTINCT ml.label_id)
		 FROM message_labels ml
		 JOIN messages m2 ON m2.id = ml.message_id AND m2.account_id = ml.account_id
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id) AS label_ids,
		(SELECT COUNT(*) FROM message_labels ml
		 JOIN messages m2 ON m2.id = ml.message_id AND m2.account_id = ml.account_id
		 WHERE m2.thread_id = m.thread_id AND m2.account_id = m.account_id
		   AND ml.label_id = 'STARRED')                            AS is_starred
	FROM messages m
`

func scanThreadSummaries(rows *sql.Rows) ([]ThreadSummary, error) {
	var out []ThreadSummary
	for rows.Next() {
		var t ThreadSummary
		var hasAttachments, isStarred int
		var labelIDs sql.NullString
		if err := rows.Scan(
			&t.ThreadID, &t.AccountID,
			&t.MessageCount, &t.UnreadCount, &t.LastDate, &hasAttachments,
			&t.Participants, &t.Subject, &t.Snippet,
			&labelIDs, &isStarred,
		); err != nil {
			return nil, err
		}
		t.HasAttachments = hasAttachments == 1
		t.IsStarred = isStarred > 0
		if labelIDs.Valid && labelIDs.String != "" {
			t.LabelIDs = strings.Split(labelIDs.String, ",")
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// inboxFilter is a subquery that returns the thread_ids that have at least one
// message carrying the INBOX label. Used to restrict both list functions to the
// actual inbox rather than all synced mail.
const inboxFilter = `
	m.thread_id IN (
		SELECT DISTINCT m2.thread_id
		FROM messages m2
		JOIN message_labels ml ON ml.message_id = m2.id AND ml.account_id = m2.account_id
		WHERE ml.label_id = 'INBOX'
	)
`

const inboxFilterForAccount = `
	m.account_id = ? AND m.thread_id IN (
		SELECT DISTINCT m2.thread_id
		FROM messages m2
		JOIN message_labels ml ON ml.message_id = m2.id AND ml.account_id = m2.account_id
		WHERE m2.account_id = ? AND ml.label_id = 'INBOX'
	)
`

// ListAllThreadSummaries returns inbox threads across all accounts, newest first.
func (s *Store) ListAllThreadSummaries(limit, offset int) ([]ThreadSummary, error) {
	rows, err := s.db.Query(
		threadSummarySQL+
			`WHERE `+inboxFilter+
			`GROUP BY m.thread_id, m.account_id ORDER BY last_date DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

// SearchAllThreadSummaries searches threads across all accounts by subject, snippet, or sender.
func (s *Store) SearchAllThreadSummaries(query string, limit int) ([]ThreadSummary, error) {
	like := "%" + query + "%"
	rows, err := s.db.Query(
		threadSummarySQL+
			`WHERE (LOWER(m.subject) LIKE LOWER(?) OR LOWER(m.snippet) LIKE LOWER(?) OR LOWER(m.from_addr) LIKE LOWER(?)) `+
			`GROUP BY m.thread_id, m.account_id ORDER BY last_date DESC LIMIT ?`,
		like, like, like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

// ListThreadSummaries returns inbox threads for a single account, newest first.
func (s *Store) ListThreadSummaries(accountID string, limit, offset int) ([]ThreadSummary, error) {
	rows, err := s.db.Query(
		threadSummarySQL+
			`WHERE `+inboxFilterForAccount+
			`GROUP BY m.thread_id, m.account_id ORDER BY last_date DESC LIMIT ? OFFSET ?`,
		accountID, accountID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

// ListAllThreadSummariesByLabel returns threads carrying a given label across all accounts.
func (s *Store) ListAllThreadSummariesByLabel(labelID string, limit, offset int) ([]ThreadSummary, error) {
	filter := `m.thread_id IN (
		SELECT DISTINCT m2.thread_id
		FROM messages m2
		JOIN message_labels ml ON ml.message_id = m2.id AND ml.account_id = m2.account_id
		WHERE ml.label_id = ?
	)`
	rows, err := s.db.Query(
		threadSummarySQL+`WHERE `+filter+
			` GROUP BY m.thread_id, m.account_id ORDER BY last_date DESC LIMIT ? OFFSET ?`,
		labelID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreadSummaries(rows)
}

// ListThreadSummariesByLabel returns threads carrying a given label for one account.
func (s *Store) ListThreadSummariesByLabel(accountID, labelID string, limit, offset int) ([]ThreadSummary, error) {
	filter := `m.account_id = ? AND m.thread_id IN (
		SELECT DISTINCT m2.thread_id
		FROM messages m2
		JOIN message_labels ml ON ml.message_id = m2.id AND ml.account_id = m2.account_id
		WHERE m2.account_id = ? AND ml.label_id = ?
	)`
	rows, err := s.db.Query(
		threadSummarySQL+`WHERE `+filter+
			` GROUP BY m.thread_id, m.account_id ORDER BY last_date DESC LIMIT ? OFFSET ?`,
		accountID, accountID, labelID, limit, offset,
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

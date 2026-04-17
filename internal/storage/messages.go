package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

type CacheState string

const (
	CacheMetadataOnly     CacheState = "metadata_only"
	CacheBodyCached       CacheState = "body_cached"
	CacheAttachmentCached CacheState = "attachments_cached"
)

type Message struct {
	ID           string
	ThreadID     string
	AccountID    string
	Subject      string
	FromAddr     string
	ToAddr       string
	Snippet      string
	Body         string
	InternalDate int64
	IsUnread     bool
	CacheState   CacheState
}

func (s *Store) UpsertMessage(m Message) error {
	isUnread := 0
	if m.IsUnread {
		isUnread = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO messages
			(id, thread_id, account_id, subject, from_addr, to_addr, snippet, body, internal_date, is_unread, cache_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			thread_id     = excluded.thread_id,
			subject       = excluded.subject,
			from_addr     = excluded.from_addr,
			to_addr       = excluded.to_addr,
			snippet       = excluded.snippet,
			body          = CASE WHEN excluded.cache_state != 'metadata_only' THEN excluded.body ELSE messages.body END,
			internal_date = excluded.internal_date,
			is_unread     = excluded.is_unread,
			cache_state   = CASE WHEN excluded.cache_state > messages.cache_state THEN excluded.cache_state ELSE messages.cache_state END
	`, m.ID, m.ThreadID, m.AccountID, m.Subject, m.FromAddr, m.ToAddr, m.Snippet,
		m.Body, m.InternalDate, isUnread, string(m.CacheState))
	return err
}

func (s *Store) GetMessage(accountID, messageID string) (Message, error) {
	var m Message
	var isUnread int
	err := s.db.QueryRow(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state
		FROM messages WHERE id = ? AND account_id = ?
	`, messageID, accountID).Scan(
		&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
		&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, fmt.Errorf("message %q not found", messageID)
	}
	m.IsUnread = isUnread == 1
	return m, err
}

func (s *Store) ListMessages(accountID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state
		FROM messages
		WHERE account_id = ?
		ORDER BY internal_date DESC
		LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *Store) ListMessagesByThread(accountID, threadID string) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state
		FROM messages
		WHERE account_id = ? AND thread_id = ?
		ORDER BY internal_date ASC
	`, accountID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListAllAccounts returns messages across all accounts, newest first.
func (s *Store) ListAllMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state
		FROM messages
		ORDER BY internal_date DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *Store) ListUncachedBodies(accountID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state
		FROM messages
		WHERE account_id = ? AND cache_state = 'metadata_only'
		ORDER BY internal_date DESC
		LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var out []Message
	for rows.Next() {
		var m Message
		var isUnread int
		if err := rows.Scan(
			&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
			&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState,
		); err != nil {
			return nil, err
		}
		m.IsUnread = isUnread == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

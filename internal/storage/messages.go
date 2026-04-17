package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type CacheState string

const (
	CacheMetadataOnly     CacheState = "metadata_only"
	CacheBodyCached       CacheState = "body_cached"
	CacheAttachmentCached CacheState = "attachments_cached"
)

type Message struct {
	ID             string
	ThreadID       string
	AccountID      string
	Subject        string
	FromAddr       string
	ToAddr         string
	Snippet        string
	Body           string
	InternalDate   int64
	IsUnread       bool
	CacheState     CacheState
	HasAttachments bool
}

func (s *Store) UpsertMessage(m Message) error {
	isUnread := 0
	if m.IsUnread {
		isUnread = 1
	}
	hasAttachments := 0
	if m.HasAttachments {
		hasAttachments = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO messages
			(id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
			 internal_date, is_unread, cache_state, has_attachments)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			thread_id       = excluded.thread_id,
			subject         = excluded.subject,
			from_addr       = excluded.from_addr,
			to_addr         = excluded.to_addr,
			snippet         = excluded.snippet,
			body            = CASE WHEN excluded.cache_state != 'metadata_only' THEN excluded.body ELSE messages.body END,
			internal_date   = excluded.internal_date,
			is_unread       = excluded.is_unread,
			cache_state     = CASE WHEN excluded.cache_state > messages.cache_state THEN excluded.cache_state ELSE messages.cache_state END,
			has_attachments = CASE WHEN excluded.has_attachments = 1 THEN 1 ELSE messages.has_attachments END
	`, m.ID, m.ThreadID, m.AccountID, m.Subject, m.FromAddr, m.ToAddr, m.Snippet,
		m.Body, m.InternalDate, isUnread, string(m.CacheState), hasAttachments)
	return err
}

func (s *Store) GetMessage(accountID, messageID string) (Message, error) {
	var m Message
	var isUnread, hasAttachments int
	err := s.db.QueryRow(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state, has_attachments
		FROM messages WHERE id = ? AND account_id = ?
	`, messageID, accountID).Scan(
		&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
		&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState, &hasAttachments,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, fmt.Errorf("message %q not found", messageID)
	}
	m.IsUnread = isUnread == 1
	m.HasAttachments = hasAttachments == 1
	return m, err
}

func (s *Store) ListMessages(accountID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state, has_attachments
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
		       internal_date, is_unread, cache_state, has_attachments
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

// ListMessagesByLabel returns messages that have the given label (by name or ID),
// newest first. labelQuery is matched case-insensitively against both the label
// ID (e.g. "SENT") and the label name (e.g. "Sent").
func (s *Store) ListMessagesByLabel(accountID, labelQuery string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.thread_id, m.account_id, m.subject, m.from_addr, m.to_addr,
		       m.snippet, m.body, m.internal_date, m.is_unread, m.cache_state, m.has_attachments
		FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id AND ml.account_id = m.account_id
		JOIN labels l          ON l.id = ml.label_id AND l.account_id = m.account_id
		WHERE m.account_id = ?
		  AND (UPPER(l.id) = UPPER(?) OR UPPER(l.name) = UPPER(?))
		ORDER BY m.internal_date DESC
		LIMIT ?
	`, accountID, labelQuery, labelQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListAllMessagesByLabel is the cross-account version of ListMessagesByLabel.
func (s *Store) ListAllMessagesByLabel(labelQuery string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.thread_id, m.account_id, m.subject, m.from_addr, m.to_addr,
		       m.snippet, m.body, m.internal_date, m.is_unread, m.cache_state, m.has_attachments
		FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id AND ml.account_id = m.account_id
		JOIN labels l          ON l.id = ml.label_id AND l.account_id = m.account_id
		WHERE UPPER(l.id) = UPPER(?) OR UPPER(l.name) = UPPER(?)
		ORDER BY m.internal_date DESC
		LIMIT ?
	`, labelQuery, labelQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListAllMessages returns messages across all accounts, newest first.
func (s *Store) ListAllMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
		       internal_date, is_unread, cache_state, has_attachments
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
		       internal_date, is_unread, cache_state, has_attachments
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

// ListUncachedBodiesForLabels returns metadata-only messages that have at
// least one of the given label IDs, newest first.
func (s *Store) ListUncachedBodiesForLabels(accountID string, labelIDs []string, limit int) ([]Message, error) {
	if len(labelIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(labelIDs))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

	args := []any{accountID}
	for _, id := range labelIDs {
		args = append(args, id)
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT DISTINCT m.id, m.thread_id, m.account_id, m.subject, m.from_addr, m.to_addr,
		       m.snippet, m.body, m.internal_date, m.is_unread, m.cache_state, m.has_attachments
		FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id AND ml.account_id = m.account_id
		WHERE m.account_id = ?
		  AND m.cache_state = 'metadata_only'
		  AND ml.label_id IN (%s)
		ORDER BY m.internal_date DESC
		LIMIT ?
	`, placeholders)

	rows, err := s.db.Query(q, args...)
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
		var isUnread, hasAttachments int
		if err := rows.Scan(
			&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
			&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState, &hasAttachments,
		); err != nil {
			return nil, err
		}
		m.IsUnread = isUnread == 1
		m.HasAttachments = hasAttachments == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

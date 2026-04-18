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

const (
	messageSelectColumns = `id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
	       internal_date, is_unread, cache_state, has_attachments, message_id, cc_addr, bcc_addr`
	messageOrderDesc = ` ORDER BY internal_date DESC`
	messageOrderAsc  = ` ORDER BY internal_date ASC`
	messageUpsertSQL = `
		INSERT INTO messages
			(id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
			 internal_date, is_unread, cache_state, has_attachments, message_id, cc_addr, bcc_addr)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			has_attachments = CASE WHEN excluded.has_attachments = 1 THEN 1 ELSE messages.has_attachments END,
			message_id      = CASE WHEN excluded.message_id != '' THEN excluded.message_id ELSE messages.message_id END,
			cc_addr         = CASE WHEN excluded.cc_addr != '' THEN excluded.cc_addr ELSE messages.cc_addr END,
			bcc_addr        = CASE WHEN excluded.bcc_addr != '' THEN excluded.bcc_addr ELSE messages.bcc_addr END
	`
)

type Message struct {
	ID             string
	ThreadID       string
	MessageID      string // RFC 2822 Message-ID header value
	AccountID      string
	Subject        string
	FromAddr       string
	ToAddr         string
	CcAddr         string
	BccAddr        string
	Snippet        string
	Body           string
	InternalDate   int64
	IsUnread       bool
	CacheState     CacheState
	HasAttachments bool
}

func (s *Store) UpsertMessage(m Message) error {
	_, err := s.db.Exec(messageUpsertSQL, messageUpsertArgs(m)...)
	return err
}

func (s *Store) GetMessage(accountID, messageID string) (Message, error) {
	var m Message
	var isUnread, hasAttachments int
	err := s.db.QueryRow(`
		SELECT `+messageSelectColumns+`
		FROM messages WHERE id = ? AND account_id = ?
	`, messageID, accountID).Scan(
		&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
		&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState, &hasAttachments, &m.MessageID,
		&m.CcAddr, &m.BccAddr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, fmt.Errorf("message %q not found", messageID)
	}
	m.IsUnread = isUnread == 1
	m.HasAttachments = hasAttachments == 1
	return m, err
}

func (s *Store) ListMessages(accountID string, limit int) ([]Message, error) {
	return s.queryMessages(`
		SELECT `+messageSelectColumns+`
		FROM messages
		WHERE account_id = ?`+messageOrderDesc+`
		LIMIT ?
	`, accountID, limit)
}

func (s *Store) ListMessagesByThread(accountID, threadID string) ([]Message, error) {
	return s.queryMessages(`
		SELECT `+messageSelectColumns+`
		FROM messages
		WHERE account_id = ? AND thread_id = ?`+messageOrderAsc,
		accountID, threadID,
	)
}

// ListMessagesByLabel returns messages that have the given label (by name or ID),
// newest first. labelQuery is matched case-insensitively against both the label
// ID (e.g. "SENT") and the label name (e.g. "Sent").
func (s *Store) ListMessagesByLabel(accountID, labelQuery string, limit int) ([]Message, error) {
	return s.queryMessages(labelFilteredMessagesSQL(true), accountID, labelQuery, labelQuery, limit)
}

// ListAllMessagesByLabel is the cross-account version of ListMessagesByLabel.
func (s *Store) ListAllMessagesByLabel(labelQuery string, limit int) ([]Message, error) {
	return s.queryMessages(labelFilteredMessagesSQL(false), labelQuery, labelQuery, limit)
}

// ListAllMessages returns messages across all accounts, newest first.
func (s *Store) ListAllMessages(limit int) ([]Message, error) {
	return s.queryMessages(`
		SELECT `+messageSelectColumns+`
		FROM messages`+messageOrderDesc+`
		LIMIT ?
	`, limit)
}

func (s *Store) ListUncachedBodies(accountID string, limit int) ([]Message, error) {
	return s.queryMessages(`
		SELECT `+messageSelectColumns+`
		FROM messages
		WHERE account_id = ? AND cache_state = ?`+messageOrderDesc+`
		LIMIT ?
	`, accountID, CacheMetadataOnly, limit)
}

// ListUncachedBodiesForLabels returns metadata-only messages that have at
// least one of the given label IDs, newest first.
func (s *Store) ListUncachedBodiesForLabels(accountID string, labelIDs []string, limit int) ([]Message, error) {
	if len(labelIDs) == 0 {
		return nil, nil
	}

	query, args := buildUncachedBodiesForLabelsQuery(accountID, labelIDs, limit)
	return s.queryMessages(query, args...)
}

func (s *Store) queryMessages(query string, args ...any) ([]Message, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func labelFilteredMessagesSQL(scopedToAccount bool) string {
	where := `WHERE UPPER(l.id) = UPPER(?) OR UPPER(l.name) = UPPER(?)`
	if scopedToAccount {
		where = `WHERE m.account_id = ?
		  AND (UPPER(l.id) = UPPER(?) OR UPPER(l.name) = UPPER(?))`
	}

	return `
		SELECT m.` + strings.ReplaceAll(messageSelectColumns, ", ", ", m.") + `
		FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id AND ml.account_id = m.account_id
		JOIN labels l          ON l.id = ml.label_id AND l.account_id = m.account_id
		` + where + messageOrderDesc + `
		LIMIT ?
	`
}

func buildUncachedBodiesForLabelsQuery(accountID string, labelIDs []string, limit int) (string, []any) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(labelIDs)), ",")
	args := make([]any, 0, len(labelIDs)+3)
	args = append(args, accountID, CacheMetadataOnly)
	for _, id := range labelIDs {
		args = append(args, id)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT DISTINCT m.%s
		FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id AND ml.account_id = m.account_id
		WHERE m.account_id = ?
		  AND m.cache_state = ?
		  AND ml.label_id IN (%s)`+messageOrderDesc+`
		LIMIT ?
	`, strings.ReplaceAll(messageSelectColumns, ", ", ", m."), placeholders)

	return query, args
}

func messageUpsertArgs(m Message) []any {
	return []any{
		m.ID,
		m.ThreadID,
		m.AccountID,
		m.Subject,
		m.FromAddr,
		m.ToAddr,
		m.Snippet,
		m.Body,
		m.InternalDate,
		boolToInt(m.IsUnread),
		string(m.CacheState),
		boolToInt(m.HasAttachments),
		m.MessageID,
		m.CcAddr,
		m.BccAddr,
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var out []Message
	for rows.Next() {
		var m Message
		var isUnread, hasAttachments int
		if err := rows.Scan(
			&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
			&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState, &hasAttachments, &m.MessageID,
			&m.CcAddr, &m.BccAddr,
		); err != nil {
			return nil, err
		}
		m.IsUnread = isUnread == 1
		m.HasAttachments = hasAttachments == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

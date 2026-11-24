package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type CacheState string

const (
	CacheMetadataOnly CacheState = "metadata_only"
	CacheBodyCached   CacheState = "body_cached"
)

const (
	messageSelectColumns = `id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
	       internal_date, is_unread, cache_state, has_attachments, message_id, cc_addr, bcc_addr,
	       reply_to, references_hdr, in_reply_to,
	       (SELECT COUNT(*) FROM message_labels ml WHERE ml.message_id = id AND ml.account_id = account_id AND ml.label_id = 'STARRED')`
	messageOrderDesc = ` ORDER BY internal_date DESC`
	messageOrderAsc  = ` ORDER BY internal_date ASC`
	messageUpsertSQL = `
		INSERT INTO messages
			(id, thread_id, account_id, subject, from_addr, to_addr, snippet, body,
			 internal_date, is_unread, cache_state, has_attachments, message_id, cc_addr, bcc_addr,
			 reply_to, references_hdr, in_reply_to)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			bcc_addr        = CASE WHEN excluded.bcc_addr != '' THEN excluded.bcc_addr ELSE messages.bcc_addr END,
			reply_to        = COALESCE(excluded.reply_to, messages.reply_to),
			references_hdr  = COALESCE(excluded.references_hdr, messages.references_hdr),
			in_reply_to     = COALESCE(excluded.in_reply_to, messages.in_reply_to)
	`
)

type Message struct {
	ID        string
	ThreadID  string
	MessageID string // RFC 2822 Message-ID header value
	AccountID string
	Subject   string
	FromAddr  string
	ToAddr    string
	CcAddr    string
	BccAddr   string
	// ReplyTo, References and InReplyTo are the raw (unfolded) reply headers.
	// HeadersKnown is false for messages synced before they were stored; the
	// three fields are then empty and mean "unknown", not "absent".
	ReplyTo, References, InReplyTo string
	HeadersKnown                   bool
	Snippet                        string
	Body                           string
	InternalDate                   int64
	IsUnread                       bool
	IsStarred                      bool
	CacheState                     CacheState
	HasAttachments                 bool
}

func (s *Store) UpsertMessage(m Message) error {
	_, err := s.db.Exec(messageUpsertSQL, messageUpsertArgs(m)...)
	return err
}

// UpsertMessageMeta stores a synced message, bumps its thread's last date and,
// when labelIDs is non-nil, replaces its labels, all in one transaction.
func (s *Store) UpsertMessageMeta(m Message, labelIDs []string, setLabels bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(messageUpsertSQL, messageUpsertArgs(m)...); err != nil {
		return err
	}
	if _, err := tx.Exec(threadUpsertSQL, m.ThreadID, m.AccountID, m.InternalDate); err != nil {
		return err
	}
	if setLabels {
		if err := setMessageLabelsTx(tx, m.AccountID, m.ID, labelIDs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetMessage(accountID, messageID string) (Message, error) {
	row := s.db.QueryRow(`
		SELECT `+messageSelectColumns+`
		FROM messages WHERE id = ? AND account_id = ?
	`, messageID, accountID)
	m, err := scanMessageRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, fmt.Errorf("message %q not found", messageID)
	}
	return m, err
}

func (s *Store) ListMessagesByThread(accountID, threadID string) ([]Message, error) {
	return s.queryMessages(`
		SELECT `+messageSelectColumns+`
		FROM messages
		WHERE account_id = ? AND thread_id = ?`+messageOrderAsc,
		accountID, threadID,
	)
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
		nullIf(m.HeadersKnown, m.ReplyTo),
		nullIf(m.HeadersKnown, m.References),
		nullIf(m.HeadersKnown, m.InReplyTo),
	}
}

// nullIf is v when known, else NULL (so an upsert keeps the stored value).
func nullIf(known bool, v string) any {
	if !known {
		return nil
	}
	return v
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int) bool { return v == 1 }

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessageRow(row rowScanner) (Message, error) {
	var m Message
	var isUnread, hasAttachments, isStarred int
	var replyTo, refs, irt sql.NullString
	if err := row.Scan(
		&m.ID, &m.ThreadID, &m.AccountID, &m.Subject, &m.FromAddr, &m.ToAddr,
		&m.Snippet, &m.Body, &m.InternalDate, &isUnread, &m.CacheState, &hasAttachments, &m.MessageID,
		&m.CcAddr, &m.BccAddr, &replyTo, &refs, &irt, &isStarred,
	); err != nil {
		return Message{}, err
	}
	m.ReplyTo, m.References, m.InReplyTo, m.HeadersKnown = replyTo.String, refs.String, irt.String, replyTo.Valid
	m.IsUnread = intToBool(isUnread)
	m.HasAttachments = intToBool(hasAttachments)
	m.IsStarred = intToBool(isStarred)
	return m, nil
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var out []Message
	for rows.Next() {
		m, err := scanMessageRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

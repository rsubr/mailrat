package storage

import (
	"database/sql"
	"errors"
)

// Attachment holds metadata and optionally the binary content of a message attachment.
type Attachment struct {
	Name         string
	MimeType     string
	Size         int64  // bytes
	AttachmentID string // Gmail part attachment ID; empty for small inline attachments
	Data         []byte // binary content; nil if not yet downloaded
}

// UpsertAttachments replaces all attachment records for (accountID, messageID).
func (s *Store) UpsertAttachments(accountID, messageID string, attachments []Attachment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if err := upsertAttachmentsTx(tx, accountID, messageID, attachments); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertAttachmentsTx(tx *sql.Tx, accountID, messageID string, attachments []Attachment) error {
	if _, err := tx.Exec(
		`DELETE FROM attachments WHERE message_id = ? AND account_id = ?`,
		messageID, accountID,
	); err != nil {
		return err
	}

	for _, a := range attachments {
		if _, err := tx.Exec(
			`INSERT INTO attachments (message_id, account_id, name, mime_type, size, attachment_id, data)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			messageID, accountID, a.Name, a.MimeType, a.Size, a.AttachmentID, a.Data,
		); err != nil {
			return err
		}
	}

	// The flag drives list paperclips and whether readers load attachments.
	if len(attachments) > 0 {
		if _, err := tx.Exec(
			`UPDATE messages SET has_attachments = 1 WHERE id = ? AND account_id = ?`,
			messageID, accountID,
		); err != nil {
			return err
		}
	}
	return nil
}

// GetAttachments returns all attachments for a message, including cached binary data.
func (s *Store) GetAttachments(accountID, messageID string) ([]Attachment, error) {
	rows, err := s.db.Query(
		`SELECT name, mime_type, size, attachment_id, data FROM attachments
		 WHERE message_id = ? AND account_id = ?
		 ORDER BY rowid`,
		messageID, accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.Name, &a.MimeType, &a.Size, &a.AttachmentID, &a.Data); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAttachmentsMeta returns attachment metadata only; the data column is never
// read, so opening a message does not load attachment bytes into memory.
func (s *Store) GetAttachmentsMeta(accountID, messageID string) ([]Attachment, error) {
	rows, err := s.db.Query(
		`SELECT name, mime_type, size, attachment_id FROM attachments
		 WHERE message_id = ? AND account_id = ?
		 ORDER BY rowid`,
		messageID, accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.Name, &a.MimeType, &a.Size, &a.AttachmentID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AttachmentData returns the cached bytes of one attachment, or nil if not cached.
func (s *Store) AttachmentData(accountID, messageID, attachmentID string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(
		`SELECT data FROM attachments WHERE message_id = ? AND account_id = ? AND attachment_id = ?`,
		messageID, accountID, attachmentID,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return data, err
}

// AttachmentCached reports whether the attachment's bytes are stored, without reading them.
func (s *Store) AttachmentCached(accountID, messageID, attachmentID string) (bool, error) {
	var cached bool
	err := s.db.QueryRow(
		`SELECT data IS NOT NULL FROM attachments WHERE message_id = ? AND account_id = ? AND attachment_id = ?`,
		messageID, accountID, attachmentID,
	).Scan(&cached)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return cached, err
}

// MaxSyncPrefetchSize is the largest attachment (bytes) cached in the background;
// bigger ones download when opened.
const MaxSyncPrefetchSize = 2 << 20

// MissingAttachment identifies an attachment row whose binary data has not yet been cached.
type MissingAttachment struct {
	MessageID    string
	AttachmentID string
	MimeType     string
	Name         string
}

// ListMissingAttachmentRows returns attachment rows (data IS NULL) for body-cached messages
// of the account, regardless of label, dated at or after sinceMs (epoch ms).
// Results are capped at limit.
func (s *Store) ListMissingAttachmentRows(accountID string, sinceMs int64, limit int) ([]MissingAttachment, error) {
	const query = `
		SELECT a.message_id, a.attachment_id, a.mime_type, a.name
		FROM attachments a
		JOIN messages m ON m.id = a.message_id AND m.account_id = a.account_id
		WHERE a.account_id = ?
		  AND a.data IS NULL
		  AND a.attachment_id != ''
		  AND NOT SUBSTR(a.attachment_id, 1, 5) = 'name:'
		  AND m.cache_state != 'metadata_only'
		  AND m.internal_date >= ?
		  AND a.size <= ?
		LIMIT ?`

	rows, err := s.db.Query(query, accountID, sinceMs, MaxSyncPrefetchSize, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MissingAttachment
	for rows.Next() {
		var ma MissingAttachment
		if err := rows.Scan(&ma.MessageID, &ma.AttachmentID, &ma.MimeType, &ma.Name); err != nil {
			return nil, err
		}
		out = append(out, ma)
	}
	return out, rows.Err()
}

// UpdateAttachmentData writes binary data into an existing attachment row.
func (s *Store) UpdateAttachmentData(accountID, messageID, attachmentID string, data []byte) error {
	_, err := s.db.Exec(
		`UPDATE attachments SET data = ?
		 WHERE message_id = ? AND account_id = ? AND attachment_id = ?`,
		data, messageID, accountID, attachmentID,
	)
	return err
}

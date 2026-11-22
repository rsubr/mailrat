package storage

import (
	"fmt"
	"strings"
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

	return tx.Commit()
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

// MissingAttachment identifies an attachment row whose binary data has not yet been cached.
type MissingAttachment struct {
	MessageID    string
	AttachmentID string
	MimeType     string
	Name         string
}

// ListMissingAttachmentRows returns attachment rows (data IS NULL) for body-cached messages
// that carry at least one of the given label IDs. Results are capped at limit.
func (s *Store) ListMissingAttachmentRows(accountID string, labelIDs []string, limit int) ([]MissingAttachment, error) {
	if len(labelIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(labelIDs))
	args := make([]any, len(labelIDs))
	for i, id := range labelIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	query := fmt.Sprintf(`
		SELECT DISTINCT a.message_id, a.attachment_id, a.mime_type, a.name
		FROM attachments a
		JOIN messages m ON m.id = a.message_id AND m.account_id = a.account_id
		JOIN message_labels ml ON ml.message_id = a.message_id AND ml.account_id = a.account_id
		WHERE a.account_id = ?
		  AND a.data IS NULL
		  AND a.attachment_id != ''
		  AND NOT SUBSTR(a.attachment_id, 1, 5) = 'name:'
		  AND m.cache_state != 'metadata_only'
		  AND ml.label_id IN (%s)
		LIMIT ?`, inClause)

	args = append([]any{accountID}, append(args, limit)...)

	rows, err := s.db.Query(query, args...)
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

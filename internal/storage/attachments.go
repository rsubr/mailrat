package storage

// Attachment holds metadata and optionally the binary content of a message attachment.
type Attachment struct {
	Name         string
	MimeType     string
	Size         int64 // bytes
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

package storage

import (
	"database/sql"
	"errors"
	"slices"
)

// SyncMeta is one synced message and, when SetLabels, its replacement labels.
type SyncMeta struct {
	Msg       Message
	LabelIDs  []string
	SetLabels bool
}

// SyncLabelEdit adds and removes labels on a stored message.
type SyncLabelEdit struct {
	MessageID   string
	Add, Remove []string
}

// SyncBatch is one unit of sync writes. ApplySyncBatch commits all of it, and
// the history cursor, in one transaction.
type SyncBatch struct {
	Metas     []SyncMeta
	Edits     []SyncLabelEdit // applied in order after Metas
	DeleteIDs []string        // applied last
	HistoryID string          // new cursor; "" leaves it unchanged
}

// ApplySyncBatch writes a sync batch atomically: after a crash either every
// change and the cursor are stored, or none.
func (s *Store) ApplySyncBatch(accountID string, b SyncBatch) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, m := range b.Metas {
		if _, err := tx.Exec(messageUpsertSQL, messageUpsertArgs(m.Msg)...); err != nil {
			return err
		}
		if _, err := tx.Exec(threadUpsertSQL, m.Msg.ThreadID, m.Msg.AccountID, m.Msg.InternalDate); err != nil {
			return err
		}
		if m.SetLabels {
			if err := setMessageLabelsTx(tx, m.Msg.AccountID, m.Msg.ID, m.LabelIDs); err != nil {
				return err
			}
		}
	}
	for _, e := range b.Edits {
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM messages WHERE id = ? AND account_id = ?`, e.MessageID, accountID).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue // not stored locally; nothing to edit
			}
			return err
		}
		rows, err := tx.Query(`SELECT label_id FROM message_labels WHERE message_id = ? AND account_id = ?`, e.MessageID, accountID)
		if err != nil {
			return err
		}
		var labels []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			labels = append(labels, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range e.Add {
			if !slices.Contains(labels, id) {
				labels = append(labels, id)
			}
		}
		labels = slices.DeleteFunc(labels, func(id string) bool { return slices.Contains(e.Remove, id) })
		if err := setMessageLabelsTx(tx, accountID, e.MessageID, labels); err != nil {
			return err
		}
	}
	for _, id := range b.DeleteIDs {
		if err := deleteMessageTx(tx, accountID, id); err != nil {
			return err
		}
	}
	if b.HistoryID != "" {
		if _, err := tx.Exec(`UPDATE accounts SET history_id = ? WHERE id = ?`, b.HistoryID, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertMessageBody stores a fetched body and its attachment records in one
// transaction, so a cached body never lacks its attachments.
func (s *Store) UpsertMessageBody(m Message, atts []Attachment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(messageUpsertSQL, messageUpsertArgs(m)...); err != nil {
		return err
	}
	if len(atts) > 0 {
		if err := upsertAttachmentsTx(tx, m.AccountID, m.ID, atts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteMessageTx is DeleteMessage inside tx; a message not stored is a no-op.
func deleteMessageTx(tx *sql.Tx, accountID, messageID string) error {
	var threadID string
	if err := tx.QueryRow(`SELECT thread_id FROM messages WHERE id = ? AND account_id = ?`, messageID, accountID).Scan(&threadID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE id = ? AND account_id = ?`, messageID, accountID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM threads WHERE id = ? AND account_id = ?
		AND NOT EXISTS (SELECT 1 FROM messages WHERE thread_id = ? AND account_id = ?)`,
		threadID, accountID, threadID, accountID)
	return err
}

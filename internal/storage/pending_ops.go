package storage

import "time"

const (
	OpMarkRead    = "mark_read"
	OpMarkUnread  = "mark_unread"
	OpTrashThread = "trash_thread"
	OpArchive     = "archive_thread"
	OpMarkSpam    = "mark_spam"
	OpMarkNotSpam = "mark_not_spam"
	OpAddLabel    = "add_label:"    // + label ID, applied to a thread
	OpRemoveLabel = "remove_label:" // + label ID, applied to a thread
)

// PendingOp is a mutation applied locally but not yet confirmed by Gmail.
// MessageID is the target: a message ID for read/unread, a thread ID otherwise.
type PendingOp struct {
	ID        int64
	OpType    string
	AccountID string
	MessageID string
	Attempts  int
}

// AddPendingOp enqueues an operation. An identical queued (op_type, account_id,
// message_id) tuple is replaced, which moves it to the back of the queue so
// that replay order always matches the order the user acted in.
func (s *Store) AddPendingOp(opType, accountID, messageID string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pending_ops (op_type, account_id, message_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		opType, accountID, messageID, time.Now().UnixMilli(),
	)
	return err
}

// ListPendingOps returns all queued operations in insertion order.
func (s *Store) ListPendingOps() ([]PendingOp, error) {
	rows, err := s.db.Query(
		`SELECT id, op_type, account_id, message_id, attempts FROM pending_ops ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingOp
	for rows.Next() {
		var op PendingOp
		if err := rows.Scan(&op.ID, &op.OpType, &op.AccountID, &op.MessageID, &op.Attempts); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

// DeletePendingOp removes a successfully applied operation.
func (s *Store) DeletePendingOp(id int64) error {
	_, err := s.db.Exec(`DELETE FROM pending_ops WHERE id = ?`, id)
	return err
}

// BumpPendingOpAttempts records one more failed attempt and returns the new count.
func (s *Store) BumpPendingOpAttempts(id int64) (int, error) {
	var n int
	err := s.db.QueryRow(
		`UPDATE pending_ops SET attempts = attempts + 1 WHERE id = ? RETURNING attempts`, id,
	).Scan(&n)
	return n, err
}

package storage

import "time"

const OpMarkRead = "mark_read"

// PendingOp is an operation that failed while offline and must be retried.
type PendingOp struct {
	ID        int64
	OpType    string
	AccountID string
	MessageID string
}

// AddPendingOp enqueues an operation. Duplicate (op_type, account_id, message_id)
// tuples are silently ignored via the UNIQUE constraint.
func (s *Store) AddPendingOp(opType, accountID, messageID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO pending_ops (op_type, account_id, message_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		opType, accountID, messageID, time.Now().UnixMilli(),
	)
	return err
}

// ListPendingOps returns all queued operations in insertion order.
func (s *Store) ListPendingOps() ([]PendingOp, error) {
	rows, err := s.db.Query(
		`SELECT id, op_type, account_id, message_id FROM pending_ops ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingOp
	for rows.Next() {
		var op PendingOp
		if err := rows.Scan(&op.ID, &op.OpType, &op.AccountID, &op.MessageID); err != nil {
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

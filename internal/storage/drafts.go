package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

type Draft struct {
	ID        string
	AccountID string
	Content   string
	UpdatedAt int64
	RemoteID  string // Gmail draft ID once synced; empty for a local-only draft
}

// ErrDraftNotFound is returned by GetDraft for an unknown draft.
var ErrDraftNotFound = errors.New("draft not found")

func (s *Store) UpsertDraft(d Draft) error {
	_, err := s.db.Exec(`
		INSERT INTO drafts (id, account_id, content, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			content    = excluded.content,
			updated_at = excluded.updated_at
	`, d.ID, d.AccountID, d.Content, d.UpdatedAt)
	return err
}

func (s *Store) GetDraft(accountID, draftID string) (Draft, error) {
	var d Draft
	err := s.db.QueryRow(`
		SELECT id, account_id, content, updated_at, remote_id FROM drafts WHERE id = ? AND account_id = ?
	`, draftID, accountID).Scan(&d.ID, &d.AccountID, &d.Content, &d.UpdatedAt, &d.RemoteID)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, fmt.Errorf("draft %q: %w", draftID, ErrDraftNotFound)
	}
	return d, err
}

func (s *Store) DeleteDraft(accountID, draftID string) error {
	_, err := s.db.Exec(`DELETE FROM drafts WHERE id = ? AND account_id = ?`, draftID, accountID)
	return err
}

func (s *Store) ListDrafts(accountID string) ([]Draft, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, content, updated_at, remote_id FROM drafts WHERE account_id = ?
		ORDER BY updated_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Draft
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Content, &d.UpdatedAt, &d.RemoteID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDraftRemoteID records the Gmail draft ID a local draft was synced to.
func (s *Store) SetDraftRemoteID(accountID, draftID, remoteID string) error {
	_, err := s.db.Exec(`UPDATE drafts SET remote_id = ? WHERE id = ? AND account_id = ?`, remoteID, draftID, accountID)
	return err
}

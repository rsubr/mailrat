package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

type Account struct {
	ID          string
	Email       string
	TokenJSON   string
	HistoryID   string
	PhotoURL    string
	DisplayName string
}

func (s *Store) UpsertAccount(a Account) error {
	_, err := s.db.Exec(`
		INSERT INTO accounts (id, email, token_json, history_id, photo_url, display_name)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			email        = excluded.email,
			token_json   = excluded.token_json,
			history_id   = excluded.history_id,
			photo_url    = excluded.photo_url,
			display_name = excluded.display_name
	`, a.ID, a.Email, a.TokenJSON, a.HistoryID, a.PhotoURL, a.DisplayName)
	return err
}

func (s *Store) GetAccount(id string) (Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT id, email, token_json, history_id, photo_url, display_name FROM accounts WHERE id = ?
	`, id).Scan(&a.ID, &a.Email, &a.TokenJSON, &a.HistoryID, &a.PhotoURL, &a.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, fmt.Errorf("account %q not found", id)
	}
	return a, err
}

func (s *Store) GetAccountByEmail(email string) (Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT id, email, token_json, history_id, photo_url, display_name FROM accounts WHERE email = ?
	`, email).Scan(&a.ID, &a.Email, &a.TokenJSON, &a.HistoryID, &a.PhotoURL, &a.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, fmt.Errorf("account %q not found", email)
	}
	return a, err
}

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT id, email, token_json, history_id, photo_url, display_name FROM accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Email, &a.TokenJSON, &a.HistoryID, &a.PhotoURL, &a.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAccount(id string) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// ResetAccountMail wipes an account's local mail (messages, threads, queued
// changes) and its history cursor so the next sync is a first sync. Labels,
// settings and sync scopes are kept; scopes are marked for backfill again.
func (s *Store) ResetAccountMail(accountID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, q := range []string{
		`DELETE FROM messages WHERE account_id = ?`,
		`DELETE FROM threads WHERE account_id = ?`,
		`DELETE FROM pending_ops WHERE account_id = ?`,
		`UPDATE sync_config SET backfilled = 0 WHERE account_id = ?`,
		`UPDATE accounts SET history_id = '' WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateAccountDisplayName(id, name string) error {
	_, err := s.db.Exec(`UPDATE accounts SET display_name = ? WHERE id = ?`, name, id)
	return err
}

func (s *Store) UpdateHistoryID(accountID, historyID string) error {
	_, err := s.db.Exec(`UPDATE accounts SET history_id = ? WHERE id = ?`, historyID, accountID)
	return err
}

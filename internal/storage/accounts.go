package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

type Account struct {
	ID        string
	Email     string
	TokenJSON string
	HistoryID string
}

func (s *Store) UpsertAccount(a Account) error {
	_, err := s.db.Exec(`
		INSERT INTO accounts (id, email, token_json, history_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			email      = excluded.email,
			token_json = excluded.token_json,
			history_id = excluded.history_id
	`, a.ID, a.Email, a.TokenJSON, a.HistoryID)
	return err
}

func (s *Store) GetAccount(id string) (Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT id, email, token_json, history_id FROM accounts WHERE id = ?
	`, id).Scan(&a.ID, &a.Email, &a.TokenJSON, &a.HistoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, fmt.Errorf("account %q not found", id)
	}
	return a, err
}

func (s *Store) GetAccountByEmail(email string) (Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT id, email, token_json, history_id FROM accounts WHERE email = ?
	`, email).Scan(&a.ID, &a.Email, &a.TokenJSON, &a.HistoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, fmt.Errorf("account %q not found", email)
	}
	return a, err
}

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT id, email, token_json, history_id FROM accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Email, &a.TokenJSON, &a.HistoryID); err != nil {
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

func (s *Store) UpdateHistoryID(accountID, historyID string) error {
	_, err := s.db.Exec(`UPDATE accounts SET history_id = ? WHERE id = ?`, historyID, accountID)
	return err
}

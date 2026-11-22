package storage

import (
	"database/sql"
	"errors"
)

type AccountSettings struct {
	AccountID  string
	SyncPeriod string // "all", "1y", "90d", "30d"
}

func (s *Store) GetAccountSettings(accountID string) (AccountSettings, error) {
	st := AccountSettings{AccountID: accountID}
	err := s.db.QueryRow(`
		SELECT sync_period FROM account_settings WHERE account_id = ?
	`, accountID).Scan(&st.SyncPeriod)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountSettings{AccountID: accountID, SyncPeriod: "30d"}, nil
	}
	if err != nil {
		return AccountSettings{}, err
	}
	return st, nil
}

func (s *Store) UpsertAccountSettings(st AccountSettings) error {
	_, err := s.db.Exec(`
		INSERT INTO account_settings (account_id, sync_period)
		VALUES (?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			sync_period = excluded.sync_period
	`, st.AccountID, st.SyncPeriod)
	return err
}

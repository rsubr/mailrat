package storage

import (
	"database/sql"
	"errors"
)

type AccountSettings struct {
	AccountID        string
	SyncPeriod       string // "all", "1y", "90d", "30d"
	FetchAttachments bool
}

func (s *Store) GetAccountSettings(accountID string) (AccountSettings, error) {
	var st AccountSettings
	var fetchAttachments int
	st.AccountID = accountID
	err := s.db.QueryRow(`
		SELECT sync_period, fetch_attachments FROM account_settings WHERE account_id = ?
	`, accountID).Scan(&st.SyncPeriod, &fetchAttachments)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountSettings{AccountID: accountID, SyncPeriod: "30d", FetchAttachments: true}, nil
	}
	if err != nil {
		return AccountSettings{}, err
	}
	st.FetchAttachments = fetchAttachments != 0
	return st, nil
}

func (s *Store) UpsertAccountSettings(st AccountSettings) error {
	fetchAttachments := 0
	if st.FetchAttachments {
		fetchAttachments = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO account_settings (account_id, sync_period, fetch_attachments)
		VALUES (?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			sync_period       = excluded.sync_period,
			fetch_attachments = excluded.fetch_attachments
	`, st.AccountID, st.SyncPeriod, fetchAttachments)
	return err
}

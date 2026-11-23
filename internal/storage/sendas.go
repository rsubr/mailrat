package storage

// SendAs is a cached Gmail send-as alias with its signature.
type SendAs struct {
	AccountID     string
	Email         string
	DisplayName   string
	SignatureHTML string
	IsPrimary     bool
	IsDefault     bool
}

// ReplaceSendAs atomically replaces the account's cached aliases.
func (s *Store) ReplaceSendAs(accountID string, list []SendAs) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM send_as WHERE account_id = ?`, accountID); err != nil {
		return err
	}
	for _, a := range list {
		if _, err := tx.Exec(`
			INSERT INTO send_as (account_id, email, display_name, signature_html, is_primary, is_default)
			VALUES (?, ?, ?, ?, ?, ?)`,
			accountID, a.Email, a.DisplayName, a.SignatureHTML, a.IsPrimary, a.IsDefault); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSendAs returns the account's cached aliases, primary first.
func (s *Store) ListSendAs(accountID string) ([]SendAs, error) {
	rows, err := s.db.Query(`
		SELECT email, display_name, signature_html, is_primary, is_default
		FROM send_as WHERE account_id = ? ORDER BY is_primary DESC, email`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SendAs
	for rows.Next() {
		a := SendAs{AccountID: accountID}
		if err := rows.Scan(&a.Email, &a.DisplayName, &a.SignatureHTML, &a.IsPrimary, &a.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

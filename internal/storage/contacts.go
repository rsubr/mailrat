package storage

type Contact struct {
	ID        string
	AccountID string
	Name      string
	Email     string
}

func (s *Store) UpsertContact(c Contact) error {
	_, err := s.db.Exec(`
		INSERT INTO contacts (id, account_id, name, email)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			name  = excluded.name,
			email = excluded.email
	`, c.ID, c.AccountID, c.Name, c.Email)
	return err
}

func (s *Store) SearchContacts(accountID, query string, limit int) ([]Contact, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, email FROM contacts
		WHERE account_id = ? AND (name LIKE ? OR email LIKE ?)
		ORDER BY name ASC
		LIMIT ?
	`, accountID, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Email); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListContacts(accountID string) ([]Contact, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, email FROM contacts WHERE account_id = ?
		ORDER BY name ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Email); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

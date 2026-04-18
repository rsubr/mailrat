package storage

type Contact struct {
	ID           string // resource_name + ":" + email
	AccountID    string
	Name         string
	Email        string
	ResourceName string // e.g. "people/123456789"
	Etag         string // required for updates
}

func (s *Store) UpsertContact(c Contact) error {
	_, err := s.db.Exec(`
		INSERT INTO contacts (id, account_id, name, email, resource_name, etag)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, account_id) DO UPDATE SET
			name          = excluded.name,
			email         = excluded.email,
			resource_name = excluded.resource_name,
			etag          = excluded.etag
	`, c.ID, c.AccountID, c.Name, c.Email, c.ResourceName, c.Etag)
	return err
}

func (s *Store) GetContact(accountID, id string) (Contact, error) {
	var c Contact
	err := s.db.QueryRow(`
		SELECT id, account_id, name, email, resource_name, etag
		FROM contacts WHERE id = ? AND account_id = ?
	`, id, accountID).Scan(&c.ID, &c.AccountID, &c.Name, &c.Email, &c.ResourceName, &c.Etag)
	return c, err
}

func (s *Store) DeleteContact(accountID, id string) error {
	_, err := s.db.Exec(`DELETE FROM contacts WHERE id = ? AND account_id = ?`, id, accountID)
	return err
}

func (s *Store) SearchContacts(accountID, query string, limit int) ([]Contact, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, email, resource_name, etag FROM contacts
		WHERE account_id = ? AND (name LIKE ? OR email LIKE ?)
		ORDER BY name ASC
		LIMIT ?
	`, accountID, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContacts(rows)
}

func (s *Store) SearchAllContacts(query string, limit int) ([]Contact, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, email, resource_name, etag FROM contacts
		WHERE name LIKE ? OR email LIKE ?
		ORDER BY name ASC
		LIMIT ?
	`, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContacts(rows)
}

func (s *Store) ListContacts(accountID string) ([]Contact, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, email, resource_name, etag FROM contacts
		WHERE account_id = ?
		ORDER BY name ASC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContacts(rows)
}

func scanContacts(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}) ([]Contact, error) {
	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Email, &c.ResourceName, &c.Etag); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

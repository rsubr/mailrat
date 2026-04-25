package storage

type GlobalSettings struct {
	SyncFrequency    string // "15m", "1h", "manual"
	DefaultAccountID string
}

func (s *Store) GetGlobalSettings() (GlobalSettings, error) {
	rows, err := s.db.Query(`SELECT key, value FROM app_settings`)
	if err != nil {
		return GlobalSettings{SyncFrequency: "15m"}, nil
	}
	defer rows.Close()

	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		m[k] = v
	}
	gs := GlobalSettings{SyncFrequency: "15m"}
	if v, ok := m["sync_frequency"]; ok {
		gs.SyncFrequency = v
	}
	if v, ok := m["default_account_id"]; ok {
		gs.DefaultAccountID = v
	}
	return gs, nil
}

func (s *Store) SetGlobalSettings(gs GlobalSettings) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	pairs := [][2]string{
		{"sync_frequency", gs.SyncFrequency},
		{"default_account_id", gs.DefaultAccountID},
	}
	for _, p := range pairs {
		if _, err := tx.Exec(`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, p[0], p[1]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

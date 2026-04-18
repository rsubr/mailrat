package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store is the central database handle.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return nil, fmt.Errorf("enable fk: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// migrate applies additive schema changes to existing databases.
// Each statement is run independently; failures are ignored so that
// already-applied migrations (e.g. column already exists) don't abort startup.
func migrate(db *sql.DB) error {
	migrations := []string{
		`ALTER TABLE contacts ADD COLUMN resource_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE contacts ADD COLUMN etag TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN has_attachments INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sync_config ADD COLUMN backfilled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN message_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN cc_addr TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN bcc_addr TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE labels ADD COLUMN text_color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE labels ADD COLUMN background_color TEXT NOT NULL DEFAULT ''`,
	}
	for _, m := range migrations {
		db.Exec(m) //nolint:errcheck — column-already-exists errors are fine
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

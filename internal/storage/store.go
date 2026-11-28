package storage

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the central database handle.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(4)
	// Close idle connections so their page caches are released between bursts.
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(30 * time.Second)

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := migrateIndexes(db); err != nil {
		return nil, fmt.Errorf("migrate indexes: %w", err)
	}

	return &Store{db: db}, nil
}

// pragmas are applied by the driver to every pooled connection (per-connection
// settings cannot be set with a one-off db.Exec).
var pragmas = []string{
	"busy_timeout(5000)",
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"foreign_keys(1)",
	"cache_size(-2000)", // 2 MB
	"temp_store(FILE)",
	"mmap_size(0)",
}

// dsn appends the connection pragmas to path.
func dsn(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var b strings.Builder
	b.WriteString(path)
	for _, p := range pragmas {
		b.WriteString(sep + "_pragma=" + url.QueryEscape(p))
		sep = "&"
	}
	return b.String()
}

// migrate applies additive schema changes to existing databases.
// Each statement is run independently; failures are ignored so that
// already-applied migrations (e.g. column already exists) don't abort startup.
func migrate(db *sql.DB) error {
	migrations := []string{
		`ALTER TABLE contacts ADD COLUMN resource_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE contacts ADD COLUMN etag TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE contacts ADD COLUMN phone TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN has_attachments INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sync_config ADD COLUMN backfilled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN message_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN cc_addr TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN bcc_addr TEXT NOT NULL DEFAULT ''`,
		// Reply headers are NULL until a message is fetched with them (unknown),
		// and '' once fetched without the header.
		`ALTER TABLE messages ADD COLUMN reply_to TEXT`,
		`ALTER TABLE messages ADD COLUMN references_hdr TEXT`,
		`ALTER TABLE messages ADD COLUMN in_reply_to TEXT`,
		`ALTER TABLE labels ADD COLUMN text_color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE labels ADD COLUMN background_color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN photo_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS account_settings (account_id TEXT PRIMARY KEY, sync_period TEXT NOT NULL DEFAULT '30d', FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`ALTER TABLE attachments ADD COLUMN attachment_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE attachments ADD COLUMN data BLOB`,
		`ALTER TABLE pending_ops ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sync_config ADD COLUMN max_emails INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE drafts ADD COLUMN remote_id TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS send_as (account_id TEXT NOT NULL, email TEXT NOT NULL, display_name TEXT NOT NULL DEFAULT '', signature_html TEXT NOT NULL DEFAULT '', is_primary INTEGER NOT NULL DEFAULT 0, is_default INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (account_id, email), FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE)`,
		// The download-attachments setting was removed: syncing always fetches them.
		`ALTER TABLE sync_config DROP COLUMN fetch_attachments`,
		`ALTER TABLE account_settings DROP COLUMN fetch_attachments`,
		// Repair rows stored before UpsertAttachments set the flag.
		`UPDATE messages SET has_attachments = 1 WHERE has_attachments = 0 AND EXISTS (SELECT 1 FROM attachments a WHERE a.message_id = messages.id AND a.account_id = messages.account_id)`,
	}
	for _, m := range migrations {
		db.Exec(m) //nolint:errcheck — column-already-exists errors are fine
	}
	return nil
}

// ftsColumns are the messages columns mirrored into the messages_fts index.
const ftsColumns = `subject, body, from_addr, to_addr, cc_addr, bcc_addr`

// migrateIndexes creates secondary indexes and the FTS5 search index. Every
// statement is idempotent and errors are returned (unlike migrate). It runs
// after migrate so the cc_addr/bcc_addr columns exist on old databases.
func migrateIndexes(db *sql.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(account_id, thread_id, internal_date)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_msg ON attachments(account_id, message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_missing ON attachments(account_id) WHERE data IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_messages_unread ON messages(account_id, thread_id) WHERE is_unread = 1`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'messages_fts'`).Scan(&n); err != nil {
		return err
	}
	fresh := n == 0

	newVals, oldVals := ftsValues("new"), ftsValues("old")
	fts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(` + ftsColumns + `, content='messages', content_rowid='rowid')`,
		`CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, ` + ftsColumns + `) VALUES (new.rowid, ` + newVals + `);
		END`,
		`CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, ` + ftsColumns + `) VALUES ('delete', old.rowid, ` + oldVals + `);
		END`,
		`CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE ON messages
		WHEN ` + ftsChanged() + ` BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, ` + ftsColumns + `) VALUES ('delete', old.rowid, ` + oldVals + `);
			INSERT INTO messages_fts(rowid, ` + ftsColumns + `) VALUES (new.rowid, ` + newVals + `);
		END`,
	}
	for _, q := range fts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("fts5: %w", err)
		}
	}
	if fresh {
		// Backfill rows that existed before the index.
		if _, err := db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES ('rebuild')`); err != nil {
			return fmt.Errorf("fts5 rebuild: %w", err)
		}
	}
	return nil
}

func ftsValues(prefix string) string {
	cols := strings.Split(ftsColumns, ", ")
	for i, c := range cols {
		cols[i] = prefix + "." + c
	}
	return strings.Join(cols, ", ")
}

func ftsChanged() string {
	cols := strings.Split(ftsColumns, ", ")
	for i, c := range cols {
		cols[i] = "old." + c + " IS NOT new." + c
	}
	return strings.Join(cols, " OR ")
}

// Close folds the WAL into the main database file (best effort) and closes it.
func (s *Store) Close() error {
	s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`) //nolint:errcheck
	return s.db.Close()
}

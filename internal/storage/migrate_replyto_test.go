package storage_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

// oldDB builds a database as the previous release left it: messages without
// the reply-header columns, and a row in it.
func oldDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE accounts (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, token_json TEXT NOT NULL, history_id TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE threads (id TEXT NOT NULL, account_id TEXT NOT NULL, last_message_date INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (id, account_id))`,
		`CREATE TABLE messages (id TEXT NOT NULL, thread_id TEXT NOT NULL, account_id TEXT NOT NULL, subject TEXT NOT NULL DEFAULT '',
			from_addr TEXT NOT NULL DEFAULT '', to_addr TEXT NOT NULL DEFAULT '', snippet TEXT NOT NULL DEFAULT '', body TEXT NOT NULL DEFAULT '',
			internal_date INTEGER NOT NULL DEFAULT 0, is_unread INTEGER NOT NULL DEFAULT 1, cache_state TEXT NOT NULL DEFAULT 'metadata_only',
			has_attachments INTEGER NOT NULL DEFAULT 0, message_id TEXT NOT NULL DEFAULT '', cc_addr TEXT NOT NULL DEFAULT '', bcc_addr TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (id, account_id))`,
		`INSERT INTO accounts (id, email, token_json) VALUES ('a1', 'me@x.com', '{}')`,
		`INSERT INTO messages (id, thread_id, account_id, subject, from_addr, message_id, body, cache_state) VALUES ('m1', 't1', 'a1', 'Old', 'ann@x.com', '<old@x>', 'hello', 'body_cached')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestMigrationAddsReplyHeadersAsUnknown(t *testing.T) {
	path := oldDB(t)
	for pass := 1; pass <= 2; pass++ { // the second open proves the migration is idempotent
		s, err := storage.Open(path)
		if err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		m, err := s.GetMessage("a1", "m1")
		if err != nil {
			t.Fatal(err)
		}
		if m.Subject != "Old" || m.Body != "hello" || m.MessageID != "<old@x>" || m.HeadersKnown || m.ReplyTo != "" || m.References != "" {
			t.Fatalf("pass %d: %+v", pass, m)
		}
		s.Close()
	}
}

func TestUpsertReplyHeaders(t *testing.T) {
	s, err := storage.Open(oldDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := storage.Message{ID: "m1", ThreadID: "t1", AccountID: "a1", CacheState: storage.CacheMetadataOnly}

	// unknown headers (a body-only upsert) never wipe stored ones
	known := base
	known.ReplyTo, known.References, known.InReplyTo, known.HeadersKnown = "r@x.com", "<a@x> <b@x>", "<b@x>", true
	if err := s.UpsertMessage(known); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(base); err != nil {
		t.Fatal(err)
	}
	m, _ := s.GetMessage("a1", "m1")
	if !m.HeadersKnown || m.ReplyTo != "r@x.com" || m.References != "<a@x> <b@x>" || m.InReplyTo != "<b@x>" {
		t.Fatalf("%+v", m)
	}

	// known-empty is stored as empty, not unknown
	empty := base
	empty.ID, empty.HeadersKnown = "m2", true
	if err := s.UpsertMessage(empty); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.GetMessage("a1", "m2"); !m.HeadersKnown || m.ReplyTo != "" {
		t.Fatalf("%+v", m)
	}
}

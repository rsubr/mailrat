package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func openFileStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func seedInbox(t *testing.T, s *Store, threads int) {
	t.Helper()
	if err := s.UpsertAccount(Account{ID: "a1", Email: "a@x.com", TokenJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertLabel(Label{ID: "INBOX", AccountID: "a1", Name: "Inbox"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < threads; i++ {
		// Two messages per thread; thread i is newer than thread i-1. Dates repeat
		// in pairs to exercise the tie-breaker.
		for j := 0; j < 2; j++ {
			id := fmt.Sprintf("m%03d-%d", i, j)
			err := s.UpsertMessageMeta(Message{
				ID: id, ThreadID: fmt.Sprintf("t%03d", i), AccountID: "a1",
				Subject: fmt.Sprintf("subject %d", i), InternalDate: int64(i/2*10 + j),
				IsUnread: true, CacheState: CacheMetadataOnly,
			}, []string{"INBOX"}, true)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestMigrationsIdempotentAndIndexesUsed(t *testing.T) {
	s, path := openFileStore(t)
	for i := 0; i < 2; i++ {
		if err := migrate(s.db); err != nil {
			t.Fatal(err)
		}
		if err := migrateIndexes(s.db); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	s.Close()
	s2, err := Open(path) // reopen existing DB
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	plan := func(q string) string {
		rows, err := s2.db.Query("EXPLAIN QUERY PLAN "+q, "a1", "t1")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			out = append(out, detail)
		}
		return strings.Join(out, "\n")
	}
	if p := plan(`SELECT * FROM messages WHERE account_id = ? AND thread_id = ? ORDER BY internal_date`); !strings.Contains(p, "idx_messages_thread") {
		t.Errorf("thread query plan: %s", p)
	}
	if p := plan(`SELECT * FROM attachments WHERE account_id = ? AND message_id = ?`); !strings.Contains(p, "idx_attachments_msg") {
		t.Errorf("attachments plan: %s", p)
	}
	if p := plan(`SELECT thread_id FROM messages WHERE account_id = ? AND thread_id = ? AND is_unread = 1`); !strings.Contains(p, "idx_messages_") {
		t.Errorf("unread plan: %s", p)
	}
	var n int
	s2.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('idx_messages_unread','messages_fts_ai','messages_fts_ad','messages_fts_au')`).Scan(&n) //nolint:errcheck
	if n != 4 {
		t.Errorf("expected unread index and 3 triggers, got %d", n)
	}
}

func TestFTSBackfillOnExistingDB(t *testing.T) {
	s, path := openFileStore(t)
	seedInbox(t, s, 1)
	for _, q := range []string{`DROP TRIGGER messages_fts_ai`, `DROP TRIGGER messages_fts_ad`, `DROP TRIGGER messages_fts_au`, `DROP TABLE messages_fts`} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.LocalSearchThreadSummaries("subject", 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("backfill search: %v %v", got, err)
	}
}

func TestFTSSearchSyncAndSanitizing(t *testing.T) {
	s, _ := openFileStore(t)
	seedInbox(t, s, 0)
	up := func(id, thread, subj, body, from string) {
		t.Helper()
		if err := s.UpsertMessage(Message{ID: id, ThreadID: thread, AccountID: "a1", Subject: subj, Body: body, FromAddr: from, CacheState: CacheBodyCached}); err != nil {
			t.Fatal(err)
		}
	}
	up("1", "t1", "Quarterly report", "numbers inside", "bob@example.com")
	up("2", "t2", "Lunch", "pizza friday", "al@example.com")
	search := func(q string) int {
		t.Helper()
		r, err := s.LocalSearchThreadSummaries(q, 10)
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		return len(r)
	}
	if search("quart") != 1 || search("PIZZA") != 1 || search("bob@example.com") != 1 {
		t.Fatal("basic/prefix/case/email search failed")
	}
	if search("report pizza") != 0 || search("report numbers") != 1 {
		t.Fatal("all terms must match")
	}
	for _, q := range []string{`"`, `AND`, `a"b`, `-x`, `col:val`, `(`, `*`, "  ", ""} {
		search(q) // must not error
	}
	// update via trigger: old text gone, new text found
	up("1", "t1", "Quarterly report", "revised budget", "bob@example.com")
	if search("numbers") != 0 || search("budget") != 1 {
		t.Fatal("update not synced")
	}
	// metadata-only upsert must not clobber indexed body
	if err := s.UpsertMessage(Message{ID: "1", ThreadID: "t1", AccountID: "a1", Subject: "Quarterly report", CacheState: CacheMetadataOnly}); err != nil {
		t.Fatal(err)
	}
	if search("budget") != 1 {
		t.Fatal("body lost")
	}
	if _, err := s.db.Exec(`DELETE FROM messages WHERE id = '1'`); err != nil {
		t.Fatal(err)
	}
	if search("budget") != 0 || search("quarterly") != 0 {
		t.Fatal("delete not synced")
	}
	// attachment filename search
	if err := s.UpsertAttachments("a1", "2", []Attachment{{Name: "Invoice-2026.pdf"}}); err != nil {
		t.Fatal(err)
	}
	if search("invoice-20") != 1 {
		t.Fatal("attachment name search failed")
	}
}

func TestThreadPagination(t *testing.T) {
	s, _ := openFileStore(t)
	seedInbox(t, s, 25)
	var all []string
	for off := 0; ; off += 7 {
		page, err := s.ListAllThreadSummaries(7, off)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range page {
			if p.MessageCount != 2 || p.UnreadCount != 2 || p.Subject == "" {
				t.Fatalf("bad summary %+v", p)
			}
			all = append(all, p.ThreadID)
		}
		if len(page) < 7 {
			break
		}
	}
	if len(all) != 25 {
		t.Fatalf("want 25 threads, got %d", len(all))
	}
	seen := map[string]bool{}
	for _, id := range all {
		if seen[id] {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = true
	}
	full, _ := s.ListThreadSummaries("a1", 100, 0)
	for i := range full {
		if full[i].ThreadID != all[i] {
			t.Fatalf("order differs at %d", i)
		}
		if i > 0 && full[i].LastDate > full[i-1].LastDate {
			t.Fatal("not newest first")
		}
	}
	if byLabel, _ := s.ListThreadSummariesByLabel("a1", "INBOX", 5, 5); len(byLabel) != 5 || byLabel[0].ThreadID != all[5] {
		t.Fatal("label paging mismatch")
	}
}

func TestPragmasOnEveryConnection(t *testing.T) {
	s, _ := openFileStore(t)
	ctx := context.Background()
	var conns []*sql.Conn
	for i := 0; i < 3; i++ {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	want := map[string]string{
		"foreign_keys": "1", "busy_timeout": "5000", "journal_mode": "wal",
		"synchronous": "1", "cache_size": "-20000", "temp_store": "2", "mmap_size": "268435456",
	}
	for i, c := range conns {
		for k, v := range want {
			var got string
			if err := c.QueryRowContext(ctx, "PRAGMA "+k).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != v {
				t.Errorf("conn %d: %s = %s, want %s", i, k, got, v)
			}
		}
		c.Close()
	}
}

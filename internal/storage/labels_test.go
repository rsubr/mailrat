package storage

import "testing"

func TestDeleteMessageRemovesEmptyThread(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(Account{ID: "a1", Email: "a@x.com", TokenJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m1", "m2"} {
		if err := s.UpsertMessageMeta(Message{ID: id, ThreadID: "t1", AccountID: "a1", CacheState: CacheMetadataOnly}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	threads := func() int {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM threads WHERE id = 't1'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := s.DeleteMessage("a1", "m1"); err != nil {
		t.Fatal(err)
	}
	if threads() != 1 {
		t.Fatal("thread removed while messages remain")
	}
	if err := s.DeleteMessage("a1", "m2"); err != nil {
		t.Fatal(err)
	}
	if threads() != 0 {
		t.Fatal("empty thread row left behind")
	}
}

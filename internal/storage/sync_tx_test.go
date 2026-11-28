package storage

import (
	"path/filepath"
	"slices"
	"testing"
)

func syncTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(Account{ID: "a1", Email: "a@x.com", TokenJSON: "{}", HistoryID: "100"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertLabel(Label{ID: "INBOX", AccountID: "a1", Name: "INBOX", LabelType: "system"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func meta(id string, labels ...string) SyncMeta {
	return SyncMeta{Msg: Message{ID: id, ThreadID: "t-" + id, AccountID: "a1", InternalDate: 1}, LabelIDs: labels, SetLabels: true}
}

// An interruption (here a failing write) mid-batch must leave neither the
// earlier writes nor the new cursor behind; replaying the batch then resumes
// cleanly with no duplicates.
func TestApplySyncBatchRollsBackAndResumes(t *testing.T) {
	s := syncTestStore(t)
	batch := SyncBatch{
		Metas:     []SyncMeta{meta("m1", "INBOX"), meta("m2", "NO_SUCH_LABEL")}, // FK failure on m2
		HistoryID: "200",
	}
	if err := s.ApplySyncBatch("a1", batch); err == nil {
		t.Fatal("expected failure")
	}
	if s.HasMessage("a1", "m1") {
		t.Fatal("partial batch visible after rollback")
	}
	if acc, _ := s.GetAccount("a1"); acc.HistoryID != "100" {
		t.Fatalf("cursor moved on failed batch: %q", acc.HistoryID)
	}

	batch.Metas[1] = meta("m2", "INBOX")
	for i := 0; i < 2; i++ { // second run = replay after a crash before the ack
		if err := s.ApplySyncBatch("a1", batch); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE account_id = 'a1'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("messages = %d, %v", n, err)
	}
	if acc, _ := s.GetAccount("a1"); acc.HistoryID != "200" {
		t.Fatalf("cursor = %q", acc.HistoryID)
	}
}

func TestApplySyncBatchEditsAndDeletes(t *testing.T) {
	s := syncTestStore(t)
	if err := s.UpsertLabel(Label{ID: "STARRED", AccountID: "a1", Name: "STARRED", LabelType: "system"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySyncBatch("a1", SyncBatch{Metas: []SyncMeta{meta("m1", "INBOX"), meta("m2", "INBOX")}}); err != nil {
		t.Fatal(err)
	}
	err := s.ApplySyncBatch("a1", SyncBatch{
		Edits:     []SyncLabelEdit{{MessageID: "m1", Add: []string{"STARRED"}, Remove: []string{"INBOX"}}, {MessageID: "gone", Add: []string{"STARRED"}}},
		DeleteIDs: []string{"m2", "unknown"},
		HistoryID: "300",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetMessageLabelIDs("a1", "m1")
	if !slices.Contains(got, "STARRED") || slices.Contains(got, "INBOX") {
		t.Fatalf("labels = %v", got)
	}
	if s.HasMessage("a1", "m2") {
		t.Fatal("m2 not deleted")
	}
	if acc, _ := s.GetAccount("a1"); acc.HistoryID != "300" {
		t.Fatalf("cursor = %q", acc.HistoryID)
	}
}

func TestUpsertMessageBodyWithAttachments(t *testing.T) {
	s := syncTestStore(t)
	m := Message{ID: "m1", ThreadID: "t1", AccountID: "a1", Body: "hi", CacheState: CacheBodyCached}
	if err := s.UpsertMessageBody(m, []Attachment{{Name: "a.txt", AttachmentID: "x"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetMessage("a1", "m1")
	atts, _ := s.GetAttachmentsMeta("a1", "m1")
	if got.Body != "hi" || !got.HasAttachments || len(atts) != 1 {
		t.Fatalf("body=%q has=%v atts=%d", got.Body, got.HasAttachments, len(atts))
	}
}

package mail

import (
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

func TestLocalDraftSaveAndDiscard(t *testing.T) {
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(storage.Account{ID: "a", Email: "a@x.com", TokenJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	msg := OutboundMessage{From: "a@x.com", To: "b@x.com", Subject: "hi", Body: "v1"}
	for _, body := range []string{"v1", "v2"} { // second save updates the same row and op
		msg.Body = body
		if err := SaveLocalDraft(s, "local-1", "a", msg); err != nil {
			t.Fatal(err)
		}
	}
	if ds, _ := s.ListDrafts("a"); len(ds) != 1 {
		t.Fatalf("want 1 draft, got %d", len(ds))
	}
	if ops, _ := s.ListPendingOps(); len(ops) != 1 || ops[0].OpType != storage.OpSyncDraft {
		t.Fatalf("ops: %+v", ops)
	}
	// Never synced: discard just removes it.
	if err := DiscardLocalDraft(s, "a", "local-1"); err != nil {
		t.Fatal(err)
	}
	if ds, _ := s.ListDrafts("a"); len(ds) != 0 {
		t.Fatal("draft not removed")
	}
	// Synced: discard keeps the row and queues a Gmail delete.
	SaveLocalDraft(s, "local-2", "a", msg)   //nolint:errcheck
	s.SetDraftRemoteID("a", "local-2", "r1") //nolint:errcheck
	if err := DiscardLocalDraft(s, "a", "local-2"); err != nil {
		t.Fatal(err)
	}
	ops, _ := s.ListPendingOps()
	if ops[len(ops)-1].OpType != storage.OpDeleteDraft {
		t.Fatalf("ops: %+v", ops)
	}
	if d, err := s.GetDraft("a", "local-2"); err != nil || d.RemoteID != "r1" {
		t.Fatalf("%+v %v", d, err)
	}
}

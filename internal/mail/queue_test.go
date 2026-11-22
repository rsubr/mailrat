package mail

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

func queueStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, acc := range []string{"a1", "a2"} {
		if err := s.UpsertAccount(storage.Account{ID: acc, Email: acc + "@x.com", TokenJSON: "{}"}); err != nil {
			t.Fatal(err)
		}
		for _, l := range []string{"INBOX", "UNREAD", "STARRED", "TRASH", "SPAM"} {
			if err := s.UpsertLabel(storage.Label{ID: l, AccountID: acc, Name: l, LabelType: "system"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Thread t1 (two messages) and t2 in account a1.
	for _, m := range []struct{ id, thread string }{{"m1", "t1"}, {"m2", "t1"}, {"m3", "t2"}} {
		if err := s.UpsertMessage(storage.Message{ID: m.id, ThreadID: m.thread, AccountID: "a1", IsUnread: true, CacheState: storage.CacheMetadataOnly}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetMessageLabels("a1", m.id, []string{"INBOX", "UNREAD"}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func labelsOf(t *testing.T, s *storage.Store, id string) []string {
	t.Helper()
	ids, err := s.GetMessageLabelIDs("a1", id)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)
	return ids
}

func TestEnqueueIsOptimisticAndNeverCallsRemote(t *testing.T) {
	s := queueStore(t)
	called := false
	q := NewQueue(s, func(context.Context, storage.PendingOp) error { called = true; return nil }, nil, nil)

	if err := q.Enqueue(storage.OpArchive, "a1", "t1"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("Enqueue must not touch the network")
	}
	if got := labelsOf(t, s, "m1"); !slices.Equal(got, []string{"UNREAD"}) {
		t.Fatalf("m1 labels = %v", got)
	}
	if got := labelsOf(t, s, "m3"); !slices.Equal(got, []string{"INBOX", "UNREAD"}) {
		t.Fatalf("other thread must be untouched, got %v", got)
	}
	if ops, _ := s.ListPendingOps(); len(ops) != 1 {
		t.Fatalf("want 1 pending op, got %d", len(ops))
	}
}

func TestMarkReadUpdatesUnreadFlag(t *testing.T) {
	s := queueStore(t)
	q := NewQueue(s, nil, nil, nil)
	if err := q.Enqueue(storage.OpMarkRead, "a1", "m1"); err != nil {
		t.Fatal(err)
	}
	m, _ := s.GetMessage("a1", "m1")
	if m.IsUnread {
		t.Fatal("m1 should be read")
	}
	if m2, _ := s.GetMessage("a1", "m2"); !m2.IsUnread {
		t.Fatal("m2 must stay unread: mark_read targets one message")
	}
}

func TestDrainAppliesInOrderAndClears(t *testing.T) {
	s := queueStore(t)
	var order []string
	q := NewQueue(s, func(_ context.Context, op storage.PendingOp) error {
		order = append(order, op.OpType+"/"+op.MessageID)
		return nil
	}, nil, nil)
	_ = q.Enqueue(storage.OpAddLabel+"STARRED", "a1", "t1")
	_ = q.Enqueue(storage.OpTrashThread, "a1", "t2")
	q.Drain(context.Background())

	want := []string{"add_label:STARRED/t1", "trash_thread/t2"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if ops, _ := s.ListPendingOps(); len(ops) != 0 {
		t.Fatalf("queue not empty: %v", ops)
	}
}

func TestRepeatedToggleKeepsLastIntentLast(t *testing.T) {
	s := queueStore(t)
	var order []string
	q := NewQueue(s, func(_ context.Context, op storage.PendingOp) error {
		order = append(order, op.OpType)
		return nil
	}, nil, nil)
	_ = q.Enqueue(storage.OpAddLabel+"STARRED", "a1", "t1")
	_ = q.Enqueue(storage.OpRemoveLabel+"STARRED", "a1", "t1")
	_ = q.Enqueue(storage.OpAddLabel+"STARRED", "a1", "t1")
	q.Drain(context.Background())

	want := []string{"remove_label:STARRED", "add_label:STARRED"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestTransientFailureRetriesAndBlocksOnlySameAccount(t *testing.T) {
	s := queueStore(t)
	fail := true
	var applied []string
	q := NewQueue(s, func(_ context.Context, op storage.PendingOp) error {
		if op.AccountID == "a1" && fail {
			return errors.New("offline")
		}
		applied = append(applied, op.AccountID+"/"+op.MessageID)
		return nil
	}, nil, nil)
	_ = q.Enqueue(storage.OpMarkRead, "a1", "m1")
	_ = q.Enqueue(storage.OpMarkRead, "a1", "m2") // must wait behind m1
	_ = q.store.AddPendingOp(storage.OpMarkRead, "a2", "x1")

	q.Drain(context.Background())
	if !slices.Equal(applied, []string{"a2/x1"}) {
		t.Fatalf("applied = %v", applied)
	}
	ops, _ := s.ListPendingOps()
	if len(ops) != 2 || ops[0].Attempts != 1 || ops[1].Attempts != 0 {
		t.Fatalf("unexpected queue after failure: %+v", ops)
	}

	fail = false
	q.Drain(context.Background())
	if !slices.Equal(applied, []string{"a2/x1", "a1/m1", "a1/m2"}) {
		t.Fatalf("applied = %v", applied)
	}
	if ops, _ := s.ListPendingOps(); len(ops) != 0 {
		t.Fatalf("queue not empty: %v", ops)
	}
}

func TestPermanentFailureDropsAndNotifies(t *testing.T) {
	s := queueStore(t)
	bad := errors.New("404 gone")
	var dropped []storage.PendingOp
	q := NewQueue(s,
		func(context.Context, storage.PendingOp) error { return bad },
		func(err error) bool { return errors.Is(err, bad) },
		func(op storage.PendingOp, _ error) { dropped = append(dropped, op) })
	_ = q.Enqueue(storage.OpTrashThread, "a1", "t1")
	q.Drain(context.Background())

	if len(dropped) != 1 || dropped[0].MessageID != "t1" {
		t.Fatalf("dropped = %+v", dropped)
	}
	if ops, _ := s.ListPendingOps(); len(ops) != 0 {
		t.Fatal("permanently failed op must leave the queue")
	}
}

func TestOpDroppedAfterMaxAttempts(t *testing.T) {
	s := queueStore(t)
	drops := 0
	q := NewQueue(s,
		func(context.Context, storage.PendingOp) error { return errors.New("offline") },
		nil,
		func(storage.PendingOp, error) { drops++ })
	_ = q.Enqueue(storage.OpMarkUnread, "a1", "m1")
	for i := 0; i < MaxOpAttempts; i++ {
		q.Drain(context.Background())
	}
	if drops != 1 {
		t.Fatalf("drops = %d, want 1", drops)
	}
	if ops, _ := s.ListPendingOps(); len(ops) != 0 {
		t.Fatal("op should have been abandoned")
	}
}

func TestPendingOpsSurviveReopen(t *testing.T) {
	// The queue lives in SQLite, so ops enqueued before a crash replay next run.
	s := queueStore(t)
	q := NewQueue(s, nil, nil, nil)
	_ = q.Enqueue(storage.OpMarkSpam, "a1", "t2")
	var got []string
	q2 := NewQueue(s, func(_ context.Context, op storage.PendingOp) error { got = append(got, op.OpType); return nil }, nil, nil)
	q2.Drain(context.Background())
	if !slices.Equal(got, []string{storage.OpMarkSpam}) {
		t.Fatalf("got %v", got)
	}
}

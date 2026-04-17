package storage_test

import (
	"testing"
	"time"

	"github.com/mailrat/mailrat/internal/storage"
)

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAccountCRUD(t *testing.T) {
	s := openTestStore(t)

	a := storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`, HistoryID: ""}
	if err := s.UpsertAccount(a); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAccount("acc1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != a.Email {
		t.Fatalf("want email %q, got %q", a.Email, got.Email)
	}

	accounts, err := s.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("want 1 account, got %d", len(accounts))
	}

	if err := s.DeleteAccount("acc1"); err != nil {
		t.Fatal(err)
	}
	accounts, _ = s.ListAccounts()
	if len(accounts) != 0 {
		t.Fatal("expected 0 accounts after delete")
	}
}

func TestMessageCRUD(t *testing.T) {
	s := openTestStore(t)

	// Need account first due to FK
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`}) //nolint:errcheck

	m := storage.Message{
		ID:           "msg1",
		ThreadID:     "thr1",
		AccountID:    "acc1",
		Subject:      "Hello",
		FromAddr:     "from@example.com",
		ToAddr:       "to@example.com",
		Snippet:      "Hi there",
		InternalDate: time.Now().UnixMilli(),
		IsUnread:     true,
		CacheState:   storage.CacheMetadataOnly,
	}
	if err := s.UpsertMessage(m); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetMessage("acc1", "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "Hello" {
		t.Fatalf("want subject Hello, got %q", got.Subject)
	}

	msgs, err := s.ListMessages("acc1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
}

func TestLabelCRUD(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`}) //nolint:errcheck
	s.UpsertMessage(storage.Message{ID: "msg1", ThreadID: "thr1", AccountID: "acc1"})         //nolint:errcheck

	l := storage.Label{ID: "INBOX", AccountID: "acc1", Name: "INBOX", LabelType: "system"}
	if err := s.UpsertLabel(l); err != nil {
		t.Fatal(err)
	}

	if err := s.SetMessageLabels("acc1", "msg1", []string{"INBOX"}); err != nil {
		t.Fatal(err)
	}

	ids, err := s.GetMessageLabelIDs("acc1", "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "INBOX" {
		t.Fatalf("want [INBOX], got %v", ids)
	}
}

func TestDraftCRUD(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`}) //nolint:errcheck

	d := storage.Draft{ID: "draft1", AccountID: "acc1", Content: "hello", UpdatedAt: time.Now().UnixMilli()}
	if err := s.UpsertDraft(d); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetDraft("acc1", "draft1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "hello" {
		t.Fatalf("want content hello, got %q", got.Content)
	}

	if err := s.DeleteDraft("acc1", "draft1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetDraft("acc1", "draft1")
	if err == nil {
		t.Fatal("expected error for deleted draft")
	}
}

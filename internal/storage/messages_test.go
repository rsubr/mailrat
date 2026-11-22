package storage_test

import (
	"testing"
	"time"

	"github.com/mailrat/mailrat/internal/storage"
)

func TestUpsertMessagePreservesBodyAndAttachmentFlags(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`}) //nolint:errcheck

	msg := storage.Message{
		ID:             "msg1",
		ThreadID:       "thr1",
		AccountID:      "acc1",
		Subject:        "Hello",
		Body:           "full body",
		InternalDate:   time.Now().UnixMilli(),
		CacheState:     storage.CacheBodyCached,
		HasAttachments: true,
	}
	if err := s.UpsertMessage(msg); err != nil {
		t.Fatal(err)
	}

	msg.Body = ""
	msg.CacheState = storage.CacheMetadataOnly
	msg.HasAttachments = false
	if err := s.UpsertMessage(msg); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetMessage("acc1", "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "full body" {
		t.Fatalf("want preserved body, got %q", got.Body)
	}
	if !got.HasAttachments {
		t.Fatal("expected has_attachments to remain true")
	}
}

// Body sync stores a message (flag 0) and then its attachments; storing
// attachments must raise the flag or readers never load them.
func TestUpsertAttachmentsSetsHasAttachments(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`}) //nolint:errcheck
	msg := storage.Message{ID: "msg1", ThreadID: "thr1", AccountID: "acc1", Subject: "Hi", InternalDate: time.Now().UnixMilli(), CacheState: storage.CacheBodyCached}
	if err := s.UpsertMessage(msg); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAttachments("acc1", "msg1", []storage.Attachment{{Name: "a.pdf", MimeType: "application/pdf", Size: 10, AttachmentID: "x"}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMessage("acc1", "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasAttachments {
		t.Fatal("has_attachments not set after UpsertAttachments")
	}
}

func TestListUncachedBodiesForLabelsFiltersByLabel(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "test@example.com", TokenJSON: `{}`}) //nolint:errcheck
	s.UpsertLabel(storage.Label{ID: "INBOX", AccountID: "acc1", Name: "Inbox"})              //nolint:errcheck
	s.UpsertLabel(storage.Label{ID: "STARRED", AccountID: "acc1", Name: "Starred"})          //nolint:errcheck

	baseTime := time.Now().UnixMilli()
	for _, msg := range []storage.Message{
		{ID: "msg1", ThreadID: "thr1", AccountID: "acc1", InternalDate: baseTime + 1, CacheState: storage.CacheMetadataOnly},
		{ID: "msg2", ThreadID: "thr2", AccountID: "acc1", InternalDate: baseTime + 2, CacheState: storage.CacheMetadataOnly},
		{ID: "msg3", ThreadID: "thr3", AccountID: "acc1", InternalDate: baseTime + 3, CacheState: storage.CacheBodyCached},
	} {
		if err := s.UpsertMessage(msg); err != nil {
			t.Fatal(err)
		}
	}

	s.SetMessageLabels("acc1", "msg1", []string{"INBOX"})   //nolint:errcheck
	s.SetMessageLabels("acc1", "msg2", []string{"STARRED"}) //nolint:errcheck
	s.SetMessageLabels("acc1", "msg3", []string{"INBOX"})   //nolint:errcheck

	got, err := s.ListUncachedBodiesForLabels("acc1", []string{"INBOX"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "msg1" {
		t.Fatalf("want only msg1, got %#v", got)
	}
}

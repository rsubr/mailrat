package storage_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

// g1Seed: INBOX threads u1 (unread), s1 (starred), at1 (attachment), plain,
// and "mixed" whose second message is unread. Thread ids sort newest first by date.
func g1Seed(t *testing.T) *storage.Store {
	t.Helper()
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "a@x.test", TokenJSON: `{}`}) //nolint:errcheck
	for _, id := range []string{"INBOX", "STARRED", "SENT"} {
		s.UpsertLabel(storage.Label{ID: id, AccountID: "acc1", Name: id, LabelType: "system"}) //nolint:errcheck
	}
	add := func(id, thread string, date int64, unread, att bool, labels ...string) {
		if err := s.UpsertMessage(storage.Message{ID: id, ThreadID: thread, AccountID: "acc1", Subject: "hello " + thread, InternalDate: date, IsUnread: unread, HasAttachments: att}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetMessageLabels("acc1", id, labels); err != nil {
			t.Fatal(err)
		}
	}
	add("m1", "u1", 100, true, false, "INBOX")
	add("m2", "s1", 90, false, false, "INBOX", "STARRED")
	add("m3", "at1", 80, false, true, "INBOX")
	add("m4", "plain", 70, false, false, "INBOX")
	add("m5", "mixed", 60, false, false, "INBOX")
	add("m6", "mixed", 65, true, false, "INBOX")
	add("m7", "sentstar", 50, true, false, "SENT", "STARRED")
	return s
}

func g1Ids(ts []storage.ThreadSummary) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.ThreadID)
	}
	return out
}

func TestG1ListThreadSummariesFiltered(t *testing.T) {
	s := g1Seed(t)
	for _, c := range []struct {
		name        string
		acct, label string
		filter      storage.ThreadFilter
		want        []string
	}{
		{"none", "acc1", "INBOX", "", []string{"u1", "s1", "at1", "plain", "mixed"}},
		{"unread", "acc1", "INBOX", storage.FilterUnread, []string{"u1", "mixed"}},
		{"starred", "acc1", "INBOX", storage.FilterStarred, []string{"s1"}},
		{"attachments", "acc1", "INBOX", storage.FilterAttachments, []string{"at1"}},
		{"unified unread", "", "INBOX", storage.FilterUnread, []string{"u1", "mixed"}},
		{"label view starred", "acc1", "SENT", storage.FilterStarred, []string{"sentstar"}},
		{"label view unread", "", "SENT", storage.FilterUnread, []string{"sentstar"}},
		{"label view no match", "acc1", "SENT", storage.FilterAttachments, nil},
	} {
		got, err := s.ListThreadSummariesFiltered(c.acct, c.label, c.filter, 10, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if g := g1Ids(got); !slices.Equal(g, c.want) {
			t.Errorf("%s: got %v want %v", c.name, g, c.want)
		}
	}
	if _, err := s.ListThreadSummariesFiltered("acc1", "INBOX", "bogus", 10, 0); err == nil {
		t.Error("unknown filter should fail")
	}
}

// Paging applies after filtering: pages hold only matching threads.
func TestG1FilteredPaging(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "a@x.test", TokenJSON: `{}`})                 //nolint:errcheck
	s.UpsertLabel(storage.Label{ID: "INBOX", AccountID: "acc1", Name: "INBOX", LabelType: "system"}) //nolint:errcheck
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("m%02d", i)
		s.UpsertMessage(storage.Message{ID: id, ThreadID: "t" + id, AccountID: "acc1", InternalDate: int64(100 - i), IsUnread: i%2 == 0}) //nolint:errcheck
		s.SetMessageLabels("acc1", id, []string{"INBOX"})                                                                                 //nolint:errcheck
	}
	p1, _ := s.ListThreadSummariesFiltered("acc1", "INBOX", storage.FilterUnread, 2, 0)
	p2, _ := s.ListThreadSummariesFiltered("acc1", "INBOX", storage.FilterUnread, 2, 2)
	p3, _ := s.ListThreadSummariesFiltered("acc1", "INBOX", storage.FilterUnread, 2, 4)
	p4, _ := s.ListThreadSummariesFiltered("acc1", "INBOX", storage.FilterUnread, 2, 6)
	if len(p1) != 2 || len(p2) != 2 || len(p3) != 1 || len(p4) != 0 {
		t.Fatalf("page sizes %d %d %d %d", len(p1), len(p2), len(p3), len(p4))
	}
	if p1[0].ThreadID != "tm00" || p2[0].ThreadID != "tm04" || p3[0].ThreadID != "tm08" {
		t.Fatalf("pages %v %v %v", g1Ids(p1), g1Ids(p2), g1Ids(p3))
	}
}

func TestG1SearchFiltered(t *testing.T) {
	s := g1Seed(t)
	all, _ := s.LocalSearchThreadSummariesFiltered("hello", "", 10)
	if len(all) != 6 {
		t.Fatalf("unfiltered search: %v", g1Ids(all))
	}
	unread, err := s.LocalSearchThreadSummariesFiltered("hello", storage.FilterUnread, 10)
	if err != nil {
		t.Fatal(err)
	}
	if g := g1Ids(unread); !slices.Equal(g, []string{"u1", "mixed", "sentstar"}) {
		t.Fatalf("filtered search: %v", g)
	}
	if none, _ := s.LocalSearchThreadSummariesFiltered("zzzz", storage.FilterUnread, 10); len(none) != 0 {
		t.Fatalf("no match: %v", none)
	}
}

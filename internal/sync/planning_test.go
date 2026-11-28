package sync

import (
	"slices"
	"testing"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

func TestScopeQuery(t *testing.T) {
	cases := []struct {
		name      string
		cfg       storage.SyncConfig
		wantQuery string
		wantLimit int64
	}{
		{"everything", storage.SyncConfig{LabelID: "INBOX"}, "in:inbox", 0},
		{"days", storage.SyncConfig{LabelID: "STARRED", DaysToSync: 14}, "is:starred newer_than:14d", 0},
		{"count wins over days", storage.SyncConfig{LabelID: "INBOX", DaysToSync: 14, MaxEmails: 250}, "in:inbox", 250},
		{"user label with space", storage.SyncConfig{LabelID: "Label_1", LabelName: "My Stuff", DaysToSync: 30}, `label:"My Stuff" newer_than:30d`, 0},
		{"unknown label", storage.SyncConfig{LabelID: "Label_2"}, "", 0},
	}
	for _, c := range cases {
		q, limit := scopeQuery(c.cfg)
		if q != c.wantQuery || limit != c.wantLimit {
			t.Errorf("%s: got (%q, %d), want (%q, %d)", c.name, q, limit, c.wantQuery, c.wantLimit)
		}
	}
}

func TestBuildLabelPlansSkipsBackfilledWhenPending(t *testing.T) {
	cfgs := []storage.SyncConfig{
		{LabelID: "INBOX", Backfilled: true},
		{LabelID: "STARRED"},
	}
	if got := buildLabelPlans(cfgs, true); len(got) != 1 || got[0].config.LabelID != "STARRED" {
		t.Fatalf("pending plans = %+v", got)
	}
	if got := buildLabelPlans(cfgs, false); len(got) != 2 {
		t.Fatalf("all plans = %d", len(got))
	}
}

func TestAllMailDefaultsTo14Days(t *testing.T) {
	var cfg storage.SyncConfig
	for _, c := range defaultBackfillConfigs("a") {
		if c.LabelID == storage.AllMailLabelID {
			cfg = c
		}
	}
	q, _ := scopeQuery(cfg)
	if q != "-in:trash -in:spam newer_than:14d" {
		t.Fatalf("query = %q", q)
	}
}

func heldStore(t *testing.T, labels []string) *storage.Store {
	t.Helper()
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(storage.Account{ID: "a1", Email: "a@x.com", TokenJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"INBOX", "TRASH"} {
		if err := s.UpsertLabel(storage.Label{ID: l, AccountID: "a1", Name: l, LabelType: "system"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertMessage(storage.Message{ID: "m1", ThreadID: "t1", AccountID: "a1", CacheState: storage.CacheMetadataOnly}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMessageLabels("a1", "m1", labels); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSyncSkipsLabelsOfHeldThread(t *testing.T) {
	s := heldStore(t, []string{"TRASH"})
	held := true
	m := &Manager{store: s, held: func(a, th string) bool { return held && a == "a1" && th == "t1" }}

	if err := m.storeMessageMetadata("a1", gmail.MessageMeta{ID: "m1", ThreadID: "t1", LabelIDs: []string{"INBOX"}}); err != nil {
		t.Fatal(err)
	}
	edits, _ := m.historyEdits("a1", []gmail.HistoryEvent{{LabelsAdded: &gmail.LabelChange{MessageID: "m1", LabelIDs: []string{"INBOX"}}}}, nil)
	if err := s.ApplySyncBatch("a1", storage.SyncBatch{Edits: edits}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetMessageLabelIDs("a1", "m1"); !slices.Contains(got, "TRASH") || slices.Contains(got, "INBOX") {
		t.Fatalf("held thread labels changed: %v", got)
	}

	held = false // window closed: sync reconciles normally
	if err := m.storeMessageMetadata("a1", gmail.MessageMeta{ID: "m1", ThreadID: "t1", LabelIDs: []string{"INBOX"}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetMessageLabelIDs("a1", "m1"); !slices.Contains(got, "INBOX") || slices.Contains(got, "TRASH") {
		t.Fatalf("labels not reconciled: %v", got)
	}
}

func TestApplyDeletions(t *testing.T) {
	s := heldStore(t, []string{"INBOX"})
	if err := s.ApplySyncBatch("a1", storage.SyncBatch{DeleteIDs: []string{"unknown", "m1"}}); err != nil {
		t.Fatal(err)
	}
	if s.HasMessage("a1", "m1") {
		t.Fatal("deleted message still stored")
	}
	if got, _ := s.GetMessageLabelIDs("a1", "m1"); len(got) != 0 {
		t.Fatalf("labels not cleaned up: %v", got)
	}
}

func TestApplyDeletionsHeldThread(t *testing.T) {
	s := heldStore(t, []string{"TRASH"})
	if err := s.ApplySyncBatch("a1", storage.SyncBatch{DeleteIDs: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	if s.HasMessage("a1", "m1") {
		t.Fatal("held message not deleted")
	}
	// A pending trash/undo on the now-empty thread must not fail.
	if err := s.ModifyLabels("a1", "t1", true, nil, []string{"TRASH"}); err != nil {
		t.Fatal(err)
	}
}

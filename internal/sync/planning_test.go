package sync

import (
	"testing"

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

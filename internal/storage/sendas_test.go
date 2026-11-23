package storage_test

import (
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

func TestSendAsRoundTrip(t *testing.T) {
	s := openTestStore(t)
	s.UpsertAccount(storage.Account{ID: "acc1", Email: "a@x.com", TokenJSON: `{}`}) //nolint:errcheck

	in := []storage.SendAs{
		{Email: "z@x.com", SignatureHTML: "<b>z</b>"},
		{Email: "a@x.com", DisplayName: "A", SignatureHTML: "<i>a</i>", IsPrimary: true, IsDefault: true},
	}
	if err := s.ReplaceSendAs("acc1", in); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListSendAs("acc1")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].Email != "a@x.com" || !got[0].IsPrimary || !got[0].IsDefault || got[0].SignatureHTML != "<i>a</i>" || got[0].DisplayName != "A" {
		t.Fatalf("bad first row: %+v", got[0])
	}

	if err := s.ReplaceSendAs("acc1", in[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSendAs("acc1"); len(got) != 1 || got[0].Email != "z@x.com" {
		t.Fatalf("replace failed: %+v", got)
	}
}

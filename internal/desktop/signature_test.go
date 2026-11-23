package desktop

import (
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

func TestPickSignature(t *testing.T) {
	list := []storage.SendAs{
		{Email: "a@x.com", SignatureHTML: "A", IsPrimary: true},
		{Email: "b@x.com", SignatureHTML: "B", IsDefault: true},
		{Email: "c@x.com", SignatureHTML: "C"},
	}
	for from, want := range map[string]string{"C@x.com": "C", "": "B", "unknown@x.com": "B"} {
		if got := pickSignature(list, from); got != want {
			t.Errorf("from %q: got %q want %q", from, got, want)
		}
	}
	if got := pickSignature(list[:1], ""); got != "A" {
		t.Errorf("primary fallback: got %q", got)
	}
}

func TestDraftOutbound(t *testing.T) {
	m := MessageItem{AccountID: "a", ThreadID: "t", ToAddr: "b@x.com", CcAddr: "c@x.com", BccAddr: "d@x.com", Subject: "S", Body: "<p>hi</p>"}
	got := draftOutbound(m, "local-1", "<p@x>")
	want := OutboundMsg{AccountID: "a", To: "b@x.com", Cc: "c@x.com", Bcc: "d@x.com", Subject: "S", Body: "<p>hi</p>", InReplyTo: "<p@x>", ThreadID: "t", DraftID: "local-1"}
	if got != want {
		t.Fatalf("got %+v", got)
	}
}

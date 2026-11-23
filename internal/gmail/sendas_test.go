package gmail

import (
	"testing"

	gmailv1 "google.golang.org/api/gmail/v1"
)

func TestMapSendAs(t *testing.T) {
	got := mapSendAs([]*gmailv1.SendAs{
		{SendAsEmail: "a@x.com", DisplayName: "A", Signature: "<p>sig</p>", IsPrimary: true, IsDefault: true},
		{SendAsEmail: "b@x.com"},
	})
	want := SendAs{Email: "a@x.com", DisplayName: "A", SignatureHTML: "<p>sig</p>", IsPrimary: true, IsDefault: true}
	if len(got) != 2 || got[0] != want || got[1].Email != "b@x.com" || got[1].IsDefault {
		t.Fatalf("got %+v", got)
	}
}

func TestMatchDraftAndHeader(t *testing.T) {
	ds := []*gmailv1.Draft{{Id: "d1", Message: &gmailv1.Message{Id: "m1"}}, {Id: "d2", Message: &gmailv1.Message{Id: "m2"}}}
	if matchDraft(ds, "m2") != "d2" || matchDraft(ds, "zz") != "" {
		t.Fatal("matchDraft")
	}
	m := &gmailv1.Message{Payload: &gmailv1.MessagePart{Headers: []*gmailv1.MessagePartHeader{{Name: "In-Reply-To", Value: "<x@y>"}}}}
	if headerValue(m, "In-Reply-To") != "<x@y>" || headerValue(&gmailv1.Message{}, "In-Reply-To") != "" {
		t.Fatal("headerValue")
	}
}

package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mailrat/mailrat/internal/gmail"
)

type redirect struct{ to *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.to.Scheme, r.to.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestSyncStoresReplyHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","labelIds":["INBOX"],"payload":{"headers":[
			{"name":"From","value":"Ann <ann@x.com>"},{"name":"Message-ID","value":"<p@x>"},
			{"name":"Reply-To","value":"Lists <list@x.com>"},
			{"name":"References","value":"<a@x>\r\n <b@x>"},{"name":"In-Reply-To","value":"<b@x>"}]}}`))
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	c, err := gmail.NewClient(context.Background(), &http.Client{Transport: redirect{u}}, "a1")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := c.GetMessageMeta(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	s := heldStore(t, nil)
	m := &Manager{store: s}
	if err := m.storeMessageMetadata("a1", meta); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMessage("a1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HeadersKnown || got.ReplyTo != "Lists <list@x.com>" || got.References != "<a@x> <b@x>" || got.InReplyTo != "<b@x>" || got.MessageID != "<p@x>" {
		t.Fatalf("%+v", got)
	}
}

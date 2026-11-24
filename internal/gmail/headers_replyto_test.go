package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func msgWith(h ...string) *gmailv1.Message {
	m := &gmailv1.Message{Id: "m1", Payload: &gmailv1.MessagePart{}}
	for i := 0; i+1 < len(h); i += 2 {
		m.Payload.Headers = append(m.Payload.Headers, &gmailv1.MessagePartHeader{Name: h[i], Value: h[i+1]})
	}
	return m
}

func TestParseReplyHeaders(t *testing.T) {
	tests := []struct {
		name               string
		hdrs               []string
		replyTo, refs, irt string
	}{
		{"absent", nil, "", "", ""},
		{"plain", []string{"Reply-To", "Ann <ann@x.com>, bob@x.com", "In-Reply-To", "<a@x>", "References", "<r1@x> <a@x>"},
			"Ann <ann@x.com>, bob@x.com", "<r1@x> <a@x>", "<a@x>"},
		{"folded references", []string{"References", "<r1@x>\r\n\t<r2@x>\r\n <r3@x>"}, "", "<r1@x> <r2@x> <r3@x>", ""},
		{"case-insensitive names", []string{"REPLY-TO", "a@x.com", "references", "<r@x>", "in-reply-to", "<i@x>"}, "a@x.com", "<r@x>", "<i@x>"},
		{"duplicates keep first", []string{"Reply-To", "first@x.com", "Reply-To", "second@x.com", "References", "<1@x>", "References", "<2@x>"}, "first@x.com", "<1@x>", ""},
		{"encoded word kept raw", []string{"Reply-To", "=?UTF-8?B?w4lsw6g=?= <e@x.com>"}, "=?UTF-8?B?w4lsw6g=?= <e@x.com>", "", ""},
		{"malformed kept raw", []string{"Reply-To", "<<not an address", "References", "garbage"}, "<<not an address", "garbage", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := parseMessageMeta(msgWith(tc.hdrs...))
			if m.ReplyTo != tc.replyTo || m.References != tc.refs || m.InReplyTo != tc.irt {
				t.Fatalf("got %q %q %q", m.ReplyTo, m.References, m.InReplyTo)
			}
		})
	}
}

func TestGetMessageMetaRequestsReplyHeaders(t *testing.T) {
	var query string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","payload":{"headers":[{"name":"Reply-To","value":"r@x.com"},{"name":"References","value":"<a@x>\r\n <b@x>"},{"name":"In-Reply-To","value":"<b@x>"}]}}`))
	}))
	defer ts.Close()
	svc, err := gmailv1.NewService(context.Background(), option.WithEndpoint(ts.URL), option.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := (&Client{svc: svc}).GetMessageMeta(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"metadataHeaders=Reply-To", "metadataHeaders=References", "metadataHeaders=In-Reply-To", "format=metadata"} {
		if !strings.Contains(query, h) {
			t.Errorf("query %q lacks %s", query, h)
		}
	}
	if meta.ReplyTo != "r@x.com" || meta.References != "<a@x> <b@x>" || meta.InReplyTo != "<b@x>" {
		t.Fatalf("%+v", meta)
	}
}

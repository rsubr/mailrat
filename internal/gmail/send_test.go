package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type captured struct {
	path, query, ctype string
	body               []byte
}

func testClient(t *testing.T, reqs *[]captured) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, captured{r.URL.Path, r.URL.RawQuery, r.Header.Get("Content-Type"), b})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1"}`))
	}))
	t.Cleanup(ts.Close)
	svc, err := gmailv1.NewService(context.Background(), option.WithEndpoint(ts.URL), option.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return &Client{svc: svc}
}

func TestSendSmallUsesJSONRaw(t *testing.T) {
	var reqs []captured
	c := testClient(t, &reqs)
	raw := "Subject: hi\r\n\r\nbody"
	if _, err := c.SendMessage(context.Background(), EncodeRaw(raw), "t1"); err != nil {
		t.Fatal(err)
	}
	var m gmailv1.Message
	if err := json.Unmarshal(reqs[0].body, &m); err != nil {
		t.Fatal(err)
	}
	got, _ := base64.URLEncoding.DecodeString(m.Raw)
	if len(reqs) != 1 || strings.Contains(reqs[0].path, "upload") || string(got) != raw || m.ThreadId != "t1" {
		t.Fatalf("%+v %s", reqs, got)
	}
}

func TestLargeMessagesUseMediaUpload(t *testing.T) {
	raw := "Subject: big\r\n\r\n" + strings.Repeat("A", mediaThreshold+10)
	for name, call := range map[string]func(*Client) error{
		"send":        func(c *Client) error { _, err := c.SendMessage(context.Background(), EncodeRaw(raw), "t9"); return err },
		"createDraft": func(c *Client) error { _, err := c.CreateDraft(context.Background(), EncodeRaw(raw)); return err },
		"updateDraft": func(c *Client) error { return c.UpdateDraft(context.Background(), "d1", EncodeRaw(raw)) },
	} {
		var reqs []captured
		if err := call(testClient(t, &reqs)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		r := reqs[0]
		if !strings.Contains(r.path, "/upload/") || !strings.Contains(r.query, "uploadType=") {
			t.Fatalf("%s: not an upload: %s?%s", name, r.path, r.query)
		}
		typ, params, err := mime.ParseMediaType(r.ctype)
		if err != nil || typ != "multipart/related" {
			t.Fatalf("%s: content type %q", name, r.ctype)
		}
		mr := multipart.NewReader(strings.NewReader(string(r.body)), params["boundary"])
		meta, _ := mr.NextPart()
		mb, _ := io.ReadAll(meta)
		if strings.Contains(string(mb), `"raw"`) {
			t.Errorf("%s: raw left in metadata", name)
		}
		if name == "send" && !strings.Contains(string(mb), "t9") {
			t.Errorf("send: threadId lost: %s", mb)
		}
		media, _ := mr.NextPart()
		got, _ := io.ReadAll(media)
		if media.Header.Get("Content-Type") != "message/rfc822" || string(got) != raw {
			t.Errorf("%s: media part %q, %d bytes", name, media.Header.Get("Content-Type"), len(got))
		}
	}
}

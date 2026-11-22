package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// latencyGmail fakes the Gmail REST API with a fixed per-request latency.
type latencyGmail struct {
	delay                 time.Duration
	messages, attachments atomic.Int32
}

func (g *latencyGmail) RoundTrip(r *http.Request) (*http.Response, error) {
	time.Sleep(g.delay)
	var body string
	if strings.Contains(r.URL.Path, "/attachments/") {
		g.attachments.Add(1)
		body = `{"data":"` + base64.URLEncoding.EncodeToString([]byte("bytes")) + `"}`
	} else {
		g.messages.Add(1)
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		body = fmt.Sprintf(`{"id":%q,"payload":{"mimeType":"multipart/mixed","parts":[
			{"mimeType":"text/plain","body":{"data":%q}},
			{"filename":"a.pdf","mimeType":"application/pdf","body":{"attachmentId":"x1","size":5}},
			{"filename":"b.pdf","mimeType":"application/pdf","body":{"attachmentId":"x2","size":5}}]}}`,
			id, base64.URLEncoding.EncodeToString([]byte("hello")))
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

// TestOpenLargeThread mimics the UI opening a 50-message thread whose bodies
// are not cached yet: bodies are fetched 6 at a time. Attachment bytes must
// not be downloaded on open (they load on demand when saved).
func TestOpenLargeThread(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(storage.Account{ID: "a", Email: "a@x.com", TokenJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	const n = 50
	for i := 0; i < n; i++ {
		m := storage.Message{ID: fmt.Sprintf("m%d", i), ThreadID: "t", AccountID: "a", InternalDate: int64(i), CacheState: storage.CacheMetadataOnly}
		if err := s.UpsertMessageMeta(m, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	fake := &latencyGmail{delay: 50 * time.Millisecond}
	client, err := gmail.NewClient(context.Background(), &http.Client{Transport: fake}, "a")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewInboxService(s, client)

	start := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if _, err := svc.GetMessage(context.Background(), "a", fmt.Sprintf("m%d", i)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	t.Logf("opened %d-message thread in %v (%d message fetches, %d attachment downloads)",
		n, time.Since(start), fake.messages.Load(), fake.attachments.Load())
	if got := fake.attachments.Load(); got != 0 {
		t.Errorf("attachment downloads on open = %d, want 0", got)
	}
	atts, _ := s.GetAttachments("a", "m0")
	if len(atts) != 2 {
		t.Errorf("attachment metadata not stored: %d", len(atts))
	}
}

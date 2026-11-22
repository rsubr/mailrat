package mail

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mailrat/mailrat/internal/storage"
)

// slowSource fakes a slow attachment transport.
type slowSource struct {
	delay   time.Duration
	calls   atomic.Int32
	active  atomic.Int32
	maxSeen atomic.Int32
}

func (s *slowSource) GetAttachmentData(ctx context.Context, _, _ string) ([]byte, error) {
	s.calls.Add(1)
	n := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		m := s.maxSeen.Load()
		if n <= m || s.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-time.After(s.delay):
		return []byte("data"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func attachmentFixture(t *testing.T, msgs, perMsg int) (*storage.Store, *AttachmentCache) {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertAccount(storage.Account{ID: "a", Email: "a@x.com", TokenJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertLabel(storage.Label{ID: "INBOX", AccountID: "a", Name: "INBOX"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < msgs; i++ {
		id := fmt.Sprintf("m%d", i)
		m := storage.Message{ID: id, ThreadID: "t", AccountID: "a", InternalDate: int64(i), CacheState: storage.CacheBodyCached, HasAttachments: true}
		if err := s.UpsertMessageMeta(m, []string{"INBOX"}, true); err != nil {
			t.Fatal(err)
		}
		var atts []storage.Attachment
		for j := 0; j < perMsg; j++ {
			atts = append(atts, storage.Attachment{Name: "f", MimeType: "application/pdf", AttachmentID: fmt.Sprintf("x%d", j)})
		}
		if err := s.UpsertAttachments("a", id, atts); err != nil {
			t.Fatal(err)
		}
	}
	return s, NewAttachmentCache(s)
}

func countCached(t *testing.T, s *storage.Store, msgs int) int {
	n := 0
	for i := 0; i < msgs; i++ {
		atts, _ := s.GetAttachments("a", fmt.Sprintf("m%d", i))
		for _, a := range atts {
			if a.Data != nil {
				n++
			}
		}
	}
	return n
}

func TestPrefetchCachesAllAndBoundsConcurrency(t *testing.T) {
	s, c := attachmentFixture(t, 5, 2)
	src := &slowSource{delay: 30 * time.Millisecond}

	// Caller returns at once when run in a goroutine; Prefetch itself is async to the UI.
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.Prefetch(context.Background(), src, "a", "t", nil) }()
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("starting prefetch blocked")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := countCached(t, s, 5); got != 10 {
		t.Errorf("cached %d, want 10", got)
	}
	if m := src.maxSeen.Load(); m > prefetchWorkers {
		t.Errorf("concurrency %d > %d", m, prefetchWorkers)
	}

	// Second run skips everything already cached.
	before := src.calls.Load()
	if err := c.Prefetch(context.Background(), src, "a", "t", nil); err != nil {
		t.Fatal(err)
	}
	if src.calls.Load() != before {
		t.Error("prefetch re-downloaded cached attachments")
	}
}

func TestPrefetchCancel(t *testing.T) {
	s, c := attachmentFixture(t, 10, 2)
	src := &slowSource{delay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Prefetch(ctx, src, "a", "t", nil) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("want cancellation error")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("prefetch did not stop on cancel")
	}
	if got := src.calls.Load(); got > prefetchWorkers {
		t.Errorf("started %d downloads after cancel, want <= %d", got, prefetchWorkers)
	}
	if got := countCached(t, s, 10); got != 0 {
		t.Errorf("cached %d after cancel", got)
	}
}

func TestOnDemandDedupesWithPrefetch(t *testing.T) {
	s, c := attachmentFixture(t, 1, 1)
	src := &slowSource{delay: 100 * time.Millisecond}
	go c.Prefetch(context.Background(), src, "a", "t", nil)
	time.Sleep(30 * time.Millisecond) // prefetch download now in flight

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, _, err := c.Get(context.Background(), src, "a", "m0", "x0")
			if err != nil || string(data) != "data" {
				t.Errorf("get: %q %v", data, err)
			}
		}()
	}
	wg.Wait()
	if got := src.calls.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1", got)
	}
	if countCached(t, s, 1) != 1 {
		t.Error("not cached")
	}
}

func TestOnDemandSurvivesCancelledPrefetch(t *testing.T) {
	_, c := attachmentFixture(t, 1, 1)
	src := &slowSource{delay: 100 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	go c.Prefetch(ctx, src, "a", "t", nil)
	time.Sleep(30 * time.Millisecond)
	res := make(chan error, 1)
	go func() { _, _, err := c.Get(context.Background(), src, "a", "m0", "x0"); res <- err }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-res; err != nil {
		t.Errorf("on-demand failed after prefetch cancel: %v", err)
	}
}

func TestPrefetchSyncedCachesAllWithoutBlockingStart(t *testing.T) {
	s, c := attachmentFixture(t, 6, 2)
	src := &slowSource{delay: 30 * time.Millisecond}
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.PrefetchSynced(context.Background(), src, "a", 100, nil) }()
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("starting sync prefetch blocked")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := countCached(t, s, 6); got != 12 {
		t.Errorf("cached %d, want 12", got)
	}
	if got := src.maxSeen.Load(); got > syncPrefetchWorkers {
		t.Errorf("concurrency %d > %d", got, syncPrefetchWorkers)
	}
	// A second cycle finds nothing missing and downloads nothing.
	before := src.calls.Load()
	if err := c.PrefetchSynced(context.Background(), src, "a", 100, nil); err != nil {
		t.Fatal(err)
	}
	if src.calls.Load() != before {
		t.Error("re-downloaded cached attachments")
	}
}

func TestPrefetchSyncedCancel(t *testing.T) {
	s, c := attachmentFixture(t, 10, 2)
	src := &slowSource{delay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.PrefetchSynced(ctx, src, "a", 100, nil) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("want cancellation error")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not stop on cancel")
	}
	if got := src.calls.Load(); got > syncPrefetchWorkers {
		t.Errorf("started %d downloads, want <= %d", got, syncPrefetchWorkers)
	}
	if countCached(t, s, 10) != 0 {
		t.Error("cached after cancel")
	}
}

func TestPrefetchSyncedSkipsCachedAndDedupesOnDemand(t *testing.T) {
	s, c := attachmentFixture(t, 3, 1)
	if err := s.UpdateAttachmentData("a", "m0", "x0", []byte("old")); err != nil {
		t.Fatal(err)
	}
	src := &slowSource{delay: 100 * time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- c.PrefetchSynced(context.Background(), src, "a", 100, nil) }()
	time.Sleep(30 * time.Millisecond) // m1 and m2 downloads in flight
	if _, _, err := c.Get(context.Background(), src, "a", "m1", "x0"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := src.calls.Load(); got != 2 {
		t.Errorf("downloads = %d, want 2 (m0 cached, m1 shared with on-demand)", got)
	}
	if countCached(t, s, 3) != 3 {
		t.Error("not all cached")
	}
}

func TestSyncAndThreadPrefetchShareDownloads(t *testing.T) {
	s, c := attachmentFixture(t, 4, 1)
	src := &slowSource{delay: 50 * time.Millisecond}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); c.PrefetchSynced(context.Background(), src, "a", 100, nil) }()
	go func() { defer wg.Done(); c.Prefetch(context.Background(), src, "a", "t", nil) }()
	wg.Wait()
	if got := src.calls.Load(); got < 4 || got > 5 {
		// A race between listing and write-back can at worst repeat none:
		// in-flight dedupe makes it exactly 4 unless one finished between checks.
		t.Errorf("downloads = %d, want 4", got)
	}
	if countCached(t, s, 4) != 4 {
		t.Error("not all cached")
	}
}

// keyedSource records when each attachment download started.
type keyedSource struct {
	delay  time.Duration
	mu     sync.Mutex
	starts map[string]time.Time
	calls  map[string]int
}

func newKeyedSource(d time.Duration) *keyedSource {
	return &keyedSource{delay: d, starts: map[string]time.Time{}, calls: map[string]int{}}
}

func (s *keyedSource) GetAttachmentData(ctx context.Context, msgID, attID string) ([]byte, error) {
	s.mu.Lock()
	k := msgID + "/" + attID
	if _, ok := s.starts[k]; !ok {
		s.starts[k] = time.Now()
	}
	s.calls[k]++
	s.mu.Unlock()
	select {
	case <-time.After(s.delay):
		return []byte("data"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *keyedSource) startOf(k string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.starts[k]
	return t, ok
}

func TestPrefetchSyncedCoversAnyLabel(t *testing.T) {
	s, c := attachmentFixture(t, 1, 1)
	if err := s.UpsertLabel(storage.Label{ID: "Label_9", AccountID: "a", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	m := storage.Message{ID: "z", ThreadID: "tz", AccountID: "a", CacheState: storage.CacheBodyCached, HasAttachments: true}
	if err := s.UpsertMessageMeta(m, []string{"Label_9"}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAttachments("a", "z", []storage.Attachment{{Name: "f", AttachmentID: "x0"}}); err != nil {
		t.Fatal(err)
	}
	src := &slowSource{delay: time.Millisecond}
	if err := c.PrefetchSynced(context.Background(), src, "a", 100, nil); err != nil {
		t.Fatal(err)
	}
	atts, _ := s.GetAttachments("a", "z")
	if len(atts) != 1 || atts[0].Data == nil {
		t.Error("attachment on a non-INBOX label not cached by sync prefetch")
	}
}

func TestThreadPrefetchPreemptsSyncPrefetch(t *testing.T) {
	// m0..m7 for sync backlog (thread "t"); a separate thread "t2" opened mid-sync.
	s, c := attachmentFixture(t, 8, 1)
	if err := s.UpsertLabel(storage.Label{ID: "Label_9", AccountID: "a", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"n0", "n1"} {
		m := storage.Message{ID: id, ThreadID: "t2", AccountID: "a", CacheState: storage.CacheBodyCached, HasAttachments: true}
		if err := s.UpsertMessageMeta(m, []string{"Label_9"}, true); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertAttachments("a", id, []storage.Attachment{{Name: "f", AttachmentID: "x0"}}); err != nil {
			t.Fatal(err)
		}
	}
	// Backlog of 10 attachments / 2 workers * 100ms = ~500ms if undisturbed.
	src := newKeyedSource(100 * time.Millisecond)
	syncDone := make(chan error, 1)
	go func() { syncDone <- c.PrefetchSynced(context.Background(), src, "a", 100, nil) }()
	time.Sleep(50 * time.Millisecond)

	// Pick a thread attachment sync has not started yet.
	opened := time.Now()
	threadDone := make(chan error, 1)
	go func() { threadDone <- c.Prefetch(context.Background(), src, "a", "t2", nil) }()
	var started [2]time.Time
	for i, k := range []string{"n0/x0", "n1/x0"} {
		for {
			if ts, ok := src.startOf(k); ok {
				started[i] = ts
				break
			}
			if time.Since(opened) > time.Second {
				t.Fatalf("%s never started", k)
			}
			time.Sleep(time.Millisecond)
		}
	}
	for i, ts := range started {
		// Sync alone would reach these only after most of the ~500ms backlog.
		if d := ts.Sub(opened); d > 60*time.Millisecond {
			t.Errorf("thread download %d started after %v, want immediate", i, d)
		}
	}
	if err := <-threadDone; err != nil {
		t.Fatal(err)
	}
	threadEnd := time.Now()

	// Sync starts no new download while the thread prefetch runs.
	if err := <-syncDone; err != nil {
		t.Fatal(err)
	}
	src.mu.Lock()
	for k, ts := range src.starts {
		if k[0] == 'm' && ts.After(opened) && ts.Before(threadEnd.Add(-5*time.Millisecond)) {
			t.Errorf("sync started %s at +%v while thread prefetch active", k, ts.Sub(opened))
		}
	}
	src.mu.Unlock()
	if got := countCached(t, s, 8); got != 8 {
		t.Errorf("sync did not resume and finish: cached %d/8", got)
	}
}

func TestSyncGateReleasedAfterThreadCancel(t *testing.T) {
	_, c := attachmentFixture(t, 6, 1)
	src := newKeyedSource(40 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	threadDone := make(chan error, 1)
	// Thread prefetch of an empty/other thread still holds the gate while it runs.
	go func() { threadDone <- c.Prefetch(ctx, src, "a", "t", nil) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-threadDone

	c.mu.Lock()
	active, idle := c.threadActive, c.idle
	c.mu.Unlock()
	if active != 0 || idle != nil {
		t.Fatalf("gate held after cancel: active=%d", active)
	}
	done := make(chan error, 1)
	go func() { done <- c.PrefetchSynced(context.Background(), src, "a", 100, nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sync prefetch starved after thread cancel")
	}
}

func TestThreadRequestJoinsInflightSyncDownload(t *testing.T) {
	s, c := attachmentFixture(t, 1, 1)
	src := newKeyedSource(150 * time.Millisecond)
	syncDone := make(chan error, 1)
	go func() { syncDone <- c.PrefetchSynced(context.Background(), src, "a", 100, nil) }()
	time.Sleep(40 * time.Millisecond) // sync download in flight
	begin := time.Now()
	if err := c.Prefetch(context.Background(), src, "a", "t", nil); err != nil {
		t.Fatal(err)
	}
	if err := <-syncDone; err != nil {
		t.Fatal(err)
	}
	if n := src.calls["m0/x0"]; n != 1 {
		t.Errorf("downloads = %d, want 1 shared", n)
	}
	if time.Since(begin) > 140*time.Millisecond {
		t.Error("thread prefetch waited instead of joining")
	}
	if countCached(t, s, 1) != 1 {
		t.Error("not cached")
	}
}

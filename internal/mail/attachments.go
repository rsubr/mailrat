package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mailrat/mailrat/internal/storage"
)

// prefetchWorkers bounds concurrent background attachment downloads. It is a
// pool of its own so prefetching never competes with message fetches.
const prefetchWorkers = 3

// syncPrefetchWorkers bounds the separate pool that caches attachments of
// synced mail in the background.
const syncPrefetchWorkers = 2

// AttachmentSource downloads attachment bytes (implemented by *gmail.Client).
type AttachmentSource interface {
	GetAttachmentData(ctx context.Context, messageID, attachmentID string) ([]byte, error)
}

type attachmentCall struct {
	done chan struct{}
	data []byte
	err  error
}

// AttachmentCache serves attachment bytes from the store, downloading and
// caching them on a miss. Concurrent requests for one attachment share a
// single download.
type AttachmentCache struct {
	store *storage.Store

	mu       sync.Mutex
	inflight map[string]*attachmentCall
	// threadActive counts running thread-open Prefetch calls; idle is non-nil
	// (and open) while it is > 0 and is closed when it drops back to zero.
	threadActive int
	idle         chan struct{}
}

func NewAttachmentCache(store *storage.Store) *AttachmentCache {
	return &AttachmentCache{store: store, inflight: map[string]*attachmentCall{}}
}

// Get returns the attachment bytes and MIME type, from cache when present.
func (c *AttachmentCache) Get(ctx context.Context, src AttachmentSource, accountID, messageID, attachmentID string) ([]byte, string, error) {
	var row *storage.Attachment
	if atts, err := c.store.GetAttachments(accountID, messageID); err == nil {
		for i := range atts {
			if atts[i].AttachmentID == attachmentID {
				if atts[i].Data != nil {
					return atts[i].Data, atts[i].MimeType, nil
				}
				row = &atts[i]
				break
			}
		}
	}
	mimeType := "application/octet-stream"
	if row != nil {
		mimeType = row.MimeType
	}

	// Inline attachments (synthetic "name:" ID) have no Gmail API path.
	if strings.HasPrefix(attachmentID, "name:") {
		return nil, "", fmt.Errorf("attachment %q not yet downloaded; sync to cache it", strings.TrimPrefix(attachmentID, "name:"))
	}

	key := accountID + "\x00" + messageID + "\x00" + attachmentID
	for {
		c.mu.Lock()
		call, ok := c.inflight[key]
		if !ok {
			call = &attachmentCall{done: make(chan struct{})}
			c.inflight[key] = call
		}
		c.mu.Unlock()

		if !ok {
			call.data, call.err = src.GetAttachmentData(ctx, messageID, attachmentID)
			if call.err == nil && row != nil {
				// Write back so future requests are served from SQLite.
				_ = c.store.UpdateAttachmentData(accountID, messageID, attachmentID, call.data)
			}
			c.mu.Lock()
			delete(c.inflight, key)
			c.mu.Unlock()
			close(call.done)
			return call.data, mimeType, call.err
		}

		select {
		case <-call.done:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		// The shared download was cancelled by its owner, not by us: retry.
		if errors.Is(call.err, context.Canceled) && ctx.Err() == nil {
			continue
		}
		return call.data, mimeType, call.err
	}
}

// Prefetch downloads and caches every uncached attachment of a thread using a
// small worker pool. It blocks until done or ctx is cancelled; callers run it
// in a goroutine. Failures are returned per attachment via onErr and are not
// retried.
func (c *AttachmentCache) Prefetch(ctx context.Context, src AttachmentSource, accountID, threadID string, onErr func(error)) error {
	c.enterThread()
	defer c.exitThread()
	msgs, err := c.store.ListMessagesByThread(accountID, threadID)
	if err != nil {
		return err
	}
	var jobs []storage.MissingAttachment
	for _, m := range msgs {
		if !m.HasAttachments {
			continue
		}
		atts, err := c.store.GetAttachments(accountID, m.ID)
		if err != nil {
			continue
		}
		for _, a := range atts {
			if a.Data == nil && a.AttachmentID != "" && !strings.HasPrefix(a.AttachmentID, "name:") {
				jobs = append(jobs, storage.MissingAttachment{MessageID: m.ID, AttachmentID: a.AttachmentID})
			}
		}
	}
	return c.download(ctx, src, accountID, jobs, prefetchWorkers, false, onErr)
}

// PrefetchSynced caches up to limit attachments still missing for any
// body-cached message of the account, on a pool of its own that yields to
// thread-open Prefetch: its workers start no new download while one is active.
// Cached attachments are skipped by the query, so repeated sync cycles only
// touch what is missing. It blocks until done or ctx is cancelled; callers run
// it in a goroutine.
func (c *AttachmentCache) PrefetchSynced(ctx context.Context, src AttachmentSource, accountID string, limit int, onErr func(error)) error {
	jobs, err := c.store.ListMissingAttachmentRows(accountID, limit)
	if err != nil {
		return err
	}
	return c.download(ctx, src, accountID, jobs, syncPrefetchWorkers, true, onErr)
}

func (c *AttachmentCache) enterThread() {
	c.mu.Lock()
	if c.threadActive == 0 {
		c.idle = make(chan struct{})
	}
	c.threadActive++
	c.mu.Unlock()
}

func (c *AttachmentCache) exitThread() {
	c.mu.Lock()
	if c.threadActive--; c.threadActive == 0 {
		close(c.idle)
		c.idle = nil
	}
	c.mu.Unlock()
}

// waitThreadIdle blocks until no thread-open prefetch is active, or ctx ends.
func (c *AttachmentCache) waitThreadIdle(ctx context.Context) error {
	for {
		c.mu.Lock()
		ch := c.idle
		c.mu.Unlock()
		if ch == nil {
			return ctx.Err()
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *AttachmentCache) download(ctx context.Context, src AttachmentSource, accountID string, jobs []storage.MissingAttachment, workers int, yield bool, onErr func(error)) error {
	ch := make(chan storage.MissingAttachment)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				// Sync work waits for thread-open prefetch; a download already
				// in flight is never interrupted, and a thread request for it
				// joins it through Get's dedupe.
				if yield && c.waitThreadIdle(ctx) != nil {
					return
				}
				if _, _, err := c.Get(ctx, src, accountID, j.MessageID, j.AttachmentID); err != nil && ctx.Err() == nil && onErr != nil {
					onErr(err)
				}
			}
		}()
	}
feed:
	for _, j := range jobs {
		select {
		case ch <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
	return ctx.Err()
}

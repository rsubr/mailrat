package sync

import (
	"context"
	"fmt"
	stdsync "sync"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

type seenMessages map[string]struct{}

func (s seenMessages) Skip(messageID string) bool {
	if _, exists := s[messageID]; exists {
		return true
	}
	s[messageID] = struct{}{}
	return false
}

type historyStats struct {
	added        int
	labelChanges int
}

// maxParallel bounds concurrent full-message fetches.
const maxParallel = 2

// syncMeta converts fetched metadata to a store write. Labels of a held thread
// are kept while its local change is still undoable.
func (m *Manager) syncMeta(accountID string, meta gmail.MessageMeta) storage.SyncMeta {
	msg := storage.Message{
		ID:        meta.ID,
		ThreadID:  meta.ThreadID,
		MessageID: meta.MessageID,
		AccountID: accountID,
		Subject:   meta.Subject,
		FromAddr:  meta.From,
		ToAddr:    meta.To,
		CcAddr:    meta.Cc,
		BccAddr:   meta.Bcc,
		ReplyTo:   meta.ReplyTo, References: meta.References, InReplyTo: meta.InReplyTo, HeadersKnown: true,
		Snippet:      meta.Snippet,
		InternalDate: meta.InternalDate,
		IsUnread:     meta.IsUnread,
		CacheState:   storage.CacheMetadataOnly,
	}
	keepLabels := false
	if m.isHeld(accountID, meta.ThreadID) {
		if existing, _ := m.store.GetMessageLabelIDs(accountID, meta.ID); len(existing) > 0 {
			keepLabels = true // local trash still undoable; keep its labels
		}
	}
	return storage.SyncMeta{Msg: msg, LabelIDs: meta.LabelIDs, SetLabels: !keepLabels}
}

// storeMessageMetadata writes metas in one transaction (with the optional
// cursor), so a batch is stored whole or not at all.
func (m *Manager) storeMessageMetadata(accountID string, metas ...gmail.MessageMeta) error {
	return m.storeBatch(accountID, storage.SyncBatch{Metas: m.syncMetas(accountID, metas)})
}

func (m *Manager) syncMetas(accountID string, metas []gmail.MessageMeta) []storage.SyncMeta {
	out := make([]storage.SyncMeta, len(metas))
	for i, meta := range metas {
		out[i] = m.syncMeta(accountID, meta)
	}
	return out
}

func (m *Manager) storeBatch(accountID string, b storage.SyncBatch) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if err := m.store.ApplySyncBatch(accountID, b); err != nil {
		return fmt.Errorf("store sync batch: %w", err)
	}
	return nil
}

// historyEdits turns label history events into store edits; messages in skip
// were just fetched fresh from Gmail, so their labels are already current.
func (m *Manager) historyEdits(accountID string, events []gmail.HistoryEvent, skip map[string]struct{}) ([]storage.SyncLabelEdit, historyStats) {
	var stats historyStats
	var edits []storage.SyncLabelEdit
	for _, event := range events {
		var c *gmail.LabelChange
		var add bool
		switch {
		case event.LabelsAdded != nil:
			c, add = event.LabelsAdded, true
		case event.LabelsRemoved != nil:
			c = event.LabelsRemoved
		default:
			continue
		}
		if _, fresh := skip[c.MessageID]; fresh || m.isHeldMessage(accountID, c.MessageID) {
			continue
		}
		e := storage.SyncLabelEdit{MessageID: c.MessageID}
		if add {
			e.Add = c.LabelIDs
		} else {
			e.Remove = c.LabelIDs
		}
		edits = append(edits, e)
		stats.labelChanges++
	}
	return edits, stats
}

func (m *Manager) fetchBodiesIntoStore(ctx context.Context, accountID string, msgs []storage.Message) error {
	return forEachParallel(len(msgs), func(i int) error {
		return m.fetchAndStoreBody(ctx, accountID, msgs[i])
	})
}

// forEachParallel runs fn(0..n-1) on up to maxParallel goroutines and returns the first error.
func forEachParallel(n int, fn func(i int) error) error {
	var (
		wg       stdsync.WaitGroup
		mu       stdsync.Mutex
		firstErr error
	)
	sem := make(chan struct{}, maxParallel)
	for i := 0; i < n; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			if err := fn(i); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (m *Manager) fetchAndStoreBody(ctx context.Context, accountID string, msg storage.Message) error {
	full, err := m.client.GetMessage(ctx, msg.ID)
	if gmail.IsNotFound(err) { // deleted on the server since it was listed
		m.writeMu.Lock()
		defer m.writeMu.Unlock()
		return m.store.DeleteMessage(accountID, msg.ID)
	}
	if err != nil {
		m.reporter.Warnf("failed to fetch body for %s: %v\n", msg.ID, err)
		return nil
	}

	msg.Body = full.Body
	msg.ReplyTo, msg.References, msg.InReplyTo, msg.HeadersKnown = full.ReplyTo, full.References, full.InReplyTo, true
	msg.CacheState = storage.CacheBodyCached

	atts := make([]storage.Attachment, len(full.Attachments))
	for i, a := range full.Attachments {
		// Bytes of non-inline attachments are cached after sync by the
		// background attachment prefetch, so they never block sync.
		atts[i] = storage.Attachment{
			Name:         a.Name,
			MimeType:     a.MimeType,
			Size:         a.Size,
			AttachmentID: a.AttachmentID,
			Data:         a.InlineData,
		}
	}
	// Body and attachment rows commit together: a body marked cached must never
	// lack its attachments, since nothing would refetch it.
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return m.store.UpsertMessageBody(msg, atts)
}

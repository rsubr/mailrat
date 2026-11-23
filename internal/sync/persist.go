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
const maxParallel = 5

func (m *Manager) storeMessageMetadata(accountID string, meta gmail.MessageMeta) error {
	msg := storage.Message{
		ID:           meta.ID,
		ThreadID:     meta.ThreadID,
		MessageID:    meta.MessageID,
		AccountID:    accountID,
		Subject:      meta.Subject,
		FromAddr:     meta.From,
		ToAddr:       meta.To,
		CcAddr:       meta.Cc,
		BccAddr:      meta.Bcc,
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
	if err := m.store.UpsertMessageMeta(msg, meta.LabelIDs, !keepLabels); err != nil {
		return fmt.Errorf("store message %s: %w", meta.ID, err)
	}
	return nil
}

// applyHistoryEvents applies label changes; messages in skip were just fetched
// fresh from Gmail, so their labels are already current.
func (m *Manager) applyHistoryEvents(accountID string, events []gmail.HistoryEvent, skip map[string]struct{}) (historyStats, error) {
	var stats historyStats
	for _, event := range events {
		switch {
		case event.LabelsAdded != nil:
			if _, fresh := skip[event.LabelsAdded.MessageID]; fresh || m.isHeldMessage(accountID, event.LabelsAdded.MessageID) {
				continue
			}
			if err := m.applyAddedLabels(accountID, event.LabelsAdded.MessageID, event.LabelsAdded.LabelIDs); err != nil {
				return stats, err
			}
			stats.labelChanges++
		case event.LabelsRemoved != nil:
			if _, fresh := skip[event.LabelsRemoved.MessageID]; fresh || m.isHeldMessage(accountID, event.LabelsRemoved.MessageID) {
				continue
			}
			if err := m.applyRemovedLabels(accountID, event.LabelsRemoved.MessageID, event.LabelsRemoved.LabelIDs); err != nil {
				return stats, err
			}
			stats.labelChanges++
		}
	}
	return stats, nil
}

// applyDeletions removes messages Gmail reports as permanently deleted. Held
// threads are not skipped: the history cursor moves on, so the delete would be
// lost, and a pending trash or undo is a no-op on missing messages.
func (m *Manager) applyDeletions(accountID string, ids []string) error {
	for _, id := range ids {
		if err := m.store.DeleteMessage(accountID, id); err != nil {
			return fmt.Errorf("delete message %s: %w", id, err)
		}
	}
	return nil
}

func (m *Manager) applyAddedLabels(accountID, messageID string, labelIDs []string) error {
	existing, err := m.store.GetMessageLabelIDs(accountID, messageID)
	if err != nil {
		return nil
	}
	_ = m.store.SetMessageLabels(accountID, messageID, mergeLabelIDs(existing, labelIDs))
	return nil
}

func (m *Manager) applyRemovedLabels(accountID, messageID string, labelIDs []string) error {
	existing, err := m.store.GetMessageLabelIDs(accountID, messageID)
	if err != nil {
		return nil
	}
	_ = m.store.SetMessageLabels(accountID, messageID, removeLabelIDs(existing, labelIDs))
	return nil
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
	if err != nil {
		m.reporter.Warnf("failed to fetch body for %s: %v\n", msg.ID, err)
		return nil
	}

	msg.Body = full.Body
	msg.CacheState = storage.CacheBodyCached
	m.writeMu.Lock()
	err = m.store.UpsertMessage(msg)
	m.writeMu.Unlock()
	if err != nil {
		return err
	}

	if len(full.Attachments) == 0 {
		return nil
	}

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
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return m.store.UpsertAttachments(accountID, msg.ID, atts)
}

func mergeLabelIDs(existing, add []string) []string {
	set := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		set[id] = struct{}{}
	}
	for _, id := range add {
		set[id] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func removeLabelIDs(existing, remove []string) []string {
	rem := make(map[string]struct{}, len(remove))
	for _, id := range remove {
		rem[id] = struct{}{}
	}
	var out []string
	for _, id := range existing {
		if _, skip := rem[id]; !skip {
			out = append(out, id)
		}
	}
	return out
}

package sync

import (
	"context"
	"fmt"

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
	if err := m.store.UpsertMessage(msg); err != nil {
		return fmt.Errorf("upsert message %s: %w", meta.ID, err)
	}
	if err := m.store.UpsertThread(storage.Thread{
		ID:              meta.ThreadID,
		AccountID:       accountID,
		LastMessageDate: meta.InternalDate,
	}); err != nil {
		return fmt.Errorf("upsert thread %s: %w", meta.ThreadID, err)
	}
	if err := m.store.SetMessageLabels(accountID, meta.ID, meta.LabelIDs); err != nil {
		return fmt.Errorf("set labels on %s: %w", meta.ID, err)
	}
	return nil
}

func (m *Manager) applyHistoryEvents(accountID string, events []gmail.HistoryEvent) (historyStats, error) {
	var stats historyStats
	for _, event := range events {
		switch {
		case event.MessageAdded != nil:
			if err := m.storeMessageMetadata(accountID, *event.MessageAdded); err != nil {
				return stats, err
			}
			stats.added++
		case event.LabelsAdded != nil:
			if err := m.applyAddedLabels(accountID, event.LabelsAdded.MessageID, event.LabelsAdded.LabelIDs); err != nil {
				return stats, err
			}
			stats.labelChanges++
		case event.LabelsRemoved != nil:
			if err := m.applyRemovedLabels(accountID, event.LabelsRemoved.MessageID, event.LabelsRemoved.LabelIDs); err != nil {
				return stats, err
			}
			stats.labelChanges++
		}
	}
	return stats, nil
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
	for _, msg := range msgs {
		if err := m.fetchAndStoreBody(ctx, accountID, msg); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) fetchAndStoreBody(ctx context.Context, accountID string, msg storage.Message) error {
	full, err := m.client.GetMessage(ctx, msg.ID)
	if err != nil {
		m.reporter.Warnf("failed to fetch body for %s: %v\n", msg.ID, err)
		return nil
	}

	msg.Body = full.Body
	msg.CacheState = storage.CacheBodyCached
	if err := m.store.UpsertMessage(msg); err != nil {
		return err
	}

	if len(full.Attachments) == 0 {
		return nil
	}

	atts := make([]storage.Attachment, len(full.Attachments))
	for i, attachment := range full.Attachments {
		atts[i] = storage.Attachment{
			Name:     attachment.Name,
			MimeType: attachment.MimeType,
			Size:     attachment.Size,
		}
	}
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

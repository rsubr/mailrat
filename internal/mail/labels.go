package mail

import (
	"context"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// LabelService manages label operations on messages.
type LabelService struct {
	store  *storage.Store
	client *gmail.Client
}

// NewLabelService creates a LabelService.
func NewLabelService(store *storage.Store, client *gmail.Client) *LabelService {
	return &LabelService{store: store, client: client}
}

// Archive removes INBOX label from a message.
func (s *LabelService) Archive(ctx context.Context, accountID, messageID string) error {
	if err := s.client.ModifyLabels(ctx, messageID, nil, []string{LabelInbox}); err != nil {
		return err
	}
	return s.removeLocalLabel(accountID, messageID, LabelInbox)
}

// MarkRead removes the UNREAD label.
func (s *LabelService) MarkRead(ctx context.Context, accountID, messageID string) error {
	if err := s.client.ModifyLabels(ctx, messageID, nil, []string{LabelUnread}); err != nil {
		return err
	}
	if err := s.removeLocalLabel(accountID, messageID, LabelUnread); err != nil {
		return err
	}
	// Update local is_unread flag.
	msg, err := s.store.GetMessage(accountID, messageID)
	if err != nil {
		return err
	}
	msg.IsUnread = false
	return s.store.UpsertMessage(msg)
}

// MarkUnread adds the UNREAD label.
func (s *LabelService) MarkUnread(ctx context.Context, accountID, messageID string) error {
	if err := s.client.ModifyLabels(ctx, messageID, []string{LabelUnread}, nil); err != nil {
		return err
	}
	ids, err := s.store.GetMessageLabelIDs(accountID, messageID)
	if err != nil {
		return err
	}
	ids = append(ids, LabelUnread)
	if err := s.store.SetMessageLabels(accountID, messageID, ids); err != nil {
		return err
	}
	msg, err := s.store.GetMessage(accountID, messageID)
	if err != nil {
		return err
	}
	msg.IsUnread = true
	return s.store.UpsertMessage(msg)
}

// Trash moves a message to the Trash.
func (s *LabelService) Trash(ctx context.Context, accountID, messageID string) error {
	if err := s.client.TrashMessage(ctx, messageID); err != nil {
		return err
	}
	return s.removeLocalLabel(accountID, messageID, LabelInbox)
}

// ApplyLabel adds a label to a message.
func (s *LabelService) ApplyLabel(ctx context.Context, accountID, messageID, labelID string) error {
	if err := s.client.ModifyLabels(ctx, messageID, []string{labelID}, nil); err != nil {
		return err
	}
	ids, err := s.store.GetMessageLabelIDs(accountID, messageID)
	if err != nil {
		return err
	}
	ids = append(ids, labelID)
	return s.store.SetMessageLabels(accountID, messageID, ids)
}

// MarkAsSpam adds the SPAM label and removes the INBOX label.
func (s *LabelService) MarkAsSpam(ctx context.Context, accountID, messageID string) error {
	if err := s.client.ModifyLabels(ctx, messageID, []string{LabelSpam}, []string{LabelInbox}); err != nil {
		return err
	}
	return s.removeLocalLabel(accountID, messageID, LabelInbox)
}

// MarkAsNotSpam removes the SPAM label and adds the INBOX label.
func (s *LabelService) MarkAsNotSpam(ctx context.Context, accountID, messageID string) error {
	if err := s.client.ModifyLabels(ctx, messageID, []string{LabelInbox}, []string{LabelSpam}); err != nil {
		return err
	}
	if err := s.removeLocalLabel(accountID, messageID, LabelSpam); err != nil {
		return err
	}
	return s.ApplyLabel(ctx, accountID, messageID, LabelInbox)
}

func (s *LabelService) removeLocalLabel(accountID, messageID, labelID string) error {
	ids, err := s.store.GetMessageLabelIDs(accountID, messageID)
	if err != nil {
		return err
	}
	var updated []string
	for _, id := range ids {
		if id != labelID {
			updated = append(updated, id)
		}
	}
	return s.store.SetMessageLabels(accountID, messageID, updated)
}

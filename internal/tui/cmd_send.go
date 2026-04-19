package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
)

func sendEmailCmd(accMgr *accounts.Manager, store *storage.Store, acctID string, outMsg mail.OutboundMessage, draftID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		httpClient, err := accMgr.GetClient(ctx, acctID)
		if err != nil {
			return msgSendDone{err: fmt.Errorf("get client: %w", err)}
		}

		gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
		if err != nil {
			return msgSendDone{err: fmt.Errorf("gmail client: %w", err)}
		}

		svc := mail.NewSendService(gmailClient)
		_, err = svc.Send(ctx, outMsg)
		if err == nil && draftID != "" {
			// Delete from local storage; Gmail deletes it from its drafts when sent.
			_ = store.DeleteDraft(acctID, draftID)
		}
		return msgSendDone{draftID: draftID, err: err}
	}
}

func saveDraftCmd(accMgr *accounts.Manager, store *storage.Store, acctID string, outMsg mail.OutboundMessage, draftID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		httpClient, err := accMgr.GetClient(ctx, acctID)
		if err != nil {
			return msgSaveDraftDone{err: fmt.Errorf("get client: %w", err)}
		}

		gmailClient, err := gmail.NewClient(ctx, httpClient, acctID)
		if err != nil {
			return msgSaveDraftDone{err: fmt.Errorf("gmail client: %w", err)}
		}

		svc := mail.NewDraftService(store, gmailClient)
		if draftID != "" {
			err = svc.UpdateDraft(ctx, acctID, draftID, outMsg)
			return msgSaveDraftDone{draftID: draftID, err: err}
		}

		newID, err := svc.SaveDraft(ctx, acctID, outMsg)
		return msgSaveDraftDone{draftID: newID, err: err}
	}
}

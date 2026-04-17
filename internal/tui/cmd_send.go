package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
)

func sendEmailCmd(accMgr *accounts.Manager, acctID string, outMsg mail.OutboundMessage) tea.Cmd {
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
		return msgSendDone{err: err}
	}
}

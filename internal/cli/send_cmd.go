package cli

import (
	"context"
	"fmt"

	"github.com/mailrat/mailrat/internal/mail"
	"github.com/spf13/cobra"
)

func newSendCmd() *cobra.Command {
	var accountID string
	var to, subject, body string

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send an email",
		RunE: withAppContext(func(ctx context.Context, a *app) error {
			var err error

			// Require at least one account.
			accountID, err = resolveAccount(a, accountID)
			if err != nil {
				return err
			}

			// Interactive prompts if flags not provided.
			reader := newPromptReader()
			if to == "" {
				to, err = promptLine(reader, "To: ")
				if err != nil {
					return err
				}
			}
			if subject == "" {
				subject, err = promptLine(reader, "Subject: ")
				if err != nil {
					return err
				}
			}
			if body == "" {
				body, err = promptMultilineBody(reader, "Body (end with a line containing only '.'):")
				if err != nil {
					return err
				}
			}

			// Find the account email for the From header.
			fromEmail, err := resolveAccountEmail(a, accountID)
			if err != nil {
				return err
			}

			client, err := a.gmailClientForAccount(ctx, accountID)
			if err != nil {
				return err
			}

			svc := mail.NewSendService(client)
			msgID, err := svc.Send(ctx, mail.OutboundMessage{
				From:    fromEmail,
				To:      to,
				Subject: subject,
				Body:    body,
			})
			if err != nil {
				return fmt.Errorf("send failed: %w", err)
			}

			fmt.Printf("Message sent (id: %s)\n", msgID)
			return nil
		}),
	}

	cmd.Flags().StringVar(&accountID, "account", "", "Account ID to send from")
	cmd.Flags().StringVar(&to, "to", "", "Recipient email")
	cmd.Flags().StringVar(&subject, "subject", "", "Subject")
	cmd.Flags().StringVar(&body, "body", "", "Message body")

	return cmd
}

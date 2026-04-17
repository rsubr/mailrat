package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mailrat/mailrat/internal/mail"
	"github.com/spf13/cobra"
)

func init() {
	var accountID string
	var to, subject, body string

	sendCmd := &cobra.Command{
		Use:   "send",
		Short: "Send an email",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()

			// Require at least one account.
			accs, err := a.accounts.ListAccounts()
			if err != nil {
				return err
			}
			if len(accs) == 0 {
				return fmt.Errorf("no accounts configured; run: mailrat accounts add")
			}

			// Use first account if not specified.
			if accountID == "" {
				accountID = accs[0].ID
			}

			// Interactive prompts if flags not provided.
			reader := bufio.NewReader(os.Stdin)
			if to == "" {
				fmt.Print("To: ")
				to, _ = reader.ReadString('\n')
				to = strings.TrimSpace(to)
			}
			if subject == "" {
				fmt.Print("Subject: ")
				subject, _ = reader.ReadString('\n')
				subject = strings.TrimSpace(subject)
			}
			if body == "" {
				fmt.Println("Body (end with a line containing only '.'):")
				var lines []string
				for {
					line, _ := reader.ReadString('\n')
					line = strings.TrimRight(line, "\r\n")
					if line == "." {
						break
					}
					lines = append(lines, line)
				}
				body = strings.Join(lines, "\n")
			}

			// Find the account email for the From header.
			var fromEmail string
			for _, acc := range accs {
				if acc.ID == accountID {
					fromEmail = acc.Email
					break
				}
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
		},
	}

	sendCmd.Flags().StringVar(&accountID, "account", "", "Account ID to send from")
	sendCmd.Flags().StringVar(&to, "to", "", "Recipient email")
	sendCmd.Flags().StringVar(&subject, "subject", "", "Subject")
	sendCmd.Flags().StringVar(&body, "body", "", "Message body")

	rootCmd.AddCommand(sendCmd)
}

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	var accountID string

	messageCmd := &cobra.Command{
		Use:   "message",
		Short: "Message commands",
	}

	readCmd := &cobra.Command{
		Use:   "read <message-id>",
		Short: "Read a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()
			messageID := args[0]

			if accountID == "" {
				// Find the account that owns this message.
				accs, err := a.accounts.ListAccounts()
				if err != nil {
					return err
				}
				for _, acc := range accs {
					msg, err := a.store.GetMessage(acc.ID, messageID)
					if err == nil {
						accountID = acc.ID
						_ = msg
						break
					}
				}
			}
			if accountID == "" {
				return fmt.Errorf("message %q not found in any account", messageID)
			}

			svc, err := a.inboxService(ctx, accountID)
			if err != nil {
				return err
			}

			msg, err := svc.GetMessage(ctx, accountID, messageID)
			if err != nil {
				return err
			}

			date := time.UnixMilli(msg.InternalDate).Format("Mon, 02 Jan 2006 15:04:05")
			fmt.Printf("From:    %s\n", msg.FromAddr)
			fmt.Printf("To:      %s\n", msg.ToAddr)
			fmt.Printf("Subject: %s\n", msg.Subject)
			fmt.Printf("Date:    %s\n", date)
			fmt.Printf("\n%s\n", msg.Body)
			return nil
		},
	}

	readCmd.Flags().StringVar(&accountID, "account", "", "Account ID")

	messageCmd.AddCommand(readCmd)
	rootCmd.AddCommand(messageCmd)
}

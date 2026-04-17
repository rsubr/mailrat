package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newMessageCmd() *cobra.Command {
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
			return withAppContext(func(ctx context.Context, a *app) error {
				messageID := args[0]
				var err error

				if accountID == "" {
					accountID, err = resolveMessageAccount(a, messageID)
					if err != nil {
						return err
					}
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
			})(cmd, args)
		},
	}

	readCmd.Flags().StringVar(&accountID, "account", "", "Account ID")

	messageCmd.AddCommand(readCmd)
	return messageCmd
}

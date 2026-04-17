package cli

import (
	"fmt"

	"github.com/mailrat/mailrat/internal/storage"
	"github.com/spf13/cobra"
)

func newInboxCmd() *cobra.Command {
	var limit int
	var accountID string
	var label string

	inboxCmd := &cobra.Command{
		Use:   "inbox",
		Short: "Inbox commands",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List messages",
		Long: `List messages from the local cache.

By default shows the unified inbox across all accounts.
Use --label to filter by any Gmail label:

  mailrat inbox list --label SENT
  mailrat inbox list --label DRAFT
  mailrat inbox list --label SPAM
  mailrat inbox list --label "My Label"

Label names and IDs are case-insensitive. Run 'mailrat labels list' to see
all available labels for your account.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			var messages []storage.Message

			switch {
			case label != "" && accountID != "":
				messages, err = a.store.ListMessagesByLabel(accountID, label, limit)
			case label != "":
				messages, err = a.store.ListAllMessagesByLabel(label, limit)
			case accountID != "":
				messages, err = a.store.ListMessages(accountID, limit)
			default:
				messages, err = a.store.ListAllMessages(limit)
			}
			if err != nil {
				return err
			}

			if len(messages) == 0 {
				if label != "" {
					fmt.Printf("No messages with label %q. Run: mailrat sync\n", label)
				} else {
					fmt.Println("No messages. Run: mailrat sync")
				}
				return nil
			}

			printMessageList(messages)
			return nil
		},
	}

	listCmd.Flags().IntVar(&limit, "limit", 50, "Max messages to show")
	listCmd.Flags().StringVar(&accountID, "account", "", "Filter by account ID")
	listCmd.Flags().StringVar(&label, "label", "", "Filter by label name or ID (e.g. SENT, DRAFT, INBOX)")

	inboxCmd.AddCommand(listCmd)
	return inboxCmd
}

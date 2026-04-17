package cli

import (
	"fmt"
	"time"

	"github.com/mailrat/mailrat/internal/storage"
	"github.com/spf13/cobra"
)

func init() {
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
	rootCmd.AddCommand(inboxCmd)
}

func printMessageList(msgs []storage.Message) {
	fmt.Printf("%-22s  %-30s  %-40s  %s\n", "DATE", "FROM", "SUBJECT", "SNIPPET")
	fmt.Printf("%s\n", "------------------------------------------------------------")
	for _, m := range msgs {
		date := time.UnixMilli(m.InternalDate).Format("2006-01-02 15:04")
		from := truncate(m.FromAddr, 30)
		subject := truncate(m.Subject, 40)
		snippet := truncate(m.Snippet, 60)
		unread := " "
		if m.IsUnread {
			unread = "*"
		}
		fmt.Printf("%s %s  %-30s  %-40s  %s\n", unread, date, from, subject, snippet)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

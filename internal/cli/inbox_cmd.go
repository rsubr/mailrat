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

	inboxCmd := &cobra.Command{
		Use:   "inbox",
		Short: "Inbox commands",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List inbox messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			var messages []storage.Message
			if accountID != "" {
				messages, err = a.store.ListMessages(accountID, limit)
			} else {
				messages, err = a.store.ListAllMessages(limit)
			}
			if err != nil {
				return err
			}

			if len(messages) == 0 {
				fmt.Println("No messages. Run: mailrat sync")
				return nil
			}

			printMessageList(messages)
			return nil
		},
	}

	listCmd.Flags().IntVar(&limit, "limit", 50, "Max messages to show")
	listCmd.Flags().StringVar(&accountID, "account", "", "Filter by account ID")

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

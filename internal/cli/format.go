package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/mailrat/mailrat/internal/storage"
)

func printContacts(contacts []storage.Contact) {
	fmt.Printf("%-35s  %-30s  %s\n", "NAME", "EMAIL", "ID")
	fmt.Println(strings.Repeat("─", 100))
	for _, c := range contacts {
		fmt.Printf("%-35s  %-30s  %s\n",
			truncate(c.Name, 35),
			truncate(c.Email, 30),
			c.ID,
		)
	}
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

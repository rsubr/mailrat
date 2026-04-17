package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	var accountID string
	var syncFirst bool

	contactsCmd := &cobra.Command{
		Use:   "contacts",
		Short: "Contact commands",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()

			if accountID == "" {
				accs, err := a.accounts.ListAccounts()
				if err != nil {
					return err
				}
				if len(accs) == 0 {
					return fmt.Errorf("no accounts configured")
				}
				accountID = accs[0].ID
			}

			svc, err := a.contactsService(ctx, accountID)
			if err != nil {
				return err
			}

			if syncFirst {
				if err := svc.Sync(ctx); err != nil {
					return err
				}
			}

			contacts, err := svc.List()
			if err != nil {
				return err
			}

			if len(contacts) == 0 {
				fmt.Println("No contacts. Run: mailrat contacts list --sync")
				return nil
			}

			fmt.Printf("%-30s  %s\n", "NAME", "EMAIL")
			for _, c := range contacts {
				fmt.Printf("%-30s  %s\n", truncate(c.Name, 30), c.Email)
			}
			return nil
		},
	}

	listCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	listCmd.Flags().BoolVar(&syncFirst, "sync", false, "Sync contacts before listing")
	contactsCmd.AddCommand(listCmd)
	rootCmd.AddCommand(contactsCmd)
}

package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	accountsCmd := &cobra.Command{
		Use:   "accounts",
		Short: "Manage Gmail accounts",
	}

	accountsCmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a Gmail account via OAuth2",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			acc, err := a.accounts.AddAccount(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("Account added: %s (id: %s)\n", acc.Email, acc.ID)
			return nil
		},
	})

	accountsCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			accs, err := a.accounts.ListAccounts()
			if err != nil {
				return err
			}
			if len(accs) == 0 {
				fmt.Println("No accounts. Run: mailrat accounts add")
				return nil
			}
			for _, acc := range accs {
				fmt.Printf("  %s  (id: %s)\n", acc.Email, acc.ID)
			}
			return nil
		},
	})

	rootCmd.AddCommand(accountsCmd)
}

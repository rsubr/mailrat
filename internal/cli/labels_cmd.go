package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	var accountID string

	labelsCmd := &cobra.Command{
		Use:   "labels",
		Short: "Label commands",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

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

			labels, err := a.store.ListLabels(accountID)
			if err != nil {
				return err
			}

			if len(labels) == 0 {
				fmt.Println("No labels. Run: mailrat sync")
				return nil
			}

			fmt.Printf("%-30s  %-15s  %s\n", "NAME", "TYPE", "ID")
			for _, l := range labels {
				fmt.Printf("%-30s  %-15s  %s\n", l.Name, l.LabelType, l.ID)
			}
			return nil
		},
	}

	listCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	labelsCmd.AddCommand(listCmd)
	rootCmd.AddCommand(labelsCmd)
}

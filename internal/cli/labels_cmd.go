package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLabelsCmd() *cobra.Command {
	var accountID string

	labelsCmd := &cobra.Command{
		Use:   "labels",
		Short: "Label commands",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List labels",
		RunE: withApp(func(a *app) error {
			aid, err := resolveAccount(a, accountID)
			if err != nil {
				if accountID == "" {
					return fmt.Errorf("no accounts configured")
				}
				return err
			}

			labels, err := a.store.ListLabels(aid)
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
		}),
	}

	listCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	labelsCmd.AddCommand(listCmd)
	return labelsCmd
}

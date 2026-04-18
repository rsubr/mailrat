package cli

import (
	"context"
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
				fmt.Println("No labels. Run: mailrat labels sync")
				return nil
			}

			fmt.Printf("%-30s  %-15s  %-9s  %-9s  %s\n", "NAME", "TYPE", "TEXT", "BG", "ID")
			for _, l := range labels {
				fmt.Printf("%-30s  %-15s  %-9s  %-9s  %s\n", l.Name, l.LabelType, l.TextColor, l.BackgroundColor, l.ID)
			}
			return nil
		}),
	}
	listCmd.Flags().StringVar(&accountID, "account", "", "Account ID")

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync labels from Gmail",
		RunE: withAppContext(func(ctx context.Context, a *app) error {
			accs, err := a.accounts.ListAccounts()
			if err != nil {
				return err
			}
			if len(accs) == 0 {
				return fmt.Errorf("no accounts configured; run: mailrat accounts add")
			}
			for _, acc := range accs {
				if accountID != "" && acc.ID != accountID {
					continue
				}
				mgr, err := a.syncManager(ctx, acc.ID)
				if err != nil {
					fmt.Printf("warning: cannot init sync for %s: %v\n", acc.Email, err)
					continue
				}
				if err := mgr.SyncLabels(ctx, acc.ID); err != nil {
					return fmt.Errorf("sync labels for %s: %w", acc.Email, err)
				}
				fmt.Printf("Labels synced for %s\n", acc.Email)
			}
			return nil
		}),
	}
	syncCmd.Flags().StringVar(&accountID, "account", "", "Account ID")

	labelsCmd.AddCommand(listCmd, syncCmd)
	return labelsCmd
}

package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var days int
	var accountID string

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync mail from Gmail",
		RunE: withAppContext(func(ctx context.Context, a *app) error {

			// Determine which accounts to sync.
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

				// Use incremental sync if history ID is stored, otherwise initial.
				stored, _ := a.store.GetAccount(acc.ID)
				if stored.HistoryID == "" {
					configs, _ := a.store.ListSyncConfigs(acc.ID)
					if err := mgr.InitialSync(ctx, acc.ID, days, configs); err != nil {
						return fmt.Errorf("initial sync for %s: %w", acc.Email, err)
					}
				} else {
					if err := mgr.IncrementalSync(ctx, acc.ID); err != nil {
						return fmt.Errorf("incremental sync for %s: %w", acc.Email, err)
					}
					// Backfill historical messages for any label whose config
					// has changed (e.g. INBOX set to "all mail") since last sync.
					configs, _ := a.store.ListSyncConfigs(acc.ID)
					if err := mgr.BackfillFromConfigs(ctx, acc.ID, configs); err != nil {
						fmt.Printf("warning: backfill for %s: %v\n", acc.Email, err)
					}
				}
			}
			return nil
		}),
	}

	syncCmd.Flags().IntVar(&days, "days", 30, "Number of days to sync on initial sync")
	syncCmd.Flags().StringVar(&accountID, "account", "", "Account ID to sync (default: all)")

	return syncCmd
}

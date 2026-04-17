package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = newRootCmd()

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mailrat",
		Short: "A native Gmail client",
		RunE: func(cmd *cobra.Command, args []string) error {
			return launchTUI()
		},
	}

	cmd.AddCommand(
		newAccountsCmd(),
		newContactsCmd(),
		newInboxCmd(),
		newLabelsCmd(),
		newMessageCmd(),
		newSendCmd(),
		newSyncCmd(),
	)

	return cmd
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd returns the mailrat root command with all CLI subcommands attached.
// rootRunE is called when mailrat is invoked with no subcommand (nil = show help).
func NewRootCmd(rootRunE func(*cobra.Command, []string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mailrat",
		Short: "A native Gmail client",
		RunE:  rootRunE,
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

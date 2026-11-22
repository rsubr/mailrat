package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2/app"
	"github.com/mailrat/mailrat/internal/cli"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/ui"
	"github.com/spf13/cobra"
)

func main() {
	root := cli.NewRootCmd(func(cmd *cobra.Command, args []string) error {
		return launchDesktop()
	})
	root.AddCommand(cli.NewTUICmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// launchDesktop starts the native Fyne desktop app.
func launchDesktop() error {
	a := app.NewWithID("com.mailrat.app")
	ui.Run(a, desktop.NewApp())
	return nil
}

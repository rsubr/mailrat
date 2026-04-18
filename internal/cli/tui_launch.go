package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/tui"
	"github.com/spf13/cobra"
)

// NewTUICmd returns the "tui" subcommand.
func NewTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return launchTUI()
		},
	}
}

func launchTUI() error {
	return runWithApp(func(a *app) error {
		p := tea.NewProgram(
			tui.New(a.store, a.accounts),
			tea.WithAltScreen(),
		)
		_, err := p.Run()
		return err
	})
}

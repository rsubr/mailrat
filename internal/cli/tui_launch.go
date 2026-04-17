package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/tui"
)

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

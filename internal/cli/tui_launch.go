package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/tui"
)

func launchTUI() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.close()

	p := tea.NewProgram(
		tui.New(a.store, a.accounts),
		tea.WithAltScreen(),
	)
	_, err = p.Run()
	return err
}

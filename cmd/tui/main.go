package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/storage"
	"github.com/mailrat/mailrat/internal/tui"
)

func main() {
	store, accMgr, err := initApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	defer store.Close()

	p := tea.NewProgram(
		tui.New(store, accMgr),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI error:", err)
		os.Exit(1)
	}
}

func initApp() (*storage.Store, *accounts.Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(home, ".mailrat")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}

	store, err := storage.Open(filepath.Join(dir, "mailrat.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	accMgr, err := accounts.NewManager(store, filepath.Join(dir, "credentials.json"))
	if err != nil {
		store.Close() //nolint:errcheck
		return nil, nil, fmt.Errorf("credentials not found — run: mailrat accounts add")
	}

	return store, accMgr, nil
}

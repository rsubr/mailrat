package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateAccounts(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.nav.view = viewInbox
			return m, nil
		case "j", "down":
			// items: 0=unified, 1..N=accounts
			if m.account.cursor < len(m.nav.allAccounts) {
				m.account.cursor++
			}
		case "k", "up":
			if m.account.cursor > 0 {
				m.account.cursor--
			}
		case "enter":
			if m.account.cursor == 0 {
				m.nav.activeAcct = ""
			} else {
				m.nav.activeAcct = m.nav.allAccounts[m.account.cursor-1].ID
			}
			m.nav.view = viewInbox
			m.inbox.cursor = 0
			return m, loadInboxCmd(m.store, m.nav.activeAcct)
		}
	}
	return m, nil
}

func (m model) renderAccounts() string {
	var sb strings.Builder
	sb.WriteString("  " + boldStyle.Render("Select Account") + "\n\n")

	items := make([]string, 0, len(m.nav.allAccounts)+1)
	items = append(items, "Unified Inbox")
	for _, a := range m.nav.allAccounts {
		items = append(items, a.Email)
	}

	for i, item := range items {
		// Mark the currently active account.
		check := "  "
		isActive := (i == 0 && m.nav.activeAcct == "") ||
			(i > 0 && m.nav.allAccounts[i-1].ID == m.nav.activeAcct)
		if isActive {
			check = accentStyle.Render("✓ ")
		}

		line := fmt.Sprintf("%s%s", check, item)
		if i == m.account.cursor {
			line = selectedStyle.Width(m.layout.width).Render("  " + item)
			if isActive {
				line = selectedStyle.Width(m.layout.width).Render(accentStyle.Render("✓ ") + item)
			}
		}

		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	if len(m.nav.allAccounts) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  No accounts. Run: mailrat accounts add"))
		sb.WriteByte('\n')
	}

	return sb.String()
}

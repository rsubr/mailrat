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
			m.view = viewInbox
			return m, nil
		case "j", "down":
			// items: 0=unified, 1..N=accounts
			if m.acctCursor < len(m.allAccounts) {
				m.acctCursor++
			}
		case "k", "up":
			if m.acctCursor > 0 {
				m.acctCursor--
			}
		case "enter":
			if m.acctCursor == 0 {
				m.activeAcct = ""
			} else {
				m.activeAcct = m.allAccounts[m.acctCursor-1].ID
			}
			m.view = viewInbox
			m.cursor = 0
			return m, loadInboxCmd(m.store, m.activeAcct)
		}
	}
	return m, nil
}

func (m model) renderAccounts() string {
	var sb strings.Builder
	sb.WriteString("  " + boldStyle.Render("Select Account") + "\n\n")

	items := make([]string, 0, len(m.allAccounts)+1)
	items = append(items, "Unified Inbox")
	for _, a := range m.allAccounts {
		items = append(items, a.Email)
	}

	for i, item := range items {
		// Mark the currently active account.
		check := "  "
		isActive := (i == 0 && m.activeAcct == "") ||
			(i > 0 && m.allAccounts[i-1].ID == m.activeAcct)
		if isActive {
			check = accentStyle.Render("✓ ")
		}

		line := fmt.Sprintf("%s%s", check, item)
		if i == m.acctCursor {
			line = selectedStyle.Width(m.width).Render("  " + item)
			if isActive {
				line = selectedStyle.Width(m.width).Render(accentStyle.Render("✓ ") + item)
			}
		}

		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	if len(m.allAccounts) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  No accounts. Run: mailrat accounts add"))
		sb.WriteByte('\n')
	}

	return sb.String()
}

package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			m.status.quitting = true
			return m, nil
		case "esc", "u":
			m.nav.view = viewInbox
			return m, nil
		case "/":
			if !m.search.inputFocused {
				m.search.inputFocused = true
				m.search.input.Focus()
				return m, nil
			}
		case "enter":
			if m.search.inputFocused {
				query := strings.TrimSpace(m.search.input.Value())
				if query == "" {
					return m, nil
				}
				m.search.inputFocused = false
				m.search.input.Blur()
				m.search.cursor = 0
				return m, searchThreadsCmd(m.store, query)
			}
			if len(m.search.results) == 0 {
				return m, nil
			}
			sel := m.search.results[m.search.cursor]
			return m, loadThreadCmd(m.store, sel.AccountID, sel.ThreadID)
		case "o":
			if !m.search.inputFocused && len(m.search.results) > 0 {
				sel := m.search.results[m.search.cursor]
				return m, loadThreadCmd(m.store, sel.AccountID, sel.ThreadID)
			}
		case "j", "down", "n":
			if !m.search.inputFocused && m.search.cursor < len(m.search.results)-1 {
				m.search.cursor++
			}
		case "k", "up", "p":
			if !m.search.inputFocused && m.search.cursor > 0 {
				m.search.cursor--
			}
		}
	}

	if m.search.inputFocused {
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) renderSearch() string {
	var sb strings.Builder

	sb.WriteString(dimStyle.Render("  Search: ") + m.search.input.View() + "\n\n")

	if len(m.search.results) == 0 {
		query := strings.TrimSpace(m.search.input.Value())
		if query != "" && !m.search.inputFocused {
			sb.WriteString(dimStyle.Render("  No results for \"" + query + "\"."))
		} else {
			sb.WriteString(dimStyle.Render("  Type terms and press Enter to search (space-separated terms ranked by match count)."))
		}
		return sb.String()
	}

	maxRows := m.contentHeight() - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if m.search.cursor >= maxRows {
		start = m.search.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.search.results) {
		end = len(m.search.results)
	}
	for i := start; i < end; i++ {
		selected := !m.search.inputFocused && i == m.search.cursor
		sb.WriteString(m.renderThreadRow(m.search.results[i], selected))
		sb.WriteByte('\n')
	}
	return sb.String()
}

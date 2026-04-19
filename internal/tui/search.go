package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgSearchDebounce:
		if msg.token == m.search.debounceToken {
			return m, searchThreadsCmd(m.store, m.nav.activeAcct, m.search.input.Value())
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "u":
			m.nav.view = viewInbox
			return m, nil
		case "enter", "o":
			if len(m.search.results) > 0 {
				sel := m.search.results[m.search.cursor]
				return m, loadThreadCmd(m.store, sel.AccountID, sel.ThreadID)
			}
			return m, nil
		case "j", "down", "n":
			if m.search.cursor < len(m.search.results)-1 {
				m.search.cursor++
			}
			return m, nil
		case "k", "up", "p":
			if m.search.cursor > 0 {
				m.search.cursor--
			}
			return m, nil
		}
	}

	prevVal := m.search.input.Value()
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	if m.search.input.Value() != prevVal {
		m.search.debounceToken++
		m.search.cursor = 0
		return m, tea.Batch(cmd, searchDebounceCmd(m.search.debounceToken))
	}
	return m, cmd
}

func (m model) renderSearch() string {
	var sb strings.Builder

	sb.WriteString(dimStyle.Render("  Search: ") + m.search.input.View() + "\n\n")

	if len(m.search.results) == 0 {
		query := strings.TrimSpace(m.search.input.Value())
		if query != "" {
			sb.WriteString(dimStyle.Render("  No results for \"" + query + "\"."))
		} else {
			sb.WriteString(dimStyle.Render("  Loading..."))
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
		sb.WriteString(m.renderThreadRow(m.search.results[i], i == m.search.cursor))
		sb.WriteByte('\n')
	}
	return sb.String()
}

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// configRow is one label's editable sync settings in the config modal.
type configRow struct {
	LabelID          string
	LabelName        string
	DaysToSync       int // 0 = all mail
	FetchAttachments bool
}

// dayPresets is the cycle order for the "d" key.
var dayPresets = []int{0, 7, 14, 30, 60, 90, 365}

func fmtDays(d int) string {
	if d == 0 {
		return "all"
	}
	return fmt.Sprintf("%dd", d)
}

func nextDayPreset(current int) int {
	for i, p := range dayPresets {
		if p == current {
			return dayPresets[(i+1)%len(dayPresets)]
		}
	}
	return dayPresets[0] // reset to "all" if current value isn't a preset
}

func (m model) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, saveConfigCmd(m.store, m.resolveAcctID(), m.configRows)
		case "j", "down":
			if m.configCursor < len(m.configRows)-1 {
				m.configCursor++
			}
		case "k", "up":
			if m.configCursor > 0 {
				m.configCursor--
			}
		case "d":
			if len(m.configRows) > 0 {
				m.configRows[m.configCursor].DaysToSync = nextDayPreset(m.configRows[m.configCursor].DaysToSync)
			}
		case "a":
			if len(m.configRows) > 0 {
				m.configRows[m.configCursor].FetchAttachments = !m.configRows[m.configCursor].FetchAttachments
			}
		}
	}
	return m, nil
}

func (m model) renderConfig() string {
	var sb strings.Builder
	sb.WriteString("  " + boldStyle.Render("Sync Configuration") + "\n\n")

	if len(m.configRows) == 0 {
		sb.WriteString(dimStyle.Render("  No labels found. Run sync first to populate labels.") + "\n")
		return sb.String()
	}

	header := fmt.Sprintf("  %-26s  %-8s  %s", "Label", "Days", "Attachments")
	sb.WriteString(dimStyle.Render(header) + "\n")
	sep := "  " + strings.Repeat("─", m.width-6)
	if m.width < 10 {
		sep = "  ──────────────────────────────"
	}
	sb.WriteString(separatorStyle.Render(sep) + "\n")

	// Virtual scrolling: keep the cursor row visible.
	maxRows := m.contentHeight() - 4 // header + blank + column header + separator
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if m.configCursor >= maxRows {
		start = m.configCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.configRows) {
		end = len(m.configRows)
	}

	for i := start; i < end; i++ {
		row := m.configRows[i]

		att := dimStyle.Render("✗")
		if row.FetchAttachments {
			att = accentStyle.Render("✓")
		}

		line := fmt.Sprintf("  %-26s  %-8s  %s", truncStr(row.LabelName, 26), fmtDays(row.DaysToSync), att)

		if i == m.configCursor {
			sb.WriteString(selectedStyle.Width(m.width).Render(line) + "\n")
		} else {
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

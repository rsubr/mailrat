package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateThread(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.view = viewInbox
			return m, nil
		case "j", "down":
			if m.threadCur < len(m.threadMsgs)-1 {
				m.threadCur++
			}
		case "k", "up":
			if m.threadCur > 0 {
				m.threadCur--
			}
		case "enter":
			if len(m.threadMsgs) == 0 {
				return m, nil
			}
			sel := m.threadMsgs[m.threadCur]
			return m, loadMessageCmd(m.store, m.accMgr, sel.AccountID, sel.ID)
		}
	}
	return m, nil
}

func (m model) renderThread() string {
	if len(m.threadMsgs) == 0 {
		return dimStyle.Render("\n  No messages in thread.")
	}

	maxRows := m.contentHeight()
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if m.threadCur >= maxRows {
		start = m.threadCur - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.threadMsgs) {
		end = len(m.threadMsgs)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		msg := m.threadMsgs[i]

		dot := "  "
		if msg.IsUnread {
			dot = unreadDotStyle.Render("• ")
		}

		date := time.UnixMilli(msg.InternalDate).Format("Jan 02 15:04")
		from := truncStr(msg.FromAddr, 30)
		subject := truncStr(msg.Subject, 50)

		line := fmt.Sprintf("%s%-10s  %-30s  %s", dot, date, from, subject)

		switch {
		case i == m.threadCur:
			line = selectedStyle.Width(m.width).Render(line)
		case msg.IsUnread:
			line = boldStyle.Render(line)
		}

		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

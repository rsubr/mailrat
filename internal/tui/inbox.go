package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/storage"
)

func (m model) updateInbox(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.messages)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if len(m.messages) == 0 {
				return m, nil
			}
			sel := m.messages[m.cursor]
			if sel.ThreadID != "" {
				return m, loadThreadCmd(m.store, sel.AccountID, sel.ThreadID)
			}
			return m, loadMessageCmd(m.store, m.accMgr, sel.AccountID, sel.ID)
		case "s":
			if !m.syncing && len(m.allAccounts) > 0 {
				m.syncing = true
				m.status = "Syncing..."
				return m, syncAllCmd(m.store, m.accMgr, m.allAccounts)
			}
		case "c":
			m.view = viewCompose
			m.replyTo = nil
			m.composeFocus = 0
			m.composeTo.Focus()
			m.composeSubject.Blur()
			m.composeBody.Blur()
			return m, nil
		case "a":
			m.view = viewAccounts
			m.acctCursor = 0
			return m, nil
		}
	}
	return m, nil
}

func (m model) renderInbox() string {
	if len(m.messages) == 0 {
		return dimStyle.Render("\n  No messages. Press 's' to sync.")
	}

	maxRows := m.contentHeight()
	if maxRows < 1 {
		maxRows = 1
	}

	// Scroll window so cursor is always visible.
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.messages) {
		end = len(m.messages)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(m.renderInboxRow(m.messages[i], i == m.cursor))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m model) renderInboxRow(msg storage.Message, selected bool) string {
	dot := "  "
	if msg.IsUnread {
		dot = unreadDotStyle.Render("• ")
	}

	date := time.UnixMilli(msg.InternalDate).Format("Jan 02 15:04")
	from := truncStr(msg.FromAddr, 24)
	subject := truncStr(msg.Subject, 42)
	snippet := truncStr(msg.Snippet, 30)

	line := fmt.Sprintf("%s%-10s  %-24s  %-42s  %s", dot, date, from, subject, snippet)

	switch {
	case selected:
		return selectedStyle.Width(m.width).Render(line)
	case msg.IsUnread:
		return boldStyle.Render(line)
	default:
		return line
	}
}

func (m model) renderHeader() string {
	title := "mailrat"
	switch {
	case m.activeAcct != "":
		for _, a := range m.allAccounts {
			if a.ID == m.activeAcct {
				title = "mailrat — " + a.Email
				break
			}
		}
	case len(m.allAccounts) > 1:
		title = "mailrat — unified inbox"
	case len(m.allAccounts) == 1:
		title = "mailrat — " + m.allAccounts[0].Email
	}
	return headerStyle.Width(m.width).Render(title)
}

func (m model) renderHelp() string {
	var keys string
	switch m.view {
	case viewInbox:
		keys = "j/k:move  enter:open  c:compose  s:sync  a:accounts  q:quit"
	case viewThread:
		keys = "j/k:move  enter:open message  esc:back"
	case viewMessage:
		keys = "↑/↓/pgup/pgdn:scroll  r:reply  esc:back  q:quit"
	case viewCompose:
		keys = "tab:next field  ctrl+s:send  esc:cancel"
	case viewAccounts:
		keys = "j/k:move  enter:select  esc:back"
	}

	right := m.status
	if m.syncing {
		right = "Syncing..."
	}

	left := helpStyle.Render(keys)
	if right == "" {
		return left
	}
	return left + "   " + dimStyle.Render(right)
}

// truncStr truncates s to at most n runes, appending "…" if truncated.
func truncStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

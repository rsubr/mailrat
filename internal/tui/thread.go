package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/storage"
)

func (m model) updateThread(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "u":
			m.nav.view = viewInbox
			return m, nil
		case "j", "down", "n":
			if m.thread.cursor < len(m.thread.messages)-1 {
				m.thread.cursor++
			}
		case "k", "up", "p":
			if m.thread.cursor > 0 {
				m.thread.cursor--
			}
		case "enter", "o":
			if len(m.thread.messages) == 0 {
				return m, nil
			}
			sel := m.thread.messages[m.thread.cursor]
			return m, loadMessageCmd(m.store, m.accMgr, sel.AccountID, sel.ID)
		case "r":
			if len(m.thread.messages) == 0 {
				return m, nil
			}
			return m.beginReply(m.thread.messages[m.thread.cursor]), nil
		case "f":
			if len(m.thread.messages) == 0 {
				return m, nil
			}
			return m.beginForward(m.thread.messages[m.thread.cursor]), nil
		case "e", "y":
			if len(m.thread.messages) == 0 {
				return m, nil
			}
			sel := m.thread.messages[m.thread.cursor]
			m.nav.view = viewInbox
			return m, archiveThreadCmd(m.store, m.accMgr, sel.AccountID, sel.ThreadID)
		case "a":
			if len(m.thread.messages) == 0 {
				return m, nil
			}
			return m.beginReplyAll(m.thread.messages[m.thread.cursor]), nil
		case "s":
			return m.unsupportedShortcut("s"), nil
		case "#":
			return m.unsupportedShortcut("#"), nil
		case "!":
			return m.unsupportedShortcut("!"), nil
		}
	}
	return m, nil
}

func (m model) renderThread() string {
	if len(m.thread.messages) == 0 {
		return dimStyle.Render("\n  No messages in thread.")
	}

	// Reserve 1 line for the subject header; messages scroll in the rest.
	maxRows := m.contentHeight() - 1
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if m.thread.cursor >= maxRows {
		start = m.thread.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.thread.messages) {
		end = len(m.thread.messages)
	}

	subject := truncStr(m.thread.messages[0].Subject, m.layout.width-30)
	header := dimStyle.Render(fmt.Sprintf("  %s  (%d messages)", subject, len(m.thread.messages)))

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')

	for i := start; i < end; i++ {
		sb.WriteString(m.renderThreadMsg(m.thread.messages[i], i, i == m.thread.cursor))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m model) renderThreadMsg(msg storage.Message, idx int, selected bool) string {
	// Root message (idx=0) is flush; every reply is indented with └.
	var indent string
	if idx == 0 {
		indent = "  "
	} else {
		indent = dimStyle.Render("  └ ")
	}

	dot := "  "
	if msg.IsUnread {
		dot = unreadDotStyle.Render("• ")
	}

	date := time.UnixMilli(msg.InternalDate).Format("Jan 02 15:04")
	from := truncStr(shortAddr(msg.FromAddr), 24)
	snippet := truncStr(msg.Snippet, 40)

	line := fmt.Sprintf("%s%s%-10s  %-24s  %s", indent, dot, date, from, snippet)

	switch {
	case selected:
		return selectedStyle.Width(m.layout.width).Render(line)
	case msg.IsUnread:
		return boldStyle.Render(line)
	default:
		return line
	}
}

// threadParticipants returns a comma-joined list of unique display names from
// the messages in a thread (used in other views that need a summary string).
func threadParticipants(msgs []storage.Message) string {
	seen := make(map[string]bool)
	var names []string
	for _, m := range msgs {
		name := shortAddr(m.FromAddr)
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

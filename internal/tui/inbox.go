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
			if m.cursor < len(m.threads)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if len(m.threads) == 0 {
				return m, nil
			}
			sel := m.threads[m.cursor]
			return m, loadThreadCmd(m.store, sel.AccountID, sel.ThreadID)
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
	if len(m.threads) == 0 {
		return dimStyle.Render("\n  No messages. Press 's' to sync.")
	}

	maxRows := m.contentHeight()
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.threads) {
		end = len(m.threads)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(m.renderThreadRow(m.threads[i], i == m.cursor))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m model) renderThreadRow(t storage.ThreadSummary, selected bool) string {
	// Unread indicator.
	dot := "  "
	if t.UnreadCount > 0 {
		dot = unreadDotStyle.Render("• ")
	}

	// Attachment indicator.
	clip := "  "
	if t.HasAttachments {
		clip = accentStyle.Render("⊕ ")
	}

	date := time.UnixMilli(t.LastDate).Format("Jan 02 15:04")

	// Participants: collapse duplicates, show at most two names, add count.
	participants := formatParticipants(t.Participants, t.MessageCount)

	// Message count badge (only for threads with more than one message).
	countBadge := ""
	if t.MessageCount > 1 {
		countBadge = dimStyle.Render(fmt.Sprintf("[%d]", t.MessageCount))
	}

	subject := truncStr(t.Subject, 36)
	snippet := truncStr(t.Snippet, 26)

	// Fixed-width columns; count badge is appended inline after participants.
	from := truncStr(participants, 22)
	line := fmt.Sprintf("%s%-10s  %-22s%s  %s%-36s  %s",
		dot, date, from,
		// pad count badge to fixed 4 chars so subject column stays aligned
		padRight(countBadge, 4),
		clip, subject, snippet,
	)

	switch {
	case selected:
		return selectedStyle.Width(m.width).Render(line)
	case t.UnreadCount > 0:
		return boldStyle.Render(line)
	default:
		return line
	}
}

// formatParticipants builds a short participant string from the comma-separated
// GROUP_CONCAT result stored in ThreadSummary.Participants.
func formatParticipants(raw string, msgCount int) string {
	if raw == "" {
		return ""
	}

	// Split and deduplicate while preserving first-seen order.
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	var unique []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			unique = append(unique, shortAddr(p))
		}
	}

	switch len(unique) {
	case 0:
		return ""
	case 1:
		return unique[0]
	case 2:
		return unique[0] + ", " + unique[1]
	default:
		return unique[0] + ", " + unique[1] + ", …"
	}
}

// shortAddr extracts the display name from "Name <email>" or returns the local
// part of a bare email address.
func shortAddr(addr string) string {
	if i := strings.Index(addr, "<"); i > 0 {
		return strings.TrimSpace(addr[:i])
	}
	if i := strings.Index(addr, "@"); i > 0 {
		return addr[:i]
	}
	return addr
}

// padRight pads s to width chars using spaces.  If s is already wider (due to
// ANSI escape codes from lipgloss), we just return s unchanged.
func padRight(s string, width int) string {
	// Strip ANSI for length measurement isn't trivial; use a fixed pad instead.
	if s == "" {
		return strings.Repeat(" ", width)
	}
	return s + " "
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
		keys = "j/k:move  enter:open thread  c:compose  s:sync  a:accounts  q:quit"
	case viewThread:
		keys = "j/k:move  enter:read  esc:back"
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

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
		case "j", "down", "n":
			if m.inbox.cursor < len(m.inbox.threads)-1 {
				m.inbox.cursor++
			}
		case "k", "up", "p":
			if m.inbox.cursor > 0 {
				m.inbox.cursor--
			}
		case "enter", "o":
			if len(m.inbox.threads) == 0 {
				return m, nil
			}
			sel := m.inbox.threads[m.inbox.cursor]
			return m, loadThreadCmd(m.store, sel.AccountID, sel.ThreadID)
		case "S":
			if !m.status.syncing && len(m.nav.allAccounts) > 0 {
				m.status.syncing = true
				m.status.text = "Syncing..."
				return m, syncAllCmd(m.store, m.accMgr, m.nav.allAccounts)
			}
		case "c":
			m = m.beginCompose()
			return m, nil
		case "A":
			m.nav.view = viewAccounts
			m.account.cursor = 0
			return m, nil
		case "e", "y":
			if len(m.inbox.threads) == 0 {
				return m, nil
			}
			sel := m.inbox.threads[m.inbox.cursor]
			return m, archiveThreadCmd(m.store, m.accMgr, sel.AccountID, sel.ThreadID)
		case "a":
			return m.unsupportedShortcut("a"), nil
		case "?":
			acctID := m.resolveAcctID()
			if acctID == "" {
				m.status.text = "No account configured"
				return m, nil
			}
			return m, loadConfigCmd(m.store, acctID)
		case "/":
			return m.unsupportedShortcut("/"), nil
		case "x":
			return m.unsupportedShortcut("x"), nil
		case "s":
			return m.unsupportedShortcut("s"), nil
		case "#":
			return m.unsupportedShortcut("#"), nil
		case "!":
			return m.unsupportedShortcut("!"), nil
		case "u":
			m.account.cursor = 0
			m.nav.view = viewAccounts
			return m, nil
		}
	}
	return m, nil
}

func (m model) renderInbox() string {
	if len(m.inbox.threads) == 0 {
		return dimStyle.Render("\n  No messages. Press 's' to sync.")
	}

	maxRows := m.contentHeight()
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if m.inbox.cursor >= maxRows {
		start = m.inbox.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.inbox.threads) {
		end = len(m.inbox.threads)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(m.renderThreadRow(m.inbox.threads[i], i == m.inbox.cursor))
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
		return selectedStyle.Width(m.layout.width).Render(line)
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
	case m.nav.activeAcct != "":
		for _, a := range m.nav.allAccounts {
			if a.ID == m.nav.activeAcct {
				title = "mailrat — " + a.Email
				break
			}
		}
	case len(m.nav.allAccounts) > 1:
		title = "mailrat — unified inbox"
	case len(m.nav.allAccounts) == 1:
		title = "mailrat — " + m.nav.allAccounts[0].Email
	}
	return headerStyle.Width(m.layout.width).Render(title)
}

func (m model) renderHelp() string {
	var keys string
	switch m.nav.view {
	case viewInbox:
		keys = "j/k,n/p:move  enter/o:open  c:compose  e/y:archive  A:accounts  S:sync  ?:config"
	case viewThread:
		keys = "j/k,n/p:move  enter/o:read  r:reply  f:forward  e/y:archive  u:back"
	case viewMessage:
		keys = "↑/↓/pgup/pgdn:scroll  r:reply  f:forward  Shift+I:unread  e/y:archive  u:back"
	case viewCompose:
		keys = "tab:next field  ctrl+s/ctrl+enter:send  esc:cancel"
	case viewAccounts:
		keys = "j/k:move  enter:select  esc:back"
	case viewConfig:
		keys = "j/k:move  d:cycle days  a:toggle attachments  esc:save & close"
	}

	right := m.status.text
	if m.status.syncing {
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

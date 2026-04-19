package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/storage"
)

func (m model) updateMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			m.status.quitting = true
			return m, nil
		case "esc", "u":
			if len(m.thread.messages) > 0 {
				m.nav.view = viewThread
			} else {
				m.nav.view = viewInbox
			}
			m.reader.ready = false
			return m, nil
		case "r":
			return m.beginReply(m.reader.current), nil
		case "f":
			return m.beginForward(m.reader.current), nil
		case "I":
			return m, markUnreadCmd(m.store, m.accMgr, m.reader.current.AccountID, m.reader.current.ID)
		case "e", "y":
			m.nav.view = viewInbox
			return m, archiveThreadCmd(m.store, m.accMgr, m.reader.current.AccountID, m.reader.current.ThreadID)
		case "a":
			return m.beginReplyAll(m.reader.current), nil
		case "s":
			return m, toggleStarCmd(m.store, m.accMgr, m.reader.current.AccountID, m.reader.current.ID)
		case "#", "d":
			m.nav.view = viewInbox
			return m, trashMessageCmd(m.store, m.accMgr, m.reader.current.AccountID, m.reader.current.ID)
		case "!":
			return m.unsupportedShortcut("!"), nil
		}
	}

	// Forward all other messages to the viewport for scrolling.
	if m.reader.ready {
		var cmd tea.Cmd
		m.reader.viewport, cmd = m.reader.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) renderMessage() string {
	if !m.reader.ready {
		return dimStyle.Render("\n  Loading message...")
	}
	return m.reader.viewport.View()
}

// formatMessageBody builds the full text of a message for display in the viewport.
func formatMessageBody(msg storage.Message, attachments []storage.Attachment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("From:    %s\n", msg.FromAddr))
	sb.WriteString(fmt.Sprintf("To:      %s\n", msg.ToAddr))
	sb.WriteString(fmt.Sprintf("Date:    %s\n", formatHeaderDate(msg.InternalDate)))
	sb.WriteString(fmt.Sprintf("Subject: %s\n", msg.Subject))

	if len(attachments) > 0 {
		sb.WriteString(fmt.Sprintf("Attach:  %s\n", formatAttachmentList(attachments)))
	}

	sb.WriteString(separatorStyle.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n\n")

	body := msg.Body
	if body == "" {
		if msg.CacheState == storage.CacheMetadataOnly {
			body = dimStyle.Render("[ Body not available offline — connect and reopen to load ]")
			if msg.Snippet != "" {
				body += "\n\n" + msg.Snippet
			}
		} else {
			body = msg.Snippet
		}
	}
	if body == "" {
		body = "(no body)"
	}

	sb.WriteString(stripHTML(body))
	return sb.String()
}

func formatHeaderDate(ts int64) string {
	return time.UnixMilli(ts).Format("Mon, 02 Jan 2006 15:04:05")
}

// formatAttachmentList formats attachments as a compact comma-separated list.
func formatAttachmentList(attachments []storage.Attachment) string {
	parts := make([]string, len(attachments))
	for i, a := range attachments {
		parts[i] = fmt.Sprintf("%s (%s)", a.Name, formatSize(a.Size))
	}
	return strings.Join(parts, ",  ")
}

// formatSize formats a byte count as a human-readable string.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// stripHTML removes HTML tags and decodes common entities for terminal display.
func stripHTML(s string) string {
	// Remove tags.
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(r)
		}
	}

	// Decode a handful of common HTML entities.
	out := buf.String()
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
		"&mdash;", "—",
		"&ndash;", "–",
	)
	out = replacer.Replace(out)

	// Collapse runs of more than two consecutive blank lines.
	lines := strings.Split(out, "\n")
	result := make([]string, 0, len(lines))
	blanks := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blanks++
			if blanks <= 2 {
				result = append(result, line)
			}
		} else {
			blanks = 0
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

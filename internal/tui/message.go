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
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if len(m.threadMsgs) > 0 {
				m.view = viewThread
			} else {
				m.view = viewInbox
			}
			m.vpReady = false
			return m, nil
		case "r":
			replyTo := m.message
			m.replyTo = &replyTo
			m.composeFocus = 2 // start in body; To+Subject pre-filled
			m.composeTo.SetValue(m.message.FromAddr)
			m.composeSubject.SetValue("Re: " + strings.TrimPrefix(m.message.Subject, "Re: "))
			m.composeTo.Blur()
			m.composeSubject.Blur()
			m.composeBody.Focus()
			m.view = viewCompose
			return m, nil
		}
	}

	// Forward all other messages to the viewport for scrolling.
	if m.vpReady {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) renderMessage() string {
	if !m.vpReady {
		return dimStyle.Render("\n  Loading message...")
	}
	return m.viewport.View()
}

// formatMessageBody builds the full text of a message for display in the viewport.
func formatMessageBody(msg storage.Message, attachments []storage.Attachment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("From:    %s\n", msg.FromAddr))
	sb.WriteString(fmt.Sprintf("To:      %s\n", msg.ToAddr))
	sb.WriteString(fmt.Sprintf("Date:    %s\n",
		time.UnixMilli(msg.InternalDate).Format("Mon, 02 Jan 2006 15:04:05")))
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

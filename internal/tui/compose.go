package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/mail"
)

func (m model) updateCompose(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.view = viewInbox
			m = clearCompose(m)
			return m, nil
		case "tab":
			// Cycle focus: To → Subject → Body → To
			m.composeFocus = (m.composeFocus + 1) % 3
			m = applyComposeFocus(m)
			return m, nil
		case "ctrl+s":
			return m.submitCompose()
		}
	}

	// Route key/other events to the focused field.
	var cmd tea.Cmd
	switch m.composeFocus {
	case 0:
		m.composeTo, cmd = m.composeTo.Update(msg)
	case 1:
		m.composeSubject, cmd = m.composeSubject.Update(msg)
	case 2:
		m.composeBody, cmd = m.composeBody.Update(msg)
	}
	return m, cmd
}

func applyComposeFocus(m model) model {
	switch m.composeFocus {
	case 0:
		m.composeTo.Focus()
		m.composeSubject.Blur()
		m.composeBody.Blur()
	case 1:
		m.composeTo.Blur()
		m.composeSubject.Focus()
		m.composeBody.Blur()
	case 2:
		m.composeTo.Blur()
		m.composeSubject.Blur()
		m.composeBody.Focus()
	}
	return m
}

func (m model) submitCompose() (tea.Model, tea.Cmd) {
	to := strings.TrimSpace(m.composeTo.Value())
	subject := strings.TrimSpace(m.composeSubject.Value())
	body := strings.TrimSpace(m.composeBody.Value())

	if to == "" {
		m.status = "To: field is required"
		return m, nil
	}

	// Pick account: active account or first available.
	acctID := m.activeAcct
	if acctID == "" && len(m.allAccounts) > 0 {
		acctID = m.allAccounts[0].ID
	}
	if acctID == "" {
		m.status = "No account configured"
		return m, nil
	}

	var fromEmail string
	for _, a := range m.allAccounts {
		if a.ID == acctID {
			fromEmail = a.Email
			break
		}
	}

	outMsg := mail.OutboundMessage{
		From:    fromEmail,
		To:      to,
		Subject: subject,
		Body:    body,
	}
	if m.replyTo != nil {
		outMsg.InReplyTo = m.replyTo.ID
		outMsg.ThreadID = m.replyTo.ThreadID
	}

	m.status = "Sending..."
	return m, sendEmailCmd(m.accMgr, acctID, outMsg)
}

func (m model) renderCompose() string {
	title := "New Message"
	if m.replyTo != nil {
		title = fmt.Sprintf("Reply to %s", truncStr(m.replyTo.FromAddr, 40))
	}

	labelStyle := dimStyle
	focusedLabelStyle := accentStyle

	var sb strings.Builder
	sb.WriteString("  " + boldStyle.Render(title) + "\n\n")

	// To
	toLabel := "  To:      "
	if m.composeFocus == 0 {
		toLabel = focusedLabelStyle.Render(toLabel)
	} else {
		toLabel = labelStyle.Render(toLabel)
	}
	sb.WriteString(toLabel + m.composeTo.View() + "\n")

	// Subject
	subjLabel := "  Subject: "
	if m.composeFocus == 1 {
		subjLabel = focusedLabelStyle.Render(subjLabel)
	} else {
		subjLabel = labelStyle.Render(subjLabel)
	}
	sb.WriteString(subjLabel + m.composeSubject.View() + "\n")

	// Separator
	sep := separatorStyle.Render("  " + strings.Repeat("─", m.width-4))
	sb.WriteString(sep + "\n")

	// Body
	bodyLabel := "  Body:\n"
	if m.composeFocus == 2 {
		bodyLabel = focusedLabelStyle.Render(bodyLabel)
	} else {
		bodyLabel = labelStyle.Render(bodyLabel)
	}
	sb.WriteString(bodyLabel)
	sb.WriteString(m.composeBody.View())

	return sb.String()
}

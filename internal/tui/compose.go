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
			m.nav.view = viewInbox
			m = clearCompose(m)
			return m, nil
		case "tab":
			// Cycle focus: To → Subject → Body → To
			m.compose.focus = (m.compose.focus + 1) % 3
			m = applyComposeFocus(m)
			return m, nil
		case "ctrl+s", "ctrl+enter":
			return m.submitCompose()
		}
	}

	// Route key/other events to the focused field.
	var cmd tea.Cmd
	switch m.compose.focus {
	case composeFocusTo:
		m.compose.to, cmd = m.compose.to.Update(msg)
	case composeFocusSubject:
		m.compose.subject, cmd = m.compose.subject.Update(msg)
	case composeFocusBody:
		m.compose.body, cmd = m.compose.body.Update(msg)
	}
	return m, cmd
}

func applyComposeFocus(m model) model {
	switch m.compose.focus {
	case composeFocusTo:
		m.compose.to.Focus()
		m.compose.subject.Blur()
		m.compose.body.Blur()
	case composeFocusSubject:
		m.compose.to.Blur()
		m.compose.subject.Focus()
		m.compose.body.Blur()
	case composeFocusBody:
		m.compose.to.Blur()
		m.compose.subject.Blur()
		m.compose.body.Focus()
	}
	return m
}

func (m model) submitCompose() (tea.Model, tea.Cmd) {
	to := strings.TrimSpace(m.compose.to.Value())
	subject := strings.TrimSpace(m.compose.subject.Value())
	body := strings.TrimSpace(m.compose.body.Value())

	if to == "" {
		m.status.text = "To: field is required"
		return m, nil
	}

	// Pick account: active account or first available.
	acctID := m.nav.activeAcct
	if acctID == "" && len(m.nav.allAccounts) > 0 {
		acctID = m.nav.allAccounts[0].ID
	}
	if acctID == "" {
		m.status.text = "No account configured"
		return m, nil
	}

	var fromEmail string
	for _, a := range m.nav.allAccounts {
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
	if m.compose.replyTo != nil {
		outMsg.InReplyTo = m.compose.replyTo.ID
		outMsg.ThreadID = m.compose.replyTo.ThreadID
	}

	m.status.text = "Sending..."
	return m, sendEmailCmd(m.accMgr, acctID, outMsg)
}

func (m model) renderCompose() string {
	title := "New Message"
	if m.compose.replyAll && m.compose.replyTo != nil {
		title = fmt.Sprintf("Reply All to %s", truncStr(m.compose.replyTo.FromAddr, 40))
	} else if m.compose.replyTo != nil {
		title = fmt.Sprintf("Reply to %s", truncStr(m.compose.replyTo.FromAddr, 40))
	}

	labelStyle := dimStyle
	focusedLabelStyle := accentStyle

	var sb strings.Builder
	sb.WriteString("  " + boldStyle.Render(title) + "\n\n")

	// To
	toLabel := "  To:      "
	if m.compose.focus == composeFocusTo {
		toLabel = focusedLabelStyle.Render(toLabel)
	} else {
		toLabel = labelStyle.Render(toLabel)
	}
	sb.WriteString(toLabel + m.compose.to.View() + "\n")

	// Subject
	subjLabel := "  Subject: "
	if m.compose.focus == composeFocusSubject {
		subjLabel = focusedLabelStyle.Render(subjLabel)
	} else {
		subjLabel = labelStyle.Render(subjLabel)
	}
	sb.WriteString(subjLabel + m.compose.subject.View() + "\n")

	// Separator
	sep := separatorStyle.Render("  " + strings.Repeat("─", m.layout.width-4))
	sb.WriteString(sep + "\n")

	// Body
	bodyLabel := "  Body:\n"
	if m.compose.focus == composeFocusBody {
		bodyLabel = focusedLabelStyle.Render(bodyLabel)
	} else {
		bodyLabel = labelStyle.Render(bodyLabel)
	}
	sb.WriteString(bodyLabel)
	sb.WriteString(m.compose.body.View())

	return sb.String()
}

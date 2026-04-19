package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/mail"
)

const numComposeFocusFields = 5

func renderLabeledField(label string, focused bool, fieldView string) string {
	if focused {
		return accentStyle.Render(label) + fieldView + "\n"
	}
	return dimStyle.Render(label) + fieldView + "\n"
}

func (m model) updateCompose(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.nav.view = viewInbox
			m = clearCompose(m)
			return m, nil
		case "tab":
			// Cycle focus: To → Cc → Bcc → Subject → Body → To
			m.compose.focus = (m.compose.focus + 1) % numComposeFocusFields
			m = applyComposeFocus(m)
			return m, nil
		case "ctrl+s":
			return m.saveDraft()
		case "ctrl+enter":
			return m.submitCompose()
		}
	}

	// Route key/other events to the focused field.
	var cmd tea.Cmd
	switch m.compose.focus {
	case composeFocusTo:
		m.compose.to, cmd = m.compose.to.Update(msg)
	case composeFocusCc:
		m.compose.cc, cmd = m.compose.cc.Update(msg)
	case composeFocusBcc:
		m.compose.bcc, cmd = m.compose.bcc.Update(msg)
	case composeFocusSubject:
		m.compose.subject, cmd = m.compose.subject.Update(msg)
	case composeFocusBody:
		m.compose.body, cmd = m.compose.body.Update(msg)
	}
	return m, cmd
}

func applyComposeFocus(m model) model {
	m.compose.to.Blur()
	m.compose.cc.Blur()
	m.compose.bcc.Blur()
	m.compose.subject.Blur()
	m.compose.body.Blur()

	switch m.compose.focus {
	case composeFocusTo:
		m.compose.to.Focus()
	case composeFocusCc:
		m.compose.cc.Focus()
	case composeFocusBcc:
		m.compose.bcc.Focus()
	case composeFocusSubject:
		m.compose.subject.Focus()
	case composeFocusBody:
		m.compose.body.Focus()
	}
	return m
}

func (m model) buildOutbound() (string, mail.OutboundMessage, error) {
	to := strings.TrimSpace(m.compose.to.Value())
	cc := strings.TrimSpace(m.compose.cc.Value())
	bcc := strings.TrimSpace(m.compose.bcc.Value())
	subject := strings.TrimSpace(m.compose.subject.Value())
	body := strings.TrimSpace(m.compose.body.Value())

	// Pick account: active account or first available.
	acctID := m.nav.activeAcct
	if acctID == "" && len(m.nav.allAccounts) > 0 {
		acctID = m.nav.allAccounts[0].ID
	}
	if acctID == "" {
		return "", mail.OutboundMessage{}, fmt.Errorf("no account configured")
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
		Cc:      cc,
		Bcc:     bcc,
		Subject: subject,
		Body:    body,
	}
	if m.compose.replyTo != nil {
		outMsg.InReplyTo = m.compose.replyTo.MessageID
		outMsg.ThreadID = m.compose.replyTo.ThreadID
	}
	return acctID, outMsg, nil
}

func (m model) submitCompose() (tea.Model, tea.Cmd) {
	acctID, outMsg, err := m.buildOutbound()
	if err != nil {
		m.status.text = err.Error()
		return m, nil
	}

	if outMsg.To == "" {
		m.status.text = "To: field is required"
		return m, nil
	}

	m.status.text = "Sending..."
	return m, sendEmailCmd(m.accMgr, m.store, acctID, outMsg, m.compose.draftID)
}

func (m model) saveDraft() (tea.Model, tea.Cmd) {
	acctID, outMsg, err := m.buildOutbound()
	if err != nil {
		m.status.text = err.Error()
		return m, nil
	}

	m.status.text = "Saving draft..."
	return m, saveDraftCmd(m.accMgr, m.store, acctID, outMsg, m.compose.draftID)
}

func (m model) handleSaveDraftDone(msg msgSaveDraftDone) (model, tea.Cmd) {
	if msg.err != nil {
		m.status.text = "Save draft error: " + msg.err.Error()
		return m, nil
	}

	m.status.text = "Draft saved"
	m.compose.draftID = msg.draftID
	return m, nil
}

func (m model) renderCompose() string {
	title := "New Message"
	if m.compose.replyAll && m.compose.replyTo != nil {
		title = fmt.Sprintf("Reply All to %s", truncStr(m.compose.replyTo.FromAddr, 40))
	} else if m.compose.replyTo != nil {
		title = fmt.Sprintf("Reply to %s", truncStr(m.compose.replyTo.FromAddr, 40))
	}

	var sb strings.Builder
	sb.WriteString("  " + boldStyle.Render(title) + "\n\n")

	sb.WriteString(renderLabeledField("  To:      ", m.compose.focus == composeFocusTo, m.compose.to.View()))
	sb.WriteString(renderLabeledField("  Cc:      ", m.compose.focus == composeFocusCc, m.compose.cc.View()))
	sb.WriteString(renderLabeledField("  Bcc:     ", m.compose.focus == composeFocusBcc, m.compose.bcc.View()))
	sb.WriteString(renderLabeledField("  Subject: ", m.compose.focus == composeFocusSubject, m.compose.subject.View()))

	// Separator
	sep := separatorStyle.Render("  " + strings.Repeat("─", m.layout.width-4))
	sb.WriteString(sep + "\n")

	// Body label (no trailing newline from helper; body view follows directly)
	bodyLabel := "  Body:\n"
	if m.compose.focus == composeFocusBody {
		bodyLabel = accentStyle.Render(bodyLabel)
	} else {
		bodyLabel = dimStyle.Render(bodyLabel)
	}
	sb.WriteString(bodyLabel)
	sb.WriteString(m.compose.body.View())

	return sb.String()
}

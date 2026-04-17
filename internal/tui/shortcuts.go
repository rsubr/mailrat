package tui

import (
	"strings"

	"github.com/mailrat/mailrat/internal/storage"
)

func gmailShortcutLabel(key string) string {
	switch key {
	case "a":
		return "Reply all"
	case "x":
		return "Select conversation"
	case "#":
		return "Delete"
	case "!":
		return "Report spam"
	case "s":
		return "Star"
	case "/":
		return "Search"
	case "g":
		return "Go to"
	default:
		return strings.ToUpper(key)
	}
}

func (m model) unsupportedShortcut(key string) model {
	m.status.text = gmailShortcutLabel(key) + " is not implemented yet"
	return m
}

func (m model) beginCompose() model {
	m.nav.view = viewCompose
	m.compose.replyTo = nil
	m.compose.focus = composeFocusTo
	m.compose.to.Focus()
	m.compose.subject.Blur()
	m.compose.body.Blur()
	return m
}

func (m model) beginReply(msg storage.Message) model {
	replyTo := msg
	m.compose.replyTo = &replyTo
	m.compose.focus = composeFocusBody
	m.compose.to.SetValue(msg.FromAddr)
	m.compose.subject.SetValue(replySubject(msg.Subject))
	m.compose.body.SetValue("")
	m.compose.to.Blur()
	m.compose.subject.Blur()
	m.compose.body.Focus()
	m.nav.view = viewCompose
	return m
}

func (m model) beginForward(msg storage.Message) model {
	m.compose.replyTo = nil
	m.compose.focus = composeFocusTo
	m.compose.to.SetValue("")
	m.compose.subject.SetValue(forwardSubject(msg.Subject))
	m.compose.body.SetValue(quoteBody(msg))
	m.compose.to.Focus()
	m.compose.subject.Blur()
	m.compose.body.Blur()
	m.nav.view = viewCompose
	return m
}

func replySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func forwardSubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "fwd:") || strings.HasPrefix(strings.ToLower(subject), "fw:") {
		return subject
	}
	return "Fwd: " + subject
}

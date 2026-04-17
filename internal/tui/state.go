package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mailrat/mailrat/internal/storage"
)

const (
	composeFocusTo = iota
	composeFocusSubject
	composeFocusBody
)

type navigationState struct {
	view        viewState
	allAccounts []storage.Account
	activeAcct  string // "" = unified inbox
}

type inboxState struct {
	threads []storage.ThreadSummary
	cursor  int
}

type threadState struct {
	messages []storage.Message
	cursor   int
}

type messageState struct {
	current     storage.Message
	attachments []storage.Attachment
	viewport    viewport.Model
	ready       bool
}

type composeState struct {
	to       textinput.Model
	subject  textinput.Model
	body     textarea.Model
	focus    int
	replyTo  *storage.Message
	replyAll bool
}

type searchState struct {
	input        textinput.Model
	results      []storage.ThreadSummary
	cursor       int
	inputFocused bool
}

type accountListState struct {
	cursor int
}

type configState struct {
	rows   []configRow
	cursor int
}

type statusState struct {
	text     string
	syncing  bool
	quitting bool
}

type layoutState struct {
	width  int
	height int
}

package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/storage"
)

type viewState int

const (
	viewInbox viewState = iota
	viewThread
	viewMessage
	viewCompose
	viewAccounts
	viewConfig
	viewSearch
)

type model struct {
	store  *storage.Store
	accMgr *accounts.Manager

	nav     navigationState
	inbox   inboxState
	thread  threadState
	reader  messageState
	compose composeState
	search  searchState
	account accountListState
	config  configState
	status  statusState
	layout  layoutState
}

// New creates the root TUI model. Call tea.NewProgram(tui.New(...)).
func New(store *storage.Store, accMgr *accounts.Manager) model {
	to := textinput.New()
	to.Placeholder = "recipient@example.com"
	to.CharLimit = 200

	subj := textinput.New()
	subj.Placeholder = "Subject"
	subj.CharLimit = 200

	body := textarea.New()
	body.Placeholder = "Write your message..."
	body.SetHeight(12)
	body.CharLimit = 0 // unlimited

	searchInput := textinput.New()
	searchInput.Placeholder = "Search mail..."
	searchInput.CharLimit = 200

	return model{
		store:  store,
		accMgr: accMgr,
		compose: composeState{
			to:      to,
			subject: subj,
			body:    body,
		},
		search: searchState{
			input: searchInput,
		},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		loadAccountsCmd(m.store),
		loadInboxCmd(m.store, m.nav.activeAcct),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		return m.handleWindowSize(sizeMsg)
	}

	if next, cmd, handled := m.handleRootMessage(msg); handled {
		return next, cmd
	}

	return m.updateActiveView(msg)
}

func (m model) View() string {
	if m.layout.width == 0 {
		return "Loading..."
	}

	return m.renderHeader() + "\n" + m.renderCurrentView() + "\n" + m.renderHelp()
}

// contentHeight is the number of lines available for the main content area.
// Header (1) + separator newline (1) + help (1) + separator newline (1) = 4.
func (m model) contentHeight() int {
	if m.layout.height < 4 {
		return 1
	}
	return m.layout.height - 4
}

// resolveAcctID returns the active account ID, falling back to the first
// available account when viewing the unified inbox.
func (m model) resolveAcctID() string {
	if m.nav.activeAcct != "" {
		return m.nav.activeAcct
	}
	if len(m.nav.allAccounts) > 0 {
		return m.nav.allAccounts[0].ID
	}
	return ""
}

// clearCompose resets the compose form fields.
func clearCompose(m model) model {
	m.compose.to.Reset()
	m.compose.subject.Reset()
	m.compose.body.Reset()
	m.compose.focus = composeFocusTo
	m.compose.replyTo = nil
	m.compose.replyAll = false
	m.compose.to.Blur()
	m.compose.subject.Blur()
	m.compose.body.Blur()
	return m
}

package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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
)

type model struct {
	store  *storage.Store
	accMgr *accounts.Manager

	// navigation
	view        viewState
	allAccounts []storage.Account
	activeAcct  string // "" = unified inbox

	// inbox (one entry per thread)
	threads []storage.ThreadSummary
	cursor  int

	// thread
	threadMsgs []storage.Message
	threadCur  int

	// message reader
	message     storage.Message
	attachments []storage.Attachment
	viewport    viewport.Model
	vpReady     bool

	// compose
	composeTo      textinput.Model
	composeSubject textinput.Model
	composeBody    textarea.Model
	composeFocus   int // 0=To, 1=Subject, 2=Body
	replyTo        *storage.Message

	// account switcher
	acctCursor int

	// sync config modal
	configRows   []configRow
	configCursor int

	// status
	status  string
	syncing bool

	// terminal dimensions
	width  int
	height int
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

	return model{
		store:          store,
		accMgr:         accMgr,
		composeTo:      to,
		composeSubject: subj,
		composeBody:    body,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		loadAccountsCmd(m.store),
		loadInboxCmd(m.store, m.activeAcct),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.vpReady {
			m.viewport.Width = msg.Width
			m.viewport.Height = m.contentHeight()
		}
		return m, nil

	case msgAccountsLoaded:
		m.allAccounts = msg.accounts
		return m, nil

	case msgInboxLoaded:
		m.threads = msg.threads
		if m.cursor >= len(m.threads) && len(m.threads) > 0 {
			m.cursor = len(m.threads) - 1
		}
		if msg.err != nil {
			m.status = "Error loading inbox: " + msg.err.Error()
		}
		return m, nil

	case msgThreadLoaded:
		if msg.err != nil {
			m.status = "Error loading thread: " + msg.err.Error()
			return m, nil
		}
		m.threadMsgs = msg.msgs
		m.threadCur = 0
		m.view = viewThread
		return m, nil

	case msgMessageLoaded:
		if msg.err != nil {
			m.status = "Error loading message: " + msg.err.Error()
			return m, nil
		}
		m.message = msg.msg
		m.attachments = msg.attachments
		m.viewport = viewport.New(m.width, m.contentHeight())
		m.viewport.SetContent(formatMessageBody(msg.msg, msg.attachments))
		m.vpReady = true
		m.view = viewMessage
		// Mark as read: optimistic local update + background API call (queued if offline).
		var markCmd tea.Cmd
		if msg.msg.IsUnread {
			markCmd = markReadCmd(m.store, m.accMgr, msg.msg.AccountID, msg.msg.ID)
		}
		return m, markCmd

	case msgMarkReadDone:
		// Decrement the thread's unread count in the inbox list.
		threadID := m.message.ThreadID
		for i, t := range m.threads {
			if t.ThreadID == threadID && t.AccountID == msg.acctID {
				if m.threads[i].UnreadCount > 0 {
					m.threads[i].UnreadCount--
				}
				break
			}
		}
		// Clear the flag on the individual message in the thread view.
		for i, m2 := range m.threadMsgs {
			if m2.ID == msg.messageID {
				m.threadMsgs[i].IsUnread = false
				break
			}
		}
		m.message.IsUnread = false
		return m, nil

	case msgDrainDone:
		if msg.applied > 0 {
			m.status = fmt.Sprintf("%d pending change(s) synced", msg.applied)
		}
		return m, nil

	case msgSyncDone:
		m.syncing = false
		if msg.err != nil {
			m.status = "Sync error: " + msg.err.Error()
			return m, loadInboxCmd(m.store, m.activeAcct)
		}
		m.status = "Sync complete"
		return m, tea.Batch(
			loadInboxCmd(m.store, m.activeAcct),
			drainPendingOpsCmd(m.store, m.accMgr),
		)

	case msgSendDone:
		if msg.err != nil {
			m.status = "Send error: " + msg.err.Error()
			return m, nil
		}
		m.status = "Message sent"
		m.view = viewInbox
		m = clearCompose(m)
		return m, nil

	case msgConfigLoaded:
		if msg.err != nil {
			m.status = "Config load error: " + msg.err.Error()
			return m, nil
		}
		m.configRows = msg.rows
		m.configCursor = 0
		m.view = viewConfig
		return m, nil

	case msgConfigSaved:
		if msg.err != nil {
			m.status = "Config save error: " + msg.err.Error()
		} else {
			m.status = "Sync config saved"
		}
		m.view = viewInbox
		return m, nil
	}

	switch m.view {
	case viewInbox:
		return m.updateInbox(msg)
	case viewThread:
		return m.updateThread(msg)
	case viewMessage:
		return m.updateMessage(msg)
	case viewCompose:
		return m.updateCompose(msg)
	case viewAccounts:
		return m.updateAccounts(msg)
	case viewConfig:
		return m.updateConfig(msg)
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var content string
	switch m.view {
	case viewInbox:
		content = m.renderInbox()
	case viewThread:
		content = m.renderThread()
	case viewMessage:
		content = m.renderMessage()
	case viewCompose:
		content = m.renderCompose()
	case viewAccounts:
		content = m.renderAccounts()
	case viewConfig:
		content = m.renderConfig()
	default:
		content = m.renderInbox()
	}

	return m.renderHeader() + "\n" + content + "\n" + m.renderHelp()
}

// contentHeight is the number of lines available for the main content area.
// Header (1) + separator newline (1) + help (1) + separator newline (1) = 4.
func (m model) contentHeight() int {
	if m.height < 4 {
		return 1
	}
	return m.height - 4
}

// resolveAcctID returns the active account ID, falling back to the first
// available account when viewing the unified inbox.
func (m model) resolveAcctID() string {
	if m.activeAcct != "" {
		return m.activeAcct
	}
	if len(m.allAccounts) > 0 {
		return m.allAccounts[0].ID
	}
	return ""
}

// clearCompose resets the compose form fields.
func clearCompose(m model) model {
	m.composeTo.Reset()
	m.composeSubject.Reset()
	m.composeBody.Reset()
	m.composeFocus = 0
	m.replyTo = nil
	m.composeTo.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	return m
}

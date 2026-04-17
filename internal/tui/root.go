package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.layout.width = msg.Width
	m.layout.height = msg.Height
	if m.reader.ready {
		m.reader.viewport.Width = msg.Width
		m.reader.viewport.Height = m.contentHeight()
	}
	return m, nil
}

func (m model) handleRootMessage(msg tea.Msg) (model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case msgAccountsLoaded:
		m.nav.allAccounts = msg.accounts
		return m, nil, true
	case msgInboxLoaded:
		m.inbox.threads = msg.threads
		if m.inbox.cursor >= len(m.inbox.threads) && len(m.inbox.threads) > 0 {
			m.inbox.cursor = len(m.inbox.threads) - 1
		}
		if msg.err != nil {
			m.status.text = "Error loading inbox: " + msg.err.Error()
		}
		return m, nil, true
	case msgThreadLoaded:
		return m.handleThreadLoaded(msg), nil, true
	case msgMessageLoaded:
		next, cmd := m.handleMessageLoaded(msg)
		return next, cmd, true
	case msgMarkReadDone:
		return m.handleMarkReadDone(msg), nil, true
	case msgDrainDone:
		if msg.applied > 0 {
			m.status.text = fmt.Sprintf("%d pending change(s) synced", msg.applied)
		}
		return m, nil, true
	case msgSyncDone:
		next, cmd := m.handleSyncDone(msg)
		return next, cmd, true
	case msgSendDone:
		next, cmd := m.handleSendDone(msg)
		return next, cmd, true
	case msgConfigLoaded:
		next, cmd := m.handleConfigLoaded(msg)
		return next, cmd, true
	case msgConfigSaved:
		next, cmd := m.handleConfigSaved(msg)
		return next, cmd, true
	default:
		return m, nil, false
	}
}

func (m model) handleThreadLoaded(msg msgThreadLoaded) model {
	if msg.err != nil {
		m.status.text = "Error loading thread: " + msg.err.Error()
		return m
	}
	m.thread.messages = msg.msgs
	m.thread.cursor = 0
	m.nav.view = viewThread
	return m
}

func (m model) handleMessageLoaded(msg msgMessageLoaded) (model, tea.Cmd) {
	if msg.err != nil {
		m.status.text = "Error loading message: " + msg.err.Error()
		return m, nil
	}

	m.reader.current = msg.msg
	m.reader.attachments = msg.attachments
	m.reader.viewport = viewport.New(m.layout.width, m.contentHeight())
	m.reader.viewport.SetContent(formatMessageBody(msg.msg, msg.attachments))
	m.reader.ready = true
	m.nav.view = viewMessage

	if msg.msg.IsUnread {
		return m, markReadCmd(m.store, m.accMgr, msg.msg.AccountID, msg.msg.ID)
	}
	return m, nil
}

func (m model) handleMarkReadDone(msg msgMarkReadDone) model {
	threadID := m.reader.current.ThreadID
	for i, t := range m.inbox.threads {
		if t.ThreadID == threadID && t.AccountID == msg.acctID {
			if m.inbox.threads[i].UnreadCount > 0 {
				m.inbox.threads[i].UnreadCount--
			}
			break
		}
	}

	for i, item := range m.thread.messages {
		if item.ID == msg.messageID {
			m.thread.messages[i].IsUnread = false
			break
		}
	}
	m.reader.current.IsUnread = false
	return m
}

func (m model) handleSyncDone(msg msgSyncDone) (model, tea.Cmd) {
	m.status.syncing = false
	if msg.err != nil {
		m.status.text = "Sync error: " + msg.err.Error()
		return m, loadInboxCmd(m.store, m.nav.activeAcct)
	}

	m.status.text = "Sync complete"
	return m, tea.Batch(
		loadInboxCmd(m.store, m.nav.activeAcct),
		drainPendingOpsCmd(m.store, m.accMgr),
	)
}

func (m model) handleSendDone(msg msgSendDone) (model, tea.Cmd) {
	if msg.err != nil {
		m.status.text = "Send error: " + msg.err.Error()
		return m, nil
	}

	m.status.text = "Message sent"
	m.nav.view = viewInbox
	m = clearCompose(m)
	return m, nil
}

func (m model) handleConfigLoaded(msg msgConfigLoaded) (model, tea.Cmd) {
	if msg.err != nil {
		m.status.text = "Config load error: " + msg.err.Error()
		return m, nil
	}

	m.config.rows = msg.rows
	m.config.cursor = 0
	m.nav.view = viewConfig
	return m, nil
}

func (m model) handleConfigSaved(msg msgConfigSaved) (model, tea.Cmd) {
	if msg.err != nil {
		m.status.text = "Config save error: " + msg.err.Error()
	} else {
		m.status.text = "Sync config saved"
	}
	m.nav.view = viewInbox
	return m, nil
}

func (m model) updateActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.nav.view {
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
	default:
		return m, nil
	}
}

func (m model) renderCurrentView() string {
	switch m.nav.view {
	case viewInbox:
		return m.renderInbox()
	case viewThread:
		return m.renderThread()
	case viewMessage:
		return m.renderMessage()
	case viewCompose:
		return m.renderCompose()
	case viewAccounts:
		return m.renderAccounts()
	case viewConfig:
		return m.renderConfig()
	default:
		return m.renderInbox()
	}
}

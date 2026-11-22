package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// draftInterval is how often a compose window checks for changes to autosave.
var draftInterval = 10 * time.Second

// compose opens a message window prefilled from draft (empty for a new message).
func (u *UI) compose(draft desktop.OutboundMsg) {
	if len(u.accounts) == 0 {
		u.showError(errNoAccounts)
		return
	}
	w := u.app.NewWindow("New Message")
	w.Resize(fyne.NewSize(760, 600))

	accountID := draft.AccountID
	if accountID == "" {
		accountID = u.defaultAccountID()
	}
	var emails []string
	selected := ""
	for _, a := range u.accounts {
		emails = append(emails, a.Email)
		if a.ID == accountID {
			selected = a.Email
		}
	}
	from := widget.NewSelect(emails, func(e string) {
		for _, a := range u.accounts {
			if a.Email == e {
				accountID = a.ID
			}
		}
	})
	from.SetSelected(selected)

	rcpt, mention := newSuggestHost(), newSuggestHost()
	to := u.recipientEntry(rcpt, func() string { return accountID }, "To")
	cc := u.recipientEntry(rcpt, func() string { return accountID }, "Cc")
	bcc := u.recipientEntry(rcpt, func() string { return accountID }, "Bcc")
	to.SetText(draft.To)
	cc.SetText(draft.Cc)
	bcc.SetText(draft.Bcc)
	subject := widget.NewEntry()
	subject.SetText(draft.Subject)
	body := newBodyEntry(&suggester{host: mention})
	body.SetText(draft.Body)
	body.CursorRow, body.CursorColumn = 0, 0
	editor := u.richEditor(w, body, mention, to, cc, func() string { return accountID })

	form := widget.NewForm(
		widget.NewFormItem("From", from),
		widget.NewFormItem("To", to),
		widget.NewFormItem("Cc", cc),
		widget.NewFormItem("Bcc", bcc),
		widget.NewFormItem("Subject", subject),
	)

	send := widget.NewButtonWithIcon("Send", icon("send", fgOnPrimary), nil)
	send.Importance = widget.HighImportance
	// Autosave: every draftInterval, save to local Drafts only if content changed.
	draftID, closed := "", false
	current := func() desktop.OutboundMsg {
		return desktop.OutboundMsg{
			AccountID: accountID, To: to.Text, Cc: cc.Text, Bcc: bcc.Text,
			Subject: subject.Text, Body: markupToHTML(body.Text), InReplyTo: draft.InReplyTo, ThreadID: draft.ThreadID,
		}
	}
	lastSaved := current()
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(draftInterval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				fyne.Do(func() {
					msg := current()
					if closed || msg == lastSaved {
						return
					}
					prev := lastSaved
					lastSaved = msg
					var id string
					var err error
					u.bg(func() { id, err = u.be.SaveDraft(draftID, msg) }, func() {
						if err != nil {
							lastSaved = prev // retry next tick
							return
						}
						draftID = id
						if closed { // closed/sent while the save was in flight
							_ = u.be.DiscardDraft(msg.AccountID, id)
						}
					})
				})
			}
		}
	}()
	finish := func(discardDraft bool) {
		if closed {
			return
		}
		closed = true
		close(stop)
		if discardDraft && draftID != "" {
			id, acc := draftID, accountID
			u.bg(func() { _ = u.be.DiscardDraft(acc, id) }, nil)
		}
	}
	w.SetOnClosed(func() { finish(false) })
	discard := widget.NewButton("Discard", func() { finish(true); w.Close() })
	send.OnTapped = func() {
		if strings.TrimSpace(to.Text) == "" {
			u.showErrorIn(w, errNoRecipient)
			return
		}
		msg := current()
		send.Disable()
		var err error
		u.bg(func() { err = u.be.SendMessage(msg) }, func() {
			if err != nil {
				send.Enable()
				u.showErrorIn(w, err)
				return
			}
			finish(true) // sent mail must not leave a stale draft
			w.Close()
			u.app.SendNotification(fyne.NewNotification("mailrat", "Message sent"))
		})
	}
	// ⌘Enter sends, as in Gmail.
	for _, k := range []fyne.KeyName{fyne.KeyReturn, fyne.KeyEnter} {
		w.Canvas().AddShortcut(cmd(k, false), func(fyne.Shortcut) {
			if !send.Disabled() {
				send.OnTapped()
			}
		})
	}
	buttons := container.NewHBox(discard, widget.NewLabel(""), send)
	w.SetContent(container.NewBorder(container.NewVBox(container.NewPadded(form), rcpt.box), container.NewPadded(container.NewBorder(nil, nil, nil, buttons)), nil, nil, container.NewPadded(editor)))
	w.Show()
	if draft.To == "" {
		w.Canvas().Focus(to)
	} else {
		w.Canvas().Focus(body)
	}
}

func (u *UI) defaultAccountID() string {
	if u.activeAccount != unified {
		return u.activeAccount
	}
	if gs, err := u.be.GetGlobalSettings(); err == nil && gs.DefaultAccountID != "" {
		return gs.DefaultAccountID
	}
	return u.accounts[0].ID
}

// splitRecipient splits a comma-separated address field into the finished head
// (kept verbatim) and the address token being typed.
func splitRecipient(text string) (head, token string) {
	i := strings.LastIndex(text, ",")
	if i < 0 {
		return "", strings.TrimSpace(text)
	}
	return text[:i+1] + " ", strings.TrimSpace(text[i+1:])
}

// recipientEntry is a text entry that suggests contacts for the address being typed.
func (u *UI) recipientEntry(host *suggestHost, account func() string, name string) *suggestEntry {
	sg := &suggester{host: host}
	e := newSuggestEntry(sg)
	e.SetPlaceHolder(name)
	sg.pick = func(c desktop.ContactItem) {
		head, _ := splitRecipient(e.Text)
		e.SetText(head + contactLabel(c) + ", ")
		e.CursorColumn = len([]rune(e.Text))
		e.Refresh()
	}
	e.OnChanged = func(text string) {
		sg.hide()
		if e.quiet {
			return
		}
		_, token := splitRecipient(text)
		if token == "" {
			return
		}
		var found []desktop.ContactItem
		u.bg(func() { found, _ = u.be.SearchContacts(account(), token, 8) }, func() {
			if e.Text == text {
				sg.show(found)
			}
		})
	}
	return e
}

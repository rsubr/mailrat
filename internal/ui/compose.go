package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// compose opens a message window prefilled from draft (empty for a new message).
func (u *UI) compose(draft desktop.OutboundMsg) {
	if len(u.accounts) == 0 {
		u.showError(errNoAccounts)
		return
	}
	w := u.app.NewWindow("New Message")
	w.Resize(fyne.NewSize(680, 560))

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

	to := u.recipientEntry(w, func() string { return accountID }, "To")
	cc := u.recipientEntry(w, func() string { return accountID }, "Cc")
	bcc := u.recipientEntry(w, func() string { return accountID }, "Bcc")
	to.SetText(draft.To)
	cc.SetText(draft.Cc)
	bcc.SetText(draft.Bcc)
	subject := widget.NewEntry()
	subject.SetText(draft.Subject)
	body := widget.NewMultiLineEntry()
	body.Wrapping = fyne.TextWrapWord
	body.SetText(draft.Body)
	body.CursorRow, body.CursorColumn = 0, 0

	form := widget.NewForm(
		widget.NewFormItem("From", from),
		widget.NewFormItem("To", to),
		widget.NewFormItem("Cc", cc),
		widget.NewFormItem("Bcc", bcc),
		widget.NewFormItem("Subject", subject),
	)

	send := widget.NewButtonWithIcon("Send", icon("send", fgOnPrimary), nil)
	send.Importance = widget.HighImportance
	discard := widget.NewButton("Discard", w.Close)
	send.OnTapped = func() {
		if strings.TrimSpace(to.Text) == "" {
			u.showErrorIn(w, errNoRecipient)
			return
		}
		msg := desktop.OutboundMsg{
			AccountID: accountID, To: to.Text, Cc: cc.Text, Bcc: bcc.Text,
			Subject: subject.Text, Body: body.Text, InReplyTo: draft.InReplyTo, ThreadID: draft.ThreadID,
		}
		send.Disable()
		var err error
		u.bg(func() { err = u.be.SendMessage(msg) }, func() {
			if err != nil {
				send.Enable()
				u.showErrorIn(w, err)
				return
			}
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
	w.SetContent(container.NewBorder(container.NewPadded(form), container.NewPadded(container.NewBorder(nil, nil, nil, buttons)), nil, nil, container.NewPadded(body)))
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

// recipientEntry is a text entry that suggests contacts for the address being typed.
func (u *UI) recipientEntry(w fyne.Window, account func() string, name string) *widget.Entry {
	e := widget.NewEntry()
	e.SetPlaceHolder(name)
	var popup *widget.PopUp
	hide := func() {
		if popup != nil {
			popup.Hide()
			popup = nil
		}
	}
	e.OnChanged = func(text string) {
		hide()
		head, token := text, text
		if i := strings.LastIndex(text, ","); i >= 0 {
			head, token = text[:i+1]+" ", strings.TrimSpace(text[i+1:])
		} else {
			head = ""
		}
		if len(token) < 2 {
			return
		}
		var found []desktop.ContactItem
		u.bg(func() { found, _ = u.be.SearchContacts(account(), token, 8) }, func() {
			if len(found) == 0 || e.Text != text {
				return
			}
			hide()
			list := container.NewVBox()
			for _, c := range found {
				c := c
				label := c.Email
				if c.Name != "" {
					label = c.Name + " <" + c.Email + ">"
				}
				b := widget.NewButton(label, func() {
					hide()
					e.SetText(head + label + ", ")
					e.CursorColumn = len(e.Text)
					e.Refresh()
				})
				b.Alignment = widget.ButtonAlignLeading
				b.Importance = widget.LowImportance
				list.Add(b)
			}
			popup = widget.NewPopUp(list, w.Canvas())
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(e)
			popup.ShowAtPosition(pos.Add(fyne.NewPos(0, e.Size().Height)))
			popup.Resize(fyne.NewSize(e.Size().Width, list.MinSize().Height))
		})
	}
	return e
}

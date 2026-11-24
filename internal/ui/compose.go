package ui

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// draftInterval is how often a compose window checks for changes to autosave.
var draftInterval = 10 * time.Second

// newDraftTicker returns the autosave tick channel and its stop func. Tests
// replace it to drive ticks deterministically.
var newDraftTicker = func(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

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
	var onFrom func()              // set once the editor exists; swaps the signature
	var moveDraft func(old string) // set once autosave exists; drops the draft left under the old account
	from := widget.NewSelect(emails, func(e string) {
		old := accountID
		for _, a := range u.accounts {
			if a.Email == e {
				accountID = a.ID
			}
		}
		if accountID != old && moveDraft != nil {
			moveDraft(old)
		}
		if onFrom != nil {
			onFrom()
		}
	})
	from.SetSelected(selected)

	rcpt := newSuggestHost()
	to := u.recipientEntry(rcpt, func() string { return accountID }, "To")
	cc := u.recipientEntry(rcpt, func() string { return accountID }, "Cc")
	bcc := u.recipientEntry(rcpt, func() string { return accountID }, "Bcc")
	to.SetText(draft.To)
	cc.SetText(draft.Cc)
	bcc.SetText(draft.Bcc)
	subject := widget.NewEntry()
	subject.SetText(draft.Subject)
	body := NewWysiwyg(w)
	// A new message or fresh reply/forward gets the signature; a reopened draft
	// already holds its own, in the editor's HTML or in the older markup.
	var sig string // signature block currently in the document
	quoteTail := 0
	fresh := draft.DraftID == "" && (strings.TrimSpace(draft.Body) == "" || strings.HasPrefix(draft.Body, quoteOpen))
	switch {
	case fresh:
		quoteTail = quoteParas(draft.Body)
		body.SetHTML(composeHTML("", draft.Body))
	case isHTML(draft.Body):
		body.SetHTML(draft.Body)
	default:
		body.SetHTML(markupToHTML(draft.Body))
	}
	editor := container.NewBorder(container.NewHScroll(NewWysiwygToolbar(body)), nil, nil, nil, body)

	form := widget.NewForm(
		widget.NewFormItem("From", from),
		widget.NewFormItem("To", to),
		widget.NewFormItem("Cc", cc),
		widget.NewFormItem("Bcc", bcc),
		widget.NewFormItem("Subject", subject),
	)

	bar := newAttachBar(u, w, draft.Attachments, func() int { return len(body.HTML()) }, body)
	attach := iconButton("clip", "Attach files", bar.pick)
	setDropHandler(w, bar.drop)
	w.Canvas().AddShortcut(cmd(fyne.KeyA, true), func(fyne.Shortcut) { bar.pick() })

	send := widget.NewButtonWithIcon("Send", icon("send", fgOnPrimary), nil)
	send.Importance = widget.HighImportance
	// Autosave: every draftInterval, save to local Drafts only if content changed.
	draftID := draft.DraftID // set when reopening a saved draft
	var closed atomic.Bool   // set on close; read by autosave callbacks
	current := func() desktop.OutboundMsg {
		return desktop.OutboundMsg{
			AccountID: accountID, To: to.Text, Cc: cc.Text, Bcc: bcc.Text,
			Subject: subject.Text, Body: body.HTML(), InReplyTo: draft.InReplyTo, References: draft.References, ThreadID: draft.ThreadID, Attachments: slices.Clone(bar.atts),
		}
	}
	lastSaved := current()
	if fresh {
		onFrom = func() {
			acc, email := accountID, from.Selected
			var s string
			u.bg(func() { s = u.be.Signature(acc, email) }, func() {
				if closed.Load() || acc != accountID {
					return
				}
				unchanged := current().Equal(lastSaved)
				if body.replaceSignature(sig, s, quoteTail) {
					sig = s
					if unchanged { // inserting the signature is not a user edit
						lastSaved = current()
					}
				}
			})
		}
		onFrom()
	}
	// Saves and discards run in call order off the UI goroutine, and draftID is
	// only touched there, so a slow save can never be followed by a second
	// "first" save (duplicate draft) or by a discard that misses it.
	saveDraft := func(msg desktop.OutboundMsg) {
		prev := lastSaved
		lastSaved = msg
		var err error
		u.bgOrdered(func() {
			var id string
			if id, err = u.be.SaveDraft(draftID, msg); err == nil {
				draftID = id
			}
		}, func() {
			if err != nil {
				lastSaved = prev // retry next tick
			}
		})
	}
	discardDraft := func(acc string) {
		u.bgOrdered(func() {
			if draftID != "" {
				_ = u.be.DiscardDraft(acc, draftID)
				draftID = ""
			}
		}, nil)
	}
	moveDraft = discardDraft // the draft is keyed by account: changing From starts a new one
	stop := make(chan struct{})
	ticks, stopTicker := newDraftTicker(draftInterval) // read before the goroutine starts
	go func() {
		defer stopTicker()
		for {
			select {
			case <-stop:
				return
			case <-ticks:
				fyne.Do(func() {
					if msg := current(); !closed.Load() && !msg.Equal(lastSaved) {
						saveDraft(msg)
					}
				})
			}
		}
	}()
	finish := func(discardIt bool) {
		if closed.Swap(true) {
			return
		}
		close(stop)
		if discardIt {
			discardDraft(accountID)
		} else if msg := current(); !msg.Equal(lastSaved) {
			saveDraft(msg) // closing must not lose edits made since the last tick
		}
	}
	w.SetOnClosed(func() { finish(false) })
	discard := widget.NewButton("Discard", func() {
		dialog.ShowConfirm("Discard draft?", "This draft will be deleted.", func(ok bool) {
			if ok {
				finish(true)
				w.Close()
			}
		}, w)
	})
	send.OnTapped = func() {
		if strings.TrimSpace(to.Text+cc.Text+bcc.Text) == "" {
			u.showErrorIn(w, errNoRecipient)
			return
		}
		msg := current()
		send.Disable()
		discard.Disable() // a send in flight cannot be taken back
		var err error
		u.bg(func() { err = u.be.SendMessage(msg) }, func() {
			if err != nil {
				send.Enable()
				discard.Enable()
				if closed.Load() { // window closed mid-send: its edits were kept as a draft
					u.showError(fmt.Errorf("%w (the message was kept in Drafts)", err))
					return
				}
				u.showErrorIn(w, err)
				return
			}
			if closed.Load() {
				discardDraft(msg.AccountID) // sent mail must not leave a stale draft
			} else {
				finish(true)
				w.Close()
			}
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
	// Border layout, but with the body listed before the buttons: Fyne tabs in
	// object order, so this gives To, Cc, Bcc, Subject, body, attach, Discard, Send.
	top := container.NewVBox(container.NewPadded(form), rcpt.box)
	bottom := container.NewVBox(container.NewPadded(bar.root), container.NewPadded(container.NewBorder(nil, nil, attach, buttons)))
	w.SetContent(container.New(layout.NewBorderLayout(top, bottom, nil, nil), top, container.NewPadded(editor), bottom))
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
	last, quoted, angle := -1, false, false
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\\' && quoted:
			i++
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '<':
			angle = true
		case c == '>':
			angle = false
		case c == ',' && !angle:
			last = i
		}
	}
	if last < 0 {
		return "", strings.TrimSpace(text)
	}
	return text[:last+1] + " ", strings.TrimSpace(text[last+1:])
}

// recipientEntry is a text entry that suggests contacts for the address being typed.
func (u *UI) recipientEntry(host *suggestHost, account func() string, name string) *suggestEntry {
	sg := &suggester{host: host}
	e := newSuggestEntry(sg)
	e.SetPlaceHolder(name)
	sg.pick = func(c desktop.ContactItem) {
		head, _ := splitRecipient(e.Text)
		e.SetText(head + contactInsert(c) + ", ")
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

// ownAddrs lists the user's own addresses: every account plus its send-as aliases.
func (u *UI) ownAddrs() []string {
	var out []string
	al, _ := u.be.(interface {
		Aliases(accountID string) []string
	})
	for _, a := range u.accounts {
		out = append(out, a.Email)
		if al != nil {
			out = append(out, al.Aliases(a.ID)...)
		}
	}
	return out
}

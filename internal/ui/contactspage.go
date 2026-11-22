package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

const frequentShown = 8

// contactRow is one line of the contacts list: a section header or a person.
// Saved is false for "Other contacts": addresses seen in mail that are not in the address book.
type contactRow struct {
	Header string
	C      desktop.ContactItem
	Saved  bool
}

func (r contactRow) key() string {
	return r.C.AccountID + "/" + r.C.ID + "/" + strings.ToLower(r.C.Email)
}

// buildContactRows lays out the list as Google Contacts does: with no query,
// "Frequently contacted", all contacts, then "Other contacts"; with a query, one
// flat list of the contacts containing it in the name, address or phone.
func buildContactRows(book, frequent []desktop.ContactItem, query string) []contactRow {
	q := strings.ToLower(strings.TrimSpace(query))
	var rows []contactRow
	if q != "" {
		for _, c := range book {
			if strings.Contains(strings.ToLower(c.Name+"\n"+c.Email+"\n"+c.Phone), q) {
				rows = append(rows, contactRow{C: c, Saved: true})
			}
		}
		return rows
	}
	inBook := map[string]desktop.ContactItem{}
	for _, c := range book {
		inBook[strings.ToLower(c.Email)] = c
	}
	var freq, other []contactRow
	for _, f := range frequent {
		if c, ok := inBook[strings.ToLower(f.Email)]; ok {
			if len(freq) < frequentShown {
				freq = append(freq, contactRow{C: c, Saved: true})
			}
		} else {
			other = append(other, contactRow{C: f})
		}
	}
	if len(freq) > 0 {
		rows = append(rows, contactRow{Header: "Frequently contacted"})
		rows = append(rows, freq...)
	}
	rows = append(rows, contactRow{Header: "Contacts (" + itoa(len(book)) + ")"})
	for _, c := range book {
		rows = append(rows, contactRow{C: c, Saved: true})
	}
	if len(other) > 0 {
		rows = append(rows, contactRow{Header: "Other contacts"})
		rows = append(rows, other...)
	}
	return rows
}

func itoa(n int) string { return strconv.Itoa(n) }

func contactDisplayName(c desktop.ContactItem) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Email
}

// openContacts shows the Contacts window (Google Contacts style list + detail pane).
func (u *UI) openContacts() {
	if u.contactsWin != nil {
		u.contactsWin.Show()
		u.contactsWin.RequestFocus()
		return
	}
	w := u.app.NewWindow("Contacts")
	w.Resize(fyne.NewSize(960, 620))
	w.SetOnClosed(func() { u.contactsWin = nil })
	u.contactsWin = w

	var (
		book, frequent []desktop.ContactItem
		rows           []contactRow
		checked        = map[string]bool{}
		list           *widget.List
	)
	account := u.activeAccount
	detail := container.NewStack()
	search := &escEntry{close: w.Close}
	search.ExtendBaseWidget(search)
	search.SetPlaceHolder("Search contacts")
	closeOnEscape(w)
	del := widget.NewButton("Delete", nil)
	del.Importance = widget.DangerImportance
	del.Hide()

	rebuild := func() {
		rows = buildContactRows(book, frequent, search.Text)
		list.Refresh()
		n := 0
		for _, r := range rows {
			if r.Saved && checked[r.key()] {
				n++
			}
		}
		del.SetText("Delete (" + itoa(n) + ")")
		if n > 0 {
			del.Show()
		} else {
			del.Hide()
		}
	}
	reload := func() {
		u.bg(func() {
			book, _ = u.be.ListContacts(account)
			frequent, _ = u.be.FrequentAddresses(account, 200)
		}, rebuild)
	}
	show := func(o fyne.CanvasObject) { detail.Objects = []fyne.CanvasObject{o}; detail.Refresh() }
	blank := func() { show(container.NewCenter(widget.NewLabel("Select a contact"))) }

	var viewContact func(r contactRow)
	editContact := func(c desktop.ContactItem) {
		name, email, phone := widget.NewEntry(), widget.NewEntry(), widget.NewEntry()
		name.SetText(c.Name)
		email.SetText(c.Email)
		phone.SetText(c.Phone)
		accID := c.AccountID
		items := []*widget.FormItem{widget.NewFormItem("Name", name), widget.NewFormItem("Email", email), widget.NewFormItem("Phone", phone)}
		if c.ID == "" && len(u.accounts) > 1 {
			var emails []string
			cur := ""
			if accID == "" {
				accID = u.defaultAccountID()
			}
			for _, a := range u.accounts {
				emails = append(emails, a.Email)
				if a.ID == accID {
					cur = a.Email
				}
			}
			sel := widget.NewSelect(emails, func(e string) {
				for _, a := range u.accounts {
					if a.Email == e {
						accID = a.ID
					}
				}
			})
			sel.SetSelected(cur)
			items = append([]*widget.FormItem{widget.NewFormItem("Save to", sel)}, items...)
		}
		if accID == "" {
			accID = u.defaultAccountID()
		}
		save := widget.NewButton("Save", nil)
		save.Importance = widget.HighImportance
		save.OnTapped = func() {
			if strings.TrimSpace(email.Text) == "" {
				u.showErrorIn(w, errNoRecipient)
				return
			}
			c.AccountID, c.Name, c.Email, c.Phone = accID, strings.TrimSpace(name.Text), strings.TrimSpace(email.Text), strings.TrimSpace(phone.Text)
			save.Disable()
			var saved desktop.ContactItem
			var err error
			u.bg(func() { saved, err = u.be.SaveContact(c) }, func() {
				if err != nil {
					save.Enable()
					u.showErrorIn(w, err)
					return
				}
				reload()
				viewContact(contactRow{C: saved, Saved: true})
			})
		}
		cancel := widget.NewButton("Cancel", func() {
			if c.ID == "" {
				blank()
			} else {
				viewContact(contactRow{C: c, Saved: true})
			}
		})
		title := "Create contact"
		if c.ID != "" {
			title = "Edit contact"
		}
		show(container.NewPadded(container.NewVBox(bold(title), widget.NewForm(items...), container.NewHBox(cancel, save))))
	}
	confirmDelete := func(cs []desktop.ContactItem) {
		msg := "Delete " + itoa(len(cs)) + " contact(s)?"
		if len(cs) == 1 {
			msg = "Delete " + contactDisplayName(cs[0]) + "?"
		}
		dialog.ShowConfirm("Delete", msg, func(ok bool) {
			if !ok {
				return
			}
			var err error
			u.bg(func() {
				for _, c := range cs {
					if err = u.be.DeleteContact(c.AccountID, c.ID); err != nil {
						return
					}
				}
			}, func() {
				if err != nil {
					u.showErrorIn(w, err)
				}
				for _, c := range cs {
					delete(checked, contactRow{C: c}.key())
				}
				blank()
				reload()
			})
		}, w)
	}
	viewContact = func(r contactRow) {
		c := r.C
		name := canvas.NewText(contactDisplayName(c), fg())
		name.TextSize = 22
		lines := []fyne.CanvasObject{container.NewHBox(avatar(initials(contactDisplayName(c)), 72, avatarColor(c.Email)), name)}
		if c.Name != "" {
			lines = append(lines, widget.NewLabel(c.Email))
		}
		if c.Phone != "" {
			lines = append(lines, widget.NewLabel(c.Phone))
		}
		mailBtn := widget.NewButtonWithIcon("Send email", icon("compose", fg()), func() { u.compose(desktop.OutboundMsg{To: contactLabel(c)}) })
		actions := []fyne.CanvasObject{mailBtn}
		if r.Saved {
			actions = append(actions,
				widget.NewButton("Edit", func() { editContact(c) }),
				widget.NewButton("Delete", func() { confirmDelete([]desktop.ContactItem{c}) }))
		} else {
			lines = append(lines, widget.NewLabel("Not in your contacts yet"))
			actions = append(actions, widget.NewButton("Add to contacts", func() { c.AccountID = ""; editContact(c) }))
		}
		show(container.NewPadded(container.NewVBox(append(lines, container.NewHBox(actions...))...)))
	}

	list = widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject {
			hdr := bold("")
			chk := widget.NewCheck("", nil)
			cells := container.NewGridWithColumns(3, ellipsisLabel(), ellipsisLabel(), ellipsisLabel())
			row := container.NewBorder(nil, nil, container.NewHBox(chk, container.NewStack()), nil, cells)
			return container.NewStack(hdr, row)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			st := o.(*fyne.Container)
			hdr, row := st.Objects[0].(*widget.Label), st.Objects[1].(*fyne.Container)
			r := rows[i]
			if r.Header != "" {
				hdr.SetText(r.Header)
				hdr.Show()
				row.Hide()
				return
			}
			hdr.Hide()
			row.Show()
			left := row.Objects[1].(*fyne.Container) // the border's left slot
			chk, av := left.Objects[0].(*widget.Check), left.Objects[1].(*fyne.Container)
			chk.OnChanged = nil
			chk.SetChecked(checked[r.key()])
			if r.Saved {
				chk.Show()
				chk.OnChanged = func(on bool) {
					if on {
						checked[r.key()] = true
					} else {
						delete(checked, r.key())
					}
					rebuild()
				}
			} else {
				chk.Hide()
			}
			av.Objects = []fyne.CanvasObject{avatar(initials(contactDisplayName(r.C)), 28, avatarColor(r.C.Email))}
			av.Refresh()
			cells := row.Objects[0].(*fyne.Container)
			for j, t := range []string{contactDisplayName(r.C), r.C.Email, r.C.Phone} {
				cells.Objects[j].(*widget.Label).SetText(t)
			}
		})
	list.OnSelected = func(i widget.ListItemID) {
		if rows[i].Header == "" {
			viewContact(rows[i])
		}
	}
	search.OnChanged = func(string) { list.UnselectAll(); rebuild() }
	del.OnTapped = func() {
		var cs []desktop.ContactItem
		for _, c := range book {
			if checked[contactRow{C: c}.key()] {
				cs = append(cs, c)
			}
		}
		confirmDelete(cs)
	}
	create := widget.NewButtonWithIcon("Create contact", icon("person", fgOnPrimary), func() {
		list.UnselectAll()
		editContact(desktop.ContactItem{AccountID: accountFor(account)})
	})
	create.Importance = widget.HighImportance

	left := container.NewBorder(container.NewVBox(container.NewBorder(nil, nil, nil, container.NewHBox(del, create), search), hline()), nil, nil, nil, list)
	split := container.NewHSplit(left, detail)
	split.Offset = 0.62
	blank()
	w.SetContent(split)
	w.Canvas().Focus(search)
	w.Show()
	reload()
}

// accountFor is the account a new contact defaults to: the viewed account, or "" (resolved later) for the unified view.
func accountFor(active string) string {
	if active == unified {
		return ""
	}
	return active
}

func ellipsisLabel() *widget.Label {
	l := widget.NewLabel("")
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

// escEntry is an Entry whose Escape key runs close instead of being swallowed.
type escEntry struct {
	widget.Entry
	close func()
}

func (e *escEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		e.close()
		return
	}
	e.Entry.TypedKey(k)
}

// closeOnEscape closes w on Escape when no dialog is open and nothing has focus.
func closeOnEscape(w fyne.Window) {
	w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		if e.Name == fyne.KeyEscape && w.Canvas().Overlays().Top() == nil {
			w.Close()
		}
	})
}

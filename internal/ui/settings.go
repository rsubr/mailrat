package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

var (
	syncPeriods = []string{"All time", "Past year", "Past 90 days", "Past 30 days"}
	periodCodes = []string{"all", "1y", "90d", "30d"}
	syncFreqs   = []string{"Every 15 minutes", "Every hour", "Manually"}
	freqCodes   = []string{"15m", "1h", "manual"}
)

func indexOf(codes []string, c string) int {
	for i, v := range codes {
		if v == c {
			return i
		}
	}
	return 0
}

// openSettings shows the accounts / general preferences window.
func (u *UI) openSettings() {
	if u.settingsWin != nil {
		u.settingsWin.Show()
		u.settingsWin.RequestFocus()
		u.settingsWin.SetContent(u.buildSettings())
		return
	}
	w := u.app.NewWindow("Settings")
	w.Resize(fyne.NewSize(620, 460))
	w.SetOnClosed(func() { u.settingsWin = nil })
	u.settingsWin = w
	w.SetContent(u.buildSettings())
	w.Show()
}

func (u *UI) buildSettings() fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItem("Accounts", u.accountsTab()),
		container.NewTabItem("Labels", u.labelsTab()),
		container.NewTabItem("General", u.generalTab()),
	)
	return container.NewPadded(tabs)
}

func (u *UI) accountsTab() fyne.CanvasObject {
	rows := container.NewVBox()
	for _, a := range u.accounts {
		a := a
		st, _ := u.be.GetAccountSettings(a.ID)
		period := widget.NewSelect(syncPeriods, nil)
		period.SetSelectedIndex(indexOf(periodCodes, st.SyncPeriod))
		attach := widget.NewCheck("Download attachments", nil)
		attach.SetChecked(st.FetchAttachments)
		save := func() {
			st.AccountID = a.ID
			st.SyncPeriod = periodCodes[period.SelectedIndex()]
			st.FetchAttachments = attach.Checked
			if err := u.be.SetAccountSettings(st); err != nil {
				u.showError(err)
			}
		}
		period.OnChanged = func(string) { save() }
		attach.OnChanged = func(bool) { save() }
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Display name")
		nameEntry.SetText(a.DisplayName)
		rename := widget.NewButton("Rename", func() {
			if err := u.be.RenameAccount(a.ID, nameEntry.Text); err != nil {
				u.showError(err)
				return
			}
			u.reloadAccounts()
		})
		remove := widget.NewButton("Remove", func() {
			dialog.ShowConfirm("Remove account", "Delete "+a.Email+" and all its local mail?", func(ok bool) {
				if !ok {
					return
				}
				if err := u.be.RemoveAccount(a.ID); err != nil {
					u.showError(err)
					return
				}
				if u.activeAccount == a.ID {
					u.activeAccount = unified
				}
				u.reloadAll()
			}, u.settingsWin)
		})
		remove.Importance = widget.DangerImportance
		name := a.Email
		if a.DisplayName != "" {
			name = a.DisplayName + "  ·  " + a.Email
		}
		card := widget.NewCard(name, "", container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Name", container.NewBorder(nil, nil, nil, rename, nameEntry)),
				widget.NewFormItem("Sync mail from", period),
			),
			container.NewBorder(nil, nil, attach, remove),
		))
		rows.Add(card)
	}
	if len(u.accounts) == 0 {
		rows.Add(widget.NewLabel("No accounts yet. Add a Google account to get started."))
	}
	add := widget.NewButton("Add Account…", u.addAccount)
	add.Importance = widget.HighImportance
	credStatus := "Google credentials: not configured"
	if u.be.HasCredentials() {
		credStatus = "Google credentials: configured"
	}
	creds := widget.NewButton("Import credentials.json…", u.chooseCredentials)
	footer := container.NewHBox(add, creds, widget.NewLabel(credStatus))
	return container.NewBorder(nil, container.NewPadded(footer), nil, nil, container.NewVScroll(rows))
}

func (u *UI) generalTab() fyne.CanvasObject {
	gs, _ := u.be.GetGlobalSettings()
	freq := widget.NewSelect(syncFreqs, nil)
	freq.SetSelectedIndex(indexOf(freqCodes, gs.SyncFrequency))
	var emails []string
	def := ""
	for _, a := range u.accounts {
		emails = append(emails, a.Email)
		if a.ID == gs.DefaultAccountID {
			def = a.Email
		}
	}
	acct := widget.NewSelect(emails, nil)
	if def == "" && len(emails) > 0 {
		def = emails[0]
	}
	acct.SetSelected(def)
	save := func() {
		out := desktop.GlobalSettingsItem{SyncFrequency: freqCodes[freq.SelectedIndex()], DefaultAccountID: gs.DefaultAccountID}
		for _, a := range u.accounts {
			if a.Email == acct.Selected {
				out.DefaultAccountID = a.ID
			}
		}
		if err := u.be.SetGlobalSettings(out); err != nil {
			u.showError(err)
		}
	}
	freq.OnChanged = func(string) { save() }
	acct.OnChanged = func(string) { save() }
	return container.NewVBox(widget.NewForm(
		widget.NewFormItem("Check for new mail", freq),
		widget.NewFormItem("Send new mail from", acct),
	))
}

var (
	scopeModes = []string{"Everything", "Last N days", "Last N emails"}
	scopeCodes = []string{desktop.SyncModeAll, desktop.SyncModeDays, desktop.SyncModeCount}
)

// labelsTab lets the user choose, per label, how much mail stays synced.
func (u *UI) labelsTab() fyne.CanvasObject {
	if len(u.accounts) == 0 {
		return widget.NewLabel("Add an account to choose what to sync.")
	}
	body := container.NewStack()
	show := func(accountID string) {
		body.Objects = []fyne.CanvasObject{u.labelScopeList(accountID)}
		body.Refresh()
	}
	var emails []string
	for _, a := range u.accounts {
		emails = append(emails, a.Email)
	}
	pick := widget.NewSelect(emails, func(sel string) {
		for _, a := range u.accounts {
			if a.Email == sel {
				show(a.ID)
			}
		}
	})
	pick.SetSelectedIndex(0)
	intro := widget.NewLabel("Choose how much mail to keep synced per label. Older mail loads on demand as you scroll.")
	intro.Wrapping = fyne.TextWrapWord
	return container.NewBorder(container.NewVBox(intro, pick), nil, nil, nil, body)
}

func (u *UI) labelScopeList(accountID string) fyne.CanvasObject {
	items, err := u.be.GetSyncConfigs(accountID)
	if err != nil {
		return widget.NewLabel(err.Error())
	}
	rows := container.NewVBox()
	for _, it := range items {
		it := it
		mode := widget.NewSelect(scopeModes, nil)
		mode.SetSelectedIndex(indexOf(scopeCodes, it.Mode))
		val := widget.NewEntry()
		val.SetText(strconv.Itoa(max(it.Value, 30)))
		val.Validator = func(s string) error {
			if n, err := strconv.Atoi(s); err != nil || n < 1 {
				return fmt.Errorf("enter a number of 1 or more")
			}
			return nil
		}
		valBox := container.NewGridWrap(fyne.NewSize(80, val.MinSize().Height), val)
		refresh := func() {
			if scopeCodes[mode.SelectedIndex()] == desktop.SyncModeAll {
				valBox.Hide()
			} else {
				valBox.Show()
			}
		}
		save := func() {
			it.Mode = scopeCodes[mode.SelectedIndex()]
			n, err := strconv.Atoi(val.Text)
			if it.Mode != desktop.SyncModeAll && (err != nil || n < 1) {
				return
			}
			it.Value = n
			if it.Mode == desktop.SyncModeAll {
				it.Value = 0
			}
			if err := u.be.SetSyncConfig(accountID, it); err != nil {
				u.showError(err)
			}
		}
		mode.OnChanged = func(string) { refresh(); save() }
		val.OnChanged = func(string) { save() }
		refresh()
		rows.Add(container.NewBorder(nil, nil, nil, container.NewHBox(mode, valBox), widget.NewLabel(it.LabelName)))
	}
	return container.NewVScroll(rows)
}

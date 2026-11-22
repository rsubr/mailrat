package ui

import (
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
				u.settingsWin.SetContent(u.buildSettings())
			}, u.settingsWin)
		})
		remove.Importance = widget.DangerImportance
		name := a.Email
		if a.DisplayName != "" {
			name = a.DisplayName + "  ·  " + a.Email
		}
		card := widget.NewCard(name, "", container.NewVBox(
			widget.NewForm(widget.NewFormItem("Sync mail from", period)),
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

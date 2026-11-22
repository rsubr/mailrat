package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// welcome is the empty state shown until the first account is connected.
func (u *UI) welcome() fyne.CanvasObject {
	title := canvas.NewText("Welcome to mailrat", fg())
	title.TextSize = 24
	title.TextStyle = fyne.TextStyle{Bold: true}
	sub := widget.NewLabel("Sign in with Google to read and send mail.")
	add := widget.NewButtonWithIcon("Add Account…", icon("person", color.White), u.addAccount)
	add.Importance = widget.HighImportance
	return container.NewCenter(container.NewVBox(
		container.NewCenter(title), container.NewCenter(sub), spacer(0, 8), container.NewCenter(add)))
}

// addAccount starts Google sign-in, first asking for credentials.json if none is installed.
func (u *UI) addAccount() {
	if !u.be.HasCredentials() {
		u.askForCredentials()
		return
	}
	var authURL string
	var err error
	u.bg(func() { authURL, err = u.be.BeginLogin() }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.showLogin(authURL)
	})
}

// askForCredentials explains the one-time Google Cloud setup and offers the file picker.
func (u *UI) askForCredentials() {
	msg := widget.NewLabel("mailrat signs in through your own Google Cloud OAuth client.\n\n" +
		"1. In Google Cloud Console, enable the Gmail API and People API.\n" +
		"2. Create an OAuth client ID of type “Desktop app”.\n" +
		"3. Add your Google address as a test user.\n" +
		"4. Download the client JSON and choose it here.")
	msg.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm("Google credentials needed", "Choose credentials.json…", "Cancel", msg, func(ok bool) {
		if ok {
			u.chooseCredentials()
		}
	}, u.parentWindow())
	d.Resize(fyne.NewSize(460, 280))
	d.Show()
}

// chooseCredentials lets the user pick a Google client file, then continues sign-in.
func (u *UI) chooseCredentials() {
	var path string
	var err error
	u.bg(func() { path, err = pickOpenFile("Choose credentials file", "*.json") }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		if path == "" {
			return
		}
		var impErr error
		u.bg(func() { impErr = u.be.ImportCredentials(path) }, func() {
			if impErr != nil {
				u.showError(impErr)
				return
			}
			if u.settingsWin != nil {
				u.settingsWin.SetContent(u.buildSettings())
			}
			u.addAccount()
		})
	})
}

// showLogin waits for the browser sign-in, with a paste fallback for machines
// where the browser cannot reach this app's local callback.
func (u *UI) showLogin(authURL string) {
	status := widget.NewLabel("Finish signing in with Google in your browser. This window closes when you're done.")
	status.Wrapping = fyne.TextWrapWord

	open := widget.NewButton("Open Browser", func() { u.be.OpenLoginBrowser() })
	copyLink := widget.NewButton("Copy Link", func() {
		u.app.Clipboard().SetContent(authURL)
	})

	paste := widget.NewEntry()
	paste.SetPlaceHolder("Paste the address from the browser after sign-in")
	submit := widget.NewButton("Continue", func() {
		if err := u.be.SubmitLoginCode(paste.Text); err != nil {
			u.showError(err)
		}
	})
	hint := widget.NewLabel("Browser on another device? After approving, copy the full address from its address bar (the page may fail to load) and paste it here.")
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	content := container.NewVBox(status, container.NewHBox(open, copyLink), widget.NewSeparator(), hint,
		container.NewBorder(nil, nil, nil, submit, paste))
	d := dialog.NewCustom("Sign in with Google", "Cancel", content, u.parentWindow())
	d.SetOnClosed(func() {
		if u.loginDlg != nil { // user dismissed; not closed by success
			u.loginDlg = nil
			u.be.CancelLogin()
		}
	})
	d.Resize(fyne.NewSize(480, 300))
	u.loginDlg = d
	d.Show()
}

// closeLogin dismisses the sign-in dialog after the backend reports an outcome.
func (u *UI) closeLogin() {
	if u.loginDlg != nil {
		d := u.loginDlg
		u.loginDlg = nil
		d.Hide()
	}
}

// parentWindow is where dialogs attach: Settings if open, otherwise the main window.
func (u *UI) parentWindow() fyne.Window {
	if u.settingsWin != nil {
		return u.settingsWin
	}
	return u.win
}

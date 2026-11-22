package ui

import (
	"errors"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

var (
	errNoAccounts  = errors.New("no accounts connected — add one in Settings")
	errNoRecipient = errors.New("add at least one recipient")
)

var fgOnPrimary color.Color = color.White

func (u *UI) showErrorIn(w fyne.Window, err error) { dialog.ShowError(err, w) }

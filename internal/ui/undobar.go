package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mailrat/mailrat/internal/mail"
)

// showUndoTrash shows a bar at the bottom of the window with an Undo button
// for the length of the backend's undo window.
func (u *UI) showUndoTrash(acct, id string) {
	if u.win == nil {
		return
	}
	u.hideUndo()
	var pop *widget.PopUp
	undo := widget.NewButton("Undo", func() {
		var err error
		u.bg(func() { err = u.be.UndoTrashThread(acct, id) }, func() {
			u.hideUndo()
			if err != nil {
				u.showError(err)
				return
			}
			u.reloadThreads()
			u.reloadLabels()
		})
	})
	pop = widget.NewPopUp(container.NewHBox(widget.NewLabel("Conversation moved to Trash"), undo), u.win.Canvas())
	u.undoPop = pop
	size := pop.MinSize()
	cs := u.win.Canvas().Size()
	pop.Resize(size)
	pop.ShowAtPosition(fyne.NewPos((cs.Width-size.Width)/2, cs.Height-size.Height-24))
	time.AfterFunc(mail.UndoWindow, func() {
		fyne.Do(func() {
			if u.undoPop == pop {
				u.hideUndo()
			}
		})
	})
}

func (u *UI) hideUndo() {
	if u.undoPop != nil {
		u.undoPop.Hide()
		u.undoPop = nil
	}
}

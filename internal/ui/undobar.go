package ui

import (
	"fmt"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/mail"
)

// undoBar is the "moved to Trash / Undo" bar. It lives in the window layout,
// not in an overlay: an overlay swallows the next click and counts as a modal.
type undoBar struct{ Content fyne.CanvasObject }

// showUndoBar draws the current bar (if any) at the bottom of the window.
func (u *UI) showUndoBar() {
	if u.undoHolder == nil {
		return
	}
	u.undoHolder.Objects = nil
	if u.undoPop != nil {
		u.undoHolder.Objects = []fyne.CanvasObject{
			layout.NewSpacer(), container.NewCenter(u.undoPop.Content), spacer(1, 24),
		}
	}
	u.undoHolder.Refresh()
}

// showUndo shows msg with an Undo button for the length of the backend's undo window.
func (u *UI) showUndo(msg string, undo func() error) {
	if u.win == nil {
		return
	}
	u.hideUndo()
	undoBtn := widget.NewButton("Undo", func() {
		var err error
		u.bg(func() { err = undo() }, func() {
			u.hideUndo()
			if err != nil {
				u.showError(err)
				return
			}
			u.reloadThreads()
			u.reloadLabels()
		})
	})
	bg := canvas.NewRectangle(col(theme.ColorNameInputBackground))
	bg.StrokeColor, bg.StrokeWidth, bg.CornerRadius = col(theme.ColorNameSeparator), 1, 6
	bar := &undoBar{Content: container.NewStack(bg, container.NewPadded(container.NewHBox(widget.NewLabel(msg), undoBtn)))}
	u.undoPop = bar
	u.showUndoBar()
	time.AfterFunc(mail.UndoWindow, func() {
		fyne.Do(func() {
			if u.undoPop == bar {
				u.hideUndo()
			}
		})
	})
}

func (u *UI) hideUndo() {
	u.undoPop = nil
	u.showUndoBar()
}

func convMsg(n int, what string) string {
	if n == 1 {
		return "Conversation " + what
	}
	return fmt.Sprintf("%d conversations %s", n, what)
}

// eachThread runs call on every thread and returns the first failure.
func eachThread(ts []desktop.ThreadItem, call func(accountID, threadID string) error) error {
	var first error
	for _, t := range ts {
		if err := call(t.AccountID, t.ID); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (u *UI) undoTrash(ts []desktop.ThreadItem) {
	if len(ts) > 0 {
		u.showUndo(convMsg(len(ts), "moved to Trash"), func() error { return eachThread(ts, u.be.UndoTrashThread) })
	}
}

// undoArchive offers to put back the threads that were in the Inbox.
func (u *UI) undoArchive(ts []desktop.ThreadItem) {
	ts = slices.DeleteFunc(slices.Clone(ts), func(t desktop.ThreadItem) bool { return !slices.Contains(t.LabelIDs, "INBOX") })
	if len(ts) > 0 {
		u.showUndo(convMsg(len(ts), "archived"), func() error {
			return eachThread(ts, func(a, id string) error { return u.be.SetThreadLabel(a, id, "INBOX", true) })
		})
	}
}

func (u *UI) undoSpam(ts []desktop.ThreadItem) {
	if len(ts) > 0 {
		u.showUndo(convMsg(len(ts), "marked as spam"), func() error { return eachThread(ts, u.be.MarkThreadNotSpam) })
	}
}

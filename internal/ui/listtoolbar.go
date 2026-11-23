package ui

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	mailrat "github.com/mailrat/mailrat/internal/desktop"
)

func threadKey(t mailrat.ThreadItem) string { return t.AccountID + "/" + t.ID }

func (u *UI) checkedThreads() []mailrat.ThreadItem {
	var out []mailrat.ThreadItem
	for _, t := range u.threads {
		if u.checked[threadKey(t)] {
			out = append(out, t)
		}
	}
	return out
}

// setChecked ticks or unticks one row and refreshes the rows and toolbar.
func (u *UI) setChecked(t mailrat.ThreadItem, on bool) {
	if u.checked == nil {
		u.checked = map[string]bool{}
	}
	if on {
		u.checked[threadKey(t)] = true
	} else {
		delete(u.checked, threadKey(t))
	}
	u.list.Refresh()
	u.refreshListBar()
}

// selectWhere ticks exactly the loaded threads that satisfy keep (nil = none).
func (u *UI) selectWhere(keep func(mailrat.ThreadItem) bool) {
	u.checked = map[string]bool{}
	for _, t := range u.threads {
		if keep != nil && keep(t) {
			u.checked[threadKey(t)] = true
		}
	}
	u.list.Refresh()
	u.refreshListBar()
}

// toggleCheckCursor is "x": tick the cursor row, as Gmail does.
func (u *UI) toggleCheckCursor() {
	if len(u.threads) == 0 {
		return
	}
	i := u.cursor
	if u.cursorNext {
		i++
	}
	i = max(0, min(i, len(u.threads)-1))
	u.cursor, u.cursorNext = i, false
	t := u.threads[i]
	u.setChecked(t, !u.checked[threadKey(t)])
}

func (u *UI) refreshListBar() {
	if u.listBar == nil {
		return
	}
	u.listBar.Objects = []fyne.CanvasObject{u.buildListBar()}
	u.listBar.Refresh()
}

func (u *UI) popMenu(at fyne.CanvasObject, items ...*fyne.MenuItem) {
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(at).Add(fyne.NewPos(0, at.Size().Height))
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), u.win.Canvas(), pos)
}

// buildListBar is Gmail's list toolbar: select-all checkbox and dropdown,
// refresh, then bulk actions once rows are ticked, and a More menu.
func (u *UI) buildListBar() fyne.CanvasObject {
	sel := u.checkedThreads()
	n := len(sel)
	master := widget.NewCheck("", nil)
	master.SetChecked(n > 0 && n == len(u.threads))
	master.OnChanged = func(on bool) {
		if on {
			u.selectWhere(func(mailrat.ThreadItem) bool { return true })
		} else {
			u.selectWhere(nil)
		}
	}
	var drop *tooltipButton
	drop = iconButton("chevrondown", "Select", func() {
		pick := func(name string, keep func(mailrat.ThreadItem) bool) *fyne.MenuItem {
			return fyne.NewMenuItem(name, func() { u.selectWhere(keep) })
		}
		u.popMenu(drop,
			pick("All", func(mailrat.ThreadItem) bool { return true }),
			pick("None", nil),
			pick("Read", func(t mailrat.ThreadItem) bool { return !t.IsUnread }),
			pick("Unread", func(t mailrat.ThreadItem) bool { return t.IsUnread }),
			pick("Starred", func(t mailrat.ThreadItem) bool { return t.IsStarred }),
			pick("Unstarred", func(t mailrat.ThreadItem) bool { return !t.IsStarred }),
		)
	})
	items := []fyne.CanvasObject{master, drop, iconButton("sync", "Refresh", u.syncNow)}

	if n > 0 {
		spamTip := "Report spam"
		if u.activeLabel == "SPAM" {
			spamTip = "Not spam"
		}
		var move, labels *tooltipButton
		move = iconButton("move", "Move to", func() { u.popMenu(move, u.moveToItems()...) })
		labels = iconButton("tag", "Labels", func() { u.popMenu(labels, u.labelItems(sel)...) })
		items = append(items, spacer(10, 1),
			iconButton("archive", "Archive", u.bulkArchive),
			iconButton("spam", spamTip, u.bulkSpam),
			iconButton("trash", "Delete", u.bulkTrash),
			spacer(10, 1),
			iconButton("read", "Mark as read", func() { u.bulkRead(true) }),
			iconButton("unread", "Mark as unread", func() { u.bulkRead(false) }),
			spacer(10, 1), move, labels,
		)
	}
	var more *tooltipButton
	more = iconButton("more", "More", func() {
		item := func(name string, fn func()) *fyne.MenuItem {
			mi := fyne.NewMenuItem(name, fn)
			mi.Disabled = n == 0
			return mi
		}
		u.popMenu(more,
			item("Mark as read", func() { u.bulkRead(true) }),
			item("Mark as unread", func() { u.bulkRead(false) }),
			fyne.NewMenuItemSeparator(),
			item("Add star", func() { u.bulkLabel("STARRED", true) }),
			item("Remove star", func() { u.bulkLabel("STARRED", false) }),
			item("Mark as important", func() { u.bulkLabel("IMPORTANT", true) }),
			item("Mark as not important", func() { u.bulkLabel("IMPORTANT", false) }),
		)
	})
	items = append(items, more)
	return container.New(layout.NewCustomPaddedLayout(0, 2, 4, 4), container.NewHBox(items...))
}

func (u *UI) moveToItems() []*fyne.MenuItem {
	out := []*fyne.MenuItem{
		fyne.NewMenuItem("Inbox", func() { u.bulkLabel("INBOX", true) }),
		fyne.NewMenuItem("Spam", u.bulkSpam),
		fyne.NewMenuItem("Trash", u.bulkTrash),
		fyne.NewMenuItemSeparator(),
	}
	for _, l := range u.labels {
		if l.Type == "user" {
			out = append(out, fyne.NewMenuItem(l.Name, func() { u.bulkMoveTo(l.ID) }))
		}
	}
	return out
}

// labelItems lists user labels, ticked when every selected thread has them.
func (u *UI) labelItems(sel []mailrat.ThreadItem) []*fyne.MenuItem {
	var out []*fyne.MenuItem
	for _, l := range u.labels {
		if l.Type != "user" {
			continue
		}
		all := !slices.ContainsFunc(sel, func(t mailrat.ThreadItem) bool { return !slices.Contains(t.LabelIDs, l.ID) })
		mi := fyne.NewMenuItem(l.Name, func() { u.bulkLabel(l.ID, !all) })
		mi.Checked = all
		out = append(out, mi)
	}
	if len(out) == 0 {
		mi := fyne.NewMenuItem("No labels", nil)
		mi.Disabled = true
		out = append(out, mi)
	}
	return out
}

// bulk runs call on every ticked thread off the UI goroutine, then applies
// the successes to the list; the first failure is shown.
func (u *UI) bulk(call func(mailrat.ThreadItem) error, apply func([]mailrat.ThreadItem)) {
	sel := u.checkedThreads()
	if len(sel) == 0 {
		return
	}
	var ok []mailrat.ThreadItem
	var first error
	u.bg(func() {
		for _, t := range sel {
			if err := call(t); err != nil {
				if first == nil {
					first = err
				}
				continue
			}
			ok = append(ok, t)
		}
	}, func() {
		if first != nil {
			u.showError(first)
		}
		apply(ok)
		u.reloadLabels()
	})
}

// removeThreads drops threads from the list and the tick set.
func (u *UI) removeThreads(ts []mailrat.ThreadItem) {
	gone := map[string]bool{}
	for _, t := range ts {
		gone[threadKey(t)] = true
		delete(u.checked, threadKey(t))
	}
	kept := u.threads[:0:0]
	for i, t := range u.threads {
		if gone[threadKey(t)] {
			u.cursor, u.cursorNext = min(u.cursor, i-1), true
			continue
		}
		kept = append(kept, t)
	}
	u.threads = kept
	if u.selected != nil && gone[threadKey(*u.selected)] {
		u.clearSelection()
	}
	u.list.Refresh()
	u.refreshListBar()
}

func (u *UI) bulkArchive() {
	u.bulk(func(t mailrat.ThreadItem) error { return u.be.ArchiveThread(t.AccountID, t.ID) }, u.removeThreads)
}

func (u *UI) bulkSpam() {
	spam := u.be.MarkThreadSpam
	if u.activeLabel == "SPAM" {
		spam = u.be.MarkThreadNotSpam
	}
	u.bulk(func(t mailrat.ThreadItem) error { return spam(t.AccountID, t.ID) }, u.removeThreads)
}

func (u *UI) bulkTrash() {
	u.bulk(func(t mailrat.ThreadItem) error { return u.be.TrashThread(t.AccountID, t.ID) }, func(ok []mailrat.ThreadItem) {
		u.removeThreads(ok)
		if len(ok) == 1 {
			u.showUndoTrash(ok[0].AccountID, ok[0].ID)
		}
	})
}

// bulkRead marks every message of the ticked threads read, or their latest unread.
func (u *UI) bulkRead(read bool) {
	u.bulk(func(t mailrat.ThreadItem) error {
		msgs, err := u.be.GetThread(t.AccountID, t.ID)
		if err != nil || len(msgs) == 0 {
			return err
		}
		if !read {
			return u.be.MarkMessageUnread(t.AccountID, msgs[len(msgs)-1].ID)
		}
		for _, m := range msgs {
			if m.IsUnread {
				if err := u.be.MarkMessageRead(m.AccountID, m.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(ok []mailrat.ThreadItem) {
		for _, t := range ok {
			u.setThreadUnread(t.ID, t.AccountID, !read)
		}
	})
}

// bulkLabel adds or removes a label on the ticked threads.
func (u *UI) bulkLabel(id string, on bool) {
	u.bulk(func(t mailrat.ThreadItem) error { return u.be.SetThreadLabel(t.AccountID, t.ID, id, on) }, func(ok []mailrat.ThreadItem) {
		u.patchLabel(ok, id, on)
		if !on && u.activeLabel == id {
			u.removeThreads(ok)
		}
	})
}

// bulkMoveTo labels the ticked threads and archives them out of the Inbox.
func (u *UI) bulkMoveTo(id string) {
	u.bulk(func(t mailrat.ThreadItem) error {
		if err := u.be.SetThreadLabel(t.AccountID, t.ID, id, true); err != nil {
			return err
		}
		return u.be.ArchiveThread(t.AccountID, t.ID)
	}, func(ok []mailrat.ThreadItem) {
		u.patchLabel(ok, id, true)
		if u.activeLabel != id {
			u.removeThreads(ok)
		}
	})
}

func (u *UI) patchLabel(ts []mailrat.ThreadItem, id string, on bool) {
	for _, t := range ts {
		u.patchThread(t.ID, t.AccountID, func(x *mailrat.ThreadItem) {
			if id == "STARRED" {
				x.IsStarred = on
			}
			ids := slices.DeleteFunc(slices.Clone(x.LabelIDs), func(s string) bool { return s == id })
			if on {
				ids = append(ids, id)
			}
			x.LabelIDs = ids
		})
	}
	u.refreshListBar()
}

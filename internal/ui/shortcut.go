package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	mailrat "github.com/mailrat/mailrat/internal/desktop"
)

// sequenceWait is how long a "g" prefix waits for its second key.
const sequenceWait = 3 * time.Second

// cmd is ⌘+key on macOS (Ctrl+key elsewhere), optionally with Shift.
func cmd(key fyne.KeyName, shift bool) *desktop.CustomShortcut {
	m := fyne.KeyModifierShortcutDefault
	if shift {
		m |= fyne.KeyModifierShift
	}
	return &desktop.CustomShortcut{KeyName: key, Modifier: m}
}

// keyList is the thread list; it forwards typed keys to the Gmail handler
// instead of swallowing them while it has focus.
type keyList struct {
	widget.List
	u *UI
}

func (l *keyList) TypedRune(r rune) { l.u.typedRune(r) }
func (l *keyList) TypedKey(e *fyne.KeyEvent) {
	if !l.u.typedKey(e.Name) {
		l.List.TypedKey(e)
	}
}

// searchEntry gives up focus on Escape, as Gmail's search box does.
type searchEntry struct {
	widget.Entry
	u *UI
}

func newSearchEntry(u *UI) *searchEntry {
	e := &searchEntry{u: u}
	e.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	e.ExtendBaseWidget(e)
	return e
}

func (e *searchEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		e.u.win.Canvas().Unfocus()
		return
	}
	e.Entry.TypedKey(k)
}

// installShortcuts binds the Gmail web keyboard shortcuts plus a few ⌘ keys.
// Plain keys act whenever no text field has focus.
func (u *UI) installShortcuts() {
	c := u.win.Canvas()
	c.SetOnTypedRune(u.typedRune)
	c.SetOnTypedKey(func(e *fyne.KeyEvent) { u.typedKey(e.Name) })

	add := func(key fyne.KeyName, shift bool, fn func()) {
		c.AddShortcut(cmd(key, shift), func(fyne.Shortcut) { fn() })
	}
	add(fyne.KeyR, false, func() { u.reply(false) })
	add(fyne.KeyR, true, func() { u.reply(true) })
	add(fyne.KeyF, true, u.forward)
	add(fyne.KeyBackspace, false, u.trash)
	add(fyne.KeyComma, false, u.openSettings)
}

// typedKey handles the non-character keys; it reports whether it consumed e.
func (u *UI) typedKey(name fyne.KeyName) bool {
	if u.win.Canvas().Overlays().Top() != nil {
		return false
	}
	switch name {
	case fyne.KeyEscape:
		u.pendingKey = 0
		u.win.Canvas().Unfocus()
		u.backToList()
		return true
	case fyne.KeyReturn, fyne.KeyEnter:
		u.openOrToggle()
		return true
	}
	return false
}

// typedRune dispatches one typed character: either a single-key shortcut or
// the second key of a "g" sequence.
func (u *UI) typedRune(r rune) {
	if u.win.Canvas().Overlays().Top() != nil {
		return
	}
	if u.pendingKey != 0 {
		fresh := time.Since(u.pendingAt) < sequenceWait
		u.pendingKey = 0
		if fresh {
			u.goTo(r)
			return
		}
	}
	if r == 'g' {
		u.pendingKey, u.pendingAt = r, time.Now()
		return
	}
	if fn := u.singleKeys()[r]; fn != nil {
		fn()
	}
}

func (u *UI) singleKeys() map[rune]func() {
	reply := func() { u.reply(false) }
	replyAll := func() { u.reply(true) }
	compose := func() { u.compose(mailrat.OutboundMsg{}) }
	return map[rune]func(){
		'c': compose, 'd': compose,
		'/': func() { u.win.Canvas().Focus(u.search) },
		'?': u.showShortcutHelp,
		'j': func() { u.moveCursor(+1) },
		'k': func() { u.moveCursor(-1) },
		'o': u.openOrToggle,
		'u': u.syncNow,
		'n': func() { u.moveMessage(+1) },
		'p': func() { u.moveMessage(-1) },
		'e': u.archive,
		'y': u.removeFromView,
		'#': u.trash,
		'!': u.spam,
		's': u.toggleStar,
		'+': func() { u.setThreadLabel("IMPORTANT", true) },
		'=': func() { u.setThreadLabel("IMPORTANT", true) },
		'-': func() { u.setThreadLabel("IMPORTANT", false) },
		'r': reply, 'R': reply,
		'a': replyAll, 'A': replyAll,
		'f': u.forward, 'F': u.forward,
		'I': u.markRead,
		'U': u.markUnread, '_': u.markUnread,
		'N': u.syncNow,
	}
}

// goTo is the second key of "g": jump to a mailbox.
func (u *UI) goTo(r rune) {
	target := map[rune]string{'i': "INBOX", 's': "STARRED", 't': "SENT", 'd': "DRAFT", 'a': "ALL_MAIL"}
	if id, ok := target[r]; ok {
		u.selectLabel(id)
		return
	}
	if r == 'l' {
		u.goToLabel()
	}
}

// goToLabel is Gmail's "Go to label" box: type to filter, Enter picks the first match.
func (u *UI) goToLabel() {
	fav, user := u.mailboxes()
	all := append([]mailrat.LabelItem{{ID: "INBOX", Name: u.labelName("INBOX")}}, fav...)
	all = append(all, user...)

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Go to label…")
	list := container.NewVBox()
	var matches []mailrat.LabelItem
	var d dialog.Dialog
	pick := func(l mailrat.LabelItem) {
		d.Hide()
		u.selectLabel(l.ID)
	}
	refill := func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		matches = nil
		list.Objects = nil
		for _, l := range all {
			if len(matches) == 8 || !strings.Contains(strings.ToLower(l.Name), q) {
				continue
			}
			matches = append(matches, l)
			b := widget.NewButton(l.Name, func() { pick(l) })
			b.Alignment = widget.ButtonAlignLeading
			b.Importance = widget.LowImportance
			list.Add(b)
		}
		list.Refresh()
	}
	entry.OnChanged = refill
	entry.OnSubmitted = func(string) {
		if len(matches) > 0 {
			pick(matches[0])
		}
	}
	refill("")
	d = dialog.NewCustom("Go to label", "Cancel", container.NewBorder(entry, nil, nil, nil, list), u.win)
	d.Resize(fyne.NewSize(360, 380))
	d.Show()
	u.win.Canvas().Focus(entry)
}

// ── list and conversation cursors ───────────────────────────────────────────

// resetCursor forgets the list position when the mailbox changes.
func (u *UI) resetCursor() {
	u.cursor, u.cursorNext = -1, true
}

// moveCursor opens the next (+1, older) or previous (-1, newer) conversation.
func (u *UI) moveCursor(dir int) {
	if len(u.threads) == 0 {
		return
	}
	i := u.cursor
	if dir > 0 || !u.cursorNext {
		i += dir
	}
	i = max(0, min(i, len(u.threads)-1))
	u.list.Select(i)
	u.list.ScrollTo(i)
}

// openOrToggle is Enter / o: open the cursor conversation, or expand or
// collapse the cursor message when one is already open.
func (u *UI) openOrToggle() {
	if u.selected == nil {
		if len(u.threads) == 0 {
			return
		}
		i := u.cursor
		if u.cursorNext {
			i++
		}
		i = max(0, min(i, len(u.threads)-1))
		u.list.Select(i)
		u.list.ScrollTo(i)
		return
	}
	if u.msgCursor >= 0 && u.msgCursor < len(u.messages) {
		m := u.messages[u.msgCursor]
		u.collapsed[m.ID] = !u.collapsed[m.ID]
		u.refreshReader()
		u.scrollToMessage(u.msgCursor)
	}
}

// moveMessage is n / p: step to the next or previous message and open it.
func (u *UI) moveMessage(dir int) {
	if u.selected == nil || len(u.messages) == 0 {
		return
	}
	i := max(0, min(u.msgCursor+dir, len(u.messages)-1))
	u.msgCursor = i
	m := u.messages[i]
	def := i == len(u.messages)-1 || m.IsUnread
	u.collapsed[m.ID] = !def // force open
	u.refreshReader()
	u.scrollToMessage(i)
}

// backToList is Esc: leave the conversation and refresh the list.
func (u *UI) backToList() {
	if u.selected == nil {
		return
	}
	u.clearSelection()
	u.reloadThreads()
}

// ── label actions ───────────────────────────────────────────────────────────

func (u *UI) toggleStar() {
	if u.selected != nil {
		u.setThreadLabel("STARRED", !u.selected.IsStarred)
	}
}

// removeFromView is y: archive from the Inbox, unstar from Starred.
func (u *UI) removeFromView() {
	switch u.activeLabel {
	case "INBOX":
		u.archive()
	case "STARRED":
		u.setThreadLabel("STARRED", false)
	}
}

// setThreadLabel adds or removes STARRED / IMPORTANT on the open thread.
func (u *UI) setThreadLabel(id string, on bool) {
	if u.selected == nil {
		return
	}
	acct, tid := u.selected.AccountID, u.selected.ID
	var err error
	u.bg(func() { err = u.be.SetThreadLabel(acct, tid, id, on) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.patchThread(tid, acct, func(t *mailrat.ThreadItem) {
			if id == "STARRED" {
				t.IsStarred = on
			}
			ids := t.LabelIDs[:0:0]
			for _, x := range t.LabelIDs {
				if x != id {
					ids = append(ids, x)
				}
			}
			if on {
				ids = append(ids, id)
			}
			t.LabelIDs = ids
		})
		if !on && u.activeLabel == id {
			u.removeSelected()
		}
	})
}

// patchThread applies fn to the list row and the open thread with this ID.
func (u *UI) patchThread(id, acct string, fn func(*mailrat.ThreadItem)) {
	for i := range u.threads {
		if u.threads[i].ID == id && u.threads[i].AccountID == acct {
			fn(&u.threads[i])
		}
	}
	if u.selected != nil && u.selected.ID == id && u.selected.AccountID == acct {
		fn(u.selected)
	}
	u.list.Refresh()
}

// ── help ────────────────────────────────────────────────────────────────────

type helpSection struct {
	title string
	rows  [][2]string
}

var shortcutHelp = []helpSection{
	{"Jumping", [][2]string{
		{"g then i", "Go to Inbox"}, {"g then s", "Go to Starred"}, {"g then t", "Go to Sent Mail"},
		{"g then d", "Go to Drafts"}, {"g then a", "Go to All Mail"}, {"g then l", "Go to label…"},
		{"Esc", "Back to conversation list"}, {"j", "Next (older) conversation"}, {"k", "Previous (newer) conversation"},
		{"o  or  Enter", "Open conversation / expand message"},
		{"n", "Next message"}, {"p", "Previous message"},
	}},
	{"Actions", [][2]string{
		{"c  or  d", "Compose"}, {"/", "Search"}, {"e  or  y", "Archive / remove from view"},
		{"#", "Move to Trash"}, {"!", "Report spam"}, {"s", "Star / unstar"},
		{"+  or  =", "Mark important"}, {"-", "Mark not important"},
		{"r  or  Shift+r", "Reply"}, {"a  or  Shift+a", "Reply all"}, {"f  or  Shift+f", "Forward"},
		{"Shift+i", "Mark as read"}, {"Shift+u  or  _", "Mark as unread"},
		{"u", "Check for new mail"}, {"Shift+n", "Check for new mail"},
		{"⌘Enter", "Send (in a message window)"}, {"?", "Show this list"},
	}},
}

func (u *UI) showShortcutHelp() {
	body := container.NewVBox()
	for _, s := range shortcutHelp {
		body.Add(bold(s.title))
		for _, r := range s.rows {
			k := widget.NewLabel(r[0])
			k.TextStyle = fyne.TextStyle{Monospace: true}
			body.Add(container.NewGridWithColumns(2, k, widget.NewLabel(r[1])))
		}
		body.Add(hline())
	}
	d := dialog.NewCustom("Keyboard shortcuts", "Close", container.NewVScroll(body), u.win)
	d.Resize(fyne.NewSize(520, 560))
	d.Show()
}

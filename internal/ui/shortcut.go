package ui

import (
	"runtime"
	"slices"
	"sort"
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

// modalOpen reports whether a dialog or menu is showing. A tooltip is an
// overlay too, but it is passive and must not disable the keyboard.
func (u *UI) modalOpen() bool {
	return u.topModal() != nil
}

// topModal is the topmost overlay that is not a tooltip.
func (u *UI) topModal() fyne.CanvasObject {
	ovs := u.win.Canvas().Overlays().List()
	for i := len(ovs) - 1; i >= 0; i-- {
		if _, tip := ovs[i].(*tipOverlay); !tip {
			return ovs[i]
		}
	}
	return nil
}

// closeModal is Esc on a dialog or menu: close the topmost one.
func (u *UI) closeModal() {
	switch o := u.topModal().(type) {
	case *widget.PopUp:
		o.Hide()
	case fyne.CanvasObject:
		u.win.Canvas().Overlays().Remove(o)
	}
}

// typedKey handles the non-character keys; it reports whether it consumed e.
func (u *UI) typedKey(name fyne.KeyName) bool {
	if u.modalOpen() {
		if name == fyne.KeyEscape {
			u.closeModal()
			return true
		}
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
	if u.modalOpen() {
		return
	}
	if u.pendingKey != 0 {
		fresh := now().Sub(u.pendingAt) < sequenceWait
		u.pendingKey = 0
		if fresh {
			u.goTo(r)
			return
		}
	}
	if r == 'g' {
		u.pendingKey, u.pendingAt = r, now()
		return
	}
	if fn := u.singleKeys()[r]; fn != nil {
		fn()
	}
}

// ifChecked picks the bulk action when list rows are ticked, else the
// open-thread one.
func (u *UI) ifChecked(bulk, single func()) func() {
	return func() {
		if len(u.checkedThreads()) > 0 {
			bulk()
		} else {
			single()
		}
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
		'x': u.toggleCheckCursor,
		'u': u.syncNow,
		'n': func() { u.moveMessage(+1) },
		'p': func() { u.moveMessage(-1) },
		'e': u.ifChecked(u.bulkArchive, u.archive),
		'y': u.removeFromView,
		'#': u.ifChecked(u.bulkTrash, u.trash),
		'!': u.ifChecked(u.bulkSpam, u.spam),
		's': u.toggleStar,
		'l': u.labelPicker,
		'+': func() { u.setThreadLabel("IMPORTANT", true) },
		'=': func() { u.setThreadLabel("IMPORTANT", true) },
		'-': func() { u.setThreadLabel("IMPORTANT", false) },
		'r': reply, 'R': reply,
		'a': replyAll, 'A': replyAll,
		'f': u.forward, 'F': u.forward,
		'I': u.markRead,
		'U': u.markUnread, '_': u.markUnread,
		'N': u.syncNow,
		'v': u.toggleMailMode,
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

	var d dialog.Dialog
	entry := &escEntry{close: func() { d.Hide() }}
	entry.ExtendBaseWidget(entry)
	entry.SetPlaceHolder("Go to label…")
	list := container.NewVBox()
	var matches []mailrat.LabelItem
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

// labelPicker is Gmail's "Label as" popup: filter, toggle checkboxes, Apply,
// or create a new label. Enter applies; Esc closes (dialog default).
func (u *UI) labelPicker() {
	targets := u.checkedThreads()
	if len(targets) == 0 {
		if u.selected == nil {
			return
		}
		targets = []mailrat.ThreadItem{*u.selected}
	}
	if !u.oneAccount(targets) {
		return
	}
	acct := targets[0].AccountID
	have := map[string]bool{} // labels carried by every target
	for _, id := range targets[0].LabelIDs {
		have[id] = !slices.ContainsFunc(targets, func(t mailrat.ThreadItem) bool { return !slices.Contains(t.LabelIDs, id) })
	}
	var all []mailrat.LabelItem
	var err error
	u.bg(func() { all, err = u.be.GetLabels(acct) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		var user []mailrat.LabelItem
		for _, l := range all {
			if l.Type == "user" {
				user = append(user, l)
			}
		}
		sort.SliceStable(user, func(i, j int) bool { return strings.ToLower(user[i].Name) < strings.ToLower(user[j].Name) })
		u.showLabelPicker(acct, targets, user, have)
	})
}

func (u *UI) showLabelPicker(acct string, targets []mailrat.ThreadItem, labels []mailrat.LabelItem, have map[string]bool) {
	want := map[string]bool{}
	for id, all := range have {
		want[id] = all // a label only some targets carry starts unticked and untouched
	}
	var d dialog.Dialog
	entry := &escEntry{close: func() { d.Hide() }}
	entry.ExtendBaseWidget(entry)
	entry.SetPlaceHolder("Label as…")
	list := container.NewVBox()
	var shown []mailrat.LabelItem
	var refill func(string)
	refill = func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		list.Objects = nil
		shown = nil
		exact := false
		for _, l := range labels {
			exact = exact || strings.ToLower(l.Name) == q
			if !strings.Contains(strings.ToLower(l.Name), q) {
				continue
			}
			shown = append(shown, l)
			list.Add(widget.NewCheck(l.Name, func(on bool) { want[l.ID] = on }))
			list.Objects[len(list.Objects)-1].(*widget.Check).SetChecked(want[l.ID])
		}
		if name := strings.TrimSpace(entry.Text); name != "" && !exact {
			b := widget.NewButton("Create \""+name+"\"", func() {
				var l mailrat.LabelItem
				var err error
				u.bg(func() { l, err = u.be.CreateLabel(acct, name) }, func() {
					if err != nil {
						u.showError(err)
						return
					}
					labels = append(labels, l)
					want[l.ID] = true
					entry.SetText("")
					refill("")
				})
			})
			b.Alignment = widget.ButtonAlignLeading
			b.Importance = widget.LowImportance
			list.Add(b)
		}
		list.Refresh()
	}
	apply := func() {
		d.Hide()
		var first error
		changed := map[string][]string{} // thread -> labels that were really changed
		u.bg(func() {
			for _, t := range targets {
				for _, l := range labels {
					if want[l.ID] == have[l.ID] {
						continue
					}
					if e := u.be.SetThreadLabel(t.AccountID, t.ID, l.ID, want[l.ID]); e != nil {
						if first == nil {
							first = e
						}
						continue
					}
					changed[threadKey(t)] = append(changed[threadKey(t)], l.ID)
				}
			}
		}, func() {
			for _, t := range targets {
				ids := changed[threadKey(t)]
				if len(ids) == 0 {
					continue
				}
				u.patchThread(t.ID, t.AccountID, func(t *mailrat.ThreadItem) {
					kept := slices.DeleteFunc(slices.Clone(t.LabelIDs), func(x string) bool { return slices.Contains(ids, x) })
					for _, id := range ids {
						if want[id] {
							kept = append(kept, id)
						}
					}
					t.LabelIDs = kept
				})
			}
			if first != nil {
				u.showError(first)
			}
			if !want[u.activeLabel] && have[u.activeLabel] {
				if len(u.checkedThreads()) > 0 {
					u.removeThreads(targets)
				} else {
					u.removeSelected()
				}
			}
		})
	}
	entry.OnChanged = refill
	entry.OnSubmitted = func(string) {
		if len(shown) == 1 {
			want[shown[0].ID] = true
		}
		apply()
	}
	refill("")
	d = dialog.NewCustomConfirm("Label as", "Apply", "Cancel",
		container.NewBorder(entry, nil, nil, nil, container.NewVScroll(list)),
		func(ok bool) {
			if ok {
				apply()
			}
		}, u.win)
	d.Resize(fyne.NewSize(360, 420))
	d.Show()
	u.win.Canvas().Focus(entry)
}

// ── list and conversation cursors ───────────────────────────────────────────

// resetCursor forgets the list position when the mailbox changes.
func (u *UI) resetCursor() {
	u.cursor, u.cursorNext = -1, true
	u.checked = nil
	u.refreshListBar()
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
	u.selectRow(i)
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
		u.selectRow(i)
		u.list.ScrollTo(i)
		return
	}
	if u.isDraftThread() {
		u.editDraft()
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
	if n := len(u.messages); n > 3 && i >= 1 && i < n-2 {
		u.collapsed[allKey] = true // the message is in the folded middle: unfold it
	}
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
	set := func(on bool) {
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
		if id == "STARRED" {
			u.syncStarButton()
		}
	}
	if id == "STARRED" {
		set(on) // optimistic: the star flips at once and is put back if the call fails
	}
	var err error
	u.bg(func() { err = u.be.SetThreadLabel(acct, tid, id, on) }, func() {
		if err != nil {
			if id == "STARRED" {
				set(!on)
			}
			u.showErrorIn(u.win, err)
			return
		}
		if id != "STARRED" {
			set(on)
		}
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
	{"Navigation", [][2]string{
		{"g then i", "Go to Inbox"}, {"g then s", "Go to Starred"}, {"g then t", "Go to Sent Mail"},
		{"g then d", "Go to Drafts"}, {"g then a", "Go to All Mail"}, {"g then l", "Go to label…"},
		{"Esc", "Back to conversation list"}, {"j", "Next (older) conversation"}, {"k", "Previous (newer) conversation"},
		{"o  or  Enter", "Open conversation / expand message"},
		{"n", "Next message"}, {"p", "Previous message"},
		{"x", "Select conversation"},
	}},
	{"Actions", [][2]string{
		{"c  or  d", "Compose"}, {"/", "Search"}, {"e  or  y", "Archive / remove from view"},
		{"#", "Move to Trash"}, {"!", "Report spam"}, {"s", "Star / unstar"}, {"l", "Label as…"},
		{"+  or  =", "Mark important"}, {"-", "Mark not important"},
		{"r  or  Shift+r", "Reply"}, {"a  or  Shift+a", "Reply all"}, {"f  or  Shift+f", "Forward"},
		{"Shift+i", "Mark as read"}, {"Shift+u  or  _", "Mark as unread"},
		{"u", "Check for new mail"}, {"Shift+n", "Check for new mail"},
		{"⌘Enter", "Send (in a message window)"}, {"⌘Shift+a", "Attach files (in a message window)"},
		{"v", "Switch mail between light and dark"},
		{"⌘r  /  ⌘Shift+r", "Reply  /  reply all"}, {"⌘Shift+f", "Forward"},
		{"⌘Backspace", "Move to Trash"}, {"⌘,", "Settings"},
		{"?", "Show this list"},
	}},
}

// cmdLabel is how the command key is written in help: ⌘ on macOS, Ctrl+ elsewhere.
var cmdLabel = func() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl+"
}()

func (u *UI) showShortcutHelp() {
	body := container.NewVBox()
	for _, s := range shortcutHelp {
		body.Add(bold(s.title))
		for _, r := range s.rows {
			k := widget.NewLabel(strings.ReplaceAll(r[0], "⌘", cmdLabel))
			k.TextStyle = fyne.TextStyle{Monospace: true}
			body.Add(container.NewGridWithColumns(2, k, widget.NewLabel(r[1])))
		}
		body.Add(hline())
	}
	d := dialog.NewCustom("Keyboard shortcuts", "Close", container.NewVScroll(body), u.win)
	d.Resize(fyne.NewSize(520, 560))
	d.Show()
}

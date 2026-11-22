// Package ui is mailrat's native desktop interface, built with Fyne and laid
// out after Mimestream: account tiles and mailboxes on the left, a thread list
// in the middle, and the conversation with its toolbar on the right.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

const (
	unified      = "unified"
	pageSize     = 60
	searchLimit  = 50
	markReadWait = 5 * time.Second
)

// UI holds all interface state. Fields are touched only on the Fyne main
// goroutine; background work hands results back through fyne.Do.
type UI struct {
	app fyne.App
	be  Backend
	win fyne.Window

	// inline runs background work synchronously; tests set it.
	inline bool

	saveTail chan struct{} // completion of the latest bgOrdered write

	accounts      []desktop.AccountItem
	activeAccount string
	activeLabel   string
	labels        []desktop.LabelItem
	unread        map[string]int
	query         string

	threads     []desktop.ThreadItem
	allLoaded   bool
	loadingMore bool
	// threadLoading is true from click until the open thread's messages arrive.
	threadLoading bool
	loadGen       int
	listErr       string

	selected     *desktop.ThreadItem
	messages     []desktop.MessageItem
	openGroups   map[string]bool // expanded nested labels, by full name
	sidebarMini  bool            // sidebar shows icons only (persisted)
	labelsHidden bool            // sidebar Labels section collapsed (persisted)
	images       map[string]bool // messages whose remote images the user chose to load
	collapsed    map[string]bool // message IDs the user toggled away from default
	readTimer    *time.Timer
	threadGen    int
	syncStatus   string
	syncAnim     *fyne.Animation // spins the sync icon while syncing
	errCount     int

	sidebar    *fyne.Container
	listTitle  *widget.RichText
	list       *keyList
	listHolder *fyne.Container
	reader     *fyne.Container
	search     *searchEntry
	convScroll *container.Scroll
	msgCards   []fyne.CanvasObject

	// keyboard state: cursor is the last list row visited (cursorNext when the
	// row was removed, so the next row is already "under" the cursor);
	// msgCursor is the message n/p act on; pendingKey is a "g" awaiting its pair.
	cursor      int
	cursorNext  bool
	msgCursor   int
	pendingKey  rune
	pendingAt   time.Time
	settingsWin fyne.Window
	contactsWin fyne.Window
	loginDlg    dialog.Dialog
	undoPop     *widget.PopUp // "moved to Trash" bar with Undo
}

const (
	prefLabelsHidden = "sidebar.labelsHidden"
	prefSidebarMini  = "sidebar.mini"
	prefDarkMode     = "theme.mode" // "", "dark" or "light"; empty follows the OS

	sidebarWidth     float32 = 240 // expanded: icons + text
	sidebarMiniWidth float32 = 64  // minimal: icons only
)

// Run builds the main window and blocks until it is closed.
func Run(a fyne.App, be Backend) {
	a.Settings().SetTheme(themeFor(a.Preferences().String(prefDarkMode)))
	u := newUI(a, be)
	be.SetEventHandler(u.handleEvent)
	be.Start(context.Background())
	u.win = a.NewWindow("mailrat")
	u.win.Resize(fyne.NewSize(1280, 800))
	u.win.SetMaster()
	u.win.SetContent(u.build())
	u.installMenu()
	u.installShortcuts()
	u.watchVariant()
	u.reloadAll()
	if len(u.accounts) > 0 {
		u.syncNow()
	}
	// Quit via the Fyne loop on SIGTERM/SIGINT (e.g. a Homebrew upgrade) so
	// Shutdown below flushes pending work instead of the process dying.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		fyne.Do(a.Quit)
	}()
	u.win.ShowAndRun()
	signal.Stop(sig)
	be.Shutdown()
}

// watchVariant rebuilds the window when the OS flips between light and dark,
// since tinted icons are generated per variant.
func (u *UI) watchVariant() {
	last := u.app.Settings().ThemeVariant()
	ch := make(chan fyne.Settings)
	u.app.Settings().AddChangeListener(ch)
	go func() {
		for s := range ch {
			if v := s.ThemeVariant(); v != last {
				last = v
				fyne.Do(func() {
					u.win.SetContent(u.build())
					u.refreshAll()
				})
			}
		}
	}()
}

func themeFor(mode string) mimeTheme {
	return mimeTheme{forced: mode != "", dark: mode == "dark"}
}

// toggleDark flips between dark and light, persists the choice and rebuilds.
func (u *UI) toggleDark() {
	mode := "dark"
	if isDark() {
		mode = "light"
	}
	u.app.Preferences().SetString(prefDarkMode, mode)
	u.app.Settings().SetTheme(themeFor(mode))
	u.win.SetContent(u.build())
	u.refreshAll()
}

func (u *UI) refreshAll() {
	u.refreshSidebar()
	u.refreshListHeader()
	u.refreshReader()
	u.list.Refresh()
}

func newUI(a fyne.App, be Backend) *UI {
	return &UI{
		labelsHidden:  a.Preferences().Bool(prefLabelsHidden),
		sidebarMini:   a.Preferences().Bool(prefSidebarMini),
		app:           a,
		be:            be,
		activeAccount: unified,
		activeLabel:   "INBOX",
		unread:        map[string]int{},
		collapsed:     map[string]bool{},
		images:        map[string]bool{},
		openGroups:    map[string]bool{},
		cursor:        -1,
		cursorNext:    true,
	}
}

// bg runs work off the UI goroutine, then done on it.
func (u *UI) bg(work func(), done func()) {
	if u.inline {
		work()
		if done != nil {
			done()
		}
		return
	}
	go func() {
		work()
		if done != nil {
			fyne.Do(done)
		}
	}()
}

// bgOrdered is bg for writes that must land in call order (a setting saved on
// every keystroke): each work waits for the previous one to finish.
func (u *UI) bgOrdered(work func(), done func()) {
	prev, next := u.saveTail, make(chan struct{})
	u.saveTail = next
	u.bg(func() {
		if prev != nil {
			<-prev
		}
		work()
		close(next)
	}, done)
}

// build assembles the three panes. Called again when the theme variant flips,
// because tinted icons are baked per variant.
func (u *UI) build() fyne.CanvasObject {
	u.sidebar = container.NewStack()
	u.listTitle = widget.NewRichText()
	u.listHolder = container.NewStack()
	u.reader = container.NewStack()
	u.search = newSearchEntry(u)
	u.list = u.newThreadList()
	u.refreshSidebar()
	u.refreshListHeader()
	u.refreshReader()

	listPane := container.NewBorder(u.listHeader(), nil, nil, nil, u.list)
	right := container.NewHSplit(listPane, u.reader)
	w := sidebarWidth
	right.SetOffset(0.34)
	if u.sidebarMini {
		// A wider list gives the single-line rows room, as in Gmail.
		w = sidebarMiniWidth
		right.SetOffset(0.5)
	}
	return container.NewBorder(nil, nil, container.New(fixedWidth{w}, u.sidebar), nil, right)
}

// toggleSidebar flips between expanded and icon-only sidebar. The list rows
// change shape with it, so the panes are rebuilt.
func (u *UI) toggleSidebar() {
	u.sidebarMini = !u.sidebarMini
	u.app.Preferences().SetBool(prefSidebarMini, u.sidebarMini)
	u.win.SetContent(u.build())
	u.refreshAll()
}

func (u *UI) installMenu() {
	compose := fyne.NewMenuItem("New Message", func() { u.compose(desktop.OutboundMsg{}) })
	compose.Shortcut = cmd(fyne.KeyN, false)
	sync := fyne.NewMenuItem("Check for New Mail", u.syncNow)
	sync.Shortcut = cmd(fyne.KeyG, false)
	u.win.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("Mailbox", compose, sync)))
}

// ── data loading ────────────────────────────────────────────────────────────

func (u *UI) reloadAll() {
	u.reloadAccounts()
	u.reloadLabels()
	u.reloadThreads()
}

func (u *UI) reloadAccounts() {
	var accs []desktop.AccountItem
	u.bg(func() { accs, _ = u.be.GetAccounts() }, func() {
		u.accounts = accs
		u.refreshSidebar()
		if u.settingsWin != nil { // reload is async; rebuild so Settings shows the new list
			u.settingsWin.SetContent(u.buildSettings())
		}
	})
}

func (u *UI) labelAccountID() string {
	if u.activeAccount != unified {
		return u.activeAccount
	}
	if len(u.accounts) > 0 {
		return u.accounts[0].ID
	}
	return ""
}

func (u *UI) reloadLabels() {
	id := u.labelAccountID()
	if id == "" {
		return
	}
	scope := u.activeAccount
	var labels []desktop.LabelItem
	var counts map[string]int
	u.bg(func() {
		labels, _ = u.be.GetLabels(id)
		counts, _ = u.be.GetLabelUnreadCounts(scope)
	}, func() {
		u.labels = labels
		if counts == nil {
			counts = map[string]int{}
		}
		u.unread = counts
		u.refreshSidebar()
		u.refreshListHeader()
	})
}

func (u *UI) fetchThreads(offset int) ([]desktop.ThreadItem, error) {
	if u.query != "" {
		return u.be.SearchInbox(u.query, searchLimit)
	}
	if u.activeLabel == "INBOX" {
		return u.be.GetInbox(u.activeAccount, pageSize, offset)
	}
	return u.be.GetLabelThreads(u.activeAccount, u.activeLabel, pageSize, offset)
}

func (u *UI) reloadThreads() {
	u.loadGen++
	gen := u.loadGen
	var ts []desktop.ThreadItem
	var err error
	u.bg(func() { ts, err = u.fetchThreads(0) }, func() {
		if gen != u.loadGen {
			return
		}
		u.threads = ts
		u.allLoaded = u.query != "" || len(ts) < pageSize
		u.listErr = ""
		if err != nil {
			u.listErr = err.Error()
		}
		u.list.Refresh()
		u.keepSelectionVisible()
	})
}

// keepSelectionVisible re-highlights the open thread after the list reloads.
func (u *UI) keepSelectionVisible() {
	if u.selected == nil {
		u.list.UnselectAll()
		return
	}
	for i, t := range u.threads {
		if t.ID == u.selected.ID && t.AccountID == u.selected.AccountID {
			u.list.Select(i)
			return
		}
	}
}

func (u *UI) loadMore() {
	if u.loadingMore || u.allLoaded {
		return
	}
	u.loadingMore = true
	gen, offset := u.loadGen, len(u.threads)
	acct, label, searching := u.activeAccount, u.activeLabel, u.query != ""
	var more []desktop.ThreadItem
	exhausted := false
	u.bg(func() {
		more, _ = u.fetchThreads(offset)
		if len(more) >= pageSize || searching {
			return
		}
		// Past what is synced locally: pull older mail for this label from Gmail.
		if n, err := u.be.LoadOlder(acct, label); err == nil && n > 0 {
			more, _ = u.fetchThreads(offset)
			return
		}
		exhausted = true
	}, func() {
		u.loadingMore = false
		if gen != u.loadGen {
			return
		}
		if searching || exhausted {
			u.allLoaded = true
		}
		u.threads = append(u.threads, more...)
		u.list.Refresh()
	})
}

// ── navigation ──────────────────────────────────────────────────────────────

func (u *UI) selectAccount(id string) {
	if u.activeAccount == id && u.query == "" && u.activeLabel == "INBOX" {
		return
	}
	u.activeAccount, u.activeLabel, u.query = id, "INBOX", ""
	u.resetCursor()
	u.search.SetText("")
	u.clearSelection()
	u.refreshSidebar()
	u.refreshListHeader()
	u.reloadLabels()
	u.reloadThreads()
}

func (u *UI) selectLabel(id string) {
	u.activeLabel, u.query = id, ""
	u.resetCursor()
	u.search.SetText("")
	u.clearSelection()
	u.refreshSidebar()
	u.refreshListHeader()
	u.reloadThreads()
}

func (u *UI) runSearch(q string) {
	q = strings.TrimSpace(q)
	if q == u.query {
		return
	}
	u.query = q
	u.resetCursor()
	u.clearSelection()
	u.refreshListHeader()
	u.reloadThreads()
}

func (u *UI) clearSelection() {
	u.selected, u.messages = nil, nil
	u.threadGen++
	if u.readTimer != nil {
		u.readTimer.Stop()
	}
	if u.list != nil {
		u.list.UnselectAll()
	}
	u.refreshReader()
}

// labelName resolves a label ID to its display name.
func (u *UI) labelName(id string) string {
	for _, l := range u.labels {
		if l.ID == id {
			return l.Name
		}
	}
	if n, ok := fallbackNames[id]; ok {
		return n
	}
	return id
}

var fallbackNames = map[string]string{
	"INBOX": "Inbox", "STARRED": "Starred", "SENT": "Sent", "DRAFT": "Drafts",
	"IMPORTANT": "Important", "SPAM": "Spam", "TRASH": "Trash", "ALL_MAIL": "All Mail",
}

func (u *UI) accountName(id string) string {
	for _, a := range u.accounts {
		if a.ID == id {
			if a.DisplayName != "" {
				return a.DisplayName
			}
			return a.Email
		}
	}
	return ""
}

// ── thread actions ──────────────────────────────────────────────────────────

func (u *UI) removeSelected() {
	if u.selected == nil {
		return
	}
	id, acct := u.selected.ID, u.selected.AccountID
	kept := u.threads[:0:0]
	for i, t := range u.threads {
		if t.ID == id && t.AccountID == acct {
			u.cursor, u.cursorNext = i-1, true // next row slides under the cursor
			continue
		}
		kept = append(kept, t)
	}
	u.threads = kept
	u.clearSelection()
	u.list.Refresh()
	u.reloadLabels()
}

// threadAction runs a mutating backend call on the open thread, then removes it from view.
func (u *UI) threadAction(call func(accountID, threadID string) error) {
	if u.selected == nil {
		return
	}
	acct, id := u.selected.AccountID, u.selected.ID
	var err error
	u.bg(func() { err = call(acct, id) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.removeSelected()
	})
}

func (u *UI) archive() { u.threadAction(u.be.ArchiveThread) }
func (u *UI) trash() {
	if u.selected == nil {
		return
	}
	acct, id := u.selected.AccountID, u.selected.ID
	var err error
	u.bg(func() { err = u.be.TrashThread(acct, id) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.removeSelected()
		u.showUndoTrash(acct, id)
	})
}
func (u *UI) spam() {
	if u.activeLabel == "SPAM" {
		u.threadAction(u.be.MarkThreadNotSpam)
		return
	}
	u.threadAction(u.be.MarkThreadSpam)
}

func (u *UI) markUnread() {
	if u.selected == nil || len(u.messages) == 0 {
		return
	}
	last := u.messages[len(u.messages)-1]
	var err error
	u.bg(func() { err = u.be.MarkMessageUnread(last.AccountID, last.ID) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.setThreadUnread(u.selected.ID, u.selected.AccountID, true)
		u.messages[len(u.messages)-1].IsUnread = true
		u.reloadLabels()
	})
}

func (u *UI) setThreadUnread(id, acct string, unread bool) {
	for i := range u.threads {
		if u.threads[i].ID == id && u.threads[i].AccountID == acct {
			u.threads[i].IsUnread = unread
		}
	}
	if u.selected != nil && u.selected.ID == id {
		u.selected.IsUnread = unread
	}
	u.list.Refresh()
}

func (u *UI) lastMessage() (desktop.MessageItem, bool) {
	if len(u.messages) == 0 {
		return desktop.MessageItem{}, false
	}
	return u.messages[len(u.messages)-1], true
}

func (u *UI) reply(all bool) {
	if m, ok := u.lastMessage(); ok {
		u.compose(replyDraft(m, all))
	}
}

func (u *UI) forward() {
	if m, ok := u.lastMessage(); ok {
		u.compose(forwardDraft(m))
	}
}

// ── sync & events ───────────────────────────────────────────────────────────

func (u *UI) syncNow() {
	if u.syncStatus != "" {
		return
	}
	u.syncStatus = "Syncing…"
	u.refreshSidebar()
	scope := u.activeAccount
	u.bg(func() {
		_ = u.be.Sync(scope)
		u.be.DrainPendingOps()
	}, nil)
}

// handleEvent is the backend's event sink; it may run on any goroutine.
func (u *UI) handleEvent(name string, data any) {
	fyne.Do(func() {
		switch name {
		case "sync:progress":
			u.syncStatus = "Syncing…"
			if m, ok := data.(map[string]string); ok && m["account"] != "" {
				u.syncStatus = "Syncing " + m["account"] + "…"
			}
			u.refreshSidebar()
		case "sync:complete":
			u.syncStatus = ""
			u.refreshSidebar()
			u.reloadLabels()
		case "inbox:updated":
			u.reloadThreads()
		case "accounts:updated":
			u.closeLogin()
			u.reloadAll()
			u.openSettings()
			u.syncNow()
		case "account:add:error":
			u.closeLogin()
			if msg := fmt.Sprint(data); msg != "" {
				u.showError(fmt.Errorf("%s", msg))
			}
		case "api:error":
			u.errCount++
			u.refreshSidebar()
		}
	})
}

func (u *UI) showError(err error) {
	if u.win != nil {
		dialog.ShowError(err, u.win)
	}
}

func (u *UI) showErrorLog() {
	errs := u.be.GetAPIErrors()
	lines := make([]string, 0, len(errs))
	for _, e := range errs {
		lines = append(lines, time.UnixMilli(e.Timestamp).Format("Jan 2 15:04:05")+"  "+e.Message)
	}
	text := "No errors."
	if len(lines) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(lines)))
		text = strings.Join(lines, "\n")
	}
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm("Error Log", "Clear", "Close", container.NewVScroll(l), func(clear bool) {
		if clear {
			u.be.ClearAPIErrors()
			u.errCount = 0
			u.refreshSidebar()
		}
	}, u.win)
	d.Resize(fyne.NewSize(560, 360))
	d.Show()
}

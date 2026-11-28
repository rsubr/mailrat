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
	olderFailed bool            // last fetch of older mail failed; stops auto-retry
	checked     map[string]bool // ticked rows, by threadKey
	listBar     *fyne.Container // list toolbar, rebuilt as the selection changes
	// threadLoading is true from click until the open thread's messages arrive.
	threadLoading bool
	loadGen       int
	listErr       string
	threadsScope  listScope         // what u.threads was loaded for
	filters       map[string]string // list filter per mailbox view (filterKey), this session only
	filterBtn     *tooltipButton

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
	launchSync   bool            // sync once when the first account load lands (accounts load async)
	syncAnim     *fyne.Animation // spins the sync icon while syncing
	errCount     int
	loadErrs     []string    // load failures shown in the error log beside backend API errors
	loadErrAt    []time.Time // when each of loadErrs happened, for ordering the log

	sidebar    *fyne.Container
	listTitle  *widget.RichText
	listErrLbl *widget.Label
	list       *keyList
	listHolder *fyne.Container
	reader     *fyne.Container
	search     *searchEntry
	starBtn    *tooltipButton // reader toolbar star toggle; mirrors u.selected.IsStarred
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
	mailMode    string          // "", "light" or "dark": the mail view's own mode; empty follows the app theme
	undoPop     *undoBar        // "moved to Trash" bar with Undo
	undoTimer   *time.Timer     // expires undoPop; stopped when the bar is replaced or hidden
	undoHolder  *fyne.Container // overlay-free slot the bar is drawn in
	listLoading bool            // first page of the current list is in flight
	emptyLbl    *widget.Label   // "No conversations" / "Loading…" over an empty list
}

const (
	prefLabelsHidden = "sidebar.labelsHidden"
	prefSidebarMini  = "sidebar.mini"
	prefDarkMode     = "theme.mode" // "", "dark" or "light"; empty follows the OS

	sidebarWidth     float32 = 240 // expanded: icons + text
	sidebarMiniWidth float32 = 64  // minimal: icons only
	listMinWidth     float32 = 200 // thread list pane; below this rows cannot fit star, clip and date
)

// Run builds the main window and blocks until it is closed.
func Run(a fyne.App, be Backend) {
	a.Settings().SetTheme(themeFor(a.Preferences().String(prefDarkMode)))
	cleanAttachTemp()
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
	u.launchSync = true
	u.reloadAll()
	// Quit via the Fyne loop on SIGTERM/SIGINT (e.g. a Homebrew upgrade) so
	// Shutdown below flushes pending work instead of the process dying.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		fyne.Do(a.Quit)
	}()
	// Window is created by Show; maximize once the loop runs (main thread).
	a.Lifecycle().SetOnStarted(func() { maximize(u.win) })
	u.win.ShowAndRun()
	signal.Stop(sig)
	cleanAttachTemp()
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
		mailMode:      a.Preferences().String(prefMailMode),
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
	u.emptyLbl = widget.NewLabel("")
	u.emptyLbl.Alignment = fyne.TextAlignCenter
	u.emptyLbl.Importance = widget.LowImportance
	u.undoHolder = container.NewVBox()
	u.showUndoBar()
	u.refreshSidebar()
	u.refreshListHeader()
	u.refreshReader()
	if u.query != "" {
		u.search.SetText(u.query) // a rebuild must not leave an active search with an empty field
	}

	listPane := container.New(minWidth{listMinWidth}, container.NewBorder(u.listHeader(), nil, nil, nil, container.NewStack(u.list, container.NewVBox(spacer(1, 40), u.emptyLbl))))
	right := container.NewHSplit(listPane, u.reader)
	w := sidebarWidth
	right.SetOffset(0.34)
	if u.sidebarMini {
		// A wider list gives the single-line rows room, as in Gmail.
		w = sidebarMiniWidth
		right.SetOffset(0.5)
	}
	return container.NewStack(container.NewBorder(nil, nil, container.New(fixedWidth{w}, u.sidebar), nil, right), u.undoHolder)
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
	var err error
	u.bg(func() { accs, err = u.be.GetAccounts() }, func() {
		u.accounts = accs
		u.noteLoadErr("accounts", err)
		if u.launchSync {
			u.launchSync = false
			if len(accs) > 0 {
				u.syncNow()
			}
		}
		u.refreshSidebar()
		u.refreshReader()
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
	var labelsErr, countsErr error
	u.bg(func() {
		labels, labelsErr = u.be.GetLabels(id)
		counts, countsErr = u.be.GetLabelUnreadCounts(scope)
	}, func() {
		u.labels = labels
		u.noteLoadErr("labels", labelsErr)
		u.noteLoadErr("unread counts", countsErr)
		if counts == nil {
			counts = map[string]int{}
		}
		u.unread = counts
		u.refreshSidebar()
		u.refreshListHeader()
	})
}

// noteLoadErr records a failed background load in the error log and sidebar badge.
func (u *UI) noteLoadErr(what string, err error) {
	if err == nil {
		return
	}
	u.noteErr("could not load "+what, err)
}

// noteErr records a failure in the error log and sidebar badge.
func (u *UI) noteErr(what string, err error) {
	if err == nil {
		return
	}
	now := time.Now()
	u.loadErrs = append(u.loadErrs, now.Format(errLogTime)+"  "+what+": "+err.Error())
	u.loadErrAt = append(u.loadErrAt, now)
	u.errCount++
}

const errLogTime = "Jan 2 15:04:05"

// listScope is what the list shows; fetches get a copy because they run off
// the UI goroutine while the fields change.
type listScope struct{ query, account, label, filter string }

func (u *UI) scope() listScope {
	return listScope{u.query, u.activeAccount, u.activeLabel, u.filter()}
}

func (u *UI) fetchThreads(sc listScope, limit, offset int) ([]desktop.ThreadItem, error) {
	if sc.filter != "" {
		return u.be.FilterThreads(sc.filter, sc.account, sc.label, sc.query, limit, offset)
	}
	if sc.query != "" {
		return u.be.SearchInbox(sc.query, searchLimit)
	}
	if sc.label == "INBOX" {
		return u.be.GetInbox(sc.account, limit, offset)
	}
	return u.be.GetLabelThreads(sc.account, sc.label, limit, offset)
}

func (u *UI) reloadThreads() {
	u.loadGen++
	gen := u.loadGen
	sc := u.scope()
	// A background refresh of the same list keeps every page already loaded.
	limit := pageSize
	if sc == u.threadsScope {
		limit = max(pageSize, len(u.threads))
	}
	u.listLoading = len(u.threads) == 0
	u.refreshEmpty()
	var ts []desktop.ThreadItem
	var err error
	u.bg(func() { ts, err = u.fetchThreads(sc, limit, 0) }, func() {
		if gen != u.loadGen {
			return
		}
		u.listLoading = false
		u.threads, u.threadsScope = ts, sc
		u.allLoaded = sc.query != "" || len(ts) < limit
		u.olderFailed = false
		u.listErr = ""
		if err != nil {
			u.listErr = err.Error()
		}
		u.refreshListHeader()
		u.refreshListBar()
		u.list.Refresh()
		u.keepSelectionVisible()
	})
}

// refreshEmpty shows why the list is blank: loading, no results, or no mail.
func (u *UI) refreshEmpty() {
	if u.emptyLbl == nil {
		return
	}
	text := ""
	if len(u.threads) == 0 && len(u.accounts) > 0 && u.listErr == "" {
		switch {
		case u.listLoading && u.query != "":
			text = "Searching…"
		case u.listLoading:
			text = "Loading…"
		case u.query != "" && u.filter() != "":
			text = "No " + filterNoun(u.filter()) + " results for “" + u.query + "”"
		case u.query != "":
			text = "No results for “" + u.query + "”"
		case u.filter() != "":
			text = "No " + filterNoun(u.filter()) + " conversations"
		case u.activeLabel == "DRAFT":
			text = "No drafts"
		default:
			text = "No conversations"
		}
	}
	u.emptyLbl.SetText(text)
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
	sc := u.scope()
	acct, label, searching := sc.account, sc.label, sc.query != ""
	var more []desktop.ThreadItem
	exhausted, failed := false, false
	var moreErr, olderErr error
	if !u.inline {
		fyne.Do(u.list.Refresh) // show the loading indicator (may be called mid-render)
	}
	u.bg(func() {
		more, moreErr = u.fetchThreads(sc, pageSize, offset)
		if moreErr != nil || len(more) >= pageSize || searching {
			return
		}
		// Past what is synced locally: pull older mail for this label from Gmail.
		n, err := u.be.LoadOlder(acct, label)
		switch {
		case err != nil:
			failed, olderErr = true, err
		case n > 0:
			more, moreErr = u.fetchThreads(sc, pageSize, offset)
			exhausted = moreErr == nil && len(more) == 0 // nothing new to show: stop asking
		default:
			exhausted = true
		}
	}, func() {
		u.loadingMore = false
		if gen != u.loadGen {
			return
		}
		u.olderFailed = failed || moreErr != nil
		u.noteLoadErr("more threads", moreErr)
		u.noteLoadErr("older mail", olderErr)
		u.refreshSidebar()
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
	if u.selected != nil {
		u.removeThread(u.selected.AccountID, u.selected.ID)
	}
}

// removeThread drops one thread from the list. When it is the open one, the
// next conversation opens; a thread the user has since moved away from only
// disappears from the list.
func (u *UI) removeThread(acct, id string) {
	wasOpen := u.selected != nil && u.selected.ID == id && u.selected.AccountID == acct
	kept := u.threads[:0:0]
	at := -1
	for i, t := range u.threads {
		if t.ID == id && t.AccountID == acct {
			at = i
			continue
		}
		kept = append(kept, t)
	}
	delete(u.checked, acct+"/"+id)
	u.threads = kept
	switch {
	case wasOpen:
		u.cursor, u.cursorNext = at-1, true // next row slides under the cursor
		u.clearSelection()
	case at >= 0 && at <= u.cursor:
		u.cursor--
	}
	u.list.Refresh()
	u.refreshEmpty()
	u.refreshListBar()
	if wasOpen && at >= 0 && len(kept) > 0 { // Gmail: open the next conversation, else the previous
		next := min(at, len(kept)-1)
		u.list.Select(next)
		u.list.ScrollTo(next)
	}
	u.reloadLabels()
}

// threadAction runs a mutating backend call on the open thread, then removes
// that thread from view (not whatever is open by the time the call returns)
// and lets then offer an undo.
func (u *UI) threadAction(call func(accountID, threadID string) error, then func([]desktop.ThreadItem)) {
	if u.selected == nil {
		return
	}
	t := *u.selected
	var err error
	u.bg(func() { err = call(t.AccountID, t.ID) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.removeThread(t.AccountID, t.ID)
		if then != nil {
			then([]desktop.ThreadItem{t})
		}
	})
}

func (u *UI) archive() { u.threadAction(u.be.ArchiveThread, u.undoArchive) }
func (u *UI) trash()   { u.threadAction(u.be.TrashThread, u.undoTrash) }
func (u *UI) spam() {
	if u.activeLabel == "SPAM" {
		u.threadAction(u.be.MarkThreadNotSpam, nil)
		return
	}
	u.threadAction(u.be.MarkThreadSpam, u.undoSpam)
}

func (u *UI) markUnread() {
	if u.selected == nil || len(u.messages) == 0 {
		return
	}
	if u.readTimer != nil {
		u.readTimer.Stop() // else the pending auto-read undoes this
	}
	t := *u.selected
	last := u.messages[len(u.messages)-1]
	var err error
	u.bg(func() { err = u.be.MarkMessageUnread(last.AccountID, last.ID) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.setThreadUnread(t.ID, t.AccountID, true)
		if u.selected != nil && u.selected.ID == t.ID && u.selected.AccountID == t.AccountID && len(u.messages) > 0 {
			u.messages[len(u.messages)-1].IsUnread = true
		}
		u.reloadLabels()
	})
}

func (u *UI) setThreadUnread(id, acct string, unread bool) {
	for i := range u.threads {
		if u.threads[i].ID == id && u.threads[i].AccountID == acct {
			u.threads[i].IsUnread = unread
		}
	}
	if u.selected != nil && u.selected.ID == id && u.selected.AccountID == acct {
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
		u.compose(replyDraft(m, all, u.ownAddrs()...))
	}
}

func (u *UI) forward() {
	m, ok := u.lastMessage()
	if !ok {
		return
	}
	d := forwardDraft(m)
	var err error
	u.bg(func() { // the original's attachments travel with the forward
		for _, a := range m.Attachments {
			var data desktop.AttachmentDataItem
			if data, err = u.be.GetAttachmentData(m.AccountID, m.ID, a.AttachmentID); err != nil {
				return
			}
			d.Attachments = append(d.Attachments, desktop.OutAttachment{Filename: a.Name, MIMEType: a.MimeType, Size: int64(len(data.Data)), Data: data.Data})
		}
	}, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.compose(d)
	})
}

// ── sync & events ───────────────────────────────────────────────────────────

func (u *UI) syncNow() {
	if u.syncStatus != "" {
		return
	}
	u.syncStatus = "Syncing…"
	u.refreshSidebar()
	scope := u.activeAccount
	var err error
	u.bg(func() {
		err = u.be.Sync(scope)
		u.be.DrainPendingOps()
	}, func() {
		if err == nil {
			return // sync:complete clears the status
		}
		u.syncStatus = ""
		u.noteErr("sync failed", err)
		u.refreshSidebar()
	})
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
		case "account:add:retry": // a code was rejected; the sign-in dialog stays open for another try
			u.showErrorIn(u.parentWindow(), fmt.Errorf("%v", data))
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
	type entry struct {
		at   time.Time
		line string
	}
	var log []entry
	for _, e := range u.be.GetAPIErrors() {
		at := time.UnixMilli(e.Timestamp)
		log = append(log, entry{at, at.Format(errLogTime) + "  " + e.Message})
	}
	for i, line := range u.loadErrs {
		log = append(log, entry{u.loadErrAt[i], line})
	}
	text := "No errors."
	if len(log) > 0 { // newest first
		sort.SliceStable(log, func(i, j int) bool { return log[i].at.After(log[j].at) })
		lines := make([]string, len(log))
		for i, e := range log {
			lines[i] = e.line
		}
		text = strings.Join(lines, "\n")
	}
	// A Label cannot select text; a multi-line Entry can. Edits are reverted so
	// it behaves as read-only while still allowing mouse select and Ctrl+C.
	l := widget.NewMultiLineEntry()
	l.Wrapping = fyne.TextWrapWord
	l.SetText(text)
	l.OnChanged = func(s string) {
		if s != text {
			l.SetText(text)
		}
	}
	d := dialog.NewCustomConfirm("Error Log", "Clear", "Close", l, func(clear bool) {
		if clear {
			u.be.ClearAPIErrors()
			u.errCount, u.loadErrs, u.loadErrAt = 0, nil, nil
			u.refreshSidebar()
		}
	}, u.win)
	d.Resize(fyne.NewSize(560, 360))
	d.Show()
}

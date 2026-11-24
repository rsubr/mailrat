package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// f1Backend wraps the fake so a test can act "while a call is in flight":
// with u.inline the call runs synchronously, so a hook models the user doing
// something before the result comes back.
type f1Backend struct {
	*fakeBackend
	hook   map[string]func()
	thread []desktop.MessageItem
}

func (b *f1Backend) run(m string) {
	if h := b.hook[m]; h != nil {
		h()
	}
}
func (b *f1Backend) ArchiveThread(a, id string) error {
	b.run("ArchiveThread")
	return b.fakeBackend.ArchiveThread(a, id)
}
func (b *f1Backend) TrashThread(a, id string) error {
	b.run("TrashThread")
	return b.fakeBackend.TrashThread(a, id)
}
func (b *f1Backend) MarkMessageUnread(a, id string) error {
	b.run("MarkMessageUnread")
	return b.fakeBackend.MarkMessageUnread(a, id)
}
func (b *f1Backend) GetThread(a, id string) ([]desktop.MessageItem, error) {
	if b.thread != nil {
		return slices.Clone(b.thread), nil
	}
	return b.fakeBackend.GetThread(a, id)
}

// f1UI is a UI at the 1470x918 target on a wrapped backend.
func f1UI(t *testing.T, opts ...testOpt) (*UI, *f1Backend) {
	t.Helper()
	u, fb := newTestUI(t, opts...)
	b := &f1Backend{fakeBackend: fb, hook: map[string]func(){}}
	u.be = b
	u.installShortcuts()
	return u, b
}

func f1Open(u *UI, id string) {
	for i, t := range u.threads {
		if t.ID == id {
			u.list.Select(i)
			return
		}
	}
}

func f1IDs(u *UI) (out []string) {
	for _, t := range u.threads {
		out = append(out, t.ID)
	}
	return
}

// M1: the call returns after the user opened another thread; the row that
// leaves the list is the archived one.
func TestF1ArchiveRemovesTheArchivedRow(t *testing.T) {
	for _, act := range []string{"archive", "trash"} {
		u, b := f1UI(t)
		f1Open(u, "t1")
		m := "ArchiveThread"
		if act == "trash" {
			m = "TrashThread"
		}
		b.hook[m] = func() { f1Open(u, "t3") }
		if act == "trash" {
			u.trash()
		} else {
			u.archive()
		}
		ids := f1IDs(u)
		if slices.Contains(ids, "t1") || !slices.Contains(ids, "t3") {
			t.Fatalf("%s: rows %v, want t1 gone and t3 kept", act, ids)
		}
		if u.selected == nil || u.selected.ID != "t3" {
			t.Fatalf("%s: the reader must stay on t3, got %v", act, u.selected)
		}
	}
}

// M2: Esc or a mailbox switch before MarkMessageUnread returns must not panic.
func TestF1MarkUnreadAfterSelectionCleared(t *testing.T) {
	u, b := f1UI(t)
	f1Open(u, "t3")
	b.hook["MarkMessageUnread"] = func() { u.clearSelection() }
	u.markUnread() // panics on a nil selection if unguarded
	if u.selected != nil {
		t.Fatal("selection should stay cleared")
	}
}

// m4b: marking unread cancels the pending auto-read.
func TestF1MarkUnreadStopsReadTimer(t *testing.T) {
	u, _ := f1UI(t)
	f1Open(u, "t3")
	fired := make(chan struct{}, 1)
	u.readTimer = time.AfterFunc(50*time.Millisecond, func() { fired <- struct{}{} })
	u.markUnread()
	select {
	case <-fired:
		t.Fatal("auto mark-read still pending after Mark unread")
	case <-time.After(150 * time.Millisecond):
	}
}

// M3: a Sync that fails must not leave "Syncing…" forever.
func TestF1SyncErrorClearsStatus(t *testing.T) {
	u, fb := f1UI(t)
	fb.errs = map[string]error{"Sync": errors.New("no accounts configured")}
	before := u.errCount
	u.syncNow()
	if u.syncStatus != "" || u.errCount != before+1 {
		t.Fatalf("status=%q errCount=%d", u.syncStatus, u.errCount)
	}
	fb.errs = nil
	u.syncNow()
	if fb.syncs != 1 {
		t.Fatalf("a later sync must still run, syncs=%d", fb.syncs)
	}
}

// M4: a label only some ticked threads carry is not ticked and not applied.
func TestF1LabelPickerPartialLabelUntouched(t *testing.T) {
	u, fb := f1UI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" || t.ID == "t3" || t.ID == "t4" })
	u.labelPicker() // t1 carries L1 only
	checks := keysChecks(u)
	if c := checks["Inquiries"]; c == nil || c.Checked {
		t.Fatalf("partially held label must start unticked: %v", c)
	}
	fb.labelled = nil
	test.Tap(keysButton(t, u, "Apply"))
	if len(fb.labelled) != 0 {
		t.Fatalf("Apply without changes touched labels: %v", fb.labelled)
	}
	if !slices.Contains(u.threads[0].LabelIDs, "L1") {
		t.Fatal("the row must keep its label chip")
	}
}

// m12: successes are patched even when another label call fails.
func TestF1LabelPickerPatchesSuccessesOnError(t *testing.T) {
	u, _ := f1UI(t)
	f1Open(u, "t3")
	u.labelPicker()
	checks := keysChecks(u)
	test.Tap(checks["Inquiries"])
	test.Tap(checks["Travel"])
	n := 0
	u.be = &f1FailSecond{f1Backend: u.be.(*f1Backend), n: &n}
	test.Tap(keysButton(t, u, "Apply"))
	var row desktop.ThreadItem
	for _, r := range u.threads {
		if r.ID == "t3" {
			row = r
		}
	}
	if len(row.LabelIDs) != 1 {
		t.Fatalf("exactly the label that was applied should show: %v", row.LabelIDs)
	}
}

type f1FailSecond struct {
	*f1Backend
	n *int
}

func (b *f1FailSecond) SetThreadLabel(a, id, l string, on bool) error {
	*b.n++
	if *b.n == 2 {
		return errors.New("denied")
	}
	return b.f1Backend.SetThreadLabel(a, id, l, on)
}

func keysButton(t *testing.T, u *UI, text string) *widget.Button {
	t.Helper()
	b := readerButton(keysOverlay(u), text)
	if b == nil {
		t.Fatalf("no %q button in %v", text, readerTexts(keysOverlay(u)))
	}
	return b
}

// Unified view: label IDs are per account, so a mixed selection is refused.
func TestF1LabelPickerRefusesMixedAccounts(t *testing.T) {
	u, fb := f1UI(t)
	u.threads[1].AccountID = "a2"
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" || t.ID == "t2" })
	fb.calls = nil
	u.labelPicker()
	if slices.Contains(fb.calls, "GetLabels") || keysChecks(u)["Inquiries"] != nil {
		t.Fatalf("label picker opened for two accounts: %v", fb.calls)
	}
	if o := keysOverlay(u); o == nil || !readerHas(o, "single account") {
		t.Fatal("the user must be told why")
	}
}

// M5: the undo bar is not an overlay, so it neither eats the next click nor
// leaves a stale entry that confuses modalOpen.
func TestF1UndoBarIsNotAnOverlay(t *testing.T) {
	u, _ := f1UI(t)
	f1Open(u, "t3")
	u.trash()
	if u.undoPop == nil || len(u.win.Canvas().Overlays().List()) != 0 {
		t.Fatalf("undo bar=%v overlays=%d", u.undoPop, len(u.win.Canvas().Overlays().List()))
	}
	if !readerHas(u.undoHolder, "Conversation moved to Trash") {
		t.Fatal("undo bar is not drawn in the window")
	}
	u.hideUndo()
	if readerHas(u.undoHolder, "Undo") {
		t.Fatal("undo bar still drawn after hide")
	}
	u.showShortcutHelp()
	if !u.modalOpen() {
		t.Fatal("a dialog after the undo bar was dismissed must count as modal")
	}
}

// A tooltip must not disable the keyboard.
func TestF1TooltipIsNotModal(t *testing.T) {
	u, _ := f1UI(t)
	b := newTooltipButton(nil, "tip", func() {})
	u.win.SetContent(container.NewStack(u.win.Content(), b))
	b.showTip()
	if len(u.win.Canvas().Overlays().List()) == 0 {
		t.Fatal("test needs a tooltip overlay")
	}
	if u.modalOpen() {
		t.Fatal("tooltip counted as a modal")
	}
	typeKeys(u, "j")
	if u.selected == nil {
		t.Fatal("j must work while a tooltip shows")
	}
}

// Live M3: archive gets the same bar and an undo that restores the Inbox.
func TestF1ArchiveShowsUndo(t *testing.T) {
	u, fb := f1UI(t)
	f1Open(u, "t1") // carries L1 only: give it INBOX like a real inbox row
	u.threads[0].LabelIDs = []string{"INBOX"}
	u.selected.LabelIDs = []string{"INBOX"}
	u.archive()
	if u.undoPop == nil || !readerHas(u.undoPop.Content, "Conversation archived") {
		t.Fatal("archive shows no undo bar")
	}
	fb.labelled = nil
	test.Tap(readerButton(u.undoPop.Content, "Undo"))
	if !slices.Equal(fb.labelled, []string{"t1:INBOX:true"}) || u.undoPop != nil {
		t.Fatalf("undo: %v", fb.labelled)
	}
	// Bulk archive and spam too.
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t2" || t.ID == "t3" })
	for i := range u.threads {
		u.threads[i].LabelIDs = []string{"INBOX"}
	}
	u.bulkArchive()
	if u.undoPop == nil || !readerHas(u.undoPop.Content, "2 conversations archived") {
		t.Fatal("bulk archive shows no undo bar")
	}
	f1Open(u, "t4")
	u.spam()
	if u.undoPop == nil || !readerHas(u.undoPop.Content, "marked as spam") {
		t.Fatal("spam shows no undo bar")
	}
}

// Esc closes dialogs, including with a text box focused where one is ours.
func TestF1EscClosesDialogs(t *testing.T) {
	u, _ := f1UI(t)
	esc := &fyne.KeyEvent{Name: fyne.KeyEscape}
	for name, open := range map[string]func(){
		"help":      u.showShortcutHelp,
		"error log": u.showErrorLog,
		"go to":     u.goToLabel,
		"label":     func() { f1Open(u, "t3"); u.labelPicker() },
	} {
		open()
		if keysOverlay(u) == nil {
			t.Fatalf("%s did not open", name)
		}
		u.win.Canvas().OnTypedKey()(esc)
		if keysOverlay(u) != nil {
			t.Errorf("Esc did not close %s", name)
		}
	}
}

// M6: a blank list says why.
func TestF1EmptyStates(t *testing.T) {
	u, fb := f1UI(t, withEmpty())
	if u.emptyLbl.Text != "No conversations" {
		t.Fatalf("empty inbox: %q", u.emptyLbl.Text)
	}
	u.selectLabel("DRAFT")
	if u.emptyLbl.Text != "No drafts" {
		t.Fatalf("empty drafts: %q", u.emptyLbl.Text)
	}
	u.runSearch("zzz")
	if u.emptyLbl.Text != "No results for “zzz”" {
		t.Fatalf("empty search: %q", u.emptyLbl.Text)
	}
	// While the first page is in flight it says so.
	u.listLoading = true
	u.refreshEmpty()
	if u.emptyLbl.Text != "Searching…" {
		t.Fatalf("loading: %q", u.emptyLbl.Text)
	}
	u.query = ""
	u.refreshEmpty()
	if u.emptyLbl.Text != "Loading…" {
		t.Fatalf("loading: %q", u.emptyLbl.Text)
	}
	u.listLoading = false
	// Rows hide the label.
	u.runSearch("")
	u.selectLabel("INBOX")
	fb.fakeBackend.threadsOverride = nil
	u.reloadThreads()
	if u.emptyLbl.Text != "" {
		t.Fatalf("label over a populated list: %q", u.emptyLbl.Text)
	}
}

// m1 + live m4: leaving a mailbox clears the ticks and the bulk bar with them.
func TestF1BulkBarResetsOnNavigation(t *testing.T) {
	u, _ := f1UI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" || t.ID == "t2" })
	if navTip(u.listBar, "Archive") == nil {
		t.Fatal("bulk bar missing with ticks")
	}
	u.selectLabel("SENT")
	if navTip(u.listBar, "Archive") != nil {
		t.Fatal("bulk bar stuck after changing mailbox")
	}
	var master *widget.Check
	navWalk(u.listBar, func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok {
			master = c
		}
	})
	if master == nil || master.Checked {
		t.Fatal("master checkbox ticked with no rows ticked")
	}
}

// Live m4: ticking rows must not change the minimum width of the list pane
// (it moved the divider by ~90px).
func TestF1BulkBarDoesNotWidenPane(t *testing.T) {
	u, _ := f1UI(t)
	before := u.listBar.MinSize().Width
	u.selectWhere(func(t desktop.ThreadItem) bool { return true })
	if after := u.listBar.MinSize().Width; after != before {
		t.Fatalf("min width %v -> %v", before, after)
	}
	// At the target size every action is still visible (no horizontal scroll needed).
	var sc *container.Scroll
	navWalk(u.listBar, func(o fyne.CanvasObject) {
		if s, ok := o.(*container.Scroll); ok {
			sc = s
		}
	})
	if sc == nil || sc.Content.MinSize().Width > sc.Size().Width {
		t.Fatalf("bulk actions clipped: content %v in %v", sc.Content.MinSize(), sc.Size())
	}
}

// m10: Move to > Spam inside Spam still reports spam (not un-spam).
func TestF1MoveToSpamIsUnconditional(t *testing.T) {
	u, fb := f1UI(t)
	u.activeLabel = "STARRED"
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" })
	u.bulkSetSpam(true)
	if !slices.Equal(fb.spammed, []string{"t1"}) {
		t.Fatalf("spammed=%v", fb.spammed)
	}
	u.activeLabel = "SPAM"
	for _, mi := range u.moveToItems() {
		if mi.Label == "Spam" {
			t.Fatal("Spam offered while viewing Spam")
		}
	}
}

// m13: the error log is newest first across months.
func TestF1ErrorLogOrder(t *testing.T) {
	u, fb := f1UI(t)
	jan := time.Date(2026, 1, 9, 9, 0, 0, 0, time.Local)
	sep := time.Date(2026, 9, 2, 9, 0, 0, 0, time.Local)
	nov := time.Date(2026, 11, 1, 9, 0, 0, 0, time.Local)
	fb.apiErrs = []desktop.APIError{{Timestamp: jan.UnixMilli(), Message: "jan-err"}, {Timestamp: nov.UnixMilli(), Message: "nov-err"}}
	u.loadErrs = []string{sep.Format(errLogTime) + "  sep-err"}
	u.loadErrAt = []time.Time{sep}
	u.showErrorLog()
	text := strings.Join(readerTexts(keysOverlay(u)), "\n")
	i, j, k := strings.Index(text, "nov-err"), strings.Index(text, "sep-err"), strings.Index(text, "jan-err")
	if !(i >= 0 && i < j && j < k) {
		t.Fatalf("order wrong: %q", text)
	}
}

// m6: a background refresh keeps the pages already loaded.
func TestF1ReloadKeepsLoadedPages(t *testing.T) {
	var many []desktop.ThreadItem
	for i := 0; i < 130; i++ {
		many = append(many, desktop.ThreadItem{ID: fmt.Sprintf("x%d", i), AccountID: "a1", Subject: "s"})
	}
	u, _ := f1UI(t, func(c *testConfig) { c.fb.threadsOverride = &many })
	u.be = &f1Pager{f1Backend: u.be.(*f1Backend), all: many}
	u.reloadThreads()
	u.loadMore()
	u.loadMore()
	if len(u.threads) != 130 {
		t.Fatalf("setup: %d rows", len(u.threads))
	}
	u.reloadThreads()
	if len(u.threads) != 130 {
		t.Fatalf("a refresh shrank the list to %d rows", len(u.threads))
	}
}

type f1Pager struct {
	*f1Backend
	all []desktop.ThreadItem
}

func (b *f1Pager) GetInbox(_ string, limit, off int) ([]desktop.ThreadItem, error) {
	return b.all[min(off, len(b.all)):min(off+limit, len(b.all))], nil
}

// m7: a failed LoadOlder reaches the error log.
func TestF1LoadOlderErrorLogged(t *testing.T) {
	var full []desktop.ThreadItem
	for i := 0; i < 60; i++ {
		full = append(full, desktop.ThreadItem{ID: fmt.Sprintf("x%d", i), AccountID: "a1"})
	}
	u, fb := f1UI(t, func(c *testConfig) { c.fb.threadsOverride = &full })
	fb.errs = map[string]error{"LoadOlder": errors.New("offline")}
	u.be = &f1Pager{f1Backend: fb, all: full}
	u.loadMore()
	if !u.olderFailed || !slices.ContainsFunc(u.loadErrs, func(s string) bool { return strings.Contains(s, "offline") }) {
		t.Fatalf("olderFailed=%v log=%v", u.olderFailed, u.loadErrs)
	}
}

func f1LongThread(n int) []desktop.MessageItem {
	var out []desktop.MessageItem
	for i := 0; i < n; i++ {
		out = append(out, desktop.MessageItem{ID: fmt.Sprintf("m%d", i), AccountID: "a1", FromAddr: "A <a@x.com>", ToAddr: "B <b@x.com>", Snippet: "hello", Body: "<p>" + strings.Repeat("line of body text ", 40) + "</p>", Date: ms(100 - i)})
	}
	return out
}

// m3: n steps into the folded middle unfold it.
func TestF1NextMessageUnfoldsHiddenGroup(t *testing.T) {
	u, b := f1UI(t)
	b.thread = f1LongThread(10)
	f1Open(u, "t3")
	u.msgCursor = 0
	u.moveMessage(+1)
	if !u.collapsed[allKey] {
		t.Fatal("hidden group still folded after n")
	}
}

// m2: re-rendering the conversation keeps the scroll position.
func TestF1ReaderKeepsScroll(t *testing.T) {
	u, b := f1UI(t)
	b.thread = f1LongThread(3)
	b.thread[2].Body = strings.Repeat("<p>paragraph of the newest message</p>", 80)
	f1Open(u, "t3")
	u.convScroll.ScrollToOffset(fyne.NewPos(0, 120))
	want := u.convScroll.Offset.Y
	if want == 0 {
		t.Fatal("test needs a scrollable conversation")
	}
	u.refreshReader()
	if got := u.convScroll.Offset.Y; got != want {
		t.Fatalf("scroll %v -> %v", want, got)
	}
	f1Open(u, "t4")
	if u.convScroll.Offset.Y != 0 {
		t.Fatal("a newly opened thread must start at the top")
	}
}

// m4a: a failed mark-read must not show the thread as read.
func TestF1MarkReadFailureKeepsUnread(t *testing.T) {
	u, fb := f1UI(t)
	f1Open(u, "t1")
	u.messages[0].IsUnread = true
	fb.errs = map[string]error{"MarkMessageRead": errors.New("queue full")}
	u.markRead()
	if !u.threads[0].IsUnread || u.errCount == 0 {
		t.Fatalf("unread=%v errCount=%d", u.threads[0].IsUnread, u.errCount)
	}
}

// m8: a failed open can be retried by clicking the same row.
func TestF1OpenThreadErrorAllowsRetry(t *testing.T) {
	u, fb := f1UI(t)
	fb.errs = map[string]error{"GetThread": errors.New("db busy")}
	f1Open(u, "t3")
	if u.selected != nil {
		t.Fatal("failed open left the thread selected")
	}
	fb.errs = nil
	f1Open(u, "t3")
	if u.selected == nil || len(u.messages) == 0 {
		t.Fatal("clicking the row again did not retry")
	}
}

// Live M2: snippets are plain text in the collapsed header and the list.
func TestF1SnippetEntitiesAreNotShown(t *testing.T) {
	u, b := f1UI(t)
	b.thread = []desktop.MessageItem{
		{ID: "m1", AccountID: "a1", FromAddr: "A <a@x.com>", Snippet: "tomorrow's meeting & Thanks &amp; R", Body: "<p>x</p>", Date: ms(30)},
		{ID: "m2", AccountID: "a1", FromAddr: "B <b@x.com>", Snippet: "last", Body: "<p>y</p>", Date: ms(1)},
	}
	f1Open(u, "t3")
	if !readerHas(u.reader, "tomorrow's meeting & Thanks &amp; R") {
		t.Fatalf("preview: %v", readerTexts(u.reader))
	}
	// A snippet-only message is rendered as text, not as markup.
	b.thread[1].Body, b.thread[1].Snippet = " ", "1 < 2 & done" // blank body: not refetched
	u.openThread(u.threads[2])
	var body string
	readerWalk(u.reader, func(o fyne.CanvasObject) {
		if sb, ok := o.(*selBody); ok {
			body += string(sb.doc.flat) + "|"
		}
	})
	if !strings.Contains(body, "1 < 2 & done") {
		t.Fatalf("snippet body %q", body)
	}
}

// m17: rebuilding the window keeps the search field in step with the query.
func TestF1RebuildKeepsSearchText(t *testing.T) {
	u, _ := f1UI(t)
	u.runSearch("invoice")
	u.win.SetContent(u.build())
	if u.search.Text != "invoice" {
		t.Fatalf("search field %q", u.search.Text)
	}
}

// m15: icon-only sidebar still exposes the error count.
func TestF1MiniSidebarShowsErrorBadge(t *testing.T) {
	u, _ := f1UI(t)
	u.sidebarMini = true
	u.errCount = 3
	u.refreshSidebar()
	if len(winButtons(u.sidebar, "3")) != 1 {
		t.Fatal("no error badge in the mini sidebar")
	}
}

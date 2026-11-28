package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// navWalk visits o and every descendant, including widget renderer objects.
func navWalk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			navWalk(c, fn)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			navWalk(c, fn)
		}
	}
}

func navText(o fyne.CanvasObject) string {
	switch v := o.(type) {
	case *widget.Label:
		return v.Text
	case *canvas.Text:
		return v.Text
	case *widget.RichText:
		return v.String()
	case *widget.Button:
		return v.Text
	}
	return ""
}

// navTexts collects the visible text under o.
func navTexts(o fyne.CanvasObject) (out []string) {
	navWalk(o, func(c fyne.CanvasObject) {
		if s := navText(c); s != "" && c.Visible() {
			out = append(out, s)
		}
	})
	return
}

func navHas(o fyne.CanvasObject, text string) bool { return slices.Contains(navTexts(o), text) }

// navRowWith returns the first tapRow under o whose text includes text.
func navRowWith(t *testing.T, o fyne.CanvasObject, text string) *tapRow {
	t.Helper()
	var found *tapRow
	navWalk(o, func(c fyne.CanvasObject) {
		if r, ok := c.(*tapRow); ok && found == nil && navHas(r, text) {
			found = r
		}
	})
	if found == nil {
		t.Fatalf("no row with text %q", text)
	}
	return found
}

// navTip returns the toolbar button with the given tooltip, or nil.
func navTip(o fyne.CanvasObject, tip string) *tooltipButton {
	var found *tooltipButton
	navWalk(o, func(c fyne.CanvasObject) {
		if b, ok := c.(*tooltipButton); ok && b.tip == tip && found == nil {
			found = b
		}
	})
	return found
}

func navTitle(u *UI) string { return u.listTitle.String() }

// navThreads is a deterministic inbox for goldens (pair with freezeNow(goldenNow)).
func navThreads() []desktop.ThreadItem {
	return []desktop.ThreadItem{
		{ID: "t1", AccountID: "a1", Subject: "Positions at Mimestream?", Participants: "Dan Smith <dan@x.com>", Snippet: "Hey Neil, I hope all is well.", Date: msAt(1), MessageCount: 5, IsUnread: true, LabelIDs: []string{"L1"}},
		{ID: "t2", AccountID: "a1", Subject: "Lake Tahoe Trip", Participants: "Keith <k@x.com>, Bruce <b@x.com>", Snippet: "I might also come in on the evening", Date: msAt(2), MessageCount: 5, HasAttachments: true, IsStarred: true, LabelIDs: []string{"L2"}},
		{ID: "t3", AccountID: "a1", Subject: "Lunch this Friday?", Participants: "Ben Johnson <ben@x.com>", Snippet: "12PM at Delarosa sounds great", Date: msAt(20), MessageCount: 3},
		{ID: "t4", AccountID: "a1", Subject: "Interesting Article", Participants: "Samantha Schultz <s@x.com>", Snippet: "I thought you might like this.", Date: msAt(60)},
		{ID: "t5", AccountID: "a1", Subject: "Referral?", Participants: "Gregory Gardner <g@x.com>", Snippet: "Hey Neil! I was interested in a job", Date: msAt(24 * 30)},
	}
}

func navUI(t *testing.T, opts ...testOpt) (*UI, *fakeBackend) {
	t.Helper()
	freezeNow(t, goldenNow)
	ts := navThreads()
	fb := &fakeBackend{threadsOverride: &ts}
	u, fb := newTestUI(t, append([]testOpt{withBackend(fb)}, opts...)...)
	fb.SetEventHandler(u.handleEvent) // Run does this; newTestUI does not
	return u, fb
}

func navLast(fb *fakeBackend) string { return fb.calls[len(fb.calls)-1] }

func TestNavAccountSwitch(t *testing.T) {
	u, fb := navUI(t)
	if u.activeAccount != unified || u.labelAccountID() != "a1" || navTitle(u) != "Inbox" {
		t.Fatalf("initial: acct=%q labels-of=%q title=%q", u.activeAccount, u.labelAccountID(), navTitle(u))
	}
	u.search.SetText("old") // switching accounts must drop an active search
	u.query = "old"
	fb.calls = nil
	test.Tap(navRowWith(t, u.sidebar, "Work"))
	if u.activeAccount != "a2" || u.activeLabel != "INBOX" || u.query != "" || u.search.Text != "" {
		t.Fatalf("after tap: acct=%q label=%q query=%q search=%q", u.activeAccount, u.activeLabel, u.query, u.search.Text)
	}
	if u.labelAccountID() != "a2" || navTitle(u) != "Inbox: Work" {
		t.Fatalf("labels-of=%q title=%q", u.labelAccountID(), navTitle(u))
	}
	for _, m := range []string{"GetLabels", "GetLabelUnreadCounts", "GetInbox"} {
		if !slices.Contains(fb.calls, m) {
			t.Errorf("%s not reloaded for new account: %v", m, fb.calls)
		}
	}
	// Back to unified.
	test.Tap(navRowWith(t, u.sidebar, "Unified"))
	if u.activeAccount != unified || navTitle(u) != "Inbox" {
		t.Fatalf("unified: acct=%q title=%q", u.activeAccount, navTitle(u))
	}
	// Re-selecting the active account is a no-op (no reload).
	fb.calls = nil
	u.selectAccount(unified)
	if len(fb.calls) != 0 {
		t.Fatalf("reselect reloaded: %v", fb.calls)
	}
}

func TestNavSidebarFavoritesAndLabels(t *testing.T) {
	u, _ := navUI(t)
	fb := u.be.(*fakeBackend)
	fb.labelsOverride = []desktop.LabelItem{
		{ID: "INBOX", Name: "Inbox", Type: "system"},
		{ID: "UNREAD", Name: "Unread", Type: "system"},
		{ID: "CATEGORY_SOCIAL", Name: "Social", Type: "system"},
		{ID: "L1", Name: "Inquiries", Type: "user"},
		{ID: "L3", Name: "Home", Type: "user"},
		{ID: "L4", Name: "Home/Remodel", Type: "user"},
	}
	u.reloadLabels()
	for _, want := range []string{"Inboxes", "Favorites", "Starred", "Drafts", "Sent", "Spam", "Trash", "Labels", "Inquiries", "Home"} {
		if !navHas(u.sidebar, want) {
			t.Errorf("sidebar missing %q: %v", want, navTexts(u.sidebar))
		}
	}
	for _, hidden := range []string{"Unread", "Social"} {
		if navHas(u.sidebar, hidden) {
			t.Errorf("hidden label %q shown", hidden)
		}
	}
	if navHas(u.sidebar, "Remodel") {
		t.Fatal("nested label visible before parent expanded")
	}
	// Unread badge: counts are Inbox 12, Drafts 2.
	if !navHas(navRowWith(t, u.sidebar, "Inbox"), "12") || !navHas(navRowWith(t, u.sidebar, "Drafts"), "2") {
		t.Error("unread badges missing")
	}
	if navHas(navRowWith(t, u.sidebar, "Sent"), "12") {
		t.Error("badge on wrong row")
	}
	// Tapping a parent expands it and opens it.
	test.Tap(navRowWith(t, u.sidebar, "Home"))
	if !u.openGroups["Home"] || u.activeLabel != "L3" || !navHas(u.sidebar, "Remodel") {
		t.Fatalf("expand: open=%v label=%q", u.openGroups, u.activeLabel)
	}
	if navTitle(u) != "Home" {
		t.Fatalf("title %q", navTitle(u))
	}
	test.Tap(navRowWith(t, u.sidebar, "Home")) // collapse again
	if u.openGroups["Home"] || navHas(u.sidebar, "Remodel") {
		t.Fatal("collapse failed")
	}
	// Collapsing the Labels section hides user labels, keeps favorites.
	test.Tap(navRowWith(t, u.sidebar, "Labels"))
	if navHas(u.sidebar, "Inquiries") || !navHas(u.sidebar, "Starred") {
		t.Fatalf("labels section: %v", navTexts(u.sidebar))
	}
}

func TestNavSelectLabel(t *testing.T) {
	u, fb := navUI(t)
	fb.labelThreads = []desktop.ThreadItem{{ID: "x1", AccountID: "a1", Subject: "Flight", Participants: "Air <a@x.com>", Date: msAt(3), LabelIDs: []string{"L2"}}}
	fb.calls = nil
	test.Tap(navRowWith(t, u.sidebar, "Travel"))
	if u.activeLabel != "L2" || navTitle(u) != "Travel" || len(u.threads) != 1 || u.threads[0].ID != "x1" {
		t.Fatalf("label=%q title=%q threads=%v", u.activeLabel, navTitle(u), u.threads)
	}
	if !slices.Contains(fb.calls, "GetLabelThreads") || slices.Contains(fb.calls, "GetInbox") {
		t.Fatalf("wrong fetch: %v", fb.calls)
	}
	if !u.allLoaded {
		t.Error("short page should mark all loaded")
	}
	// Under a single account the title names the account.
	u.selectAccount("a2")
	u.selectLabel("STARRED")
	if navTitle(u) != "Starred: Work" {
		t.Fatalf("title %q", navTitle(u))
	}
	// Selecting a label clears an active search.
	u.runSearch("abc")
	u.selectLabel("SENT")
	if u.query != "" || navTitle(u) != "Sent: Work" {
		t.Fatalf("query=%q title=%q", u.query, navTitle(u))
	}
}

func TestNavThreadRowContent(t *testing.T) {
	u, _ := navUI(t)
	ts := u.threads
	row := newThreadRow(false)

	u.setRow(row, ts[0]) // unread, chip, 1h ago (11:00 AM on the frozen day)
	if row.when.Text != "11:00 AM" || !row.when.TextStyle.Bold {
		t.Errorf("unread date %q bold=%v", row.when.Text, row.when.TextStyle.Bold)
	}
	if got := row.count.String(); got != "  5" {
		t.Errorf("count %q", got)
	}
	if got := row.head.String(); got != "Dan Smith" {
		t.Errorf("sender %q", got)
	}
	if !navHas(row.chips, "Inquiries") || row.clip.Visible() || row.star.Visible() {
		t.Errorf("chips=%v clip=%v star=%v", navTexts(row.chips), row.clip.Visible(), row.star.Visible())
	}

	u.setRow(row, ts[1]) // starred + attachment, multiple senders shortened to first names
	if got := row.head.String(); got != "Keith, Bruce" {
		t.Errorf("participants %q", got)
	}
	if !row.clip.Visible() || !row.star.Visible() || row.when.TextStyle.Bold || !navHas(row.chips, "Travel") {
		t.Errorf("flags clip=%v star=%v", row.clip.Visible(), row.star.Visible())
	}

	u.setRow(row, ts[2]) // 20h ago crosses midnight
	if row.when.Text != "Yesterday" || len(row.chips.Objects) != 0 {
		t.Errorf("date %q chips=%d", row.when.Text, len(row.chips.Objects))
	}
	u.setRow(row, ts[3]) // 60h ago
	if row.when.Text != "Thursday" {
		t.Errorf("weekday date %q", row.when.Text)
	}
	u.setRow(row, ts[4]) // 30 days ago, same year
	if row.when.Text != "Feb 12" {
		t.Errorf("short date %q", row.when.Text)
	}
	if got := row.count.String(); got != "" {
		t.Errorf("single message should show no count: %q", got)
	}
}

func TestNavListSelectionOpensThread(t *testing.T) {
	u, _ := navUI(t)
	if u.list.Length() != 6 { // 5 threads + footer
		t.Fatalf("length %d", u.list.Length())
	}
	u.list.Select(5) // the footer is not a thread
	if u.selected != nil {
		t.Fatal("footer selection opened a thread")
	}
	u.list.Select(1)
	if u.selected == nil || u.selected.ID != "t2" || u.cursor != 1 {
		t.Fatalf("selected=%v cursor=%d", u.selected, u.cursor)
	}
}

func TestNavPaginationFooter(t *testing.T) {
	u, fb := navUI(t)
	if !u.allLoaded {
		t.Fatal("5 < pageSize should be fully loaded")
	}
	c := u.newListCell()
	u.showFooter(c)
	if c.text.Text != "No more emails" || c.btn.Visible() || c.bar.Visible() {
		t.Fatalf("end footer %q btn=%v", c.text.Text, c.btn.Visible())
	}
	fb.calls = nil
	u.loadMore()
	if len(fb.calls) != 0 || len(u.threads) != 5 {
		t.Fatalf("loadMore after end hit backend: %v", fb.calls)
	}

	// A full page leaves the Fetch more button.
	full := make([]desktop.ThreadItem, pageSize)
	for i := range full {
		full[i] = desktop.ThreadItem{ID: fmt.Sprintf("p%d", i), AccountID: "a1", Subject: "s", Participants: "A <a@x.com>", Date: msAt(i)}
	}
	fb.threadsOverride = &full
	u.reloadThreads()
	if u.allLoaded || len(u.threads) != pageSize {
		t.Fatalf("allLoaded=%v n=%d", u.allLoaded, len(u.threads))
	}
	u.showFooter(c)
	if !c.btn.Visible() || c.text.Visible() {
		t.Fatal("more-available footer should show only the button")
	}
	// Another full page is appended without asking Gmail for older mail.
	fb.calls = nil
	u.loadMore()
	if len(u.threads) != 2*pageSize || slices.Contains(fb.calls, "LoadOlder") || u.allLoaded {
		t.Fatalf("n=%d calls=%v", len(u.threads), fb.calls)
	}
}

func TestNavPaginationExhaustsAndRetries(t *testing.T) {
	u, fb := navUI(t)
	full := make([]desktop.ThreadItem, pageSize)
	for i := range full {
		full[i] = desktop.ThreadItem{ID: fmt.Sprintf("p%d", i), AccountID: "a1", Subject: "s", Participants: "A <a@x.com>", Date: msAt(i)}
	}
	fb.threadsOverride = &full
	u.reloadThreads()

	// Local mail runs out and Gmail has nothing older: end of list.
	empty := []desktop.ThreadItem{}
	fb.threadsOverride = &empty
	fb.calls = nil
	u.loadMore()
	if !slices.Contains(fb.calls, "LoadOlder") || !u.allLoaded || len(u.threads) != pageSize {
		t.Fatalf("calls=%v allLoaded=%v n=%d", fb.calls, u.allLoaded, len(u.threads))
	}

	// A failed fetch of older mail shows the retry state; Fetch more retries.
	u.reloadThreads()
	full2 := full
	fb.threadsOverride = &full2
	u.reloadThreads()
	fb.threadsOverride = &empty
	fb.errs = map[string]error{"LoadOlder": errors.New("offline")}
	u.loadMore()
	if !u.olderFailed || u.allLoaded {
		t.Fatalf("olderFailed=%v allLoaded=%v", u.olderFailed, u.allLoaded)
	}
	c := u.newListCell()
	u.showFooter(c)
	if c.text.Text != "Could not fetch older emails" || !c.btn.Visible() {
		t.Fatalf("failed footer %q btn=%v", c.text.Text, c.btn.Visible())
	}
	fb.errs = nil
	fb.calls = nil
	test.Tap(c.btn)
	if u.olderFailed || !u.allLoaded || !slices.Contains(fb.calls, "LoadOlder") {
		t.Fatalf("retry: olderFailed=%v allLoaded=%v calls=%v", u.olderFailed, u.allLoaded, fb.calls)
	}
}

func TestNavLoadingFooter(t *testing.T) {
	u, _ := navUI(t)
	u.loadingMore = true
	c := u.newListCell()
	u.showFooter(c)
	if c.text.Text != "Loading older emails…" || !c.bar.Visible() || c.btn.Visible() {
		t.Fatalf("loading footer %q", c.text.Text)
	}
}

func TestNavEmptyInbox(t *testing.T) {
	u, fb := navUI(t, withEmpty())
	if len(u.threads) != 0 || u.list.Length() != 0 || u.listErr != "" {
		t.Fatalf("threads=%d length=%d err=%q", len(u.threads), u.list.Length(), u.listErr)
	}
	u.loadMore() // nothing to page; must not hit Gmail
	if slices.Contains(fb.calls, "LoadOlder") {
		t.Fatal("empty inbox asked for older mail")
	}
	// Bulk actions with nothing ticked do nothing.
	fb.calls = nil
	u.bulkArchive()
	if len(fb.calls) != 0 {
		t.Fatalf("calls %v", fb.calls)
	}
}

func TestNavListLoadError(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"GetInbox": errors.New("boom")}}
	u, _ := newTestUI(t, withBackend(fb))
	if u.listErr != "boom" || len(u.threads) != 0 {
		t.Fatalf("err=%q threads=%d", u.listErr, len(u.threads))
	}
	// Recovery: the next successful load clears the error.
	fb.errs = nil
	u.reloadThreads()
	if u.listErr != "" || len(u.threads) != 5 {
		t.Fatalf("err=%q threads=%d", u.listErr, len(u.threads))
	}
}

func TestNavListErrorIsShownToUser(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"GetInbox": errors.New("boom")}}
	u, _ := newTestUI(t, withBackend(fb))
	if !winHasText(u.win.Content(), "Could not load mail: boom") {
		t.Fatalf("list error not shown: %v", winTexts(u.win.Content()))
	}
	// Recovery clears it.
	fb.errs = nil
	u.reloadThreads()
	if winHasText(u.win.Content(), "Could not load mail") {
		t.Fatal("stale list error after successful reload")
	}
}

func TestNavReaderEmptyStateAfterAccountsLoad(t *testing.T) {
	u, _ := newTestUI(t)
	if winHasText(u.reader, "Welcome to mailrat") || !winHasText(u.reader, "No Conversation Selected") {
		t.Fatalf("reader after accounts load: %v", winTexts(u.reader))
	}
}

func TestNavLoadErrorsReachErrorLog(t *testing.T) {
	for _, tc := range []struct{ method, what string }{
		{"GetAccounts", "accounts"},
		{"GetLabels", "labels"},
		{"GetLabelUnreadCounts", "unread counts"},
	} {
		t.Run(tc.method, func(t *testing.T) {
			fb := &fakeBackend{errs: map[string]error{tc.method: errors.New("down")}}
			u, _ := newTestUI(t, withBackend(fb))
			if u.errCount == 0 || !slices.ContainsFunc(u.loadErrs, func(s string) bool {
				return strings.Contains(s, "could not load "+tc.what+": down")
			}) {
				t.Fatalf("errCount=%d loadErrs=%v", u.errCount, u.loadErrs)
			}
		})
	}
}

func TestNavLoadMoreErrorReachesErrorLog(t *testing.T) {
	u, fb := navUI(t)
	u.allLoaded = false
	fb.errs = map[string]error{"GetInbox": errors.New("page down")}
	before := u.errCount
	u.loadMore()
	if u.errCount != before+1 || !u.olderFailed || !strings.Contains(u.loadErrs[len(u.loadErrs)-1], "page down") {
		t.Fatalf("errCount=%d olderFailed=%v loadErrs=%v", u.errCount, u.olderFailed, u.loadErrs)
	}
}

func TestNavSearch(t *testing.T) {
	u, fb := navUI(t)
	fb.searchThreads = []desktop.ThreadItem{{ID: "s1", AccountID: "a1", Subject: "Invoice", Participants: "Bob <b@x.com>", Date: msAt(2)}}
	fb.calls = nil
	u.win.Canvas().Focus(u.search)
	test.Type(u.search, "invoice ")
	u.search.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if u.query != "invoice" || navTitle(u) != "Search: invoice" {
		t.Fatalf("query=%q title=%q", u.query, navTitle(u))
	}
	if len(u.threads) != 1 || u.threads[0].ID != "s1" || !u.allLoaded {
		t.Fatalf("threads=%v allLoaded=%v", u.threads, u.allLoaded)
	}
	if !slices.Contains(fb.calls, "SearchInbox") {
		t.Fatalf("calls %v", fb.calls)
	}
	// Search pages are never extended.
	fb.calls = nil
	u.loadMore()
	if len(fb.calls) != 0 {
		t.Fatalf("loadMore during search: %v", fb.calls)
	}
	// Re-submitting the same query does not refetch.
	u.runSearch(" invoice ")
	if len(fb.calls) != 0 {
		t.Fatalf("same query refetched: %v", fb.calls)
	}
	// Escape gives up focus but keeps the results.
	u.search.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if u.win.Canvas().Focused() != nil || u.query != "invoice" {
		t.Fatalf("esc: focused=%v query=%q", u.win.Canvas().Focused(), u.query)
	}
	// Emptying the field restores the inbox.
	u.search.SetText("")
	if u.query != "" || len(u.threads) != 5 || navTitle(u) != "Inbox" {
		t.Fatalf("clear: query=%q n=%d title=%q", u.query, len(u.threads), navTitle(u))
	}
}

func TestNavSearchNoResultsAndError(t *testing.T) {
	u, fb := navUI(t)
	u.runSearch("nothing")
	if len(u.threads) != 0 || u.listErr != "" || u.list.Length() != 0 {
		t.Fatalf("no-result search: n=%d err=%q", len(u.threads), u.listErr)
	}
	fb.errs = map[string]error{"SearchInbox": errors.New("quota")}
	u.runSearch("again")
	if u.listErr != "quota" {
		t.Fatalf("err=%q", u.listErr)
	}
}

func TestNavSearchKeyFocusesField(t *testing.T) {
	u, _ := navUI(t)
	u.installShortcuts()
	u.win.Canvas().Focus(u.list)
	typeKeys(u, "/")
	if u.win.Canvas().Focused() != fyne.Focusable(u.search) {
		t.Fatal("/ should focus the search field")
	}
}

func navTickAll(t *testing.T, u *UI) {
	t.Helper()
	var check *widget.Check
	navWalk(u.listBar, func(c fyne.CanvasObject) {
		if ch, ok := c.(*widget.Check); ok && check == nil {
			check = ch
		}
	})
	test.TapAt(check, fyne.NewPos(2, check.Size().Height/2)) // the box is centred in a bar taller than itself
}

func TestNavBulkToolbarAppearsWithSelection(t *testing.T) {
	u, _ := navUI(t)
	for _, tip := range []string{"Archive", "Delete", "Mark as read", "Move to", "Labels"} {
		if navTip(u.listBar, tip) != nil {
			t.Errorf("%q shown with nothing ticked", tip)
		}
	}
	if navTip(u.listBar, "Refresh") == nil || navTip(u.listBar, "Select") == nil {
		t.Fatal("base toolbar missing")
	}
	navTickAll(t, u)
	if len(u.checkedThreads()) != 5 {
		t.Fatalf("select all ticked %d", len(u.checkedThreads()))
	}
	for _, tip := range []string{"Archive", "Report spam", "Delete", "Move to", "Labels"} {
		if navTip(u.listBar, tip) == nil {
			t.Errorf("%q missing after select all", tip)
		}
	}
	navTickAll(t, u) // the master check is now on; tapping clears
	if len(u.checkedThreads()) != 0 || navTip(u.listBar, "Archive") != nil {
		t.Fatal("select none did not clear toolbar")
	}
	// Spam button reads "Not spam" in the Spam folder.
	u.selectLabel("SPAM")
	u.threads = navThreads()
	u.selectWhere(func(desktop.ThreadItem) bool { return true })
	if navTip(u.listBar, "Not spam") == nil || navTip(u.listBar, "Report spam") != nil {
		t.Fatal("spam tooltip not swapped in Spam")
	}
}

func TestNavSelectWherePredicates(t *testing.T) {
	u, _ := navUI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.IsUnread })
	if got := u.checkedThreads(); len(got) != 1 || got[0].ID != "t1" {
		t.Fatalf("unread: %v", got)
	}
	u.selectWhere(func(t desktop.ThreadItem) bool { return !t.IsUnread })
	if len(u.checkedThreads()) != 4 {
		t.Fatalf("read: %d", len(u.checkedThreads()))
	}
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.IsStarred })
	if got := u.checkedThreads(); len(got) != 1 || got[0].ID != "t2" {
		t.Fatalf("starred: %v", got)
	}
	u.selectWhere(nil)
	if len(u.checkedThreads()) != 0 {
		t.Fatal("none")
	}
	// Keyboard x ticks the cursor row and toggles it.
	u.cursor, u.cursorNext = 2, false
	u.toggleCheckCursor()
	if got := u.checkedThreads(); len(got) != 1 || got[0].ID != "t3" {
		t.Fatalf("x: %v", got)
	}
	u.toggleCheckCursor()
	if len(u.checkedThreads()) != 0 {
		t.Fatal("x did not untick")
	}
}

func TestNavBulkArchive(t *testing.T) {
	u, fb := navUI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t2" || t.ID == "t4" })
	test.Tap(navTip(u.listBar, "Archive"))
	if !slices.Equal(fb.archived, []string{"t2", "t4"}) {
		t.Fatalf("archived %v", fb.archived)
	}
	if len(u.threads) != 3 || len(u.checked) != 0 || navTip(u.listBar, "Archive") != nil {
		t.Fatalf("threads=%d checked=%v", len(u.threads), u.checked)
	}
}

func TestNavBulkTrashShowsUndoForSingle(t *testing.T) {
	u, fb := navUI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t3" })
	test.Tap(navTip(u.listBar, "Delete"))
	if !slices.Equal(fb.trashed, []string{"t3"}) || len(u.threads) != 4 || u.undoPop == nil {
		t.Fatalf("trashed=%v n=%d undo=%v", fb.trashed, len(u.threads), u.undoPop)
	}
	u.hideUndo()
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" || t.ID == "t2" })
	u.bulkTrash()
	if len(fb.trashed) != 3 || u.undoPop == nil || !readerHas(u.undoPop.Content, "2 conversations moved to Trash") {
		t.Fatalf("multi-trash: %v undo=%v", fb.trashed, u.undoPop)
	}
	test.Tap(readerButton(u.undoPop.Content, "Undo"))
	if !slices.Equal(fb.untrashed, []string{"t1", "t2"}) || u.undoPop != nil {
		t.Fatalf("multi undo: %v", fb.untrashed)
	}
}

func TestNavBulkErrorKeepsThreadsAndShowsDialog(t *testing.T) {
	u, fb := navUI(t)
	fb.errs = map[string]error{"ArchiveThread": errors.New("denied")}
	u.selectWhere(func(desktop.ThreadItem) bool { return true })
	u.bulkArchive()
	if len(u.threads) != 5 || len(fb.archived) != 0 {
		t.Fatalf("failed archive removed threads: %d", len(u.threads))
	}
	ov := u.win.Canvas().Overlays().Top()
	if ov == nil || !navHas(ov, "Denied") { // dialog capitalises the message
		t.Fatalf("error dialog not shown: %v", ov)
	}
}

func TestNavBulkSpamInSpamFolderRestores(t *testing.T) {
	u, fb := navUI(t)
	fb.labelThreads = navThreads()[:2]
	u.selectLabel("SPAM")
	u.selectWhere(func(desktop.ThreadItem) bool { return true })
	fb.calls = nil
	test.Tap(navTip(u.listBar, "Not spam"))
	if !slices.Contains(fb.calls, "MarkThreadNotSpam") || slices.Contains(fb.calls, "MarkThreadSpam") || len(u.threads) != 0 {
		t.Fatalf("calls=%v n=%d", fb.calls, len(u.threads))
	}
	u.selectLabel("INBOX")
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t5" })
	u.bulkSpam()
	if !slices.Equal(fb.spammed, []string{"t5"}) {
		t.Fatalf("spammed %v", fb.spammed)
	}
}

func TestNavBulkMarkReadUnread(t *testing.T) {
	u, fb := navUI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" })
	u.bulkRead(true) // the bar offers it through the More menu
	if u.threads[0].IsUnread {
		t.Fatal("t1 still unread")
	}
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t3" })
	fb.calls = nil
	u.bulkRead(false)
	if !u.threads[2].IsUnread || !slices.Contains(fb.calls, "MarkMessageUnread") {
		t.Fatalf("unread=%v calls=%v", u.threads[2].IsUnread, fb.calls)
	}
	// A failing backend leaves the row alone and reports the error.
	fb.errs = map[string]error{"MarkMessageUnread": errors.New("nope")}
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t4" })
	u.bulkRead(false)
	if u.threads[3].IsUnread || u.win.Canvas().Overlays().Top() == nil {
		t.Fatal("failure should not flip state and should show a dialog")
	}
}

func TestNavBulkLabelMenuAndMoveTo(t *testing.T) {
	u, fb := navUI(t)
	// t1 has Inquiries, t2 has Travel: neither label is ticked for both.
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t1" || t.ID == "t2" })
	items := u.labelItems(u.checkedThreads())
	find := func(items []*fyne.MenuItem, name string) *fyne.MenuItem {
		for _, mi := range items {
			if mi.Label == name {
				return mi
			}
		}
		t.Fatalf("menu item %q missing", name)
		return nil
	}
	if find(items, "Inquiries").Checked {
		t.Fatal("Inquiries ticked though only t1 has it")
	}
	find(items, "Inquiries").Action() // add to both
	for _, id := range []string{"t1", "t2"} {
		if !slices.Contains(fb.labelled, id+":L1:true") {
			t.Fatalf("labelled %v", fb.labelled)
		}
	}
	if !find(u.labelItems(u.checkedThreads()), "Inquiries").Checked {
		t.Fatal("Inquiries should now be ticked for the whole selection")
	}
	find(u.labelItems(u.checkedThreads()), "Inquiries").Action() // all have it: removes
	if !slices.Contains(fb.labelled, "t2:L1:false") {
		t.Fatalf("remove not sent: %v", fb.labelled)
	}

	// Move to: label then archive, and drop from the inbox view.
	fb.labelled, fb.archived = nil, nil
	find(u.moveToItems(), "Travel").Action()
	if !slices.Equal(fb.archived, []string{"t1", "t2"}) || len(fb.labelled) != 2 || len(u.threads) != 3 {
		t.Fatalf("move: archived=%v labelled=%v n=%d", fb.archived, fb.labelled, len(u.threads))
	}
	// With no user labels the menu says so.
	fb.labelsOverride = []desktop.LabelItem{{ID: "INBOX", Name: "Inbox", Type: "system"}}
	u.reloadLabels()
	if mi := u.labelItems(nil); len(mi) != 1 || mi[0].Label != "No labels" || !mi[0].Disabled {
		t.Fatalf("empty label menu: %v", mi)
	}
}

func TestNavBulkStarAndUnstarInStarredView(t *testing.T) {
	u, fb := navUI(t)
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t3" })
	u.bulkLabel("STARRED", true)
	if !u.threads[2].IsStarred || !slices.Contains(fb.labelled, "t3:STARRED:true") || len(u.threads) != 5 {
		t.Fatalf("star: %v starred=%v", fb.labelled, u.threads[2].IsStarred)
	}
	fb.labelThreads = navThreads()[1:3]
	u.selectLabel("STARRED")
	u.selectWhere(func(t desktop.ThreadItem) bool { return t.ID == "t2" })
	u.bulkLabel("STARRED", false)
	if len(u.threads) != 1 || u.threads[0].ID != "t3" {
		t.Fatalf("unstar should remove from Starred view: %v", u.threads)
	}
}

func TestNavThemeToggle(t *testing.T) {
	u, _ := navUI(t)
	if isDark() {
		t.Fatal("expected light by default")
	}
	test.Tap(navTip(u.sidebar, "Toggle app theme"))
	if !isDark() || u.app.Preferences().String(prefDarkMode) != "dark" {
		t.Fatalf("dark=%v pref=%q", isDark(), u.app.Preferences().String(prefDarkMode))
	}
	if len(u.threads) != 5 || navTitle(u) != "Inbox" {
		t.Fatal("state lost across rebuild")
	}
	test.Tap(navTip(u.sidebar, "Toggle app theme"))
	if isDark() || u.app.Preferences().String(prefDarkMode) != "light" {
		t.Fatalf("dark=%v pref=%q", isDark(), u.app.Preferences().String(prefDarkMode))
	}
	if themeFor("") != (mimeTheme{}) || !themeFor("dark").dark || !themeFor("dark").forced {
		t.Fatal("themeFor")
	}
}

func TestNavSyncStatusAndEvents(t *testing.T) {
	u, fb := navUI(t)
	u.syncNow()
	if fb.syncs != 1 || u.syncStatus != "Syncing…" || !navHas(u.sidebar, "Syncing…") {
		t.Fatalf("syncs=%d status=%q", fb.syncs, u.syncStatus)
	}
	u.syncNow() // already syncing
	test.Tap(navTip(u.listBar, "Refresh"))
	if fb.syncs != 1 {
		t.Fatalf("sync ran while syncing: %d", fb.syncs)
	}
	fb.fire("sync:progress", map[string]string{"account": "Work"})
	if !navHas(u.sidebar, "Syncing Work…") {
		t.Fatalf("progress text: %v", navTexts(u.sidebar))
	}
	fb.calls = nil
	fb.fire("sync:complete", nil)
	if u.syncStatus != "" || navHas(u.sidebar, "Syncing Work…") || !slices.Contains(fb.calls, "GetLabels") {
		t.Fatalf("complete: status=%q calls=%v", u.syncStatus, fb.calls)
	}
	test.Tap(navTip(u.sidebar, "Check for new mail"))
	if fb.syncs != 2 {
		t.Fatalf("manual sync after complete: %d", fb.syncs)
	}
	fb.fire("sync:complete", nil) // stops the spinner animation
}

func TestNavInboxUpdatedReloadsList(t *testing.T) {
	u, fb := navUI(t)
	more := append(navThreads(), desktop.ThreadItem{ID: "t6", AccountID: "a1", Subject: "New", Participants: "N <n@x.com>", Date: msAt(0)})
	fb.threadsOverride = &more
	fb.fire("inbox:updated", nil)
	if len(u.threads) != 6 || u.list.Length() != 7 {
		t.Fatalf("n=%d", len(u.threads))
	}
}

func TestNavInboxUpdatedKeepsOpenThreadSelected(t *testing.T) {
	u, fb := navUI(t)
	u.list.Select(2)
	reordered := append([]desktop.ThreadItem{{ID: "n0", AccountID: "a1", Subject: "Fresh", Participants: "F <f@x.com>", Date: msAt(0)}}, navThreads()...)
	fb.threadsOverride = &reordered
	fb.fire("inbox:updated", nil)
	if u.selected == nil || u.selected.ID != "t3" || u.threads[3].ID != "t3" {
		t.Fatalf("selected=%v", u.selected)
	}
}

func TestNavErrorLogIndicator(t *testing.T) {
	u, fb := navUI(t)
	fb.apiErrs = []desktop.APIError{{Timestamp: goldenNow.UnixMilli(), Message: "rate limited"}}
	fb.fire("api:error", nil)
	if u.errCount != 1 {
		t.Fatalf("errCount %d", u.errCount)
	}
	var warn *widget.Button
	navWalk(u.sidebar, func(c fyne.CanvasObject) {
		if b, ok := c.(*widget.Button); ok && b.Text == "1" {
			warn = b
		}
	})
	if warn == nil {
		t.Fatal("error count button missing")
	}
	test.Tap(warn)
	ov := u.win.Canvas().Overlays().Top()
	if ov == nil || !navHas(ov, goldenNow.Local().Format("Jan 2 15:04:05")+"  rate limited") { // log uses local time
		t.Fatalf("error log: %v", ov)
	}
	var clear *widget.Button
	navWalk(ov, func(c fyne.CanvasObject) {
		if b, ok := c.(*widget.Button); ok && b.Text == "Clear" {
			clear = b
		}
	})
	test.Tap(clear)
	if u.errCount != 0 {
		t.Fatalf("errCount after clear: %d", u.errCount)
	}
	for _, s := range navTexts(u.sidebar) {
		if s == "1" {
			t.Fatal("indicator still shown")
		}
	}
}

func TestNavErrorLogEmpty(t *testing.T) {
	u, _ := navUI(t)
	u.showErrorLog()
	if ov := u.win.Canvas().Overlays().Top(); ov == nil || !navHas(ov, "No errors.") {
		t.Fatal("empty error log text missing")
	}
}

func TestNavAccountEventErrorShowsDialog(t *testing.T) {
	u, fb := navUI(t)
	fb.fire("account:add:error", "token expired")
	if ov := u.win.Canvas().Overlays().Top(); ov == nil || !strings.Contains(strings.ToLower(strings.Join(navTexts(ov), " ")), "token expired") {
		t.Fatal("sync/account error not surfaced")
	}
}

func TestNavGoldens(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []testOpt
		prep func(*UI)
	}{
		{"nav_inbox_light", nil, nil},
		{"nav_inbox_dark", []testOpt{withDark()}, nil},
		{"nav_inbox_bulk", nil, func(u *UI) {
			u.selectWhere(func(t desktop.ThreadItem) bool { return t.IsUnread || t.IsStarred })
		}},
		{"nav_inbox_mini", nil, func(u *UI) { u.toggleSidebar() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, _ := navUI(t, tc.opts...)
			if tc.prep != nil {
				tc.prep(u)
			}
			golden(t, tc.name, u.win.Canvas().Capture())
		})
	}
}

func TestLaunchSyncAfterAccountsLoad(t *testing.T) {
	u, fb := navUI(t)
	before := fb.syncs
	u.launchSync = true
	u.reloadAccounts()
	if fb.syncs != before+1 || u.launchSync {
		t.Fatalf("syncs=%d launchSync=%v", fb.syncs, u.launchSync)
	}
	u.reloadAccounts() // only once per launch
	if fb.syncs != before+1 {
		t.Fatalf("second load synced: %d", fb.syncs)
	}
}

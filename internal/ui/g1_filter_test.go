package ui

import (
	"fmt"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

// g1Call is one FilterThreads call as the UI made it.
type g1Call struct {
	filter, account, label, query string
	limit, offset                 int
}

var g1Calls = map[*fakeBackend]*[]g1Call{}

// FilterThreads records its arguments and applies the filter in memory to the
// list the unfiltered call would have returned.
func (f *fakeBackend) FilterThreads(filter, account, label, query string, limit, offset int) ([]desktop.ThreadItem, error) {
	if err := f.fail("FilterThreads"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	if g1Calls[f] == nil {
		g1Calls[f] = new([]g1Call)
	}
	*g1Calls[f] = append(*g1Calls[f], g1Call{filter, account, label, query, limit, offset})
	f.mu.Unlock()
	var base []desktop.ThreadItem
	switch {
	case query != "":
		base = f.searchThreads
	case label == "INBOX" && f.threadsOverride != nil:
		base = *f.threadsOverride
	default:
		base = f.labelThreads
	}
	var out []desktop.ThreadItem
	for _, t := range base {
		if filter == "unread" && t.IsUnread || filter == "starred" && t.IsStarred || filter == "attachments" && t.HasAttachments {
			out = append(out, t)
		}
	}
	if offset >= len(out) {
		return nil, nil
	}
	return out[offset:min(len(out), offset+limit)], nil
}

func g1Last(fb *fakeBackend) g1Call {
	c := *g1Calls[fb]
	return c[len(c)-1]
}

func g1IDs(u *UI) (out []string) {
	for _, t := range u.threads {
		out = append(out, t.ID)
	}
	return
}

func g1Menu(u *UI) []string {
	var out []string
	for _, it := range u.filterItems() {
		out = append(out, it.Label)
	}
	return out
}

func g1Checked(u *UI) string {
	for _, it := range u.filterItems() {
		if it.Checked {
			return it.Label
		}
	}
	return ""
}

func TestG1FilterMenuAndBackendCall(t *testing.T) {
	u, fb := navUI(t)
	if want := []string{"All", "Unread", "Starred", "With attachments"}; !slices.Equal(g1Menu(u), want) {
		t.Fatalf("menu %v", g1Menu(u))
	}
	if g1Checked(u) != "All" || u.filterBtn.tip != "Filter conversations" {
		t.Fatalf("default %q tip %q", g1Checked(u), u.filterBtn.tip)
	}
	// All keeps using the plain list call.
	if g1Calls[fb] != nil {
		t.Fatalf("filter call without filter: %v", *g1Calls[fb])
	}
	for _, c := range []struct{ f, label string }{{"unread", "Unread"}, {"starred", "Starred"}, {"attachments", "With attachments"}} {
		u.setFilter(c.f)
		got := g1Last(fb)
		if got.filter != c.f || got.account != unified || got.label != "INBOX" || got.offset != 0 || got.limit != pageSize {
			t.Fatalf("%s: call %+v", c.f, got)
		}
		if g1Checked(u) != c.label {
			t.Fatalf("%s: checked %q", c.f, g1Checked(u))
		}
	}
	u.setFilter("unread")
	if want := []string{"t1"}; !slices.Equal(g1IDs(u), want) {
		t.Fatalf("unread rows %v", g1IDs(u))
	}
	fb.calls = nil
	u.setFilter("")
	if len(u.threads) != len(navThreads()) || !slices.Contains(fb.calls, "GetInbox") {
		t.Fatalf("All: rows=%d calls=%v", len(u.threads), fb.calls)
	}
}

func TestG1FilterTapOpensMenuItem(t *testing.T) {
	u, _ := navUI(t)
	test.Tap(u.filterBtn)
	if n := len(u.win.Canvas().Overlays().List()); n == 0 {
		t.Fatal("tapping the glyph opened no menu")
	}
}

func TestG1FilterActiveIcon(t *testing.T) {
	u, _ := navUI(t)
	off := u.filterBtn.Icon.Name()
	u.setFilter("starred")
	on := u.filterBtn.Icon.Name()
	if on == off {
		t.Fatalf("icon unchanged when filter is on: %s", on)
	}
	u.setFilter("")
	if u.filterBtn.Icon.Name() != off {
		t.Fatal("icon not restored for All")
	}
}

func TestG1FilterEmptyState(t *testing.T) {
	u, fb := navUI(t)
	none := []desktop.ThreadItem{}
	fb.threadsOverride = &none
	for f, want := range map[string]string{
		"unread": "No unread conversations", "starred": "No starred conversations",
		"attachments": "No attachment conversations",
	} {
		u.setFilter(f)
		if u.emptyLbl.Text != want {
			t.Errorf("%s: %q want %q", f, u.emptyLbl.Text, want)
		}
	}
	u.setFilter("")
	if u.emptyLbl.Text != "No conversations" {
		t.Errorf("All: %q", u.emptyLbl.Text)
	}
}

func TestG1FilterWithSearchAndLabel(t *testing.T) {
	u, fb := navUI(t)
	fb.searchThreads = []desktop.ThreadItem{
		{ID: "s1", AccountID: "a1", Subject: "Invoice", IsUnread: true, Date: msAt(2)},
		{ID: "s2", AccountID: "a1", Subject: "Invoice 2", Date: msAt(3)},
	}
	u.setFilter("unread")
	u.runSearch("invoice")
	if c := g1Last(fb); c.query != "invoice" || c.filter != "unread" {
		t.Fatalf("search call %+v", c)
	}
	if !slices.Equal(g1IDs(u), []string{"s1"}) {
		t.Fatalf("rows %v", g1IDs(u))
	}
	// No match: empty text mentions both.
	fb.searchThreads = fb.searchThreads[1:]
	u.query = ""
	u.runSearch("invoice")
	if u.emptyLbl.Text != "No unread results for “invoice”" {
		t.Fatalf("empty %q", u.emptyLbl.Text)
	}
	u.runSearch("")

	// A label view has its own filter and the call carries that label.
	fb.labelThreads = []desktop.ThreadItem{{ID: "l1", AccountID: "a1", Subject: "Sent", IsStarred: true, Date: msAt(1)}}
	u.selectLabel("SENT")
	if u.filter() != "" || g1Checked(u) != "All" {
		t.Fatalf("filter leaked into SENT: %q", u.filter())
	}
	u.setFilter("starred")
	if c := g1Last(fb); c.label != "SENT" || c.filter != "starred" || c.query != "" {
		t.Fatalf("label call %+v", c)
	}
	if !slices.Equal(g1IDs(u), []string{"l1"}) {
		t.Fatalf("rows %v", g1IDs(u))
	}
	// Going back to the inbox restores that view's own filter (unread), kept for the session.
	u.selectLabel("INBOX")
	if u.filter() != "unread" {
		t.Fatalf("inbox filter %q", u.filter())
	}
}

func TestG1FilterResetsPagination(t *testing.T) {
	var many []desktop.ThreadItem
	for i := 0; i < 130; i++ {
		many = append(many, desktop.ThreadItem{ID: fmt.Sprintf("t%03d", i), AccountID: "a1", Subject: "x", IsUnread: i%2 == 0, Date: msAt(1)})
	}
	freezeNow(t, goldenNow)
	fb := &fakeBackend{threadsOverride: &many}
	u, fb := newTestUI(t, withBackend(fb))
	fb.SetEventHandler(u.handleEvent)
	u.allLoaded = false
	u.loadMore()
	if len(u.threads) <= pageSize {
		t.Fatalf("setup: only %d rows after loadMore", len(u.threads))
	}
	u.setFilter("unread")
	if got := g1Last(fb); got.offset != 0 || got.limit != pageSize {
		t.Fatalf("filter call %+v", got)
	}
	if len(u.threads) != pageSize || u.allLoaded {
		t.Fatalf("rows %d allLoaded %v: want first page of 65 matches", len(u.threads), u.allLoaded)
	}
	u.loadMore()
	if c := g1Last(fb); c.offset != pageSize || c.filter != "unread" {
		t.Fatalf("next page call %+v", c)
	}
	if len(u.threads) != 65 {
		t.Fatalf("rows after second page: %d", len(u.threads))
	}
}

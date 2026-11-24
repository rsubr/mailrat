package ui

import (
	"testing"

	"github.com/mailrat/mailrat/internal/desktop"
)

// slowBodies is a backend whose lazy body fetch (a Gmail round trip) runs the
// atFetch hook first, so a test can inspect the UI at the moment the fetch starts.
type slowBodies struct {
	*fakeBackend
	atFetch func()
}

func (s slowBodies) GetThread(_, id string) ([]desktop.MessageItem, error) {
	return []desktop.MessageItem{{ID: "m1", ThreadID: id, AccountID: "a1", Subject: "S", FromAddr: "A <a@x.com>", Snippet: "snippet text", Date: ms(2)}}, nil
}
func (s slowBodies) GetMessage(_, id string) (desktop.MessageItem, error) {
	s.atFetch()
	return desktop.MessageItem{ID: id, ThreadID: "t", AccountID: "a1", FromAddr: "A <a@x.com>", Body: "full body", Date: ms(2)}, nil
}

// Clicking mail whose body is not stored must show it (snippet) without waiting
// for the body round trip, then swap the body in. Runs inline (single goroutine,
// like the fyne test driver) so the mid-fetch state is read without racing.
func TestOpenThreadShowsStoredMailBeforeBodyFetch(t *testing.T) {
	u, fb := newTestUI(t)
	var atFetch []desktop.MessageItem
	u.be = slowBodies{fb, func() { atFetch = append([]desktop.MessageItem(nil), u.messages...) }}
	u.openThread(desktop.ThreadItem{ID: "t1", AccountID: "a1", Subject: "S"})
	if len(atFetch) != 1 || atFetch[0].Body != "" {
		t.Fatalf("reader not shown with stored mail when the body fetch started: %+v", atFetch)
	}
	if len(u.messages) != 1 || u.messages[0].Body != "full body" || u.threadLoading {
		t.Fatalf("body never swapped in: %+v loading=%v", u.messages, u.threadLoading)
	}
}

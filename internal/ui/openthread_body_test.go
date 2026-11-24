package ui

import (
	"testing"
	"time"

	"github.com/mailrat/mailrat/internal/desktop"
)

// slowBodies is a backend whose lazy body fetch (a Gmail round trip) never returns until released.
type slowBodies struct {
	*fakeBackend
	release chan struct{}
}

func (s slowBodies) GetThread(_, id string) ([]desktop.MessageItem, error) {
	return []desktop.MessageItem{{ID: "m1", ThreadID: id, AccountID: "a1", Subject: "S", FromAddr: "A <a@x.com>", Snippet: "snippet text", Date: ms(2)}}, nil
}
func (s slowBodies) GetMessage(_, id string) (desktop.MessageItem, error) {
	<-s.release
	return desktop.MessageItem{ID: id, ThreadID: "t", AccountID: "a1", FromAddr: "A <a@x.com>", Body: "full body", Date: ms(2)}, nil
}

// Clicking mail whose body is not stored must show it (snippet) without waiting
// for the body round trip, then swap the body in.
func TestOpenThreadShowsStoredMailBeforeBodyFetch(t *testing.T) {
	u, fb := newTestUI(t)
	u.inline = false
	be := slowBodies{fb, make(chan struct{})}
	u.be = be
	released := false
	defer func() {
		if !released {
			close(be.release)
		}
	}()
	u.openThread(desktop.ThreadItem{ID: "t1", AccountID: "a1", Subject: "S"})
	waitFor := func(cond func() bool) bool {
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(time.Millisecond) {
			if cond() {
				return true
			}
		}
		return false
	}
	if !waitFor(func() bool { return len(u.messages) == 1 }) {
		t.Fatal("reader not shown while the body fetch is pending")
	}
	close(be.release)
	released = true
	if !waitFor(func() bool { return len(u.messages) == 1 && u.messages[0].Body == "full body" && !u.threadLoading }) {
		t.Fatal("body never swapped in")
	}
}

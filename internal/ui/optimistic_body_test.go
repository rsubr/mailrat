package ui

import (
	"errors"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/mailrat/mailrat/internal/desktop"
)

// Delete is optimistic: the row is gone before the (possibly sync-blocked)
// backend call returns, and a failed call brings the list back.
func TestTrashIsOptimistic(t *testing.T) {
	u, b := f1UI(t)
	f1Open(u, "t1")
	var during []string
	b.hook["TrashThread"] = func() { during = f1IDs(u) }
	u.trash()
	if slices.Contains(during, "t1") {
		t.Fatalf("row still listed while TrashThread ran: %v", during)
	}
	b.errs = map[string]error{"TrashThread": errors.New("db busy")}
	f1Open(u, "t3")
	u.trash()
	if !slices.Contains(f1IDs(u), "t3") {
		t.Fatalf("failed trash must restore the row: %v", f1IDs(u))
	}
}

// "#" is typed with Shift held; the auto-opened next row must not be ticked.
func TestTrashAdvanceWithShiftHeldDoesNotTick(t *testing.T) {
	u, _ := f1UI(t)
	f1Open(u, "t1")
	old := currentModifiers
	currentModifiers = func() fyne.KeyModifier { return fyne.KeyModifierShift }
	t.Cleanup(func() { currentModifiers = old })
	typeKeys(u, "#")
	if len(u.checked) != 0 || u.selected == nil {
		t.Fatalf("checked=%v selected=%v", u.checked, u.selected)
	}
}

// A body that fails to load shows the error with Retry, never a blank body.
func TestBodyFetchFailureShowsRetry(t *testing.T) {
	u, b := f1UI(t)
	b.thread = []desktop.MessageItem{{ID: "m1", ThreadID: "t1", AccountID: "a1", FromAddr: "x@y.z", Subject: "s"}}
	b.errs = map[string]error{"GetMessage": errors.New("offline")}
	f1Open(u, "t1")
	if !readerHas(u.reader, "Could not load this message") {
		t.Fatal("no error state for a failed body fetch")
	}
	btn := readerButton(u.reader, "Retry")
	if btn == nil {
		t.Fatal("no Retry button")
	}
	b.errs = nil
	btn.OnTapped()
	if readerHas(u.reader, "Could not load this message") || readerHas(u.reader, "Loading message") {
		t.Fatal("error state should clear after a successful retry")
	}
	if !readerHas(u.reader, "no content") {
		t.Fatal("an empty body must say so")
	}
}

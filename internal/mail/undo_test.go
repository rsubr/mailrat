package mail

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDelayedCommitsAfterDelay(t *testing.T) {
	d := NewDelayed(20 * time.Millisecond)
	done := make(chan struct{})
	d.Hold("k", func() { close(done) }, func() { t.Error("reverted") })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("not committed")
	}
	if d.Undo("k") {
		t.Fatal("undo after commit should fail")
	}
}

func TestDelayedUndoCancelsCommit(t *testing.T) {
	d := NewDelayed(30 * time.Millisecond)
	var committed, reverted atomic.Int32
	d.Hold("k", func() { committed.Add(1) }, func() { reverted.Add(1) })
	if !d.Undo("k") {
		t.Fatal("undo failed")
	}
	time.Sleep(80 * time.Millisecond)
	if committed.Load() != 0 || reverted.Load() != 1 {
		t.Fatalf("committed=%d reverted=%d", committed.Load(), reverted.Load())
	}
}

func TestDelayedFlushCommitsNow(t *testing.T) {
	d := NewDelayed(time.Hour)
	var n atomic.Int32
	d.Hold("a", func() { n.Add(1) }, func() {})
	d.Hold("b", func() { n.Add(1) }, func() {})
	d.Flush()
	if n.Load() != 2 || d.Undo("a") {
		t.Fatalf("n=%d", n.Load())
	}
}

func TestDelayedRepeatKeyCommitsEarlier(t *testing.T) {
	d := NewDelayed(time.Hour)
	var n atomic.Int32
	d.Hold("a", func() { n.Add(1) }, func() {})
	d.Hold("a", func() { n.Add(10) }, func() {})
	if n.Load() != 1 {
		t.Fatalf("n=%d", n.Load())
	}
}

func TestUndoWindowIsTenSeconds(t *testing.T) {
	if UndoWindow != 10*time.Second {
		t.Fatalf("UndoWindow=%v", UndoWindow)
	}
}

func TestDelayedIsHeld(t *testing.T) {
	d := NewDelayed(time.Hour)
	d.Hold("a/t1", func() {}, func() {})
	if !d.IsHeld("a/t1") || d.IsHeld("a/t2") {
		t.Fatal("IsHeld wrong while held")
	}
	d.Undo("a/t1")
	if d.IsHeld("a/t1") {
		t.Fatal("still held after undo")
	}
}

func TestDelayedDropPrefixSkipsCommitAndRevert(t *testing.T) {
	d := NewDelayed(20 * time.Millisecond)
	var n atomic.Int32
	cb := func() { n.Add(1) }
	d.Hold("a1/t1", cb, cb)
	d.Hold("a1/t2", cb, cb)
	d.Hold("a2/t1", func() { n.Add(100) }, cb)
	if got := d.DropPrefix("a1/"); got != 2 {
		t.Fatalf("dropped %d", got)
	}
	time.Sleep(80 * time.Millisecond)
	if n.Load() != 100 { // only a2 commits
		t.Fatalf("n=%d", n.Load())
	}
}

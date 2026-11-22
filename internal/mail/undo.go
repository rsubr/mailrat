package mail

import (
	"strings"
	"sync"
	"time"
)

// UndoWindow is how long a delete can be undone before it is committed.
const UndoWindow = 10 * time.Second

// Delayed holds actions for a grace period. Each is committed when its timer
// fires, unless Undo cancels it first.
type Delayed struct {
	delay time.Duration
	mu    sync.Mutex
	held  map[string]held
}

type held struct {
	timer  *time.Timer
	commit func()
	revert func()
}

// NewDelayed builds a Delayed with the given grace period.
func NewDelayed(delay time.Duration) *Delayed {
	return &Delayed{delay: delay, held: map[string]held{}}
}

// Hold schedules commit after the grace period. revert runs if Undo is called
// first. Holding a key that is already held commits the earlier one now.
func (d *Delayed) Hold(key string, commit, revert func()) {
	d.mu.Lock()
	prev, had := d.held[key]
	if had {
		prev.timer.Stop()
	}
	h := held{commit: commit, revert: revert}
	h.timer = time.AfterFunc(d.delay, func() { d.fire(key) })
	d.held[key] = h
	d.mu.Unlock()
	if had {
		prev.commit()
	}
}

func (d *Delayed) take(key string) (held, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.held[key]
	if ok {
		h.timer.Stop()
		delete(d.held, key)
	}
	return h, ok
}

func (d *Delayed) fire(key string) {
	if h, ok := d.take(key); ok {
		h.commit()
	}
}

// Undo cancels a held action and reverts it. It reports false when the action
// was already committed (or never held).
func (d *Delayed) Undo(key string) bool {
	h, ok := d.take(key)
	if ok {
		h.revert()
	}
	return ok
}

// IsHeld reports whether key still has an uncommitted, undoable action.
func (d *Delayed) IsHeld(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.held[key]
	return ok
}

// DropPrefix cancels every held action whose key starts with prefix, running
// neither commit nor revert. It returns how many were dropped.
func (d *Delayed) DropPrefix(prefix string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for k, h := range d.held {
		if strings.HasPrefix(k, prefix) {
			h.timer.Stop()
			delete(d.held, k)
			n++
		}
	}
	return n
}

// Flush commits every held action now (used on shutdown).
func (d *Delayed) Flush() {
	d.mu.Lock()
	keys := make([]string, 0, len(d.held))
	for k := range d.held {
		keys = append(keys, k)
	}
	d.mu.Unlock()
	for _, k := range keys {
		d.fire(k)
	}
}

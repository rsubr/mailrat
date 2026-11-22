package mail

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/storage"
)

// MaxOpAttempts is how many transient failures an operation survives before it
// is dropped and the local state reconciled with Gmail.
const MaxOpAttempts = 10

// retryEvery is how often the worker retries a queue that has pending work.
const retryEvery = 30 * time.Second

// opEffect describes what an operation does to labels. thread is true when the
// target is a thread ID rather than a message ID.
type opEffect struct {
	thread      bool
	add, remove []string
	trash       bool
}

func effectOf(opType string) (opEffect, bool) {
	switch {
	case opType == storage.OpMarkRead:
		return opEffect{remove: []string{LabelUnread}}, true
	case opType == storage.OpMarkUnread:
		return opEffect{add: []string{LabelUnread}}, true
	case opType == storage.OpTrashThread:
		return opEffect{thread: true, add: []string{LabelTrash}, remove: []string{LabelInbox}, trash: true}, true
	case opType == storage.OpArchive:
		return opEffect{thread: true, remove: []string{LabelInbox}}, true
	case opType == storage.OpMarkSpam:
		return opEffect{thread: true, add: []string{LabelSpam}, remove: []string{LabelInbox}}, true
	case opType == storage.OpMarkNotSpam:
		return opEffect{thread: true, add: []string{LabelInbox}, remove: []string{LabelSpam}}, true
	case strings.HasPrefix(opType, storage.OpAddLabel):
		return opEffect{thread: true, add: []string{strings.TrimPrefix(opType, storage.OpAddLabel)}}, true
	case strings.HasPrefix(opType, storage.OpRemoveLabel):
		return opEffect{thread: true, remove: []string{strings.TrimPrefix(opType, storage.OpRemoveLabel)}}, true
	}
	return opEffect{}, false
}

// ApplyOpRemote performs a queued operation against Gmail.
func ApplyOpRemote(ctx context.Context, client *gmail.Client, op storage.PendingOp) error {
	e, ok := effectOf(op.OpType)
	if !ok {
		return nil // unknown op from a newer/older version: drop it
	}
	switch {
	case e.trash:
		return client.TrashThread(ctx, op.MessageID)
	case e.thread:
		return client.ModifyThreadLabels(ctx, op.MessageID, e.add, e.remove)
	default:
		return client.ModifyLabels(ctx, op.MessageID, e.add, e.remove)
	}
}

// OpIsThread reports whether the op targets a thread (versus a single message).
func OpIsThread(opType string) bool {
	e, _ := effectOf(opType)
	return e.thread
}

// Queue applies user mutations to the local store immediately and replays them
// against Gmail in the background, in order, with retry. The UI never waits on
// the network.
type Queue struct {
	store *storage.Store
	// apply performs one op remotely. Returning an error for which permanent
	// reports true drops the op; any other error retries it later.
	apply     func(ctx context.Context, op storage.PendingOp) error
	permanent func(error) bool
	// onDrop is called after an op is abandoned so the caller can roll the
	// local state back to what Gmail says. May be nil.
	onDrop func(op storage.PendingOp, err error)
	wake   chan struct{}
	drain  sync.Mutex // one drain at a time keeps replay in order
}

// NewQueue builds a Queue. permanent may be nil (nothing is permanent; ops
// are dropped only after MaxOpAttempts).
func NewQueue(store *storage.Store, apply func(context.Context, storage.PendingOp) error, permanent func(error) bool, onDrop func(storage.PendingOp, error)) *Queue {
	return &Queue{store: store, apply: apply, permanent: permanent, onDrop: onDrop, wake: make(chan struct{}, 1)}
}

// Enqueue applies the op's effect locally, persists it, and wakes the worker.
func (q *Queue) Enqueue(opType, accountID, targetID string) error {
	if e, ok := effectOf(opType); ok {
		if err := q.store.ModifyLabels(accountID, targetID, e.thread, e.add, e.remove); err != nil {
			return err
		}
	}
	if err := q.store.AddPendingOp(opType, accountID, targetID); err != nil {
		return err
	}
	q.Wake()
	return nil
}

// Wake asks the worker to drain now.
func (q *Queue) Wake() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Run drains the queue on every wake-up and periodically, until ctx ends.
func (q *Queue) Run(ctx context.Context) {
	t := time.NewTicker(retryEvery)
	defer t.Stop()
	q.Wake() // replay anything left over from the last session
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-t.C:
		}
		q.Drain(ctx)
	}
}

// Drain replays queued ops in order. A transient failure stops further ops for
// the same account (to keep their order) but not other accounts.
func (q *Queue) Drain(ctx context.Context) {
	q.drain.Lock()
	defer q.drain.Unlock()
	ops, err := q.store.ListPendingOps()
	if err != nil {
		return
	}
	blocked := map[string]bool{}
	for _, op := range ops {
		if ctx.Err() != nil {
			return
		}
		if blocked[op.AccountID] {
			continue
		}
		err := q.apply(ctx, op)
		if err == nil {
			_ = q.store.DeletePendingOp(op.ID)
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if q.permanent != nil && q.permanent(err) {
			q.drop(op, err)
			continue
		}
		n, _ := q.store.BumpPendingOpAttempts(op.ID)
		if n >= MaxOpAttempts {
			q.drop(op, err)
			continue
		}
		blocked[op.AccountID] = true
	}
}

func (q *Queue) drop(op storage.PendingOp, err error) {
	_ = q.store.DeletePendingOp(op.ID)
	if q.onDrop != nil {
		q.onDrop(op, err)
	}
}

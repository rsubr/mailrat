package ui

import (
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// keysWalk visits o and every descendant, descending into widget renderers.
func keysWalk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			keysWalk(c, fn)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			keysWalk(c, fn)
		}
	}
}

func keysOverlay(u *UI) fyne.CanvasObject { return u.win.Canvas().Overlays().Top() }

// keysChecks lists the label checkboxes in the top overlay by text.
func keysChecks(u *UI) (out map[string]*widget.Check) {
	out = map[string]*widget.Check{}
	keysWalk(keysOverlay(u), func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok {
			out[c.Text] = c
		}
	})
	return out
}

// keysTap finds the button with this text in the top overlay and taps it.
func keysTap(t *testing.T, u *UI, text string) {
	t.Helper()
	var b *widget.Button
	keysWalk(keysOverlay(u), func(o fyne.CanvasObject) {
		if x, ok := o.(*widget.Button); ok && x.Text == text {
			b = x
		}
	})
	if b == nil {
		t.Fatalf("no %q button in overlay", text)
	}
	test.Tap(b)
}

// keysEntryTexts collects the text of every entry under the newest window.
func keysEntryTexts(u *UI) (out []string) {
	ws := u.app.Driver().AllWindows()
	keysWalk(ws[len(ws)-1].Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok {
			out = append(out, e.Text)
		}
	})
	return out
}

// keysAdvance moves the frozen UI clock to at.
func keysAdvance(t *testing.T, at time.Time) { freezeNow(t, at) }

func TestKeysChordTimeout(t *testing.T) {
	t0 := goldenNow
	u, _ := newTestUI(t)
	u.installShortcuts()

	// Second key just inside the window completes the chord.
	freezeNow(t, t0)
	typeKeys(u, "g")
	if u.pendingKey != 'g' {
		t.Fatal("g should arm the prefix")
	}
	keysAdvance(t, t0.Add(sequenceWait-time.Millisecond))
	typeKeys(u, "s")
	if u.activeLabel != "STARRED" || u.pendingKey != 0 {
		t.Fatalf("g s inside window: label=%s pending=%d", u.activeLabel, u.pendingKey)
	}

	// At exactly the timeout the prefix is stale: the key acts alone.
	freezeNow(t, t0)
	typeKeys(u, "g")
	keysAdvance(t, t0.Add(sequenceWait))
	typeKeys(u, "i") // 'i' alone is not a shortcut, so nothing may navigate
	if u.activeLabel != "STARRED" || u.pendingKey != 0 {
		t.Fatalf("stale g i must not go to Inbox: label=%s pending=%d", u.activeLabel, u.pendingKey)
	}
	freezeNow(t, t0)
	typeKeys(u, "g")
	keysAdvance(t, t0.Add(sequenceWait+time.Second))
	u.selected = nil
	typeKeys(u, "?") // stale prefix must not swallow a real single key
	if keysOverlay(u) == nil {
		t.Fatal("stale g swallowed ?")
	}
}

func TestKeysGoToTargets(t *testing.T) {
	u, _ := newTestUI(t)
	u.installShortcuts()
	for keys, want := range map[string]string{"gt": "SENT", "gd": "DRAFT", "ga": "ALL_MAIL", "gs": "STARRED", "gi": "INBOX"} {
		typeKeys(u, keys)
		if u.activeLabel != want {
			t.Fatalf("%s: got %s want %s", keys, u.activeLabel, want)
		}
	}
	typeKeys(u, "gg") // second g is not a target; prefix cleared, not re-armed
	if u.pendingKey != 0 {
		t.Fatal("gg should leave no pending prefix")
	}
}

func TestKeysGoToLabelDialog(t *testing.T) {
	u, _ := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "gl")
	esc, ok := u.win.Canvas().Focused().(*escEntry)
	if !ok || esc.PlaceHolder != "Go to label…" {
		t.Fatalf("g l should focus the label box, got %T", u.win.Canvas().Focused())
	}
	test.Type(esc, "trav")
	esc.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) // Enter picks first match
	if u.activeLabel != "L2" {
		t.Fatalf("want Travel (L2), got %s", u.activeLabel)
	}
	if keysOverlay(u) != nil {
		t.Fatal("dialog should close after picking")
	}

	// A filter with no match: Enter does nothing, dialog stays.
	typeKeys(u, "gl")
	esc = u.win.Canvas().Focused().(*escEntry)
	test.Type(esc, "zzzz")
	esc.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if u.activeLabel != "L2" || keysOverlay(u) == nil {
		t.Fatal("empty match list must not navigate or close")
	}
}

func TestKeysModalSuppressesKeys(t *testing.T) {
	u, _ := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "?")
	if keysOverlay(u) == nil {
		t.Fatal("? should open help")
	}
	n := len(u.app.Driver().AllWindows())
	typeKeys(u, "jcg")
	if u.selected != nil || u.pendingKey != 0 || len(u.app.Driver().AllWindows()) != n {
		t.Fatal("keys leaked through the help dialog")
	}
	if u.typedKey(fyne.KeyReturn) {
		t.Fatal("typedKey must not consume keys under a modal")
	}
	if !u.typedKey(fyne.KeyEscape) || keysOverlay(u) != nil {
		t.Fatal("Esc should close the dialog")
	}
	typeKeys(u, "?")

	// After the dialog goes away the keys work again.
	u.win.Canvas().Overlays().Remove(keysOverlay(u))
	typeKeys(u, "j")
	if u.selected == nil {
		t.Fatal("j should work once the dialog is closed")
	}
}

func TestKeysUndoBarIsNotModal(t *testing.T) {
	u, fb := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "j#")
	if len(fb.trashed) != 1 || u.undoPop == nil {
		t.Fatalf("# should trash and show the undo bar: %v", fb.trashed)
	}
	if u.modalOpen() {
		t.Fatal("undo bar must not count as a modal")
	}
	typeKeys(u, "j")
	if u.selected == nil {
		t.Fatal("keys must keep working while the undo bar shows")
	}
}

func TestKeysEscape(t *testing.T) {
	u, _ := newTestUI(t)
	u.installShortcuts()

	// Esc cancels a pending "g" and is consumed.
	typeKeys(u, "g")
	if !u.typedKey(fyne.KeyEscape) || u.pendingKey != 0 {
		t.Fatal("Esc should consume and clear the prefix")
	}
	typeKeys(u, "s")
	if u.activeLabel != "INBOX" {
		t.Fatalf("g Esc s must not jump: %s", u.activeLabel)
	}

	// Wired through the canvas handler.
	typeKeys(u, "j")
	if u.selected == nil {
		t.Fatal("setup: j opens a thread")
	}
	u.win.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if u.selected != nil {
		t.Fatal("Esc via canvas should return to the list")
	}

	// Esc in the search box drops focus without leaving other state alone.
	typeKeys(u, "/")
	if u.win.Canvas().Focused() != fyne.Focusable(u.search) {
		t.Fatal("/ should focus search")
	}
	u.search.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if u.win.Canvas().Focused() != nil {
		t.Fatal("Esc should unfocus the search box")
	}
}

func TestKeysEnterOpensAndToggles(t *testing.T) {
	for _, name := range []fyne.KeyName{fyne.KeyReturn, fyne.KeyEnter} {
		u, _ := newTestUI(t)
		u.installShortcuts()
		h := u.win.Canvas().OnTypedKey()
		h(&fyne.KeyEvent{Name: name})
		if u.selected == nil || u.selected.ID != "t1" {
			t.Fatalf("%s should open the first row: %+v", name, u.selected)
		}
		// Open thread: Enter toggles the cursor message like "o".
		cur := u.messages[u.msgCursor]
		before := u.collapsed[cur.ID]
		h(&fyne.KeyEvent{Name: name})
		if u.collapsed[cur.ID] == before {
			t.Fatalf("%s should toggle the cursor message", name)
		}
	}
}

func TestKeysCursorClampsAndEmptyList(t *testing.T) {
	u, _ := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "jjjjjjjj")
	if u.selected == nil || u.selected.ID != "t5" {
		t.Fatalf("j should stop at the last row: %+v", u.selected)
	}
	typeKeys(u, "kkkkkkkk")
	if u.selected.ID != "t1" {
		t.Fatalf("k should stop at the first row: %s", u.selected.ID)
	}

	e, _ := newTestUI(t, withEmpty())
	e.installShortcuts()
	typeKeys(e, "jkoxe#!sly")
	e.typedKey(fyne.KeyReturn)
	if e.selected != nil || len(e.checkedThreads()) != 0 {
		t.Fatal("empty list: nothing should be selected or ticked")
	}
}

func TestKeysTickAndBulkActions(t *testing.T) {
	cases := []struct {
		keys string
		got  func(*fakeBackend) []string
	}{
		{"e", func(f *fakeBackend) []string { return f.archived }},
		{"#", func(f *fakeBackend) []string { return f.trashed }},
		{"!", func(f *fakeBackend) []string { return f.spammed }},
	}
	for _, c := range cases {
		t.Run(c.keys, func(t *testing.T) {
			u, fb := newTestUI(t)
			u.installShortcuts()
			typeKeys(u, "x")
			if ck := u.checkedThreads(); len(ck) != 1 || ck[0].ID != "t1" {
				t.Fatalf("x should tick the first row: %+v", ck)
			}
			typeKeys(u, c.keys)
			if got := c.got(fb); len(got) != 1 || got[0] != "t1" {
				t.Fatalf("%s with a ticked row should act on t1 only: %v", c.keys, got)
			}
			if len(u.threads) != 4 || len(u.checkedThreads()) != 0 {
				t.Fatalf("row should leave the list and ticks clear: %d rows", len(u.threads))
			}
		})
	}

	u, _ := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "x")
	typeKeys(u, "x")
	if len(u.checkedThreads()) != 0 {
		t.Fatal("second x should untick")
	}
}

func TestKeysRemoveFromViewByMailbox(t *testing.T) {
	u, fb := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "jy")
	if len(fb.archived) != 1 || fb.archived[0] != "t1" {
		t.Fatalf("y in Inbox archives: %v", fb.archived)
	}

	fb.labelThreads = []desktop.ThreadItem{{ID: "s1", AccountID: "a1", Subject: "Starred one", IsStarred: true, LabelIDs: []string{"STARRED"}, Date: msAt(1)}}
	typeKeys(u, "gs")
	typeKeys(u, "jy")
	if !slices.Contains(fb.labelled, "s1:STARRED:false") || len(u.threads) != 0 {
		t.Fatalf("y in Starred unstars and removes: %v rows=%d", fb.labelled, len(u.threads))
	}

	fb.labelThreads = []desktop.ThreadItem{{ID: "o1", AccountID: "a1", Subject: "Sent one", LabelIDs: []string{"SENT"}, Date: msAt(1)}}
	typeKeys(u, "gt")
	typeKeys(u, "jy")
	if len(fb.archived) != 1 || len(fb.labelled) != 1 || len(u.threads) != 1 {
		t.Fatalf("y elsewhere is a no-op: archived=%v labelled=%v", fb.archived, fb.labelled)
	}
}

func TestKeysSpamInSpamFolderRestores(t *testing.T) {
	u, fb := newTestUI(t)
	u.installShortcuts()
	fb.labelThreads = []desktop.ThreadItem{{ID: "p1", AccountID: "a1", Subject: "Spammy", LabelIDs: []string{"SPAM"}, Date: msAt(1)}}
	u.selectLabel("SPAM")
	typeKeys(u, "j!")
	if len(fb.spammed) != 0 || !slices.Contains(fb.calls, "MarkThreadNotSpam") {
		t.Fatalf("! in Spam should mark not-spam: spammed=%v calls=%v", fb.spammed, fb.calls)
	}
}

func TestKeysImportantAndRead(t *testing.T) {
	u, fb := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "jjj") // t3: read thread
	typeKeys(u, "=")
	typeKeys(u, "-")
	if want := []string{"t3:IMPORTANT:true", "t3:IMPORTANT:false"}; !slices.Equal(fb.labelled, want) {
		t.Fatalf("= and -: %v", fb.labelled)
	}

	typeKeys(u, "U")
	if !u.selected.IsUnread || !slices.Contains(fb.calls, "MarkMessageUnread") {
		t.Fatalf("U should mark unread: %v", fb.calls)
	}
	typeKeys(u, "I")
	if u.selected.IsUnread || !slices.Contains(fb.calls, "MarkMessageRead") {
		t.Fatalf("I should mark read: %v", fb.calls)
	}
	typeKeys(u, "_")
	if !u.selected.IsUnread {
		t.Fatal("_ should mark unread")
	}
}

func TestKeysReplyForwardCompose(t *testing.T) {
	u, _ := newTestUI(t)
	u.installShortcuts()
	closeComposeWindows(t, u)

	// Nothing open: reply/forward keys do nothing.
	base := len(u.app.Driver().AllWindows())
	typeKeys(u, "rRaAfF")
	if len(u.app.Driver().AllWindows()) != base {
		t.Fatal("reply/forward without an open thread must not open windows")
	}

	typeKeys(u, "c")
	typeKeys(u, "d")
	if len(u.app.Driver().AllWindows()) != base+2 {
		t.Fatal("c and d should each open a compose window")
	}

	typeKeys(u, "jjj") // t3 has three messages; last is from Ben
	for keys, prefix := range map[string]string{"r": "Re: ", "A": "Re: ", "F": "Fwd: "} {
		typeKeys(u, keys)
		if !slices.Contains(keysEntryTexts(u), prefix+"Lunch this Friday?") {
			t.Fatalf("%s: no %q subject in %v", keys, prefix, keysEntryTexts(u))
		}
	}
}

func TestKeysCmdShortcuts(t *testing.T) {
	u, fb := newTestUI(t)
	u.installShortcuts()
	closeComposeWindows(t, u)
	h, ok := u.win.Canvas().(interface{ TypedShortcut(fyne.Shortcut) })
	if !ok {
		t.Skip("test canvas does not dispatch shortcuts")
	}
	windows := func() int { return len(u.app.Driver().AllWindows()) }

	h.TypedShortcut(cmd(fyne.KeyR, false)) // no thread open: no-op
	base := windows()
	if base != 1 {
		t.Fatalf("setup windows: %d", base)
	}

	typeKeys(u, "jjj")
	h.TypedShortcut(cmd(fyne.KeyR, false))
	h.TypedShortcut(cmd(fyne.KeyR, true))
	h.TypedShortcut(cmd(fyne.KeyF, true))
	if windows() != base+3 {
		t.Fatalf("cmd+r, cmd+shift+r, cmd+shift+f should open 3 windows, got %d", windows()-base)
	}

	h.TypedShortcut(cmd(fyne.KeyComma, false))
	if windows() != base+4 || u.settingsWin == nil {
		t.Fatal("cmd+, should open settings")
	}

	h.TypedShortcut(cmd(fyne.KeyBackspace, false))
	if len(fb.trashed) != 1 || fb.trashed[0] != "t3" {
		t.Fatalf("cmd+backspace should trash the open thread: %v", fb.trashed)
	}
}

func TestKeysSyncKeyGuardsWhileSyncing(t *testing.T) {
	u, fb := newTestUI(t)
	u.installShortcuts()
	typeKeys(u, "u")
	typeKeys(u, "N")
	if fb.syncs != 1 {
		t.Fatalf("u while a sync is running must not start another: %d", fb.syncs)
	}
}

func TestKeysLabelPicker(t *testing.T) {
	t.Run("no target does nothing", func(t *testing.T) {
		u, _ := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "l")
		if keysOverlay(u) != nil {
			t.Fatal("l without a thread must not open the picker")
		}
	})

	t.Run("lists user labels with current state", func(t *testing.T) {
		u, _ := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "jl") // t1 carries L1 (Inquiries)
		checks := keysChecks(u)
		for _, n := range []string{"Home", "Home/Remodel", "Inquiries", "Travel"} {
			if checks[n] == nil {
				t.Fatalf("missing label %q in %v", n, checks)
			}
		}
		if checks["Inbox"] != nil {
			t.Fatal("system labels must not be offered")
		}
		if !checks["Inquiries"].Checked || checks["Travel"].Checked {
			t.Fatal("check state should mirror the thread's labels")
		}
	})

	t.Run("apply sends only changes", func(t *testing.T) {
		u, fb := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "jl")
		checks := keysChecks(u)
		checks["Travel"].SetChecked(true)
		checks["Inquiries"].SetChecked(false)
		keysTap(t, u, "Apply")
		slices.Sort(fb.labelled)
		if want := []string{"t1:L1:false", "t1:L2:true"}; !slices.Equal(fb.labelled, want) {
			t.Fatalf("labelled %v, want %v", fb.labelled, want)
		}
		if keysOverlay(u) != nil {
			t.Fatal("picker should close on apply")
		}
		if !slices.Contains(u.selected.LabelIDs, "L2") || slices.Contains(u.selected.LabelIDs, "L1") {
			t.Fatalf("open thread labels not patched: %v", u.selected.LabelIDs)
		}
	})

	t.Run("unchanged apply sends nothing", func(t *testing.T) {
		u, fb := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "jl")
		keysTap(t, u, "Apply")
		if len(fb.labelled) != 0 {
			t.Fatalf("no changes expected: %v", fb.labelled)
		}
	})

	t.Run("enter on single match applies it", func(t *testing.T) {
		u, fb := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "jl")
		entry := u.win.Canvas().Focused().(*escEntry)
		test.Type(entry, "trav")
		if c := keysChecks(u); len(c) != 1 || c["Travel"] == nil {
			t.Fatalf("filter should leave only Travel: %v", c)
		}
		entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
		if !slices.Contains(fb.labelled, "t1:L2:true") || keysOverlay(u) != nil {
			t.Fatalf("Enter should apply the single match: %v", fb.labelled)
		}
	})

	t.Run("escape closes without applying", func(t *testing.T) {
		u, fb := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "jl")
		keysChecks(u)["Travel"].SetChecked(true)
		u.win.Canvas().Focused().(*escEntry).TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		if keysOverlay(u) != nil || len(fb.labelled) != 0 {
			t.Fatalf("Esc must close and discard: %v", fb.labelled)
		}
	})

	t.Run("create new label", func(t *testing.T) {
		u, fb := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "jl")
		test.Type(u.win.Canvas().Focused().(*escEntry), "Fresh")
		keysTap(t, u, `Create "Fresh"`)
		if !keysChecks(u)["Fresh"].Checked {
			t.Fatal("new label should appear ticked")
		}
		keysTap(t, u, "Apply")
		if !slices.Contains(fb.labelled, "t1:new-Fresh:true") {
			t.Fatalf("new label not applied: %v", fb.labelled)
		}
	})

	t.Run("ticked row is the target without an open thread", func(t *testing.T) {
		u, fb := newTestUI(t)
		u.installShortcuts()
		typeKeys(u, "xl")
		if u.selected != nil {
			t.Fatal("setup: nothing should be open")
		}
		keysChecks(u)["Travel"].SetChecked(true)
		keysTap(t, u, "Apply")
		if !slices.Contains(fb.labelled, "t1:L2:true") {
			t.Fatalf("ticked row should be labelled: %v", fb.labelled)
		}
	})

	t.Run("GetLabels error surfaces", func(t *testing.T) {
		fb := &fakeBackend{}
		u, _ := newTestUI(t, withBackend(fb))
		u.installShortcuts()
		typeKeys(u, "j")
		fb.errs = map[string]error{"GetLabels": keysErr("labels down")}
		typeKeys(u, "l")
		if keysOverlay(u) == nil {
			t.Fatal("an error dialog should be shown")
		}
		if len(keysChecks(u)) != 0 {
			t.Fatal("picker must not open on error")
		}
	})
}

type keysErr string

func (e keysErr) Error() string { return string(e) }

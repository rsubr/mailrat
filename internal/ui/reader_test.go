package ui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// readerWalk visits o and every object below it, descending through
// containers and widget renderers.
func readerWalk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			readerWalk(c, fn)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			readerWalk(c, fn)
		}
	}
}

// readerTexts is every visible string under o.
func readerTexts(o fyne.CanvasObject) (out []string) {
	readerWalk(o, func(c fyne.CanvasObject) {
		switch v := c.(type) {
		case *widget.Label:
			out = append(out, v.Text)
		case *widget.RichText:
			out = append(out, v.String())
		case *widget.Button:
			out = append(out, v.Text)
		case *canvas.Text:
			out = append(out, v.Text)
		}
	})
	return
}

// readerHas reports whether some visible string contains sub, ignoring case
// (dialogs capitalise messages) and whitespace runs.
func readerHas(o fyne.CanvasObject, sub string) bool {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	all := readerTexts(o) // flowed text (titles) is one word per object, so also try the joined text
	all = append(all, strings.Join(all, " "))
	for _, s := range all {
		if strings.Contains(norm(s), norm(sub)) {
			return true
		}
	}
	return false
}

func readerButton(o fyne.CanvasObject, text string) (b *widget.Button) {
	readerWalk(o, func(c fyne.CanvasObject) {
		if v, ok := c.(*widget.Button); ok && v.Text == text && b == nil {
			b = v
		}
	})
	return
}

func readerTipButton(o fyne.CanvasObject, tip string) (b *tooltipButton) {
	readerWalk(o, func(c fyne.CanvasObject) {
		if v, ok := c.(*tooltipButton); ok && v.tip == tip && b == nil {
			b = v
		}
	})
	return
}

func readerTapRows(o fyne.CanvasObject) (rows []*tapRow) {
	readerWalk(o, func(c fyne.CanvasObject) {
		if r, ok := c.(*tapRow); ok {
			rows = append(rows, r)
		}
	})
	return
}

func readerEntries(o fyne.CanvasObject) (out []string) {
	readerWalk(o, func(c fyne.CanvasObject) {
		if e, ok := c.(*widget.Entry); ok {
			out = append(out, e.Text)
		}
		if e, ok := c.(*suggestEntry); ok {
			out = append(out, e.Text)
		}
	})
	return
}

// readerComposes lists the open windows other than the main one.
func readerComposes(u *UI) (out []fyne.Window) {
	for _, w := range u.app.Driver().AllWindows() {
		if w != u.win && w.Title() != "" { // the test driver keeps an untitled extra window
			out = append(out, w)
		}
	}
	return
}

// readerCompose returns the newest compose window.
func readerCompose(t *testing.T, u *UI) fyne.Window {
	t.Helper()
	ws := readerComposes(u)
	if len(ws) == 0 {
		t.Fatal("no compose window opened")
	}
	return ws[len(ws)-1]
}

func readerOpen(t *testing.T, opts ...testOpt) (*UI, *fakeBackend) {
	t.Helper()
	u, fb := newTestUI(t, opts...)
	closeComposeWindows(t, u)
	u.list.Select(2) // t3 "Lunch this Friday?", 3 messages
	if u.selected == nil || len(u.messages) != 3 {
		t.Fatalf("thread not opened: %+v", u.selected)
	}
	return u, fb
}

func readerMsgs(n int) []desktop.MessageItem {
	var ms []desktop.MessageItem
	for i := 0; i < n; i++ {
		id := string(rune('a' + i))
		ms = append(ms, desktop.MessageItem{ID: id, ThreadID: "t3", AccountID: "a1", Subject: "S",
			FromAddr: "P" + id + " <" + id + "@x.com>", ToAddr: "n@x.com", Snippet: "snip-" + id, Body: "body-" + id, Date: msAt(n - i)})
	}
	return ms
}

func TestReaderNoSelection(t *testing.T) {
	u, _ := newTestUI(t)
	if !readerHas(u.reader, "No Conversation Selected") {
		t.Fatalf("empty reader: %v", readerTexts(u.reader))
	}
	for _, tip := range []string{"Archive", "Trash", "Mark unread", "Spam"} {
		if b := readerTipButton(u.reader, tip); b == nil || !b.Disabled() {
			t.Fatalf("%s should be disabled with no thread open", tip)
		}
	}
}

func TestReaderThreadView(t *testing.T) {
	freezeNow(t, goldenNow)
	u, _ := readerOpen(t)
	if u.msgCursor != 2 || u.threadLoading {
		t.Fatalf("cursor=%d loading=%v", u.msgCursor, u.threadLoading)
	}
	// Subject, label chips, newest message expanded, older one collapsed to its snippet.
	if !readerHas(u.reader, "Lunch this Friday?") || !readerHas(u.reader, "12PM at Delarosa sounds great") {
		t.Fatalf("thread missing: %v", readerTexts(u.reader))
	}
	if !readerHas(u.reader, "To:  Neil Jhaveri") {
		t.Fatal("expanded message shows no recipients line")
	}
	if readerHas(u.reader, "going to be in town") {
		t.Fatal("older read message should be collapsed")
	}
	// Tapping the collapsed header expands it.
	readerTapRows(u.msgCards[0])[0].Tapped(nil)
	if !readerHas(u.reader, "going to be in town") {
		t.Fatal("tapping header did not expand message")
	}
	// ...and tapping again collapses it.
	readerTapRows(u.msgCards[0])[0].Tapped(nil)
	if readerHas(u.reader, "going to be in town") {
		t.Fatal("second tap did not collapse message")
	}
}

func TestReaderUnreadMessageOpensExpanded(t *testing.T) {
	u, _ := readerOpen(t)
	u.messages = readerMsgs(3)
	u.messages[0].IsUnread = true
	u.refreshReader()
	if !readerHas(u.reader, "body-a") || readerHas(u.reader, "body-b") || !readerHas(u.reader, "body-c") {
		t.Fatalf("unread + newest expanded, middle collapsed: %v", readerTexts(u.reader))
	}
}

func TestReaderLongThreadFoldsMiddle(t *testing.T) {
	u, _ := readerOpen(t)
	u.messages = readerMsgs(6)
	u.msgCursor = 5
	u.refreshReader()
	if u.msgCards[1] != u.msgCards[2] || u.msgCards[2] != u.msgCards[3] || u.msgCards[0] == u.msgCards[1] {
		t.Fatal("messages 2-4 should share the hidden bar")
	}
	if !readerHas(u.msgCards[1], "3") {
		t.Fatalf("hidden bar lacks count: %v", readerTexts(u.msgCards[1]))
	}
	readerTapRows(u.msgCards[1])[0].Tapped(nil)
	if !u.collapsed[allKey] || u.msgCards[1] == u.msgCards[2] {
		t.Fatal("tapping the bar did not reveal hidden messages")
	}
	if !readerHas(u.msgCards[1], "snip-b") {
		t.Fatalf("revealed message not shown: %v", readerTexts(u.msgCards[1]))
	}
}

func TestReaderThreadLoadError(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"GetThread": errors.New("gmail down")}}
	u, _ := newTestUI(t, withBackend(fb))
	u.list.Select(2)
	if u.threadLoading || len(u.messages) != 0 {
		t.Fatalf("loading=%v messages=%d", u.threadLoading, len(u.messages))
	}
	top := u.win.Canvas().Overlays().Top()
	if top == nil || !readerHas(top, "gmail down") {
		t.Fatal("error dialog not shown")
	}
}

func TestReaderRenderedBody(t *testing.T) {
	u, _ := readerOpen(t)
	// Last message is HTML: tags gone, text and list items present, remote image blocked.
	for _, want := range []string{"12PM at Delarosa sounds great to me!", "delarosa.com/menu", "Bring", "cash", "[Delarosa]"} {
		if !readerHas(u.msgCards[2], want) {
			t.Errorf("rendered body lacks %q: %v", want, readerTexts(u.msgCards[2]))
		}
	}
	if readerHas(u.msgCards[2], "<p>") || readerHas(u.msgCards[2], "style=") {
		t.Error("raw markup leaked into the body")
	}
	if readerButton(u.msgCards[2], "Load 1 remote image(s)") == nil {
		t.Fatal("no load-images button for the blocked image")
	}
	// Quoted reply is folded behind "…".
	if readerHas(u.msgCards[2], "sounds good") {
		t.Fatal("quoted text should be folded")
	}
	readerButton(u.msgCards[2], "…").OnTapped()
	if !readerHas(u.msgCards[2], "On Tue, Neil wrote: sounds good") {
		t.Fatalf("quoted text not revealed: %v", readerTexts(u.msgCards[2]))
	}
	readerButton(u.msgCards[2], "…").OnTapped()
	if readerHas(u.msgCards[2], "sounds good") {
		t.Fatal("quoted text not folded again")
	}
}

// readerShot shows only the reader pane in the main window (the list pane is
// covered by the inbox goldens) with a fixed time zone, and returns the capture.
func readerShot(t *testing.T, u *UI, name string) {
	t.Helper()
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	u.win.SetContent(u.reader)
	u.refreshReader()
	golden(t, name, u.win.Canvas().Capture())
}

func TestReaderGoldenLight(t *testing.T) {
	freezeNow(t, goldenNow)
	u, _ := newTestUI(t)
	u.list.Select(2)
	u.messages[0].Date, u.messages[1].Date, u.messages[2].Date = msAt(20), msAt(14), msAt(5)
	u.refreshReader()
	readerShot(t, u, "reader_thread_light")
}

func TestReaderGoldenDark(t *testing.T) {
	freezeNow(t, goldenNow)
	u, _ := newTestUI(t, withDark())
	u.list.Select(2)
	u.messages[0].Date, u.messages[1].Date, u.messages[2].Date = msAt(20), msAt(14), msAt(5)
	u.refreshReader()
	readerShot(t, u, "reader_thread_dark")
}

func TestReaderGoldenAttachments(t *testing.T) {
	freezeNow(t, goldenNow)
	u, _ := newTestUI(t)
	u.list.Select(1)
	u.messages = []desktop.MessageItem{{ID: "m1", ThreadID: "t2", AccountID: "a1", Subject: "Lake Tahoe Trip",
		FromAddr: "Keith <k@x.com>", ToAddr: "Neil <n@x.com>", Body: "<p>Photos and the itinerary are attached.</p>", Date: msAt(2),
		Attachments: []desktop.AttachmentItem{
			{Name: "beach.png", MimeType: "image/png", Size: 2048, AttachmentID: "x1"},
			{Name: "itinerary.pdf", MimeType: "application/pdf", Size: 345678, AttachmentID: "x2"},
		}}}
	u.refreshReader()
	readerShot(t, u, "reader_attachments")
}

func TestReaderToolbarArchive(t *testing.T) {
	u, fb := readerOpen(t)
	b := readerTipButton(u.reader, "Archive")
	if b == nil || b.Disabled() {
		t.Fatal("archive button missing or disabled with a thread open")
	}
	test.Tap(b)
	if !slices.Equal(fb.archived, []string{"t3"}) || len(u.threads) != 4 {
		t.Fatalf("archived=%v threads=%d", fb.archived, len(u.threads))
	}
}

func TestReaderToolbarFailureKeepsThread(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"ArchiveThread": errors.New("denied"), "TrashThread": errors.New("denied")}}
	u, _ := readerOpen(t, withBackend(fb))
	test.Tap(readerTipButton(u.reader, "Archive"))
	test.Tap(readerTipButton(u.reader, "Trash"))
	if len(u.threads) != 5 || u.selected == nil || u.selected.ID != "t3" {
		t.Fatalf("thread removed despite failure: %d", len(u.threads))
	}
	if u.undoPop != nil {
		t.Fatal("undo bar shown for a failed trash")
	}
	if top := u.win.Canvas().Overlays().Top(); top == nil || !readerHas(top, "denied") {
		t.Fatal("error not shown")
	}
}

func TestReaderTrashAndUndo(t *testing.T) {
	u, fb := readerOpen(t)
	test.Tap(readerTipButton(u.reader, "Trash"))
	if !slices.Equal(fb.trashed, []string{"t3"}) || len(u.threads) != 4 {
		t.Fatalf("trashed=%v threads=%d", fb.trashed, len(u.threads))
	}
	if u.undoPop == nil || !readerHas(u.undoPop.Content, "Conversation moved to Trash") {
		t.Fatal("undo bar not shown")
	}
	test.Tap(readerButton(u.undoPop.Content, "Undo"))
	if !slices.Equal(fb.untrashed, []string{"t3"}) || u.undoPop != nil {
		t.Fatalf("untrashed=%v pop=%v", fb.untrashed, u.undoPop)
	}
	if !slices.Contains(fb.calls, "GetInbox") {
		t.Fatal("list not reloaded after undo")
	}
}

func TestReaderUndoFailureShowsError(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"UndoTrashThread": errors.New("too late")}}
	u, _ := readerOpen(t, withBackend(fb))
	test.Tap(readerTipButton(u.reader, "Trash"))
	test.Tap(readerButton(u.undoPop.Content, "Undo"))
	if u.undoPop != nil {
		t.Fatal("undo bar should close")
	}
	if top := u.win.Canvas().Overlays().Top(); top == nil || !readerHas(top, "too late") {
		t.Fatal("undo error not shown")
	}
}

func TestReaderSpamAndNotSpam(t *testing.T) {
	u, fb := readerOpen(t)
	test.Tap(readerTipButton(u.reader, "Spam"))
	if !slices.Contains(fb.calls, "MarkThreadSpam") || len(fb.spammed) != 1 || len(u.threads) != 4 {
		t.Fatalf("spam: %v %v", fb.spammed, fb.calls)
	}
	u.activeLabel = "SPAM"
	u.list.Select(0)
	u.spam()
	if !slices.Contains(fb.calls, "MarkThreadNotSpam") || len(fb.spammed) != 1 {
		t.Fatalf("in Spam the action must un-spam: %v", fb.calls)
	}
}

func TestReaderMarkUnread(t *testing.T) {
	u, fb := readerOpen(t)
	if u.selected.IsUnread || u.messages[2].IsUnread {
		t.Fatal("precondition: thread read")
	}
	test.Tap(readerTipButton(u.reader, "Mark unread"))
	if !slices.Contains(fb.calls, "MarkMessageUnread") || !u.selected.IsUnread || !u.messages[2].IsUnread || !u.threads[2].IsUnread {
		t.Fatalf("not marked unread: calls=%v", fb.calls)
	}
	// No messages loaded: nothing to mark.
	fb.calls = nil
	u.messages = nil
	u.markUnread()
	if len(fb.calls) != 0 {
		t.Fatalf("marked with no messages: %v", fb.calls)
	}
}

func TestReaderMarkRead(t *testing.T) {
	u, fb := newTestUI(t)
	u.list.Select(0) // t1 is unread; the fake serves its messages as read
	u.messages[1].IsUnread, u.messages[2].IsUnread = true, true
	if !u.selected.IsUnread {
		t.Fatal("precondition: thread unread")
	}
	u.markRead()
	if got := strings.Count(strings.Join(fb.calls, ","), "MarkMessageRead"); got != 2 {
		t.Fatalf("MarkMessageRead called %d times, want 2 (only unread messages): %v", got, fb.calls)
	}
	if u.selected.IsUnread || u.threads[0].IsUnread || u.messages[1].IsUnread || u.messages[2].IsUnread {
		t.Fatal("unread flags not cleared")
	}
}

func TestReaderRemoveLabelChip(t *testing.T) {
	u, fb := newTestUI(t)
	u.list.Select(0) // t1 carries label L1 "Inquiries"
	if !readerHas(u.reader, "Inquiries") {
		t.Fatalf("label chip missing: %v", readerTexts(u.reader))
	}
	u.removeLabel("L1")
	if !slices.Equal(fb.labelled, []string{"t1:L1:false"}) {
		t.Fatalf("labelled=%v", fb.labelled)
	}
	if len(u.selected.LabelIDs) != 0 || len(u.threads[0].LabelIDs) != 0 || readerHas(u.reader, "Inquiries") {
		t.Fatal("chip still shown after removal")
	}
}

func TestReaderRemoveLabelFailureKeepsChip(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"SetThreadLabel": errors.New("nope")}}
	u, _ := newTestUI(t, withBackend(fb))
	u.list.Select(0)
	u.removeLabel("L1")
	if len(u.selected.LabelIDs) != 1 || !readerHas(u.reader, "Inquiries") {
		t.Fatal("label dropped despite backend failure")
	}
}

func TestReaderAttachmentOpenWritesTempFile(t *testing.T) {
	u, fb := newTestUI(t)
	t.Cleanup(cleanAttachTemp)
	m := desktop.MessageItem{ID: "m1", AccountID: "a1"}
	u.openAttachment(m, desktop.AttachmentItem{Name: "../evil/report.pdf", AttachmentID: "x"})
	if !slices.Contains(fb.calls, "GetAttachmentData") {
		t.Fatal("attachment not fetched")
	}
	var found []string
	_ = filepath.WalkDir(attachTempRoot(), func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 1 || filepath.Base(found[0]) != "report.pdf" || !strings.HasPrefix(found[0], attachTempRoot()) {
		t.Fatalf("temp files: %v", found)
	}
}

func TestReaderAttachmentOpenError(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"GetAttachmentData": errors.New("expired")}}
	u, _ := newTestUI(t, withBackend(fb))
	u.openAttachment(desktop.MessageItem{ID: "m1", AccountID: "a1"}, desktop.AttachmentItem{Name: "a.txt", AttachmentID: "x"})
	if top := u.win.Canvas().Overlays().Top(); top == nil || !readerHas(top, "expired") {
		t.Fatal("download error not shown")
	}
}

func TestReaderAttachmentCardsAndMenu(t *testing.T) {
	u, _ := newTestUI(t)
	a1 := desktop.AttachmentItem{Name: "a.txt", MimeType: "text/plain", Size: 2048, AttachmentID: "1"}
	a2 := desktop.AttachmentItem{Name: "b.png", MimeType: "image/png", Size: 5, AttachmentID: "2"}
	body := u.messageBody(desktop.MessageItem{ID: "m1", AccountID: "a1", Body: "hi", Attachments: []desktop.AttachmentItem{a1, a2}})
	if !readerHas(body, "a.txt") || !readerHas(body, "b.png") || !readerHas(body, "2 KB") {
		t.Fatalf("cards: %v", readerTexts(body))
	}
	// Right-click on the second card offers Save and (2 files) Save all.
	rows := readerTapRows(body)
	if len(rows) != 2 {
		t.Fatalf("want 2 attachment cards, got %d", len(rows))
	}
	rows[1].onSecondary(fyne.NewPos(10, 10))
	top := u.win.Canvas().Overlays().Top()
	if top == nil || !readerHas(top, "Save") || !readerHas(top, "Save all to folder…") {
		t.Fatalf("menu: %v", top)
	}
	u.win.Canvas().Overlays().Remove(top)
	// A single attachment has no "Save all".
	body = u.messageBody(desktop.MessageItem{ID: "m1", AccountID: "a1", Body: "hi", Attachments: []desktop.AttachmentItem{a1}})
	readerTapRows(body)[0].onSecondary(fyne.NewPos(10, 10))
	top = u.win.Canvas().Overlays().Top()
	if top == nil || !readerHas(top, "Save") || readerHas(top, "Save all") {
		t.Fatal("single attachment menu wrong")
	}
}

func TestReaderSaveAllAttachmentsDedupesNames(t *testing.T) {
	u, fb := newTestUI(t)
	dir := t.TempDir()
	old := pickFolder
	t.Cleanup(func() { pickFolder = old })
	pickFolder = func(string) (string, error) { return dir, nil }
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := desktop.MessageItem{ID: "m1", AccountID: "a1", Attachments: []desktop.AttachmentItem{
		{Name: "a.txt", AttachmentID: "1"}, {Name: "a.txt", AttachmentID: "2"}}}
	u.saveAllAttachments(m)
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"a (1).txt", "a (2).txt", "a.txt"}) {
		t.Fatalf("files: %v", names)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "existing" {
		t.Fatal("existing file overwritten")
	}
	// Cancelled chooser writes nothing and fetches nothing more.
	pickFolder = func(string) (string, error) { return "", nil }
	fb.calls = nil
	u.saveAllAttachments(m)
	if len(fb.calls) != 0 {
		t.Fatalf("fetched after cancel: %v", fb.calls)
	}
}

func TestReaderSaveAttachmentFetchError(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"GetAttachmentData": errors.New("gone")}}
	u, _ := newTestUI(t, withBackend(fb))
	old := pickSaveFile
	t.Cleanup(func() { pickSaveFile = old })
	out := filepath.Join(t.TempDir(), "x.txt")
	pickSaveFile = func(_, _, _ string) (string, error) { return out, nil }
	u.saveAttachment(desktop.MessageItem{ID: "m1", AccountID: "a1"}, desktop.AttachmentItem{Name: "x.txt", AttachmentID: "id"})
	if _, err := os.Stat(out); err == nil {
		t.Fatal("file written despite fetch error")
	}
	if top := u.win.Canvas().Overlays().Top(); top == nil || !readerHas(top, "gone") {
		t.Fatal("error not shown")
	}
}

func TestReaderReplyForwardBar(t *testing.T) {
	u, _ := readerOpen(t)
	test.Tap(readerButton(u.reader, "Reply"))
	w := readerCompose(t, u)
	if got := readerEntries(w.Content()); !slices.Contains(got, "Ben Johnson <ben@x.com>") || !slices.Contains(got, "Re: Lunch this Friday?") {
		t.Fatalf("reply fields: %q", got)
	}
	if h := composeEditor(t, u).HTML(); !strings.Contains(h, "12PM at Delarosa") {
		t.Fatalf("reply lacks quote: %q", h)
	}
	w.Close()

	test.Tap(readerButton(u.reader, "Reply all"))
	got := readerEntries(readerCompose(t, u).Content())
	if !slices.Contains(got, "Neil Jhaveri <n@x.com>") { // Cc carries the original To
		t.Fatalf("reply-all fields: %q", got)
	}
	readerCompose(t, u).Close()

	test.Tap(readerButton(u.reader, "Forward"))
	w = readerCompose(t, u)
	got = readerEntries(w.Content())
	if !slices.Contains(got, "Fwd: Lunch this Friday?") || slices.Contains(got, "Ben Johnson <ben@x.com>") {
		t.Fatalf("forward fields: %q", got)
	}
	if h := composeEditor(t, u).HTML(); !strings.Contains(h, "12PM at Delarosa") {
		t.Fatalf("forward lacks original: %q", h)
	}
}

func TestReaderPerMessageReplyButton(t *testing.T) {
	u, _ := readerOpen(t)
	// Only the expanded (newest) message carries the header reply button.
	if readerTipButton(u.msgCards[0], "Reply") != nil {
		t.Fatal("collapsed message has a reply button")
	}
	test.Tap(readerTipButton(u.msgCards[2], "Reply"))
	if got := readerEntries(readerCompose(t, u).Content()); !slices.Contains(got, "Re: Lunch this Friday?") {
		t.Fatalf("fields: %q", got)
	}
}

func TestReaderReplyWithoutMessagesDoesNothing(t *testing.T) {
	u, _ := newTestUI(t)
	closeComposeWindows(t, u)
	u.reply(false)
	u.forward()
	if n := len(readerComposes(u)); n != 0 {
		t.Fatalf("%d compose windows opened", n)
	}
}

func TestReaderDraftThreadEditDraft(t *testing.T) {
	u, fb := readerOpen(t)
	fb.draftMsg = desktop.OutboundMsg{AccountID: "a1", DraftID: "d9", To: "carol@x.com", Subject: "Half written", Body: "<p>work in progress</p>"}
	u.selected.LabelIDs = []string{"DRAFT"}
	u.refreshReader()
	if got := readerTexts(u.reader); slices.Contains(got, "Reply") || !slices.Contains(got, "Edit draft") {
		t.Fatalf("draft bar: %v", got)
	}
	test.Tap(readerButton(u.reader, "Edit draft"))
	if !slices.Equal(fb.opened, []string{"m3"}) {
		t.Fatalf("OpenDraft ids: %v", fb.opened)
	}
	w := readerCompose(t, u)
	if got := readerEntries(w.Content()); !slices.Contains(got, "carol@x.com") || !slices.Contains(got, "Half written") {
		t.Fatalf("draft fields: %q", got)
	}
	if !strings.Contains(composeEditor(t, u).HTML(), "work in progress") {
		t.Fatal("draft body not loaded")
	}
}

func TestReaderEditDraftErrorOpensNothing(t *testing.T) {
	fb := &fakeBackend{errs: map[string]error{"OpenDraft": errors.New("draft gone")}}
	u, _ := readerOpen(t, withBackend(fb))
	u.selected.LabelIDs = []string{"DRAFT"}
	u.editDraft()
	if n := len(readerComposes(u)); n != 0 {
		t.Fatalf("%d compose windows opened", n)
	}
	if top := u.win.Canvas().Overlays().Top(); top == nil || !readerHas(top, "draft gone") {
		t.Fatal("error not shown")
	}
}

func TestReaderEditDraftIgnoredForNormalThread(t *testing.T) {
	u, fb := readerOpen(t)
	u.editDraft()
	if len(fb.opened) != 0 {
		t.Fatalf("opened draft of a non-draft thread: %v", fb.opened)
	}
}

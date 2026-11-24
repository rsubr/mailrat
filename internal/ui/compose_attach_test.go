package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// attOpen opens a compose window and captures the drop handler it registers.
func attOpen(t *testing.T, draft desktop.OutboundMsg) (*UI, *fakeBackend, fyne.Window, func(fyne.Position, []fyne.URI)) {
	t.Helper()
	freezeNow(t, goldenNow)
	u, fb := newTestUI(t, withDark())
	closeComposeWindows(t, u)
	var dropW fyne.Window
	var drop func(fyne.Position, []fyne.URI)
	old := setDropHandler
	t.Cleanup(func() { setDropHandler = old })
	setDropHandler = func(w fyne.Window, fn func(fyne.Position, []fyne.URI)) { dropW, drop = w, fn }
	u.compose(draft)
	w := compflowWin(u)
	if dropW != w {
		t.Fatal("drop handler must be registered on the compose window")
	}
	w.Resize(fyne.NewSize(1470, 918))
	return u, fb, w, drop
}

func attFile(t *testing.T, name string, size int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func attPick(t *testing.T, paths ...string) {
	t.Helper()
	old := pickOpenFiles
	t.Cleanup(func() { pickOpenFiles = old })
	pickOpenFiles = func(string) ([]string, error) { return paths, nil }
}

func attChips(w fyne.Window) (out []*attachChip) {
	compflowWalk(w.Content(), func(o fyne.CanvasObject) {
		if c, ok := o.(*attachChip); ok {
			out = append(out, c)
		}
	})
	return
}

func attTexts(o fyne.CanvasObject) (out []string) {
	compflowWalk(o, func(o fyne.CanvasObject) {
		if l, ok := o.(*widget.Label); ok {
			out = append(out, l.Text)
		}
	})
	return
}

func attTap(t *testing.T, w fyne.Window) {
	t.Helper()
	var found *tooltipButton
	compflowWalk(w.Content(), func(o fyne.CanvasObject) {
		if b, ok := o.(*tooltipButton); ok && b.tip == "Attach files" {
			found = b
		}
	})
	if found == nil {
		t.Fatal("no Attach files button")
	}
	found.OnTapped()
}

func attURI(p string) fyne.URI { return storage.NewFileURI(p) }

func TestAttachPickerAddsChips(t *testing.T) {
	_, _, w, _ := attOpen(t, desktop.OutboundMsg{})
	a, b := attFile(t, "report.pdf", 2048), attFile(t, "photo.png", 10)
	attPick(t, a, b)
	attTap(t, w)
	chips := attChips(w)
	if len(chips) != 2 {
		t.Fatalf("chips %d", len(chips))
	}
	all := strings.Join(attTexts(w.Content()), "|")
	for _, want := range []string{"report.pdf", "2 KB", "photo.png", "10 B", "2 files, 2 KB of 25 MB"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %q", want, all)
		}
	}
}

func TestAttachPickerCancelled(t *testing.T) {
	_, _, w, _ := attOpen(t, desktop.OutboundMsg{})
	attPick(t)
	attTap(t, w)
	if len(attChips(w)) != 0 {
		t.Fatal("cancel must not attach")
	}
}

func TestAttachDropTargetsComposeWindow(t *testing.T) {
	u, _, w, drop := attOpen(t, desktop.OutboundMsg{})
	drop(fyne.Position{}, []fyne.URI{attURI(attFile(t, "a.txt", 5)), attURI(attFile(t, "b.txt", 5))})
	if len(attChips(w)) != 2 {
		t.Fatalf("chips %d", len(attChips(w)))
	}
	if len(attChips(u.win)) != 0 {
		t.Fatal("main window must not get chips")
	}
}

func TestAttachDropDirectoryRejected(t *testing.T) {
	_, _, w, drop := attOpen(t, desktop.OutboundMsg{})
	dir := t.TempDir()
	drop(fyne.Position{}, []fyne.URI{attURI(dir), attURI(attFile(t, "ok.txt", 5))})
	if len(attChips(w)) != 1 {
		t.Fatalf("chips %d, want only the file", len(attChips(w)))
	}
	if got := strings.Join(compflowOverlayTexts(w), " "); !strings.Contains(got, "folder") {
		t.Fatalf("no folder error: %q", got)
	}
}

func TestAttachOversizeRejected(t *testing.T) {
	_, _, w, drop := attOpen(t, desktop.OutboundMsg{})
	p := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(desktop.MaxAttachmentBytes + 1)
	f.Close()
	drop(fyne.Position{}, []fyne.URI{attURI(p)})
	if len(attChips(w)) != 0 {
		t.Fatal("oversize file attached")
	}
	if got := compflowOverlayTexts(w); len(got) == 0 || !strings.Contains(strings.Join(got, " "), "big.bin") {
		t.Fatalf("no clear error: %v", got)
	}
}

func TestAttachRemove(t *testing.T) {
	_, _, w, drop := attOpen(t, desktop.OutboundMsg{})
	drop(fyne.Position{}, []fyne.URI{attURI(attFile(t, "a.txt", 5)), attURI(attFile(t, "b.txt", 5)), attURI(attFile(t, "c.txt", 5))})
	// keyboard: focus the first chip, Delete removes it and focus moves on
	chips := attChips(w)
	w.Canvas().Focus(chips[0])
	chips[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
	chips = attChips(w)
	if len(chips) != 2 || w.Canvas().Focused() != chips[0] {
		t.Fatalf("chips %d focus %v", len(chips), w.Canvas().Focused())
	}
	// the x button
	var rm *tooltipButton
	compflowWalk(chips[1], func(o fyne.CanvasObject) {
		if b, ok := o.(*tooltipButton); ok {
			rm = b
		}
	})
	rm.OnTapped()
	chips = attChips(w)
	if len(chips) != 1 || !strings.Contains(strings.Join(attTexts(chips[0]), "|"), "b.txt") {
		t.Fatalf("wrong chip left: %d", len(chips))
	}
	chips[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if len(attChips(w)) != 0 {
		t.Fatal("last chip not removed")
	}
}

func TestAttachSendIncludesAttachments(t *testing.T) {
	_, fb, w, drop := attOpen(t, desktop.OutboundMsg{})
	p := attFile(t, "contract.pdf", 100)
	drop(fyne.Position{}, []fyne.URI{attURI(p)})
	compflowRecipient(t, w, "To").SetText("x@y.com")
	compflowButton(t, w.Content(), "Send").OnTapped()
	compflowUntil(t, "send", func() bool { return len(fb.sent) == 1 })
	got := fb.sent[0].Attachments
	if len(got) != 1 || got[0].Filename != "contract.pdf" || got[0].Path != p || got[0].Size != 100 {
		t.Fatalf("sent %+v", got)
	}
}

func TestAttachAutosaveAndReopen(t *testing.T) {
	tick := compflowTicks(t)
	_, fb, w, drop := attOpen(t, desktop.OutboundMsg{})
	drop(fyne.Position{}, []fyne.URI{attURI(attFile(t, "a.txt", 7))})
	tick()
	compflowUntil(t, "draft saved", func() bool { return fb.draftCount() == 1 })
	ds, _ := fb.savedDrafts()
	if len(ds[0].Attachments) != 1 || ds[0].Attachments[0].Filename != "a.txt" {
		t.Fatalf("draft %+v", ds[0].Attachments)
	}
	// removing the attachment is a draft change
	attChips(w)[0].bar.remove(0)
	tick()
	compflowUntil(t, "second save", func() bool { return fb.draftCount() == 2 })

	_, _, w2, _ := attOpen(t, desktop.OutboundMsg{DraftID: "d1", Body: "<p>hi</p>",
		Attachments: []desktop.OutAttachment{{Filename: "kept.zip", Size: 3 << 20, Data: []byte("zz")}}})
	if c := attChips(w2); len(c) != 1 || !strings.Contains(strings.Join(attTexts(w2.Content()), "|"), "kept.zip") {
		t.Fatalf("reopened chips %d", len(c))
	}
}

func TestAttachForwardCarriesAttachments(t *testing.T) {
	freezeNow(t, goldenNow)
	u, fb := newTestUI(t)
	closeComposeWindows(t, u)
	m := qcMsg()
	m.Attachments = []desktop.AttachmentItem{{AttachmentID: "x1", Name: "orig.pdf", MimeType: "application/pdf"}}
	u.messages = []desktop.MessageItem{m}
	u.forward()
	w := compflowWin(u)
	if c := attChips(w); len(c) != 1 || !strings.Contains(strings.Join(attTexts(c[0]), "|"), "orig.pdf") {
		t.Fatalf("forward chips %d", len(attChips(w)))
	}
	if compflowCalls(fb, "GetAttachmentData") == 0 {
		t.Fatal("attachment bytes not fetched")
	}
	u.messages = []desktop.MessageItem{qcMsg()}
	u.reply(false)
	if len(attChips(compflowWin(u))) != 0 {
		t.Fatal("reply must not carry attachments")
	}
}

func TestAttachGoldens(t *testing.T) {
	_, _, w, drop := attOpen(t, desktop.OutboundMsg{})
	drop(fyne.Position{}, []fyne.URI{
		attURI(attFile(t, "Q3-budget.xlsx", 184*1024)),
		attURI(attFile(t, "team-photo.png", 3<<20)),
		attURI(attFile(t, "a-very-long-descriptive-file-name-for-the-signed-contract-final-v7.pdf", 1<<20)),
	})
	golden(t, "compose_attach_chips", w.Canvas().Capture())
	drop(fyne.Position{}, []fyne.URI{
		attURI(attFile(t, "notes.txt", 1200)), attURI(attFile(t, "logo.svg", 5000)),
		attURI(attFile(t, "data.csv", 70000)), attURI(attFile(t, "slides.pdf", 9<<20)),
	})
	golden(t, "compose_attach_many", w.Canvas().Capture())
}

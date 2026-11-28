package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

func TestThreadFlowAndActions(t *testing.T) {
	u, fb, _ := testUI(t)
	if len(u.threads) != 5 || len(u.accounts) != 2 || u.unread["INBOX"] != 12 {
		t.Fatalf("initial load: %d threads %d accounts", len(u.threads), len(u.accounts))
	}
	u.list.Select(2)
	if u.selected == nil || u.selected.ID != "t3" || len(u.messages) != 3 {
		t.Fatalf("open thread: %+v", u.selected)
	}
	u.archive()
	if len(fb.archived) != 1 || fb.archived[0] != "t3" || len(u.threads) != 4 || u.selected == nil || u.selected.ID != "t4" {
		t.Fatalf("archive: %v threads=%d", fb.archived, len(u.threads))
	}
	u.list.Select(0)
	u.trash()
	if len(fb.trashed) != 1 || len(u.threads) != 3 {
		t.Fatalf("trash: %v", fb.trashed)
	}
}

func TestAddAccountFlow(t *testing.T) {
	u, fb, _ := testUI(t)
	u.addAccount()
	if fb.loginStarted != 1 || u.loginDlg == nil {
		t.Fatalf("sign-in not started: started=%d dlg=%v", fb.loginStarted, u.loginDlg)
	}
	// Backend reports success: dialog closes without cancelling the (finished) login.
	u.handleEvent("accounts:updated", nil)
	if u.loginDlg != nil || fb.cancelled != 0 {
		t.Fatalf("dialog should close cleanly: dlg=%v cancelled=%d", u.loginDlg, fb.cancelled)
	}
}

func TestSettingsRefreshesAfterAccountsUpdated(t *testing.T) {
	u, _, _ := testUI(t)
	u.accounts = nil
	u.openSettings()
	u.handleEvent("accounts:updated", nil)
	if len(u.accounts) != 2 {
		t.Fatalf("accounts not reloaded: %d", len(u.accounts))
	}
}

func TestAddAccountWithoutCredentialsAsksForThem(t *testing.T) {
	u, fb, _ := testUI(t)
	fb.noCreds = true
	u.addAccount()
	if fb.loginStarted != 0 {
		t.Fatal("must not start sign-in without credentials")
	}
}

func TestLoginErrorShown(t *testing.T) {
	u, _, _ := testUI(t)
	u.showLogin("https://x")
	u.handleEvent("account:add:error", "") // cancellation: silent
	if u.loginDlg != nil {
		t.Fatal("dialog should close on cancel event")
	}
}

func TestReplyDraft(t *testing.T) {
	u, _, _ := testUI(t)
	u.list.Select(2)
	last, _ := u.lastMessage()
	d := replyDraft(last, true)
	if d.Subject != "Re: Lunch this Friday?" || d.InReplyTo != "<m3@x.com>" || d.To != "Ben Johnson <ben@x.com>" {
		t.Fatalf("%+v", d)
	}
	if want := "12PM at Delarosa sounds great to me!"; !contains(d.Body, want) || !contains(d.Body, `<div class="gmail_quote">`) || !contains(d.Body, "<blockquote>") {
		t.Fatalf("quote missing in %q", d.Body)
	}
}

// TestScreenshot renders the window to a PNG when MAILRAT_SHOT is set.
func TestScreenshot(t *testing.T) {
	path := os.Getenv("MAILRAT_SHOT")
	if path == "" {
		t.Skip("set MAILRAT_SHOT=/path/out.png")
	}
	u, _, w := testUI(t)
	u.list.Select(2)
	w.Resize(fyne.NewSize(uxWindowW+1, uxWindowH))
	w.Resize(fyne.NewSize(uxWindowW, uxWindowH))
	img := w.Canvas().Capture()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestGmailShortcuts(t *testing.T) {
	u, fb, _ := testUI(t)
	u.installShortcuts()

	typeKeys(u, "j") // first conversation
	if u.selected == nil || u.selected.ID != "t1" {
		t.Fatalf("j should open first thread: %+v", u.selected)
	}
	typeKeys(u, "jj")
	if u.selected.ID != "t3" {
		t.Fatalf("jj: want t3, got %s", u.selected.ID)
	}
	typeKeys(u, "k")
	if u.selected.ID != "t2" {
		t.Fatalf("k: want t2, got %s", u.selected.ID)
	}

	typeKeys(u, "s")
	if len(fb.labelled) != 1 || fb.labelled[0] != "t2:STARRED:true" || !u.selected.IsStarred {
		t.Fatalf("star: %v", fb.labelled)
	}
	typeKeys(u, "s")
	if fb.labelled[1] != "t2:STARRED:false" {
		t.Fatalf("unstar: %v", fb.labelled)
	}
	typeKeys(u, "+")
	if fb.labelled[2] != "t2:IMPORTANT:true" {
		t.Fatalf("important: %v", fb.labelled)
	}

	typeKeys(u, "e") // archive t2, cursor lands on next row
	if len(fb.archived) != 1 || fb.archived[0] != "t2" || u.selected == nil || u.selected.ID != "t3" {
		t.Fatalf("e should open the next row: %v %+v", fb.archived, u.selected)
	}
	typeKeys(u, "#")
	if len(fb.trashed) != 1 || fb.trashed[0] != "t3" {
		t.Fatalf("#: %v", fb.trashed)
	}
	typeKeys(u, "j!")
	if len(fb.spammed) != 1 {
		t.Fatalf("!: %v", fb.spammed)
	}

	typeKeys(u, "N")
	if fb.syncs == 0 {
		t.Fatal("N should sync")
	}
}

func TestGmailGoToAndMessageKeys(t *testing.T) {
	u, _, _ := testUI(t)
	u.installShortcuts()

	typeKeys(u, "gs")
	if u.activeLabel != "STARRED" {
		t.Fatalf("g s: %s", u.activeLabel)
	}
	typeKeys(u, "gi")
	if u.activeLabel != "INBOX" || u.pendingKey != 0 {
		t.Fatalf("g i: %s", u.activeLabel)
	}
	typeKeys(u, "gz") // unknown pair: ignored, prefix cleared
	if u.activeLabel != "INBOX" || u.pendingKey != 0 {
		t.Fatal("bad sequence should reset")
	}
	typeKeys(u, "g")
	u.pendingAt = time.Now().Add(-time.Minute) // stale prefix: second key acts alone
	typeKeys(u, "j")
	if u.selected == nil {
		t.Fatal("stale g must not swallow j")
	}

	// Thread of three messages: cursor starts on the newest.
	u.list.Select(2)
	if u.msgCursor != 2 {
		t.Fatalf("cursor: %d", u.msgCursor)
	}
	typeKeys(u, "p")
	if u.msgCursor != 1 || !u.expanded(u.messages[1], false) {
		t.Fatalf("p should open message 1: cursor=%d", u.msgCursor)
	}
	typeKeys(u, "pp")
	if u.msgCursor != 0 {
		t.Fatalf("clamp at first: %d", u.msgCursor)
	}
	typeKeys(u, "nn")
	if u.msgCursor != 2 {
		t.Fatalf("n: %d", u.msgCursor)
	}
	before := u.expanded(u.messages[2], true)
	typeKeys(u, "o")
	if u.expanded(u.messages[2], true) == before {
		t.Fatal("o should toggle the cursor message")
	}

	u.typedKey(fyne.KeyEscape)
	if u.selected != nil {
		t.Fatal("Esc should return to the list")
	}
	typeKeys(u, "?") // help dialog builds without panic
	if u.win.Canvas().Overlays().Top() == nil {
		t.Fatal("? should show help")
	}
	typeKeys(u, "e") // overlay open: keys ignored
}

func TestGmailSearchKey(t *testing.T) {
	u, _, _ := testUI(t)
	u.installShortcuts()
	typeKeys(u, "/")
	if u.win.Canvas().Focused() != fyne.Focusable(u.search) {
		t.Fatal("/ should focus search")
	}
}

func TestLabelsSectionCollapsePersists(t *testing.T) {
	u, _, _ := testUI(t)
	if u.labelsHidden {
		t.Fatal("labels should start expanded")
	}
	u.toggleLabels()
	if !u.labelsHidden || !u.app.Preferences().Bool(prefLabelsHidden) {
		t.Fatal("collapse not stored")
	}
	if u2 := newUI(u.app, &fakeBackend{}); !u2.labelsHidden {
		t.Fatal("state not restored from preferences")
	}
	u.toggleLabels()
	if u.labelsHidden || u.app.Preferences().Bool(prefLabelsHidden) {
		t.Fatal("expand not stored")
	}
}

func TestSidebarMinimalToggleAndRowShape(t *testing.T) {
	u, _, _ := testUI(t)
	if u.sidebarMini {
		t.Fatal("sidebar should start expanded")
	}
	th := u.threads[0]
	two := newThreadRow(false)
	u.setRow(two, th)

	u.toggleSidebar()
	if !u.sidebarMini || !u.app.Preferences().Bool(prefSidebarMini) {
		t.Fatal("minimal state not stored")
	}
	if u2 := newUI(u.app, &fakeBackend{}); !u2.sidebarMini {
		t.Fatal("state not restored from preferences")
	}
	one := newThreadRow(true)
	u.setRow(one, th)
	if one.MinSize().Height >= two.MinSize().Height {
		t.Fatalf("single-line row (%v) should be shorter than two-line (%v)", one.MinSize().Height, two.MinSize().Height)
	}

	// Unread rows are bold, read rows are not.
	th.IsUnread = true
	u.setRow(one, th)
	if !one.when.TextStyle.Bold || !one.head.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold {
		t.Fatal("unread row not bold")
	}
	th.IsUnread = false
	u.setRow(one, th)
	if one.when.TextStyle.Bold {
		t.Fatal("read row should not be bold")
	}

	u.toggleSidebar()
	if u.sidebarMini || u.app.Preferences().Bool(prefSidebarMini) {
		t.Fatal("expand not stored")
	}
}
func TestAttachmentCard(t *testing.T) {
	u, _, _ := testUI(t)
	pic := desktop.AttachmentItem{Name: "p.png", MimeType: "image/png", Size: 2048}
	doc := desktop.AttachmentItem{Name: "a.pdf", MimeType: "application/pdf", Size: 10}
	tapped := false
	r := findTapRow(u.attachmentCard(doc, newMailTheme(true), func() { tapped = true }, nil))
	if r == nil {
		t.Fatal("card has no tappable row")
	}
	r.Tapped(nil)
	if !tapped {
		t.Fatal("tapping the card did not fire")
	}
	body := u.messageBody(desktop.MessageItem{ID: "x", Body: "hi", Attachments: []desktop.AttachmentItem{pic, doc}})
	if findTapRow(body) == nil {
		t.Fatal("message body shows no attachment cards")
	}
}

func TestSaveAttachmentUsesNativeDialog(t *testing.T) {
	u, _, _ := testUI(t)
	oldSave := pickSaveFile
	t.Cleanup(func() { pickSaveFile = oldSave })
	out := filepath.Join(t.TempDir(), "x.txt")
	var gotName string
	pickSaveFile = func(_, name, _ string) (string, error) { gotName = name; return out, nil }
	u.saveAttachment(desktop.MessageItem{ID: "m1", AccountID: "a1"}, desktop.AttachmentItem{Name: "x.txt", AttachmentID: "id"})
	if gotName != "x.txt" {
		t.Fatalf("dialog got name %q", gotName)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("attachment not written: %v", err)
	}
	// Cancelled dialog writes nothing.
	pickSaveFile = func(_, _, _ string) (string, error) { return "", nil }
	os.Remove(out)
	u.saveAttachment(desktop.MessageItem{ID: "m1", AccountID: "a1"}, desktop.AttachmentItem{Name: "x.txt", AttachmentID: "id"})
	if _, err := os.Stat(out); err == nil {
		t.Fatal("cancelled save wrote a file")
	}
}

func TestChooseCredentialsUsesNativeDialog(t *testing.T) {
	u, fb, _ := testUI(t)
	old := pickOpenFile
	t.Cleanup(func() { pickOpenFile = old })
	pickOpenFile = func(string, ...string) (string, error) { return "/tmp/creds.json", nil }
	u.chooseCredentials()
	if fb.imported != "/tmp/creds.json" {
		t.Fatalf("imported %q", fb.imported)
	}
}

func TestReplyBarAtBottom(t *testing.T) {
	u, _, _ := testUI(t)
	u.list.Select(2)
	if got := buttonLabels(u.replyBar()); len(got) != 3 || got[0] != "Reply" || got[1] != "Reply all" || got[2] != "Forward" {
		t.Fatalf("reply bar: %v", got)
	}
	if got := buttonLabels(u.toolbar()); len(got) != 0 {
		t.Fatalf("toolbar still has labelled buttons: %v", got)
	}
}

func TestComposeAutosavesOnlyOnChange(t *testing.T) {
	tick := compflowTicks(t)
	u, fb, _ := testUI(t)
	u.accounts = []desktop.AccountItem{{ID: "a1", Email: "a@x.com"}}
	u.compose(desktop.OutboundMsg{AccountID: "a1", To: "b@x.com", Subject: "s"})
	t.Cleanup(func() { // closing the compose window stops its autosave goroutine
		// Close from the UI goroutine, as in production; iterate a copy since Close removes windows.
		fyne.DoAndWait(func() {
			for _, w := range append([]fyne.Window(nil), u.app.Driver().AllWindows()...) {
				if w != u.win {
					w.Close()
				}
			}
		})
	})
	tick()
	tick()
	if n := fb.draftCount(); n != 0 {
		t.Fatalf("unchanged compose saved %d drafts", n)
	}
}

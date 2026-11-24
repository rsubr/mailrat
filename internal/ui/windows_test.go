package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	fdesktop "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

// winWalk visits every visible object under o, descending into containers and
// widget renderers.
func winWalk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	if o == nil || !o.Visible() {
		return
	}
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			winWalk(c, fn)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			winWalk(c, fn)
		}
	}
}

func winFind[T fyne.CanvasObject](root fyne.CanvasObject) (out []T) {
	winWalk(root, func(o fyne.CanvasObject) {
		if v, ok := o.(T); ok {
			out = append(out, v)
		}
		if ee, ok := o.(*escEntry); ok { // an escEntry is an Entry for these purposes
			if v, ok := any(&ee.Entry).(T); ok {
				out = append(out, v)
			}
		}
	})
	return
}

func winTexts(root fyne.CanvasObject) (out []string) {
	winWalk(root, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Label:
			out = append(out, v.Text)
		case *widget.Button:
			out = append(out, v.Text)
		case *canvas.Text:
			out = append(out, v.Text)
		}
	})
	return
}

func winHasText(root fyne.CanvasObject, sub string) bool {
	for _, s := range winTexts(root) {
		if strings.Contains(strings.ToLower(s), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func winButtons(root fyne.CanvasObject, text string) (out []*widget.Button) {
	for _, b := range winFind[*widget.Button](root) {
		if b.Text == text {
			out = append(out, b)
		}
	}
	return
}

func winButton(t *testing.T, root fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	bs := winButtons(root, text)
	if len(bs) == 0 {
		t.Fatalf("no button %q in %v", text, winTexts(root))
	}
	return bs[0]
}

func winEntries(root fyne.CanvasObject, placeholder string) (out []*widget.Entry) {
	for _, e := range winFind[*widget.Entry](root) {
		if e.PlaceHolder == placeholder {
			out = append(out, e)
		}
	}
	return
}

// winTop is the topmost overlay (dialog) of w, or nil.
func winTop(w fyne.Window) fyne.CanvasObject {
	if top := w.Canvas().Overlays().Top(); top != nil {
		return top
	}
	return nil
}

func winStubOpen(t *testing.T, path string, err error) {
	t.Helper()
	old := pickOpenFile
	t.Cleanup(func() { pickOpenFile = old })
	pickOpenFile = func(string, ...string) (string, error) { return path, err }
}

func winContains(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

// ── add account ─────────────────────────────────────────────────────────────

func TestWinWelcomeAddAccountStartsLogin(t *testing.T) {
	u, fb := newTestUI(t, withNoAccounts())
	if !winHasText(u.win.Content(), "Welcome to mailrat") {
		t.Fatalf("no welcome screen: %v", winTexts(u.win.Content()))
	}
	test.Tap(winButton(t, u.win.Content(), "Add Account…"))
	if fb.loginStarted != 1 || u.loginDlg == nil {
		t.Fatalf("sign-in not started: %d %v", fb.loginStarted, u.loginDlg)
	}
}

func TestWinLoginPasteSubmitsRawText(t *testing.T) {
	u, fb, w := testUI(t)
	u.addAccount()
	top := winTop(w)
	if top == nil || !winHasText(top, "Sign in with Google") {
		t.Fatalf("login dialog missing: %v", winTexts(w.Canvas().Overlays().Top()))
	}
	paste := winEntries(top, "Paste the address from the browser after sign-in")
	if len(paste) != 1 {
		t.Fatal("paste field missing")
	}
	paste[0].SetText("http://127.0.0.1:9/cb?code=abc&state=s")
	test.Tap(winButton(t, top, "Continue"))
	if fb.submitted != "http://127.0.0.1:9/cb?code=abc&state=s" {
		t.Fatalf("submitted %q", fb.submitted)
	}
	// A successful submit leaves closing to the backend's accounts:updated event.
	if u.loginDlg == nil {
		t.Fatal("dialog must stay until the backend reports an outcome")
	}
}

func TestWinLoginPasteErrorShownAndDialogKept(t *testing.T) {
	u, fb, w := testUI(t)
	u.addAccount()
	fb.errs = map[string]error{"SubmitLoginCode": errors.New("bad code")}
	test.Tap(winButton(t, winTop(w), "Continue"))
	if winContains(fb.calls, "SubmitLoginCode") != 1 {
		t.Fatalf("calls %v", fb.calls)
	}
	if !winHasText(winTop(w), "bad code") {
		t.Fatalf("error not shown: %v", winTexts(winTop(w)))
	}
	if u.loginDlg == nil {
		t.Fatal("login dialog should remain so the user can retry")
	}
}

func TestWinLoginCancelCancelsBackend(t *testing.T) {
	u, fb, w := testUI(t)
	u.addAccount()
	test.Tap(winButton(t, winTop(w), "Cancel"))
	if fb.cancelled != 1 || u.loginDlg != nil {
		t.Fatalf("cancelled=%d dlg=%v", fb.cancelled, u.loginDlg)
	}
	if winTop(w) != nil {
		t.Fatal("dialog still open")
	}
}

func TestWinLoginCopyLinkPutsURLOnClipboard(t *testing.T) {
	u, _, w := testUI(t)
	u.addAccount()
	test.Tap(winButton(t, winTop(w), "Copy Link"))
	if got := u.app.Clipboard().Content(); got != "https://accounts.example/auth?x=1" {
		t.Fatalf("clipboard %q", got)
	}
}

func TestWinBeginLoginErrorShown(t *testing.T) {
	u, fb, w := newTestUIWith(t, &fakeBackend{errs: map[string]error{"BeginLogin": errors.New("no network")}})
	u.addAccount()
	if u.loginDlg != nil || fb.loginStarted != 0 {
		t.Fatal("login dialog must not open on error")
	}
	if !winHasText(winTop(w), "no network") {
		t.Fatalf("error not shown: %v", winTexts(winTop(w)))
	}
}

func newTestUIWith(t *testing.T, fb *fakeBackend) (*UI, *fakeBackend, fyne.Window) {
	t.Helper()
	u, fb := newTestUI(t, withBackend(fb))
	return u, fb, u.win
}

func TestWinAddAccountEventsCloseLogin(t *testing.T) {
	u, fb, w := testUI(t)
	u.addAccount()
	u.handleEvent("account:add:error", "token exchange failed")
	if u.loginDlg != nil || fb.cancelled != 0 {
		t.Fatalf("dlg=%v cancelled=%d", u.loginDlg, fb.cancelled)
	}
	if !winHasText(winTop(w), "token exchange failed") {
		t.Fatalf("error not shown: %v", winTexts(winTop(w)))
	}
}

func TestWinAccountsUpdatedOpensSettingsAndSyncs(t *testing.T) {
	u, fb, _ := testUI(t)
	u.addAccount()
	u.accounts = nil
	u.handleEvent("accounts:updated", nil)
	if u.loginDlg != nil || fb.cancelled != 0 {
		t.Fatal("login dialog should close without cancelling")
	}
	if len(u.accounts) != 2 {
		t.Fatalf("accounts not reloaded: %d", len(u.accounts))
	}
	if u.settingsWin == nil {
		t.Fatal("settings not opened")
	}
	if fb.syncs != 1 {
		t.Fatalf("syncs=%d", fb.syncs)
	}
}

// ── credentials ─────────────────────────────────────────────────────────────

func TestWinCredentialsPromptImportsThenSignsIn(t *testing.T) {
	u, fb, w := newTestUIWith(t, &fakeBackend{noCreds: true, accounts: &[]desktop.AccountItem{}})
	winStubOpen(t, "/tmp/client.json", nil)
	u.addAccount()
	if fb.loginStarted != 0 {
		t.Fatal("must not sign in without credentials")
	}
	top := winTop(w)
	if top == nil || !winHasText(top, "Google credentials needed") || !winHasText(top, "Desktop app") {
		t.Fatalf("prompt missing: %v", winTexts(top))
	}
	test.Tap(winButton(t, top, "Choose credentials.json…"))
	if fb.imported != "/tmp/client.json" {
		t.Fatalf("imported %q", fb.imported)
	}
	if fb.loginStarted != 1 || u.loginDlg == nil {
		t.Fatal("sign-in should continue after import")
	}
}

func TestWinCredentialsPromptCancelDoesNothing(t *testing.T) {
	u, fb, w := newTestUIWith(t, &fakeBackend{noCreds: true})
	u.addAccount()
	test.Tap(winButton(t, winTop(w), "Cancel"))
	if fb.imported != "" || fb.loginStarted != 0 || winTop(w) != nil {
		t.Fatalf("imported=%q started=%d top=%v", fb.imported, fb.loginStarted, winTop(w))
	}
}

func TestWinChooseCredentialsPaths(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		pickErr    error
		importErr  error
		wantImport string
		wantErr    string
		wantLogin  int
		noAccounts bool
	}{
		{name: "picker cancelled", path: ""},
		{name: "picker error", pickErr: errors.New("zenity missing"), wantErr: "zenity missing"},
		{name: "import error", path: "/x.json", importErr: errors.New("invalid client json"), wantErr: "invalid client json"},
		{name: "ok first account", path: "/x.json", wantImport: "/x.json", wantLogin: 1, noAccounts: true},
		{name: "replace file with accounts keeps sign-in closed", path: "/x.json", wantImport: "/x.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fb := &fakeBackend{}
			if c.importErr != nil {
				fb.errs = map[string]error{"ImportCredentials": c.importErr}
			}
			if c.noAccounts {
				fb.accounts = &[]desktop.AccountItem{}
			}
			u, fb, w := newTestUIWith(t, fb)
			winStubOpen(t, c.path, c.pickErr)
			u.chooseCredentials()
			if fb.imported != c.wantImport || fb.loginStarted != c.wantLogin {
				t.Fatalf("imported=%q started=%d", fb.imported, fb.loginStarted)
			}
			if c.wantErr != "" && !winHasText(winTop(w), c.wantErr) {
				t.Fatalf("error %q not shown: %v", c.wantErr, winTexts(winTop(w)))
			}
			if c.wantErr == "" && c.wantLogin == 0 && winTop(w) != nil {
				t.Fatal("cancel must be silent")
			}
		})
	}
}

func TestWinChooseCredentialsRefreshesOpenSettings(t *testing.T) {
	u, fb, _ := newTestUIWith(t, &fakeBackend{noCreds: true})
	u.openSettings()
	if !winHasText(u.settingsWin.Content(), "Google credentials: not configured") {
		t.Fatalf("status: %v", winTexts(u.settingsWin.Content()))
	}
	winStubOpen(t, "/x.json", nil)
	u.chooseCredentials()
	if fb.imported == "" || !winHasText(u.settingsWin.Content(), "Google credentials: configured") {
		t.Fatalf("settings not refreshed: %v", winTexts(u.settingsWin.Content()))
	}
}

// ── settings ────────────────────────────────────────────────────────────────

func TestWinSettingsAccountsTab(t *testing.T) {
	u, fb, _ := testUI(t)
	u.openSettings()
	c := u.settingsWin.Content()
	for _, want := range []string{"Personal  ·  neil@personal.com", "Work  ·  neil@work.com", "Google credentials: configured"} {
		if !winHasText(c, want) {
			t.Fatalf("missing %q in %v", want, winTexts(c))
		}
	}
	if sel := winFind[*widget.Select](c); len(sel) != 2 || sel[0].Selected != "Past 90 days" {
		t.Fatalf("period selects: %v", sel)
	}
	if u.app.Driver().AllWindows()[len(u.app.Driver().AllWindows())-1] != u.settingsWin {
		t.Fatal("settings window not registered")
	}
	w := u.settingsWin
	u.openSettings()
	if u.settingsWin != w {
		t.Fatal("second openSettings must reuse the window")
	}

	sel := winFind[*widget.Select](u.settingsWin.Content())[0]
	sel.SetSelected("Past 30 days")
	if winContains(fb.callLog(), "GetAccountSettings") == 0 {
		t.Fatalf("calls %v", fb.callLog())
	}
	if len(fb.savedAccount) != 1 || fb.savedAccount[0].AccountID != "a1" || fb.savedAccount[0].SyncPeriod != "30d" {
		t.Fatalf("saved account settings %+v", fb.savedAccount)
	}
}

func TestWinSettingsRename(t *testing.T) {
	u, fb, _ := testUI(t)
	u.openSettings()
	entries := winEntries(u.settingsWin.Content(), "Display name")
	if len(entries) != 2 || entries[1].Text != "Work" {
		t.Fatalf("name entries: %v", entries)
	}
	entries[1].SetText("Office")
	test.Tap(winButtons(u.settingsWin.Content(), "Rename")[1])
	if len(fb.renamed) != 1 || fb.renamed[0] != "a2=Office" {
		t.Fatalf("renamed %v", fb.renamed)
	}
}

func TestWinSettingsRenameErrorShown(t *testing.T) {
	u, fb, w := testUI(t)
	fb.errs = map[string]error{"RenameAccount": errors.New("rename failed")}
	u.openSettings()
	test.Tap(winButtons(u.settingsWin.Content(), "Rename")[0])
	if top := winTop(u.settingsWin); top == nil || !winHasText(top, "rename failed") {
		t.Fatalf("error not shown on settings window: %v", winTexts(winTop(u.settingsWin)))
	}
	if winTop(w) != nil {
		t.Fatalf("error leaked to main window: %v", winTexts(winTop(w)))
	}
}

func TestWinSettingsRemoveErrorShownOnSettings(t *testing.T) {
	u, fb, w := testUI(t)
	fb.errs = map[string]error{"RemoveAccount": errors.New("remove failed")}
	u.openSettings()
	sw := u.settingsWin
	test.Tap(winButtons(sw.Content(), "Remove")[0])
	test.Tap(winButton(t, winTop(sw), "Yes"))
	if top := winTop(sw); top == nil || !winHasText(top, "remove failed") {
		t.Fatalf("error not shown on settings window: %v", winTexts(winTop(sw)))
	}
	if winTop(w) != nil {
		t.Fatalf("error leaked to main window: %v", winTexts(winTop(w)))
	}
}

func TestWinSettingsRemoveAccountConfirm(t *testing.T) {
	u, fb, _ := testUI(t)
	u.activeAccount = "a1"
	u.openSettings()
	sw := u.settingsWin
	test.Tap(winButtons(sw.Content(), "Remove")[0])
	top := winTop(sw)
	if top == nil || !winHasText(top, "Delete neil@personal.com and all its local mail?") {
		t.Fatalf("confirm missing: %v", winTexts(top))
	}
	// Declining removes nothing.
	test.Tap(winButton(t, top, "No"))
	if winContains(fb.calls, "RemoveAccount") != 0 || u.activeAccount != "a1" {
		t.Fatal("declined confirm must not remove")
	}
	test.Tap(winButtons(sw.Content(), "Remove")[0])
	test.Tap(winButton(t, winTop(sw), "Yes"))
	if winContains(fb.calls, "RemoveAccount") != 1 {
		t.Fatalf("calls %v", fb.calls)
	}
	if u.activeAccount != unified {
		t.Fatalf("active account %q after removing it", u.activeAccount)
	}
}

func TestWinSettingsRefetchConfirmAndError(t *testing.T) {
	u, fb, w := testUI(t)
	u.openSettings()
	sw := u.settingsWin
	test.Tap(winButtons(sw.Content(), "Refetch all mail")[0])
	test.Tap(winButton(t, winTop(sw), "No"))
	if winContains(fb.calls, "RefetchAccount") != 0 {
		t.Fatal("declined confirm must not refetch")
	}
	fb.errs = map[string]error{"RefetchAccount": errors.New("refetch failed")}
	test.Tap(winButtons(sw.Content(), "Refetch all mail")[0])
	test.Tap(winButton(t, winTop(sw), "Yes"))
	top := winTop(sw)
	if winContains(fb.callLog(), "RefetchAccount") != 1 || top == nil || !winHasText(top, "refetch failed") {
		t.Fatalf("calls %v top %v", fb.callLog(), winTexts(top))
	}
	if winTop(w) != nil {
		t.Fatalf("error leaked to main window: %v", winTexts(winTop(w)))
	}
}

func TestWinSettingsGeneralTab(t *testing.T) {
	u, fb, w := testUI(t)
	u.openSettings()
	tabs := winFind[*container.AppTabs](u.settingsWin.Content())[0]
	tabs.SelectIndex(2)
	sels := winFind[*widget.Select](u.settingsWin.Content())
	if len(sels) != 2 || sels[0].Selected != "Every 15 minutes" || sels[1].Selected != "neil@personal.com" {
		t.Fatalf("general selects: %+v", sels)
	}
	sels[0].SetSelected("Every hour")
	if len(fb.savedGlobal) != 1 || fb.savedGlobal[0].SyncFrequency != "1h" || fb.savedGlobal[0].DefaultAccountID != "a1" {
		t.Fatalf("saved global settings %+v", fb.savedGlobal)
	}
	fb.errs = map[string]error{"SetGlobalSettings": errors.New("save failed")}
	sels[1].SetSelected("neil@work.com")
	if top := winTop(u.settingsWin); top == nil || !winHasText(top, "save failed") {
		t.Fatalf("error not shown on settings window: %v", winTexts(winTop(u.settingsWin)))
	}
	if winTop(w) != nil {
		t.Fatalf("error leaked to main window: %v", winTexts(winTop(w)))
	}
}

func TestWinSettingsLabelsTabSyncScope(t *testing.T) {
	f2NoDebounce(t)
	u, fb, _ := testUI(t)
	u.openSettings()
	tabs := winFind[*container.AppTabs](u.settingsWin.Content())[0]
	tabs.SelectIndex(1)
	c := u.settingsWin.Content()
	if !winHasText(c, "Inbox") {
		t.Fatalf("labels missing: %v", winTexts(c))
	}
	var mode *widget.Select
	for _, s := range winFind[*widget.Select](c) {
		if s.Selected == "Everything" {
			mode = s
		}
	}
	if mode == nil {
		t.Fatal("no scope select")
	}
	if len(winFind[*widget.Entry](c)) != 0 {
		t.Fatal("value box must be hidden for Everything")
	}
	mode.SetSelected("Last N days")
	if len(fb.savedSync) != 1 || fb.savedSync[0].LabelID != "INBOX" || fb.savedSync[0].Mode != desktop.SyncModeDays || fb.savedSync[0].Value != 30 {
		t.Fatalf("saved sync config %+v", fb.savedSync)
	}
	vals := winFind[*widget.Entry](c)
	if len(vals) != 1 || vals[0].Text != "30" {
		t.Fatalf("value box: %v", vals)
	}
	// Invalid numbers are not saved; valid ones are.
	vals[0].SetText("0")
	vals[0].SetText("abc")
	if len(fb.savedSync) != 1 {
		t.Fatalf("invalid value saved: %+v", fb.savedSync)
	}
	vals[0].SetText("7")
	if len(fb.savedSync) != 2 || fb.savedSync[1].Value != 7 || fb.savedSync[1].Mode != desktop.SyncModeDays {
		t.Fatalf("valid value not saved: %+v", fb.savedSync)
	}
}

func TestWinSettingsLabelsTabNoAccounts(t *testing.T) {
	u, _ := newTestUI(t, withNoAccounts(), withNoCreds())
	u.openSettings()
	c := u.settingsWin.Content()
	if !winHasText(c, "No accounts yet. Add a Google account to get started.") || !winHasText(c, "Google credentials: not configured") {
		t.Fatalf("texts %v", winTexts(c))
	}
	winFind[*container.AppTabs](c)[0].SelectIndex(1)
	if !winHasText(u.settingsWin.Content(), "Add an account to choose what to sync.") {
		t.Fatalf("texts %v", winTexts(u.settingsWin.Content()))
	}
}

func TestWinSettingsEscapeClosesUnlessDialogOpen(t *testing.T) {
	u, _, _ := testUI(t)
	u.openSettings()
	sw := u.settingsWin
	esc := &fyne.KeyEvent{Name: fyne.KeyEscape}
	test.Tap(winButtons(sw.Content(), "Remove")[0])
	sw.Canvas().OnTypedKey()(esc)
	if u.settingsWin == nil {
		t.Fatal("Escape with a dialog open must not close the window")
	}
	test.Tap(winButton(t, winTop(sw), "No"))
	sw.Canvas().OnTypedKey()(esc)
	if u.settingsWin != nil {
		t.Fatal("Escape should close settings")
	}
}

func TestWinSettingsGolden(t *testing.T) {
	u, _ := newTestUI(t)
	u.openSettings()
	u.settingsWin.Resize(fyne.NewSize(620, 460))
	golden(t, "windows_settings_accounts", u.settingsWin.Canvas().Capture())
	winFind[*container.AppTabs](u.settingsWin.Content())[0].SelectIndex(2)
	golden(t, "windows_settings_general", u.settingsWin.Canvas().Capture())
}

// ── contacts ────────────────────────────────────────────────────────────────

func winContactsUI(t *testing.T) (*UI, *fakeBackend) {
	t.Helper()
	fb := &fakeBackend{
		book: []desktop.ContactItem{
			{ID: "1", AccountID: "a1", Name: "Ann Lee", Email: "ann@x.com", Phone: "555-0100"},
			{ID: "2", AccountID: "a1", Name: "Bob Ray", Email: "bob@x.com"},
		},
		frequent: []desktop.ContactItem{{Email: "bob@x.com", Count: 9}, {Email: "zed@x.com", Count: 3}},
	}
	u, fb := newTestUI(t, withBackend(fb))
	u.openContacts()
	return u, fb
}

func TestWinContactsListSectionsAndReuse(t *testing.T) {
	u, _ := winContactsUI(t)
	w := u.contactsWin
	list := winFind[*widget.List](w.Content())[0]
	// Frequently contacted(+Bob), Contacts(2)(+Ann,+Bob), Other contacts(+Zed)
	if list.Length() != 7 {
		t.Fatalf("rows %d", list.Length())
	}
	for _, want := range []string{"Frequently contacted", "Contacts (2)", "Other contacts", "Ann Lee", "zed@x.com"} {
		if !winHasText(w.Content(), want) {
			t.Fatalf("missing %q: %v", want, winTexts(w.Content()))
		}
	}
	if !winHasText(w.Content(), "Select a contact") {
		t.Fatal("no placeholder detail")
	}
	u.openContacts()
	if u.contactsWin != w {
		t.Fatal("second openContacts must reuse the window")
	}
}

func TestWinContactsSearchFilters(t *testing.T) {
	u, _ := winContactsUI(t)
	c := u.contactsWin.Content()
	winFind[*escEntry](c)[0].SetText("555")
	list := winFind[*widget.List](c)[0]
	if list.Length() != 1 {
		t.Fatalf("rows %d", list.Length())
	}
	if !winHasText(c, "Ann Lee") || winHasText(c, "Bob Ray") {
		t.Fatalf("filter wrong: %v", winTexts(c))
	}
	winFind[*escEntry](c)[0].SetText("nobody")
	if list.Length() != 1 || !winHasText(c, "No contacts match") {
		t.Fatalf("rows %d: %v", list.Length(), winTexts(c))
	}
}

func TestWinContactsViewEditSave(t *testing.T) {
	u, fb := winContactsUI(t)
	c := u.contactsWin.Content()
	winFind[*widget.List](c)[0].Select(3) // Ann
	if !winHasText(c, "ann@x.com") || !winHasText(c, "555-0100") {
		t.Fatalf("detail: %v", winTexts(c))
	}
	test.Tap(winButton(t, c, "Edit"))
	if !winHasText(c, "Edit contact") {
		t.Fatalf("no edit form: %v", winTexts(c))
	}
	var name *widget.Entry
	for _, e := range winFind[*widget.Entry](c) {
		if e.Text == "Ann Lee" {
			name = e
		}
	}
	name.SetText("  Ann Smith ")
	test.Tap(winButton(t, c, "Save"))
	if len(fb.savedContacts) != 1 {
		t.Fatalf("saved %v", fb.savedContacts)
	}
	got := fb.savedContacts[0]
	if got.ID != "1" || got.Name != "Ann Smith" || got.Email != "ann@x.com" || got.AccountID != "a1" {
		t.Fatalf("saved %+v", got)
	}
}

func TestWinContactsCreateRequiresEmailAndSaves(t *testing.T) {
	u, fb := winContactsUI(t)
	w := u.contactsWin
	c := w.Content()
	test.Tap(winButton(t, c, "Create contact"))
	if !winHasText(c, "Create contact") || !winHasText(c, "Save to") {
		t.Fatalf("no create form with account chooser: %v", winTexts(c))
	}
	test.Tap(winButton(t, c, "Save"))
	if len(fb.savedContacts) != 0 || winTop(w) == nil {
		t.Fatalf("empty email must be rejected with an error: saved=%v", fb.savedContacts)
	}
	test.Tap(winButton(t, winTop(w), "OK"))
	fb.errs = map[string]error{"SaveContact": errors.New("quota")}
	entries := winFind[*widget.Entry](c)
	email := entries[len(entries)-2]
	email.SetText("new@x.com")
	test.Tap(winButton(t, c, "Save"))
	if !winHasText(winTop(w), "quota") {
		t.Fatalf("save error not shown: %v", winTexts(winTop(w)))
	}
	test.Tap(winButton(t, winTop(w), "OK"))
	fb.errs = nil
	test.Tap(winButton(t, c, "Save")) // button was re-enabled after the failure
	if len(fb.savedContacts) != 1 || fb.savedContacts[0].Email != "new@x.com" {
		t.Fatalf("saved %v", fb.savedContacts)
	}
}

func TestWinContactsDeleteConfirm(t *testing.T) {
	u, fb := winContactsUI(t)
	w := u.contactsWin
	c := w.Content()
	winFind[*widget.List](c)[0].Select(4) // Bob in the Contacts section
	test.Tap(winButton(t, c, "Delete"))
	if !winHasText(winTop(w), "Delete Bob Ray?") {
		t.Fatalf("confirm: %v", winTexts(winTop(w)))
	}
	test.Tap(winButton(t, winTop(w), "No"))
	if len(fb.deletedContacts) != 0 {
		t.Fatal("declined delete deleted")
	}
	test.Tap(winButton(t, c, "Delete"))
	test.Tap(winButton(t, winTop(w), "Yes"))
	if len(fb.deletedContacts) != 1 || fb.deletedContacts[0] != "2" {
		t.Fatalf("deleted %v", fb.deletedContacts)
	}
	if !winHasText(c, "Select a contact") {
		t.Fatal("detail pane should reset after delete")
	}
}

func TestWinContactsOtherContactOffersAdd(t *testing.T) {
	u, _ := winContactsUI(t)
	c := u.contactsWin.Content()
	winFind[*widget.List](c)[0].Select(6) // zed
	if !winHasText(c, "Not in your contacts yet") || len(winButtons(c, "Edit")) != 0 {
		t.Fatalf("detail: %v", winTexts(c))
	}
	test.Tap(winButton(t, c, "Add to contacts"))
	if !winHasText(c, "Create contact") {
		t.Fatalf("no create form: %v", winTexts(c))
	}
}

func TestWinContactsSendEmailOpensCompose(t *testing.T) {
	u, _ := winContactsUI(t)
	before := len(u.app.Driver().AllWindows())
	c := u.contactsWin.Content()
	winFind[*widget.List](c)[0].Select(3)
	test.Tap(winButton(t, c, "Send email"))
	if len(u.app.Driver().AllWindows()) != before+1 {
		t.Fatal("compose window not opened")
	}
	closeComposeWindows(t, u)
}

func TestWinContactsEscapeClosesFromSearch(t *testing.T) {
	u, _ := winContactsUI(t)
	w := u.contactsWin
	f, ok := w.Canvas().Focused().(fyne.Focusable)
	if !ok {
		t.Fatal("search entry should have focus")
	}
	f.(interface{ TypedKey(*fyne.KeyEvent) }).TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if u.contactsWin != nil {
		t.Fatal("Escape in search should close the window")
	}
}

func TestWinContactsGolden(t *testing.T) {
	u, _ := winContactsUI(t)
	u.contactsWin.Resize(fyne.NewSize(960, 620))
	winFind[*widget.List](u.contactsWin.Content())[0].Select(3)
	golden(t, "windows_contacts", u.contactsWin.Canvas().Capture())
}

// ── sync status ─────────────────────────────────────────────────────────────

func TestWinSyncStatusEvents(t *testing.T) {
	u, fb, w := testUI(t)
	u.handleEvent("sync:progress", map[string]string{"account": "neil@work.com"})
	if u.syncStatus != "Syncing neil@work.com…" || !winHasText(w.Content(), "Syncing neil@work.com…") {
		t.Fatalf("status %q texts %v", u.syncStatus, winTexts(w.Content()))
	}
	u.handleEvent("sync:progress", nil)
	if u.syncStatus != "Syncing…" {
		t.Fatalf("status %q", u.syncStatus)
	}
	before := winContains(fb.calls, "GetLabels")
	u.handleEvent("sync:complete", nil)
	if u.syncStatus != "" || winHasText(w.Content(), "Syncing") {
		t.Fatalf("status not cleared: %q", u.syncStatus)
	}
	if winContains(fb.calls, "GetLabels") <= before {
		t.Fatal("labels not reloaded after sync")
	}
}

func TestWinSyncNowIsNoopWhileSyncing(t *testing.T) {
	u, fb, w := testUI(t)
	u.syncNow()
	if fb.syncs != 1 || u.syncStatus != "Syncing…" || !winHasText(w.Content(), "Syncing…") {
		t.Fatalf("syncs=%d status=%q", fb.syncs, u.syncStatus)
	}
	u.syncNow()
	if fb.syncs != 1 {
		t.Fatalf("overlapping sync started: %d", fb.syncs)
	}
	u.handleEvent("sync:complete", nil)
	u.syncNow()
	if fb.syncs != 2 {
		t.Fatalf("syncs=%d", fb.syncs)
	}
}

func TestWinInboxUpdatedReloadsThreads(t *testing.T) {
	u, fb, _ := testUI(t)
	before := winContains(fb.calls, "GetInbox")
	u.handleEvent("inbox:updated", nil)
	if winContains(fb.calls, "GetInbox") <= before {
		t.Fatal("threads not reloaded")
	}
}

func TestWinAPIErrorIndicatorAndLog(t *testing.T) {
	u, fb, w := testUI(t)
	fb.apiErrs = []desktop.APIError{{Timestamp: time.Date(2026, 3, 1, 9, 5, 0, 0, time.Local).UnixMilli(), Message: "429 rate limited"}}
	if winHasText(w.Content(), "1") && len(winButtons(w.Content(), "1")) != 0 {
		t.Fatal("indicator shown before any error")
	}
	u.handleEvent("api:error", nil)
	badge := winButtons(w.Content(), "1")
	if u.errCount != 1 || len(badge) != 1 {
		t.Fatalf("errCount=%d badge=%d", u.errCount, len(badge))
	}
	test.Tap(badge[0])
	if !winHasText(winTop(w), "429 rate limited") || !winHasText(winTop(w), "Mar 1 09:05:00") {
		t.Fatalf("log: %v", winTexts(winTop(w)))
	}
	test.Tap(winButton(t, winTop(w), "Clear"))
	if u.errCount != 0 || len(winButtons(w.Content(), "1")) != 0 {
		t.Fatal("clear should reset the indicator")
	}
}

// ── tooltips ────────────────────────────────────────────────────────────────

func TestWinTooltipShowsAndDismisses(t *testing.T) {
	test.NewTempApp(t)
	taps := 0
	b := newTooltipButton(nil, "Archive", func() { taps++ })
	w := newWindow(t, b, 120, 60)
	c := w.Canvas()

	// Hovering arms a delayed tooltip; leaving before it fires cancels it.
	b.MouseIn(&desktopMouse)
	if b.timer == nil || c.Overlays().Top() != nil {
		t.Fatal("hover should arm the timer without showing yet")
	}
	b.MouseOut()
	if b.timer != nil {
		t.Fatal("MouseOut should cancel the pending tooltip")
	}

	b.showTip()
	top := c.Overlays().Top()
	if top == nil || top != fyne.CanvasObject(b.over) {
		t.Fatalf("tooltip overlay missing: %v", top)
	}
	if got := b.over.box.(*tipBox).text.Text; got != "Archive" {
		t.Fatalf("tip text %q", got)
	}
	b.MouseOut()
	if c.Overlays().Top() != nil || b.over != nil {
		t.Fatal("MouseOut should remove the tooltip")
	}

	// Tapping the overlay inside the button hides the tip and forwards the tap.
	b.showTip()
	b.over.Tapped(&fyne.PointEvent{AbsolutePosition: fyne.NewPos(5, 5)})
	if taps != 1 || c.Overlays().Top() != nil {
		t.Fatalf("taps=%d top=%v", taps, c.Overlays().Top())
	}
	// A tap outside the button only dismisses.
	b.showTip()
	b.over.Tapped(&fyne.PointEvent{AbsolutePosition: fyne.NewPos(500, 500)})
	if taps != 1 || c.Overlays().Top() != nil {
		t.Fatalf("outside tap: taps=%d", taps)
	}
}

func TestWinTooltipEmptyTipNeverArms(t *testing.T) {
	test.NewTempApp(t)
	b := newTooltipButton(nil, "", func() {})
	newWindow(t, b, 120, 60)
	b.MouseIn(&desktopMouse)
	if b.timer != nil {
		t.Fatal("empty tip must not arm a timer")
	}
}

var desktopMouse = fdesktop.MouseEvent{}

// ── F2 fixes ────────────────────────────────────────────────────────────────

func f2NoDebounce(t *testing.T) {
	t.Helper()
	old := scopeSaveDelay
	scopeSaveDelay = 0
	t.Cleanup(func() { scopeSaveDelay = old })
}

func TestF2TooltipFlipsAboveAtWindowBottom(t *testing.T) {
	u, _ := newTestUI(t)
	c := u.win.Canvas()
	b := newTooltipButton(nil, "Settings", func() {})
	b.Resize(fyne.NewSize(32, 32))
	size := fyne.NewSize(1470, 918)
	for _, y := range []float32{100, 880} {
		o := newTipOverlay(b, c, fyne.NewPos(40, y))
		c.Overlays().Add(o) // the box needs a renderer to know its size
		o.Resize(size)
		box := o.box
		c.Overlays().Remove(o)
		if box.Position().Y+box.Size().Height > size.Height || box.Position().Y < 0 {
			t.Fatalf("button at y=%v: tooltip at %v size %v is off canvas", y, box.Position(), box.Size())
		}
		if y == 100 && box.Position().Y < y+32 {
			t.Fatalf("tooltip should sit below a button with room: %v", box.Position())
		}
		if y == 880 && box.Position().Y+box.Size().Height > y {
			t.Fatalf("tooltip should flip above the footer button: %v", box.Position())
		}
	}
}

func TestF2SettingsKeepTabAcrossRebuildAndReopen(t *testing.T) {
	u, _, _ := testUI(t)
	u.openSettings()
	winFind[*container.AppTabs](u.settingsWin.Content())[0].SelectIndex(2)
	u.reloadAccounts()
	if got := winFind[*container.AppTabs](u.settingsWin.Content())[0].SelectedIndex(); got != 2 {
		t.Fatalf("rebuild reset the tab to %d", got)
	}
	u.openSettings()
	if got := winFind[*container.AppTabs](u.settingsWin.Content())[0].SelectedIndex(); got != 2 {
		t.Fatalf("reopen reset the tab to %d", got)
	}
}

func TestF2SettingsTypedScopeValueSavedOnceAfterPause(t *testing.T) {
	old := scopeSaveDelay
	scopeSaveDelay = 40 * time.Millisecond
	t.Cleanup(func() { scopeSaveDelay = old })
	u, fb, _ := testUI(t)
	u.openSettings()
	winFind[*container.AppTabs](u.settingsWin.Content())[0].SelectIndex(1)
	c := u.settingsWin.Content()
	for _, s := range winFind[*widget.Select](c) {
		if s.Selected == "Everything" {
			s.SetSelected("Last N days")
			break
		}
	}
	base := len(fb.syncSaves())
	val := winFind[*widget.Entry](c)[0]
	val.SetText("3")
	val.SetText("36")
	val.SetText("365")
	if len(fb.syncSaves()) != base {
		t.Fatal("saved while still typing")
	}
	for end := time.Now().Add(5 * time.Second); len(fb.syncSaves()) == base && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // a second (wrong) save would land by now
	if got := fb.syncSaves(); len(got) != base+1 || got[base].Value != 365 {
		t.Fatalf("want one save of 365, got %+v", got[base:])
	}
}

func TestF2SettingsLabelNamesAreFriendly(t *testing.T) {
	fb := &fakeBackend{syncConfigs: []desktop.SyncConfigItem{
		{LabelID: "DRAFT", LabelName: "DRAFT"}, {LabelID: "Label_1", LabelName: "Receipts"}}}
	u, _ := newTestUI(t, withBackend(fb))
	u.openSettings()
	winFind[*container.AppTabs](u.settingsWin.Content())[0].SelectIndex(1)
	if !winHasText(u.settingsWin.Content(), "Drafts") || !winHasText(u.settingsWin.Content(), "Receipts") {
		t.Fatalf("labels: %v", winTexts(u.settingsWin.Content()))
	}
	for _, txt := range winTexts(u.settingsWin.Content()) {
		if txt == "INBOX" || txt == "DRAFT" || txt == "ALL_MAIL" {
			t.Fatalf("raw label id %q shown", txt)
		}
	}
}

func TestF2SettingsRenameEntryEscapeAndEnter(t *testing.T) {
	u, fb, _ := testUI(t)
	u.openSettings()
	e := winEntries(u.settingsWin.Content(), "Display name")[0]
	e.SetText("Home")
	e.OnSubmitted("Home")
	if len(fb.renamed) != 1 || fb.renamed[0] != "a1=Home" {
		t.Fatalf("Enter did not rename: %v", fb.renamed)
	}
	esc := winFind[*escEntry](u.settingsWin.Content())[0]
	u.settingsWin.Canvas().Focus(esc)
	esc.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if u.settingsWin != nil {
		t.Fatal("Escape in a settings entry should close the window")
	}
}

func TestF2RefetchTellsUserItStarted(t *testing.T) {
	u, _, _ := testUI(t)
	u.openSettings()
	sw := u.settingsWin
	test.Tap(winButtons(sw.Content(), "Refetch all mail")[0])
	test.Tap(winButton(t, winTop(sw), "Yes"))
	if top := winTop(sw); top == nil || !winHasText(top, "Downloading all mail") {
		t.Fatalf("no feedback: %v", winTexts(winTop(sw)))
	}
}

func TestF2LoginPasteTrimmedEnterSubmitsAndRetryKeepsDialog(t *testing.T) {
	u, fb, w := testUI(t)
	u.addAccount()
	top := winTop(w)
	paste := winEntries(top, "Paste the address from the browser after sign-in")[0]
	paste.SetText("  http://127.0.0.1:9/cb?code=abc&state=s \n")
	paste.OnSubmitted(paste.Text)
	if fb.submitted != "http://127.0.0.1:9/cb?code=abc&state=s" {
		t.Fatalf("submitted %q", fb.submitted)
	}
	u.handleEvent("account:add:retry", "token exchange failed: invalid_grant")
	if u.loginDlg == nil {
		t.Fatal("a rejected code must keep the sign-in dialog open")
	}
	if !winHasText(winTop(w), "invalid_grant") {
		t.Fatalf("error not shown: %v", winTexts(winTop(w)))
	}
}

func TestF2ShowLoginTwiceReplacesDialogWithoutCancelling(t *testing.T) {
	u, fb, _ := testUI(t)
	u.showLogin("http://a")
	first := u.loginDlg
	u.showLogin("http://b")
	if u.loginDlg == first || fb.cancelled != 0 {
		t.Fatalf("dlg replaced=%v cancelled=%d", u.loginDlg != first, fb.cancelled)
	}
}

func TestF2LoginErrorShownOnSettingsWhenOpen(t *testing.T) {
	u, fb, w := testUI(t)
	u.openSettings()
	u.addAccount()
	fb.errs = map[string]error{"SubmitLoginCode": errors.New("bad code")}
	test.Tap(winButton(t, winTop(u.settingsWin), "Continue"))
	if !winHasText(winTop(u.settingsWin), "bad code") || winTop(w) != nil {
		t.Fatalf("error should appear over Settings: %v", winTexts(winTop(u.settingsWin)))
	}
}

func TestF2HelpUsesPlatformCommandKey(t *testing.T) {
	u, _, w := testUI(t)
	u.showShortcutHelp()
	top := winTop(w)
	if cmdLabel == "Ctrl+" && (winHasText(top, "⌘") || !winHasText(top, "Ctrl+Enter")) {
		t.Fatalf("help: %v", winTexts(top))
	}
	if !winHasText(top, "Navigation") {
		t.Fatalf("help sections: %v", winTexts(top))
	}
}

func TestF2ContactsFailedEditDoesNotChangeCancelView(t *testing.T) {
	u, fb := winContactsUI(t)
	c := u.contactsWin.Content()
	winFind[*widget.List](c)[0].Select(3) // Ann
	test.Tap(winButton(t, c, "Edit"))
	for _, e := range winFind[*widget.Entry](c) {
		if e.Text == "Ann Lee" {
			e.SetText("Mistyped")
		}
	}
	fb.errs = map[string]error{"SaveContact": errors.New("quota")}
	test.Tap(winButton(t, c, "Save"))
	test.Tap(winButton(t, winTop(u.contactsWin), "OK"))
	test.Tap(winButton(t, c, "Cancel"))
	if !winHasText(c, "Ann Lee") || winHasText(c, "Mistyped") {
		t.Fatalf("cancel shows unsaved edits: %v", winTexts(c))
	}
}

func TestF2ContactsEmptyEmailMessage(t *testing.T) {
	u, _ := winContactsUI(t)
	w := u.contactsWin
	test.Tap(winButton(t, w.Content(), "Create contact"))
	test.Tap(winButton(t, w.Content(), "Save"))
	top := winTop(w)
	if !winHasText(top, "email address") || winHasText(top, "recipient") {
		t.Fatalf("copy: %v", winTexts(top))
	}
}

func TestF2ContactsEnterInPhoneSaves(t *testing.T) {
	u, fb := winContactsUI(t)
	c := u.contactsWin.Content()
	test.Tap(winButton(t, c, "Create contact"))
	entries := winFind[*widget.Entry](c)
	entries[len(entries)-2].SetText("enter@x.com")
	phone := entries[len(entries)-1]
	phone.OnSubmitted("")
	if len(fb.savedContacts) != 1 || fb.savedContacts[0].Email != "enter@x.com" {
		t.Fatalf("saved %v", fb.savedContacts)
	}
}

func TestF2ContactsSelectionClearedAfterDeleteAndOnHeader(t *testing.T) {
	u, _ := winContactsUI(t)
	c := u.contactsWin.Content()
	list := winFind[*widget.List](c)[0]
	list.Select(3)
	test.Tap(winButton(t, c, "Delete"))
	test.Tap(winButton(t, winTop(u.contactsWin), "Yes"))
	// Re-selecting the same row must reopen the detail pane, so nothing may stay selected.
	list.Select(3)
	if !winHasText(c, "Edit") {
		t.Fatalf("detail not reopened: %v", winTexts(c))
	}
	list.Select(0) // header row
	list.Select(3)
	if !winHasText(c, "Edit") {
		t.Fatalf("detail not reopened after a header click: %v", winTexts(c))
	}
}

func TestF2ContactsSearchNoMatchRow(t *testing.T) {
	u, _ := winContactsUI(t)
	search := winEntries(u.contactsWin.Content(), "Search contacts")[0]
	search.SetText("qqqq")
	if !winHasText(u.contactsWin.Content(), "No contacts match") {
		t.Fatalf("empty state: %v", winTexts(u.contactsWin.Content()))
	}
}

func TestF2ContactsReopenOnOtherAccountRebuilds(t *testing.T) {
	u, _ := winContactsUI(t)
	first := u.contactsWin
	u.activeAccount = "a2"
	u.openContacts()
	if u.contactsWin == first || u.contactsWin == nil || u.contactsWin.Title() == first.Title() {
		t.Fatalf("window not rebuilt for the new account: %q", u.contactsWin.Title())
	}
	second := u.contactsWin
	u.openContacts()
	if u.contactsWin != second {
		t.Fatal("same account must reuse the window")
	}
}

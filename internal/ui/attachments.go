package ui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"

	"github.com/mailrat/mailrat/internal/desktop"
)

// pickFolder is the native folder chooser; a variable so tests can stub it.
var pickFolder = func(title string) (string, error) {
	return cancelled(zenity.SelectFile(zenity.Title(title), zenity.Directory()))
}

// sanitizeFilename reduces an attachment name to a safe single path element:
// no directories, control characters or characters reserved on Windows.
func sanitizeFilename(name string) string {
	name = strings.NewReplacer("\\", "/").Replace(name)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "attachment"
	}
	return name
}

// uniqueName sanitizes name and, if it is already in used, appends " (1)",
// " (2)"... before the extension. The result is recorded in used.
func uniqueName(name string, used map[string]bool) string {
	name = sanitizeFilename(name)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	out := name
	for i := 1; used[strings.ToLower(out)]; i++ {
		out = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	used[strings.ToLower(out)] = true
	return out
}

// attachTempRoot is the per-user directory holding files opened from mail.
func attachTempRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "mailrat", "open")
}

// cleanAttachTemp removes every temp file opened by this or an earlier run.
func cleanAttachTemp() { _ = os.RemoveAll(attachTempRoot()) }

// writeTempAttachment writes data to a fresh private subdirectory of root
// under the sanitized name and returns the file path.
func writeTempAttachment(root, name string, data []byte) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, "a")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, sanitizeFilename(name))
	return p, os.WriteFile(p, data, 0o600)
}

// fileURL builds a file:// URL for an absolute local path.
func fileURL(p string) *url.URL {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows drive path
	}
	return &url.URL{Scheme: "file", Path: p}
}

// openAttachment downloads (local-first) and opens it in the OS default app.
func (u *UI) openAttachment(m desktop.MessageItem, a desktop.AttachmentItem) {
	var err error
	var path string
	u.bg(func() {
		var data desktop.AttachmentDataItem
		if data, err = u.be.GetAttachmentData(m.AccountID, m.ID, a.AttachmentID); err != nil {
			return
		}
		path, err = writeTempAttachment(attachTempRoot(), a.Name, data.Data)
	}, func() {
		if err == nil {
			err = u.app.OpenURL(fileURL(path))
		}
		if err != nil {
			u.showError(err)
		}
	})
}

// saveAllAttachments writes every attachment of m into a chosen folder.
func (u *UI) saveAllAttachments(m desktop.MessageItem) {
	var err error
	u.bg(func() {
		var dir string
		if dir, err = pickFolder("Save all attachments to"); err != nil || dir == "" {
			return
		}
		used := map[string]bool{}
		if ents, e := os.ReadDir(dir); e == nil {
			for _, en := range ents {
				used[strings.ToLower(en.Name())] = true
			}
		}
		for _, a := range m.Attachments {
			var data desktop.AttachmentDataItem
			if data, err = u.be.GetAttachmentData(m.AccountID, m.ID, a.AttachmentID); err != nil {
				return
			}
			if err = os.WriteFile(filepath.Join(dir, uniqueName(a.Name, used)), data.Data, 0o644); err != nil {
				return
			}
		}
	}, func() {
		if err != nil {
			u.showError(err)
		}
	})
}

// attachmentMenu is the right-click menu: Save, plus Save all for 2+ files.
func (u *UI) attachmentMenu(m desktop.MessageItem, a desktop.AttachmentItem, pos fyne.Position) {
	items := []*fyne.MenuItem{fyne.NewMenuItem("Save", func() { u.saveAttachment(m, a) })}
	if len(m.Attachments) >= 2 {
		items = append(items, fyne.NewMenuItem("Save all to folder…", func() { u.saveAllAttachments(m) }))
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), u.win.Canvas(), pos)
}

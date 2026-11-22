package ui

import (
	"errors"
	"path/filepath"

	"github.com/ncruces/zenity"
)

// Native OS file dialogs (NSOpenPanel/NSSavePanel via AppleScript on macOS,
// zenity/kdialog on Linux, Win32 on Windows). They block, so call them off the
// UI goroutine. A cancelled dialog returns "" with a nil error. Variables so
// tests can stub them.
var (
	pickOpenFile = func(title string, exts ...string) (string, error) {
		opts := []zenity.Option{zenity.Title(title)}
		if len(exts) > 0 {
			opts = append(opts, zenity.FileFilter{Name: "Files", Patterns: exts, CaseFold: true})
		}
		return cancelled(zenity.SelectFile(opts...))
	}
	pickSaveFile = func(title, name, dir string) (string, error) {
		opts := []zenity.Option{zenity.Title(title), zenity.ConfirmOverwrite()}
		if dir != "" {
			opts = append(opts, zenity.Filename(filepath.Join(dir, name)))
		} else {
			opts = append(opts, zenity.Filename(name))
		}
		return cancelled(zenity.SelectFileSave(opts...))
	}
)

func cancelled(path string, err error) (string, error) {
	if errors.Is(err, zenity.ErrCanceled) {
		return "", nil
	}
	return path, err
}

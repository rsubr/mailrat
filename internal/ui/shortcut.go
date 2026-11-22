package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// cmd is ⌘+key on macOS (Ctrl+key elsewhere), optionally with Shift.
func cmd(key fyne.KeyName, shift bool) *desktop.CustomShortcut {
	m := fyne.KeyModifierShortcutDefault
	if shift {
		m |= fyne.KeyModifierShift
	}
	return &desktop.CustomShortcut{KeyName: key, Modifier: m}
}

// installShortcuts binds Mimestream-style reply / forward / trash keys.
func (u *UI) installShortcuts() {
	add := func(key fyne.KeyName, shift bool, fn func()) {
		u.win.Canvas().AddShortcut(cmd(key, shift), func(fyne.Shortcut) { fn() })
	}
	add(fyne.KeyR, false, func() { u.reply(false) })
	add(fyne.KeyR, true, func() { u.reply(true) })
	add(fyne.KeyF, true, u.forward)
	add(fyne.KeyBackspace, false, u.trash)
	add(fyne.KeyComma, false, u.openSettings)
}

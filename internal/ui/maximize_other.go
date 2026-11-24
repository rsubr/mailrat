//go:build !darwin

package ui

import "fyne.io/fyne/v2"

// maximize is a no-op off macOS: Fyne has no public maximize API, so the
// window keeps its default size.
func maximize(fyne.Window) {}

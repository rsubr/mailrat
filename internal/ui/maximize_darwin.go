//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// Same as clicking the green zoom button: fill the usable screen area,
// menu bar and dock stay visible. Must run on the main thread.
static void zoomWindow(uintptr_t w) {
	NSWindow *win = (NSWindow *)w;
	if (win != nil && ![win isZoomed]) {
		[win zoom:nil];
	}
}
*/
import "C"

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// maximize zooms the window to the screen's usable area. Call on the Fyne
// main thread after the window has been shown.
func maximize(w fyne.Window) {
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		if c, ok := ctx.(driver.MacWindowContext); ok && c.NSWindow != 0 {
			C.zoomWindow(C.uintptr_t(c.NSWindow))
		}
	})
}

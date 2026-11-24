//go:build darwin

package richclip

/*
#cgo darwin LDFLAGS: -framework AppKit
#include <stdbool.h>
#include <stdlib.h>
bool richclipSet(const char *html, const char *text);
char *richclipGet(bool html);
*/
import "C"

import (
	"errors"
	"unsafe"
)

type system struct{}

// Set replaces the general pasteboard with public.html and public.utf8-plain-text.
func (system) Set(html, text string) error {
	h := C.CString("<meta charset=\"utf-8\">" + html)
	t := C.CString(text)
	defer C.free(unsafe.Pointer(h))
	defer C.free(unsafe.Pointer(t))
	if !C.richclipSet(h, t) {
		return errors.New("richclip: pasteboard write failed")
	}
	return nil
}

// ReadHTML returns the pasteboard's public.html and plain text ("" when absent).
func (system) ReadHTML() (string, string, error) {
	get := func(html bool) string {
		p := C.richclipGet(C.bool(html))
		if p == nil {
			return ""
		}
		defer C.free(unsafe.Pointer(p))
		return C.GoString(p)
	}
	return get(true), get(false), nil
}

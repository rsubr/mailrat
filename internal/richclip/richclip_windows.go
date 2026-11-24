//go:build windows

package richclip

import (
	"errors"
	"runtime"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procOpenClipboard     = user32.NewProc("OpenClipboard")
	procCloseClipboard    = user32.NewProc("CloseClipboard")
	procEmptyClipboard    = user32.NewProc("EmptyClipboard")
	procSetClipboardData  = user32.NewProc("SetClipboardData")
	procGetClipboardData  = user32.NewProc("GetClipboardData")
	procRegisterFormat    = user32.NewProc("RegisterClipboardFormatW")
	procIsFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalSize        = kernel32.NewProc("GlobalSize")
	procGlobalAlloc       = kernel32.NewProc("GlobalAlloc")
	procGlobalFree        = kernel32.NewProc("GlobalFree")
	procGlobalLock        = kernel32.NewProc("GlobalLock")
	procGlobalUnlock      = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText uintptr = 13
	gmemMoveable  uintptr = 0x2
)

type system struct{}

func htmlFormat() (uintptr, error) {
	n, err := windows.UTF16PtrFromString("HTML Format")
	if err != nil {
		return 0, err
	}
	f, _, e := procRegisterFormat.Call(uintptr(unsafe.Pointer(n)))
	if f == 0 {
		return 0, e
	}
	return f, nil
}

// open retries briefly: another program may hold the clipboard.
func open() error {
	var err error
	for i := 0; i < 10; i++ {
		if r, _, e := procOpenClipboard.Call(0); r != 0 {
			return nil
		} else {
			err = e
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}

// put hands data to the clipboard as a moveable global block, which the clipboard then owns.
func put(format uintptr, data []byte) error {
	h, _, e := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return e
	}
	p, _, e := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return e
	}
	copy(unsafe.Slice((*byte)(ptr(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	if r, _, e := procSetClipboardData.Call(format, h); r == 0 {
		procGlobalFree.Call(h)
		return e
	}
	return nil
}

// Set replaces the clipboard with "HTML Format" and CF_UNICODETEXT in one sequence.
func (system) Set(html, text string) error {
	hf, err := htmlFormat()
	if err != nil {
		return err
	}
	u := utf16.Encode([]rune(text))
	u = append(u, 0)
	ub := unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := open(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if r, _, e := procEmptyClipboard.Call(); r == 0 {
		return e
	}
	if err := put(hf, append(buildCFHTML(html), 0)); err != nil {
		return err
	}
	return put(cfUnicodeText, ub)
}

// get copies the global block behind the clipboard format; the clipboard must be open.
func get(format uintptr) []byte {
	if r, _, _ := procIsFormatAvailable.Call(format); r == 0 {
		return nil
	}
	h, _, _ := procGetClipboardData.Call(format)
	if h == 0 {
		return nil
	}
	size, _, _ := procGlobalSize.Call(h)
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 || size == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	return append([]byte(nil), unsafe.Slice((*byte)(ptr(p)), int(size))...)
}

// ReadHTML returns the HTML fragment and text on the clipboard ("" for what is absent).
func (system) ReadHTML() (string, string, error) {
	hf, err := htmlFormat()
	if err != nil {
		return "", "", err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := open(); err != nil {
		return "", "", errors.New("richclip: clipboard busy: " + err.Error())
	}
	defer procCloseClipboard.Call()
	var html, text string
	if b := get(hf); b != nil {
		html = parseCFHTML(b)
	}
	if b := get(cfUnicodeText); len(b) >= 2 {
		u := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
		for i, c := range u {
			if c == 0 {
				u = u[:i]
				break
			}
		}
		text = string(utf16.Decode(u))
	}
	return html, text, nil
}

// ptr converts a locked global block address; the memory is not Go-managed.
func ptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// Package richclip writes text/html and text/plain to the system clipboard in
// one operation. Fyne's clipboard is text-only, so rich copy needs platform
// code; where none exists System reports ErrUnsupported and callers fall back
// to plain text.
package richclip

import "errors"

// ErrUnsupported means this platform has no rich clipboard writer.
var ErrUnsupported = errors.New("richclip: rich clipboard not supported on this platform")

// Clipboard writes both representations of one copy.
type Clipboard interface {
	Set(html, text string) error
}

// Reader reads the HTML flavour of the clipboard. plain is the text flavour when
// the owner offers one; html is "" when there is no HTML. ErrUnsupported means
// this platform cannot read HTML.
type Reader interface {
	ReadHTML() (html, plain string, err error)
}

// System is the platform clipboard (macOS: NSPasteboard, Windows: CF_HTML,
// Linux: X11 selection owner; elsewhere unsupported).
func System() Clipboard { return system{} }

// SystemReader reads the platform clipboard.
func SystemReader() Reader { return system{} }

// Fake records what was copied, for tests.
type Fake struct {
	HTML, Text string
	Calls      int
	Err        error // returned by Set when non-nil
	ReadErr    error // returned by ReadHTML when non-nil
	ReadCalls  int
}

func (f *Fake) Set(html, text string) error {
	f.Calls++
	if f.Err != nil {
		return f.Err
	}
	f.HTML, f.Text = html, text
	return nil
}

// ReadHTML returns what Set stored last, so a copy/paste round trip works in tests.
func (f *Fake) ReadHTML() (string, string, error) {
	f.ReadCalls++
	if f.ReadErr != nil {
		return "", "", f.ReadErr
	}
	return f.HTML, f.Text, nil
}

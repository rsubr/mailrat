//go:build linux

package richclip

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Linux: under X11 (also XWayland, which Fyne uses by default) we own the
// CLIPBOARD selection ourselves and serve text/html and the plain-text targets
// together, since xclip and wl-copy publish one MIME type per invocation. A
// background goroutine answers requests until another client takes the
// selection. On a pure Wayland session there is no writer: Set reports
// ErrUnsupported (wl-copy offers one type per call), and callers copy plain text.

type system struct{}

const (
	readTimeout = time.Second
	maxRead     = 4 << 20 // larger HTML from another program is ignored
)

func init() { xgb.Logger.SetOutput(io.Discard) } // xgb logs auth and close noise

// ownWin is the window that owns the selection we published, 0 if none.
var ownWin atomic.Uint32

type x11 struct {
	c    *xgb.Conn
	win  xproto.Window
	atom map[string]xproto.Atom
}

func dial() (*x11, error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, ErrUnsupported
	}
	c, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	x := &x11{c: c, atom: map[string]xproto.Atom{}}
	x.win, err = xproto.NewWindowId(c)
	if err == nil {
		root := xproto.Setup(c).DefaultScreen(c).Root
		err = xproto.CreateWindowChecked(c, 0, x.win, root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0,
			xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check()
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return x, nil
}

func (x *x11) intern(name string) xproto.Atom {
	if a, ok := x.atom[name]; ok {
		return a
	}
	r, err := xproto.InternAtom(x.c, false, uint16(len(name)), name).Reply()
	if err != nil {
		return xproto.AtomNone
	}
	x.atom[name] = r.Atom
	return r.Atom
}

// next polls for an event until the deadline.
func (x *x11) next(deadline time.Time) xgb.Event {
	for time.Now().Before(deadline) {
		if ev, _ := x.c.PollForEvent(); ev != nil {
			return ev
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// timestamp fetches a server time through a property change on our window.
func (x *x11) timestamp() (xproto.Timestamp, error) {
	p := x.intern("RICHCLIP_TIME")
	xproto.ChangeProperty(x.c, xproto.PropModeAppend, x.win, p, xproto.AtomString, 8, 0, nil)
	deadline := time.Now().Add(readTimeout)
	for ev := x.next(deadline); ev != nil; ev = x.next(deadline) {
		if n, ok := ev.(xproto.PropertyNotifyEvent); ok && n.Window == x.win && n.Atom == p {
			return n.Time, nil
		}
	}
	return 0, errors.New("richclip: no X server timestamp")
}

// latin1 is the STRING target: ISO 8859-1, with '?' for other characters.
func latin1(s string) []byte {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xff {
			r = '?'
		}
		b = append(b, byte(r))
	}
	return b
}

type target struct {
	typ    xproto.Atom
	format byte
	data   []byte
}

// Set publishes html and text together as the CLIPBOARD selection.
func (system) Set(html, text string) error {
	x, err := dial()
	if err != nil {
		return err
	}
	// ChangeProperty must fit one request (no INCR on the sending side): too much data degrades to plain text.
	limit := int(xproto.Setup(x.c).MaximumRequestLength)*4 - 64
	h := []byte("<meta charset=\"utf-8\">" + html)
	if len(h) > limit || len(text) > limit {
		x.c.Close()
		return ErrUnsupported
	}
	ts, err := x.timestamp()
	if err != nil {
		x.c.Close()
		return err
	}
	utf8, plain := x.intern("UTF8_STRING"), x.intern("text/plain")
	targets := map[xproto.Atom]target{
		x.intern("text/html"):                {x.intern("text/html"), 8, h},
		utf8:                                 {utf8, 8, []byte(text)},
		xproto.AtomString:                    {xproto.AtomString, 8, latin1(text)},
		x.intern("text/plain;charset=utf-8"): {x.intern("text/plain;charset=utf-8"), 8, []byte(text)},
		plain:                                {plain, 8, []byte(text)},
	}
	list := []xproto.Atom{x.intern("TARGETS"), x.intern("TIMESTAMP"), x.intern("MULTIPLE")}
	for a := range targets {
		list = append(list, a)
	}
	var tb bytes.Buffer
	for _, a := range list {
		_ = binary.Write(&tb, binary.LittleEndian, uint32(a))
	}
	targets[x.intern("TARGETS")] = target{xproto.AtomAtom, 32, tb.Bytes()}
	var tsb [4]byte
	binary.LittleEndian.PutUint32(tsb[:], uint32(ts))
	targets[x.intern("TIMESTAMP")] = target{xproto.AtomInteger, 32, tsb[:]}

	clip := x.intern("CLIPBOARD")
	xproto.SetSelectionOwner(x.c, x.win, clip, ts)
	if r, err := xproto.GetSelectionOwner(x.c, clip).Reply(); err != nil || r.Owner != x.win {
		x.c.Close()
		return errors.New("richclip: could not take the clipboard")
	}
	ownWin.Store(uint32(x.win))
	go x.serve(clip, targets)
	return nil
}

// serve answers selection requests until another client takes the selection.
func (x *x11) serve(clip xproto.Atom, targets map[xproto.Atom]target) {
	defer x.c.Close()
	multiple := x.intern("MULTIPLE")
	convert := func(req xproto.Window, tgt, prop xproto.Atom) bool {
		t, ok := targets[tgt]
		if !ok || prop == xproto.AtomNone {
			return false
		}
		xproto.ChangeProperty(x.c, xproto.PropModeReplace, req, prop, t.typ, t.format, uint32(len(t.data))/uint32(t.format/8), t.data)
		return true
	}
	for {
		ev, err := x.c.WaitForEvent()
		if ev == nil && err == nil {
			return // connection closed
		}
		switch e := ev.(type) {
		case xproto.SelectionClearEvent:
			ownWin.CompareAndSwap(uint32(x.win), 0)
			return
		case xproto.SelectionRequestEvent:
			prop, ok := e.Property, false
			if e.Selection == clip {
				if prop == xproto.AtomNone {
					prop = e.Target // obsolete clients
				}
				if e.Target == multiple {
					ok = x.multiple(e, convert)
				} else {
					ok = convert(e.Requestor, e.Target, prop)
				}
			}
			if !ok {
				prop = xproto.AtomNone
			}
			n := xproto.SelectionNotifyEvent{Time: e.Time, Requestor: e.Requestor, Selection: e.Selection, Target: e.Target, Property: prop}
			xproto.SendEvent(x.c, false, e.Requestor, 0, string(n.Bytes()))
		}
	}
}

// multiple answers a MULTIPLE request: the property holds (target, property) pairs.
func (x *x11) multiple(e xproto.SelectionRequestEvent, convert func(xproto.Window, xproto.Atom, xproto.Atom) bool) bool {
	r, err := xproto.GetProperty(x.c, false, e.Requestor, e.Property, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	if err != nil || r.Format != 32 || len(r.Value)%8 != 0 {
		return false
	}
	for i := 0; i+8 <= len(r.Value); i += 8 {
		tgt := xproto.Atom(binary.LittleEndian.Uint32(r.Value[i:]))
		prop := xproto.Atom(binary.LittleEndian.Uint32(r.Value[i+4:]))
		if !convert(e.Requestor, tgt, prop) {
			binary.LittleEndian.PutUint32(r.Value[i+4:], uint32(xproto.AtomNone))
		}
	}
	xproto.ChangeProperty(x.c, xproto.PropModeReplace, e.Requestor, e.Property, r.Type, 32, uint32(len(r.Value)/4), r.Value)
	return true
}

// ReadHTML reads text/html and the text flavour of the CLIPBOARD selection.
func (system) ReadHTML() (string, string, error) {
	if os.Getenv("DISPLAY") == "" {
		return readWayland()
	}
	x, err := dial()
	if err != nil {
		return "", "", err
	}
	defer x.c.Close()
	clip := x.intern("CLIPBOARD")
	r, err := xproto.GetSelectionOwner(x.c, clip).Reply()
	if err != nil || r.Owner == xproto.WindowNone {
		return "", "", err
	}
	if uint32(r.Owner) != ownWin.Load() && x.ownedByUs(r.Owner) {
		// Fyne itself owns the selection, and only answers on the thread
		// that would be waiting here. It never offers HTML anyway.
		return "", "", nil
	}
	html := decodeHTML(x.fetch(clip, "text/html"))
	return html, string(x.fetch(clip, "UTF8_STRING")), nil
}

// ownedByUs reports whether w belongs to this process (its _NET_WM_PID is ours).
func (x *x11) ownedByUs(w xproto.Window) bool {
	r, err := xproto.GetProperty(x.c, false, w, x.intern("_NET_WM_PID"), xproto.AtomCardinal, 0, 1).Reply()
	return err == nil && r.Format == 32 && len(r.Value) == 4 && int(binary.LittleEndian.Uint32(r.Value)) == os.Getpid()
}

// fetch asks the selection owner for one target and returns its bytes, or nil.
func (x *x11) fetch(clip xproto.Atom, name string) []byte {
	tgt, prop := x.intern(name), x.intern("RICHCLIP_DATA")
	xproto.ConvertSelection(x.c, x.win, clip, tgt, prop, xproto.TimeCurrentTime)
	deadline := time.Now().Add(readTimeout)
	for ev := x.next(deadline); ev != nil; ev = x.next(deadline) {
		n, ok := ev.(xproto.SelectionNotifyEvent)
		if !ok || n.Requestor != x.win {
			continue
		}
		if n.Property == xproto.AtomNone {
			return nil
		}
		typ, data := x.take(prop)
		if typ == x.intern("INCR") {
			return x.incr(prop, deadline)
		}
		return data
	}
	return nil
}

// take reads and deletes a property of our window.
func (x *x11) take(prop xproto.Atom) (xproto.Atom, []byte) {
	var data []byte
	var typ xproto.Atom
	for off := uint32(0); ; {
		r, err := xproto.GetProperty(x.c, false, x.win, prop, xproto.GetPropertyTypeAny, off, 1<<18).Reply()
		if err != nil {
			break
		}
		typ = r.Type
		data = append(data, r.Value...)
		off += uint32(len(r.Value)) / 4
		if r.BytesAfter == 0 || len(data) > maxRead {
			break
		}
	}
	xproto.DeleteProperty(x.c, x.win, prop)
	return typ, data
}

// incr receives a chunked transfer; it gives up past maxRead.
func (x *x11) incr(prop xproto.Atom, deadline time.Time) []byte {
	var data []byte
	for ev := x.next(deadline); ev != nil; ev = x.next(deadline) {
		n, ok := ev.(xproto.PropertyNotifyEvent)
		if !ok || n.Window != x.win || n.Atom != prop || n.State != xproto.PropertyNewValue {
			continue
		}
		_, chunk := x.take(prop)
		if len(chunk) == 0 {
			return data
		}
		if data = append(data, chunk...); len(data) > maxRead {
			return nil
		}
		deadline = time.Now().Add(readTimeout)
	}
	return nil
}

// readWayland uses wl-paste on a Wayland-only session.
func readWayland() (string, string, error) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return "", "", ErrUnsupported
	}
	path, err := exec.LookPath("wl-paste")
	if err != nil {
		return "", "", ErrUnsupported
	}
	paste := func(args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, path, append([]string{"--no-newline"}, args...)...).Output()
		if err != nil || len(out) > maxRead {
			return ""
		}
		return string(out)
	}
	return decodeHTML([]byte(paste("--type", "text/html"))), paste("--type", "text/plain"), nil
}

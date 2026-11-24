package ui

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"

	"github.com/mailrat/mailrat/internal/desktop"
)

// pickOpenFiles is the native multi-file chooser; a variable so tests can stub
// it. A cancelled dialog returns no paths and a nil error.
var pickOpenFiles = func(title string) ([]string, error) {
	paths, err := zenity.SelectFileMultiple(zenity.Title(title))
	if errors.Is(err, zenity.ErrCanceled) {
		return nil, nil
	}
	return paths, err
}

// setDropHandler registers fn for files dropped on w; a variable so tests can
// capture the handler (the test driver ignores it).
var setDropHandler = func(w fyne.Window, fn func(fyne.Position, []fyne.URI)) { w.SetOnDropped(fn) }

const (
	attachChipW = 300 // fixed chip width: Gmail-style tiles that wrap
	attachChipH = 38
)

// attachBar lists the files attached to one compose window as removable chips,
// with a "N files, X MB of 25 MB" total.
type attachBar struct {
	u        *UI
	w        fyne.Window
	atts     []desktop.OutAttachment
	bodyLen  func() int     // size of the message body, for the total limit
	fallback fyne.Focusable // focused after the last chip is removed
	chips    *fyne.Container
	scroll   *container.Scroll
	total    *widget.Label
	root     *fyne.Container
}

func newAttachBar(u *UI, w fyne.Window, initial []desktop.OutAttachment, bodyLen func() int, fallback fyne.Focusable) *attachBar {
	b := &attachBar{u: u, w: w, atts: slices.Clone(initial), bodyLen: bodyLen, fallback: fallback}
	b.chips = container.NewGridWrap(fyne.NewSize(attachChipW, attachChipH))
	b.scroll = container.NewVScroll(b.chips)
	b.total = widget.NewLabel("")
	b.total.Importance = widget.LowImportance
	b.root = container.NewVBox(b.scroll, b.total)
	b.rebuild(-1)
	return b
}

// rebuild redraws the chips; focus, when >= 0, is the index of the chip to focus.
func (b *attachBar) rebuild(focus int) {
	b.chips.Objects = nil
	for i, a := range b.atts {
		b.chips.Add(newAttachChip(b, i, a))
	}
	b.chips.Refresh()
	n := len(b.atts)
	if n == 0 {
		b.root.Hide()
		if c := b.w.Content(); c != nil {
			c.Refresh()
		}
		return
	}
	rows := 1
	if n > 3 {
		rows = 2
	}
	b.scroll.SetMinSize(fyne.NewSize(0, float32(rows*(attachChipH+8))))
	var sum int64
	for _, a := range b.atts {
		sum += a.Size
	}
	files := "files"
	if n == 1 {
		files = "file"
	}
	b.total.SetText(fmt.Sprintf("%d %s, %s of %d MB", n, files, humanSize(sum), desktop.MaxAttachmentBytes>>20))
	b.root.Show()
	if c := b.w.Content(); c != nil {
		c.Refresh() // the bar's height changed: re-lay out the window
	}
	if focus >= 0 {
		b.w.Canvas().Focus(b.chips.Objects[min(focus, n-1)].(*attachChip))
	}
}

// remove drops attachment i and keeps keyboard focus on a neighbouring chip.
func (b *attachBar) remove(i int) {
	if i < 0 || i >= len(b.atts) {
		return
	}
	b.atts = slices.Delete(slices.Clone(b.atts), i, i+1)
	if len(b.atts) == 0 {
		b.rebuild(-1)
		b.w.Canvas().Focus(b.fallback)
		return
	}
	b.rebuild(min(i, len(b.atts)-1))
}

// pick opens the native file chooser and attaches the chosen files.
func (b *attachBar) pick() {
	var paths []string
	var err error
	b.u.bg(func() { paths, err = pickOpenFiles("Attach files") }, func() {
		if err != nil {
			b.u.showErrorIn(b.w, err)
			return
		}
		b.add(paths)
	})
}

// drop attaches files dropped on the window; non-file URIs are ignored.
func (b *attachBar) drop(_ fyne.Position, uris []fyne.URI) {
	var paths []string
	for _, u := range uris {
		if u.Scheme() == "file" {
			paths = append(paths, u.Path())
		}
	}
	b.add(paths)
}

// add validates and attaches each path. Folders, unreadable files and files
// that would push the message over the size limit are skipped; one error
// dialog lists every skipped file.
func (b *attachBar) add(paths []string) {
	if len(paths) == 0 {
		return
	}
	got := make([]desktop.OutAttachment, len(paths))
	errs := make([]error, len(paths))
	b.u.bg(func() {
		for i, p := range paths {
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				errs[i] = errors.New("is a folder; attach files, not folders")
			} else {
				got[i], errs[i] = desktop.StatAttachment(p)
			}
		}
	}, func() {
		var msgs []string
		next := slices.Clone(b.atts)
		for i, p := range paths {
			if errs[i] == nil {
				if errs[i] = desktop.CheckAttachments(append(slices.Clone(next), got[i]), b.bodyLen()); errs[i] == nil {
					next = append(next, got[i])
					continue
				}
			}
			msgs = append(msgs, fmt.Sprintf("%s: %v", filepath.Base(p), errs[i]))
		}
		if len(next) > len(b.atts) {
			b.atts = next
			b.rebuild(-1)
		}
		if len(msgs) > 0 {
			b.u.showErrorIn(b.w, errors.New(strings.Join(msgs, "\n")))
		}
	})
}

func attachIconName(a desktop.OutAttachment) string {
	if strings.HasPrefix(a.MIMEType, "image/") {
		return "image"
	}
	return "file"
}

// attachChip is one attachment tile: icon, name, size and a remove button. It
// takes keyboard focus; Delete or Backspace removes it.
type attachChip struct {
	widget.BaseWidget
	bar     *attachBar
	idx     int
	ring    *canvas.Rectangle
	content fyne.CanvasObject
}

func newAttachChip(b *attachBar, idx int, a desktop.OutAttachment) *attachChip {
	c := &attachChip{bar: b, idx: idx}
	name := widget.NewLabel(a.Filename)
	name.Truncation = fyne.TextTruncateEllipsis
	size := widget.NewLabel(humanSize(a.Size))
	size.Importance = widget.LowImportance
	rm := newTooltipButton(icon("close", fg()), "Remove "+a.Filename, func() { b.remove(idx) })
	rm.Importance = widget.LowImportance
	c.ring = canvas.NewRectangle(color.Transparent)
	c.ring.CornerRadius = 8
	c.ring.StrokeWidth = 1
	c.ring.StrokeColor = col(theme.ColorNameSeparator)
	c.content = container.NewStack(c.ring, container.NewBorder(nil, nil,
		container.NewCenter(widget.NewIcon(icon(attachIconName(a), fg()))),
		container.NewHBox(size, rm), name))
	c.ExtendBaseWidget(c)
	return c
}

func (c *attachChip) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(c.content) }

func (c *attachChip) Tapped(*fyne.PointEvent) {
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c); cv != nil {
		cv.Focus(c)
	}
}

func (c *attachChip) FocusGained() {
	c.ring.StrokeColor = col(theme.ColorNamePrimary)
	c.ring.StrokeWidth = 2
	c.ring.Refresh()
}

func (c *attachChip) FocusLost() {
	c.ring.StrokeColor = col(theme.ColorNameSeparator)
	c.ring.StrokeWidth = 1
	c.ring.Refresh()
}

func (c *attachChip) TypedRune(rune) {}

func (c *attachChip) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyDelete || k.Name == fyne.KeyBackspace {
		c.bar.remove(c.idx)
	}
}

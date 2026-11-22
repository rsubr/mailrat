package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

var composeFonts = []string{"Sans Serif", "Serif", "Fixed Width", "Arial", "Georgia", "Tahoma", "Trebuchet MS", "Verdana", "Garamond", "Comic Sans MS"}
var composeSizes = []string{"small", "normal", "large", "huge"}
var composeColors = []struct{ name, hex string }{
	{"Black", "#000000"}, {"Grey", "#666666"}, {"Red", "#cc0000"}, {"Orange", "#e69138"},
	{"Green", "#38761d"}, {"Blue", "#1155cc"}, {"Purple", "#674ea7"},
}

// richEditor is the Gmail-style body editor: a formatting toolbar over the body entry,
// with an Edit/Preview switch (the entry holds markup, see richtext.go) and @/+ mentions.
func (u *UI) richEditor(w fyne.Window, body *bodyEntry, mention *suggestHost, to, cc *suggestEntry, account func() string) fyne.CanvasObject {
	apply := func(open, close string) func() {
		return func() { body.wrap(open, close); w.Canvas().Focus(body) }
	}
	menu := func(label string, items map[string]string, keys []string, open func(v string) (string, string)) *widget.Button {
		var b *widget.Button
		b = widget.NewButton(label, func() {
			var mi []*fyne.MenuItem
			for _, k := range keys {
				v := items[k]
				mi = append(mi, fyne.NewMenuItem(k, func() { o, c := open(v); apply(o, c)() }))
			}
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
			widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", mi...), w.Canvas(), pos.Add(fyne.NewPos(0, b.Size().Height)))
		})
		b.Importance = widget.LowImportance
		return b
	}
	btn := func(label string, fn func()) *widget.Button {
		b := widget.NewButton(label, fn)
		b.Importance = widget.LowImportance
		return b
	}

	font := widget.NewSelect(composeFonts, nil)
	font.PlaceHolder = "Font"
	font.OnChanged = func(f string) {
		if f != "" {
			apply("[font="+f+"]", "[/font]")()
			font.ClearSelected()
		}
	}
	size := widget.NewSelect(composeSizes, nil)
	size.PlaceHolder = "Size"
	size.OnChanged = func(s string) {
		if s != "" {
			apply("[size="+s+"]", "[/size]")()
			size.ClearSelected()
		}
	}
	colors := map[string]string{}
	var colorNames []string
	for _, c := range composeColors {
		colors[c.name] = c.hex
		colorNames = append(colorNames, c.name)
	}
	aligns := map[string]string{"Align left": "left", "Align center": "center", "Align right": "right", "Justify": "justify"}

	link := btn("Link", func() {
		url := widget.NewEntry()
		url.SetPlaceHolder("https://")
		dialog.ShowForm("Insert link", "Insert", "Cancel", []*widget.FormItem{widget.NewFormItem("URL", url)}, func(ok bool) {
			if !ok || url.Text == "" {
				return
			}
			if body.SelectedText() == "" {
				body.insert("[url=" + url.Text + "]" + url.Text + "[/url]")
			} else {
				body.wrap("[url="+url.Text+"]", "[/url]")
			}
			w.Canvas().Focus(body)
		}, w)
	})
	bar := container.NewHScroll(container.NewHBox(
		font, size,
		btn("B", apply("[b]", "[/b]")), btn("I", apply("[i]", "[/i]")), btn("U", apply("[u]", "[/u]")),
		menu("Color", colors, colorNames, func(v string) (string, string) { return "[color=" + v + "]", "[/color]" }),
		menu("Align", aligns, []string{"Align left", "Align center", "Align right", "Justify"}, func(v string) (string, string) { return "[align=" + v + "]", "[/align]" }),
		btn("1.", func() { body.prefixLines("1. "); w.Canvas().Focus(body) }),
		btn("•", func() { body.prefixLines("- "); w.Canvas().Focus(body) }),
		btn("Indent", apply("[indent]", "[/indent]")),
		btn("Quote", apply("[quote]", "[/quote]")),
		link,
		btn("Clear", func() { body.clearFormatting(); w.Canvas().Focus(body) }),
	))

	// Edit / Preview: the preview renders the HTML that will be sent.
	preview := container.NewVScroll(widget.NewRichText())
	preview.Hide()
	toggle := widget.NewButtonWithIcon("Preview", theme.VisibilityIcon(), nil)
	toggle.Importance = widget.LowImportance
	toggle.OnTapped = func() {
		if preview.Visible() {
			preview.Hide()
			body.Show()
			toggle.SetText("Preview")
			w.Canvas().Focus(body)
			return
		}
		res := renderMail(markupToHTML(body.Text), renderOpts{bg: col(theme.ColorNameBackground), fg: fg()})
		rt := widget.NewRichText(res.segments...)
		rt.Wrapping = fyne.TextWrapWord
		preview.Content = rt
		preview.Refresh()
		body.Hide()
		preview.Show()
		toggle.SetText("Edit")
	}

	// @ / + mentions: insert a mailto link and make sure the person is a recipient.
	sg := body.sg
	sg.pick = func(c desktop.ContactItem) {
		if q, ok := mentionAt([]rune(body.Text), body.CursorTextOffset()); ok {
			for range q + "@" {
				body.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
			}
		}
		body.insert(mentionMarkup(c))
		if t, added := addRecipient(to.Text, cc.Text, c); added {
			to.setQuietly(t)
		}
	}
	body.OnChanged = func(string) {
		sg.hide()
		q, ok := mentionAt([]rune(body.Text), body.CursorTextOffset())
		if !ok {
			return
		}
		var found []desktop.ContactItem
		u.bg(func() { found, _ = u.be.SearchContacts(account(), q, 8) }, func() {
			if q2, ok := mentionAt([]rune(body.Text), body.CursorTextOffset()); ok && q2 == q {
				sg.show(found)
			}
		})
	}

	top := container.NewBorder(nil, nil, nil, toggle, bar)
	return container.NewBorder(container.NewVBox(top, mention.box), nil, nil, nil, container.NewStack(body, preview))
}

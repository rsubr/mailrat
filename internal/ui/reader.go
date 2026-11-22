package ui

import (
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
)

func (u *UI) refreshReader() {
	if u.reader == nil {
		return
	}
	u.reader.Objects = []fyne.CanvasObject{u.buildReader()}
	u.reader.Refresh()
}

func (u *UI) buildReader() fyne.CanvasObject {
	return container.NewBorder(u.toolbar(), nil, nil, nil, u.conversation())
}

// toolbar mirrors Mimestream's: archive, trash, mark unread, | spam, reply,
// reply all, forward — with the search field pinned right.
func (u *UI) toolbar() fyne.CanvasObject {
	btns := container.NewHBox(
		iconButton("archive", "Archive", u.archive),
		iconButton("trash", "Trash", u.trash),
		iconButton("unread", "Mark unread", u.markUnread),
		iconButton("spam", "Spam", u.spam),
		iconButton("reply", "Reply", func() { u.reply(false) }),
		iconButton("replyall", "Reply all", func() { u.reply(true) }),
		iconButton("forward", "Forward", u.forward),
	)
	if u.selected == nil {
		for _, o := range btns.Objects {
			o.(*widget.Button).Disable()
		}
	}
	u.search.SetPlaceHolder("Search")
	u.search.ActionItem = widget.NewIcon(icon("search", subtle()))
	u.search.OnSubmitted = u.runSearch
	u.search.OnChanged = func(s string) {
		if s == "" && u.query != "" {
			u.runSearch("")
		}
	}
	field := container.New(layout.NewGridWrapLayout(fyne.NewSize(250, u.search.MinSize().Height)), u.search)
	return container.NewVBox(container.NewPadded(container.NewBorder(nil, nil, btns, field)), hline())
}

func (u *UI) conversation() fyne.CanvasObject {
	if u.selected == nil {
		t := canvas.NewText("No Conversation Selected", subtle())
		t.TextSize = 18
		t.TextStyle = fyne.TextStyle{Bold: true}
		return container.NewCenter(t)
	}
	title := widget.NewRichText(&widget.TextSegment{
		Text: u.selected.Subject,
		Style: widget.RichTextStyle{
			SizeName: theme.SizeNameHeadingText, TextStyle: fyne.TextStyle{Bold: true},
			ColorName: theme.ColorNameForeground, Inline: false,
		},
	})
	title.Wrapping = fyne.TextWrapWord
	head := []fyne.CanvasObject{title}
	for _, id := range u.selected.LabelIDs {
		if l, ok := u.findLabel(id); ok && l.Type == "user" {
			head = append(head, chip(l.Name, col(colorChip), fg()))
		}
	}
	titleRow := container.NewBorder(nil, nil, nil, container.NewHBox(head[1:]...), head[0])

	cards := []fyne.CanvasObject{container.New(layout.NewCustomPaddedLayout(8, 8, 8, 8), titleRow)}
	for i, m := range u.messages {
		cards = append(cards, u.messageCard(m, i == len(u.messages)-1))
	}
	return container.NewVScroll(container.NewVBox(cards...))
}

// expanded reports whether a message shows its body. The newest message and
// any unread message open by default; clicking the header toggles.
func (u *UI) expanded(m desktop.MessageItem, last bool) bool {
	def := last || m.IsUnread
	if u.collapsed[m.ID] {
		return !def
	}
	return def
}

func (u *UI) messageCard(m desktop.MessageItem, last bool) fyne.CanvasObject {
	open := u.expanded(m, last)
	name := displayName(m.FromAddr)

	nameL := widget.NewRichText(&widget.TextSegment{
		Text:  name,
		Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}, ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameSubHeadingText, Inline: true},
	})
	nameL.Truncation = fyne.TextTruncateEllipsis
	when := widget.NewLabel(formatMessageDate(m.Date, time.Now()))
	when.Importance = widget.LowImportance

	var sub fyne.CanvasObject
	if open {
		to := widget.NewLabel("To:  " + displayRecipients(m.ToAddr))
		to.Importance = widget.LowImportance
		to.Truncation = fyne.TextTruncateEllipsis
		sub = to
	} else {
		sn := widget.NewLabel(m.Snippet)
		sn.Truncation = fyne.TextTruncateEllipsis
		sub = sn
	}
	header := container.NewBorder(nil, nil, container.NewVBox(avatar(initials(name), 40, avatarColor(m.FromAddr))), nil,
		container.NewVBox(container.NewBorder(nil, nil, nil, when, nameL), sub))
	tap := newTapRow(header, false, func() {
		u.collapsed[m.ID] = !u.collapsed[m.ID]
		u.refreshReader()
	})
	tap.radius = 0

	parts := []fyne.CanvasObject{tap}
	if open {
		parts = append(parts, u.messageBody(m))
	}
	parts = append(parts, hline())
	return container.NewVBox(parts...)
}

func displayRecipients(s string) string {
	var out []string
	for _, p := range splitAddrs(s) {
		out = append(out, displayName(p))
	}
	return joinComma(out)
}

func (u *UI) messageBody(m desktop.MessageItem) fyne.CanvasObject {
	text := htmlToText(m.Body)
	if text == "" {
		text = m.Snippet
	}
	body := widget.NewLabel(text)
	body.Wrapping = fyne.TextWrapWord
	body.Selectable = true
	items := []fyne.CanvasObject{container.New(layout.NewCustomPaddedLayout(4, 8, 52, 8), body)}
	if len(m.Attachments) > 0 {
		var chips []fyne.CanvasObject
		for _, a := range m.Attachments {
			a := a
			b := widget.NewButtonWithIcon(a.Name+"  "+humanSize(a.Size), icon("clip", fg()), func() { u.saveAttachment(m, a) })
			chips = append(chips, b)
		}
		items = append(items, container.New(layout.NewCustomPaddedLayout(0, 10, 52, 8), container.NewHBox(chips...)))
	}
	return container.NewVBox(items...)
}

func (u *UI) saveAttachment(m desktop.MessageItem, a desktop.AttachmentItem) {
	var data desktop.AttachmentDataItem
	var err error
	u.bg(func() { data, err = u.be.GetAttachmentData(m.AccountID, m.ID, a.AttachmentID) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
			if err != nil {
				u.showError(err)
				return
			}
			if w == nil {
				return
			}
			defer w.Close()
			if _, err := w.Write(data.Data); err != nil {
				u.showError(err)
			}
		}, u.win)
		d.SetFileName(a.Name)
		if home, err := os.UserHomeDir(); err == nil {
			if l, err := storage.ListerForURI(storage.NewFileURI(filepath.Join(home, "Downloads"))); err == nil {
				d.SetLocation(l)
			}
		}
		d.Show()
	})
}

// openThread loads a conversation, then fills in full bodies and attachments.
func (u *UI) openThread(t desktop.ThreadItem) {
	u.threadGen++
	gen := u.threadGen
	if u.readTimer != nil {
		u.readTimer.Stop()
	}
	u.selected = &t
	u.messages = nil
	u.collapsed = map[string]bool{}
	u.refreshReader()

	var msgs []desktop.MessageItem
	var err error
	u.bg(func() {
		msgs, err = u.be.GetThread(t.AccountID, t.ID)
		for i, m := range msgs {
			if m.Body == "" || m.HasAttachments {
				if full, e := u.be.GetMessage(m.AccountID, m.ID); e == nil {
					msgs[i] = full
				}
			}
		}
	}, func() {
		if gen != u.threadGen {
			return
		}
		if err != nil {
			u.showError(err)
			return
		}
		u.messages = msgs
		u.refreshReader()
		for _, m := range msgs {
			if m.IsUnread {
				u.scheduleMarkRead(t, gen)
				break
			}
		}
	})
}

// scheduleMarkRead marks the thread read after it has stayed open a few seconds.
func (u *UI) scheduleMarkRead(t desktop.ThreadItem, gen int) {
	if u.inline {
		return
	}
	u.readTimer = time.AfterFunc(markReadWait, func() {
		fyne.Do(func() {
			if gen != u.threadGen {
				return
			}
			msgs := append([]desktop.MessageItem(nil), u.messages...)
			u.bg(func() {
				for _, m := range msgs {
					if m.IsUnread {
						_ = u.be.MarkMessageRead(m.AccountID, m.ID)
					}
				}
			}, func() {
				for i := range u.messages {
					u.messages[i].IsUnread = false
				}
				u.setThreadUnread(t.ID, t.AccountID, false)
				u.reloadLabels()
			})
		})
	})
}

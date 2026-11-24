package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/richclip"
)

func (u *UI) refreshReader() {
	if u.reader == nil {
		return
	}
	prev := u.convScroll
	u.reader.Objects = []fyne.CanvasObject{u.buildReader()}
	if prev != nil && u.convScroll != nil { // rebuilt in place (expand, images): stay where the user was
		u.convScroll.Offset = prev.Offset
	}
	u.reader.Refresh()
}

func (u *UI) buildReader() fyne.CanvasObject {
	u.convScroll = nil
	return container.NewBorder(u.toolbar(), nil, nil, nil, u.conversation())
}

// toolbar mirrors Mimestream's: archive, trash, mark unread, | spam —
// with the search field pinned right.
func (u *UI) toolbar() fyne.CanvasObject {
	var buttons []*tooltipButton
	btn := func(name, tip string, fn func()) fyne.CanvasObject {
		b := iconButton(name, tip, fn)
		buttons = append(buttons, b)
		return b
	}
	gap := func() fyne.CanvasObject { return spacer(10, 1) }
	btns := container.NewHBox(
		btn("archive", "Archive", u.archive),
		btn("trash", "Trash", u.trash),
		btn("unread", "Mark unread", u.markUnread),
		gap(),
		btn("spam", "Spam", u.spam),
		gap(),
		u.starButton(),
		btn("tag", "Labels", u.labelPicker),
	)
	buttons = append(buttons, u.starBtn)
	if u.selected == nil {
		for _, b := range buttons {
			b.Disable()
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
	toggle := u.mailToggle() // the mail-view light/dark switch is right-most; it takes its width from the field so the pane's minimum width is unchanged
	field := container.New(layout.NewGridWrapLayout(fyne.NewSize(250-toggle.MinSize().Width-theme.Padding(), u.search.MinSize().Height)), u.search)
	right := container.NewHBox(field, toggle)
	return container.NewVBox(container.NewPadded(container.NewBorder(nil, nil, btns, right)), hline())
}

func (u *UI) conversation() fyne.CanvasObject {
	if len(u.accounts) == 0 {
		return u.welcome()
	}
	if u.selected == nil {
		t := canvas.NewText("No Conversation Selected", subtle())
		t.TextSize = 18
		t.TextStyle = fyne.TextStyle{Bold: true}
		return container.NewCenter(t)
	}
	var chips []fyne.CanvasObject
	for _, id := range u.selected.LabelIDs {
		id := id
		if l, ok := u.findLabel(id); ok && (l.Type == "user" || id == "INBOX") {
			bg, fgc := labelColors(l.BackgroundColor, l.TextColor)
			chips = append(chips, labelChip(l.Name, bg, fgc, func() { u.selectLabel(id) }, func() { u.removeLabel(id) }))
		}
	}
	wrapW := u.reader.Size().Width - 16
	if wrapW <= 16 {
		wrapW = 600
	}
	titleRow := subjectFlow(u.selected.Subject, 24, chips, wrapW)

	cards := []fyne.CanvasObject{container.New(layout.NewCustomPaddedLayout(8, 8, 8, 8), titleRow)}
	if u.threadLoading {
		cards = append(cards, widget.NewProgressBarInfinite())
	}
	u.msgCards = nil
	// Gmail folds a long thread: first and the last two stay, the middle hides
	// behind a count bar until clicked.
	n := len(u.messages)
	hideFrom, hideTo := 1, n-2
	if n <= 3 || u.collapsed[allKey] {
		hideFrom, hideTo = 0, 0
	}
	for i, m := range u.messages {
		if i >= hideFrom && i < hideTo {
			if i == hideFrom {
				cards = append(cards, u.hiddenBar(hideTo-hideFrom))
			}
			u.msgCards = append(u.msgCards, cards[len(cards)-1])
			continue
		}
		card := u.messageCard(m, i == n-1, n > 1 && i == u.msgCursor)
		u.msgCards = append(u.msgCards, card)
		cards = append(cards, card)
	}
	cards = append(cards, u.replyBar())
	// Wide content (tables, images, attachments) scrolls inside the pane; the
	// scroll reports no minimum width so it can never push the split wider.
	u.convScroll = container.NewScroll(container.NewVBox(cards...))
	return container.New(shrinkable{}, u.convScroll)
}

// allKey in u.collapsed records that the user expanded a thread's hidden middle.
const allKey = "*all"

// hiddenBar is the count badge and rule standing in for hidden messages.
func (u *UI) hiddenBar(n int) fyne.CanvasObject {
	r := canvas.NewRectangle(col(colorChip))
	r.CornerRadius = 12
	t := canvas.NewText(strconv.Itoa(n), fg())
	t.TextSize, t.TextStyle = 13, fyne.TextStyle{Bold: true}
	badge := container.NewStack(r, container.New(layout.NewCustomPaddedLayout(2, 2, 9, 9), t))
	row := container.NewBorder(nil, nil, container.New(layout.NewCustomPaddedLayout(6, 6, 8, 12), container.NewCenter(badge)), nil, container.NewCenter(hline()))
	tap := newTapRow(row, false, func() {
		u.collapsed[allKey] = true
		u.refreshReader()
	})
	tap.radius = 0
	return tap
}

// removeLabel takes a label off the open thread.
func (u *UI) removeLabel(id string) {
	if u.selected == nil {
		return
	}
	acct, tid := u.selected.AccountID, u.selected.ID
	var err error
	u.bg(func() { err = u.be.SetThreadLabel(acct, tid, id, false) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		drop := func(ids []string) []string {
			out := ids[:0:0]
			for _, x := range ids {
				if x != id {
					out = append(out, x)
				}
			}
			return out
		}
		for i := range u.threads {
			if u.threads[i].ID == tid && u.threads[i].AccountID == acct {
				u.threads[i].LabelIDs = drop(u.threads[i].LabelIDs)
			}
		}
		if u.selected != nil && u.selected.ID == tid && u.selected.AccountID == acct {
			u.selected.LabelIDs = drop(u.selected.LabelIDs)
		}
		u.list.Refresh()
		u.refreshReader()
		u.reloadLabels()
	})
}

// isDraftThread reports whether the open conversation is a saved draft.
func (u *UI) isDraftThread() bool {
	if u.selected == nil {
		return false
	}
	for _, l := range u.selected.LabelIDs {
		if l == "DRAFT" {
			return true
		}
	}
	return false
}

// editDraft reopens the conversation's last message (the draft) in a compose window.
func (u *UI) editDraft() {
	m, ok := u.lastMessage()
	if !ok || !u.isDraftThread() {
		return
	}
	var d desktop.OutboundMsg
	var err error
	u.bg(func() { d, err = u.be.OpenDraft(m.AccountID, m.ID) }, func() {
		if err != nil {
			u.showError(err)
			return
		}
		u.compose(d)
	})
}

// replyBar is Gmail's Reply / Reply all / Forward row under the conversation.
func (u *UI) replyBar() fyne.CanvasObject {
	btn := func(label, ic string, fn func()) *widget.Button {
		b := widget.NewButtonWithIcon(label, icon(ic, fg()), fn)
		b.Importance = widget.MediumImportance
		return b
	}
	if u.isDraftThread() {
		return container.New(layout.NewCustomPaddedLayout(12, 16, 52, 8),
			container.NewHBox(btn("Edit draft", "compose", u.editDraft)))
	}
	row := container.NewHBox(
		btn("Reply", "reply", func() { u.reply(false) }),
		btn("Reply all", "replyall", func() { u.reply(true) }),
		btn("Forward", "forward", u.forward),
	)
	return container.New(layout.NewCustomPaddedLayout(12, 16, 52, 8), row)
}

// scrollToMessage brings message i to the top of the conversation pane.
func (u *UI) scrollToMessage(i int) {
	if u.convScroll == nil || i < 0 || i >= len(u.msgCards) {
		return
	}
	u.convScroll.ScrollToOffset(fyne.NewPos(0, u.msgCards[i].Position().Y))
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

func (u *UI) messageCard(m desktop.MessageItem, last, cursor bool) fyne.CanvasObject {
	open := u.expanded(m, last)
	name := displayName(m.FromAddr)

	nameL := widget.NewRichText(&widget.TextSegment{
		Text:  name,
		Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}, ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameSubHeadingText, Inline: true},
	})
	nameL.Truncation = fyne.TextTruncateEllipsis
	when := widget.NewLabel(formatMessageDate(m.Date, now()))
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
	var right fyne.CanvasObject = when
	if open {
		reply := iconButton("reply", "Reply", func() { u.compose(replyDraft(m, false, u.ownAddrs()...)) })
		right = container.NewHBox(when, reply)
	}
	header := container.NewBorder(nil, nil, container.NewVBox(avatar(initials(name), 40, avatarColor(m.FromAddr))), nil,
		container.NewVBox(container.NewBorder(nil, nil, nil, right, nameL), sub))
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
	card := container.NewVBox(parts...)
	if !cursor {
		return card
	}
	// Gmail's message cursor: an accent bar down the left edge.
	bar := canvas.NewRectangle(col(theme.ColorNamePrimary))
	bar.SetMinSize(fyne.NewSize(3, 1))
	return container.NewBorder(nil, nil, bar, nil, card)
}

func displayRecipients(s string) string {
	var out []string
	for _, p := range splitAddrs(s) {
		out = append(out, displayName(p))
	}
	return joinComma(out)
}

func (u *UI) messageBody(m desktop.MessageItem) fyne.CanvasObject {
	src := m.Body
	if strings.TrimSpace(htmlToText(src)) == "" {
		src = m.Snippet
	}
	th := newMailTheme(u.mailLight())
	text := th.Color(theme.ColorNameForeground, theme.VariantLight)
	res := mailRender(src, th, u.images[m.ID])
	body := u.newBody(res.segments)
	body.th = th
	body.setBands(res.bands)
	items := []fyne.CanvasObject{}
	if res.blocked > 0 {
		load := widget.NewButtonWithIcon(fmt.Sprintf("Load %d remote image(s)", res.blocked), icon("image", text), func() {
			u.images[m.ID] = true
			u.refreshReader()
		})
		load.Importance = widget.LowImportance
		items = append(items, container.New(layout.NewCustomPaddedLayout(2, 2, 52, 8), container.NewHBox(load)))
	}
	items = append(items, container.New(layout.NewCustomPaddedLayout(4, 8, 52, 8), body))
	if len(res.quoted) > 0 {
		key := "q:" + m.ID
		more := widget.NewButton("…", func() {
			u.collapsed[key] = !u.collapsed[key]
			u.refreshReader()
		})
		items = append(items, container.New(layout.NewCustomPaddedLayout(0, 4, 52, 8), container.NewHBox(more)))
		if u.collapsed[key] {
			q := widget.NewRichText(res.quoted...)
			q.Wrapping = fyne.TextWrapWord
			items = append(items, container.New(layout.NewCustomPaddedLayout(0, 8, 52, 8), q))
		}
	}
	if len(m.Attachments) > 0 {
		var cards []fyne.CanvasObject
		for _, a := range m.Attachments {
			a := a
			cards = append(cards, u.attachmentCard(a, th, func() { u.openAttachment(m, a) }, func(p fyne.Position) { u.attachmentMenu(m, a, p) }))
		}
		items = append(items, container.New(layout.NewCustomPaddedLayout(0, 10, 52, 8), container.NewHBox(cards...)))
	}
	return mailSurface(th, container.NewVBox(items...))
}

// richClipboard writes HTML + plain text copies; tests swap in a fake.
var richClipboard richclip.Clipboard = richclip.System()

// selOwner is the body holding the current text selection; starting another clears it.
var selOwner *selBody

// newBody is a selectable message body whose copies carry text/html and text/plain.
func (u *UI) newBody(segs []widget.RichTextSegment) *selBody {
	b := newSelBody(segs)
	b.copyFn = func(h, t string) {
		if err := richClipboard.Set(h, t); err != nil {
			u.app.Clipboard().SetContent(t) // no rich clipboard on this platform
		}
	}
	b.active = func(n *selBody) {
		if selOwner != nil && selOwner != n {
			selOwner.setSel(0, 0)
		}
		selOwner = n
	}
	b.key, b.rune = u.typedKey, u.typedRune
	return b
}

// attachmentIcon picks the card icon: image for pictures, a document otherwise.
func attachmentIcon(a desktop.AttachmentItem) string {
	if strings.HasPrefix(a.MimeType, "image/") {
		return "image"
	}
	return "file"
}

// attachmentCard is Gmail's bordered tile: icon, filename and size; tap opens, right-click saves.
func (u *UI) attachmentCard(a desktop.AttachmentItem, th mailTheme, onTap func(), onMenu func(fyne.Position)) fyne.CanvasObject {
	name := widget.NewLabel(a.Name)
	name.Truncation = fyne.TextTruncateEllipsis
	size := widget.NewLabel(humanSize(a.Size))
	size.Importance = widget.LowImportance
	ic := widget.NewIcon(icon(attachmentIcon(a), th.Color(theme.ColorNameForeground, theme.VariantLight)))
	text := container.NewVBox(name, size)
	inner := container.NewBorder(nil, nil, container.NewCenter(ic), nil, text)
	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 8
	border.StrokeWidth = 1
	border.StrokeColor = th.Color(theme.ColorNameSeparator, theme.VariantLight)
	tap := newTapRow(container.New(layout.NewCustomPaddedLayout(2, 2, 8, 8), inner), false, onTap)
	tap.onSecondary = onMenu
	return container.NewGridWrap(fyne.NewSize(220, 58), container.NewStack(border, tap))
}

func (u *UI) saveAttachment(m desktop.MessageItem, a desktop.AttachmentItem) {
	var err error
	u.bg(func() {
		dir := ""
		if home, e := os.UserHomeDir(); e == nil {
			dir = filepath.Join(home, "Downloads")
		}
		var path string
		if path, err = pickSaveFile("Save attachment", a.Name, dir); err != nil || path == "" {
			return
		}
		var data desktop.AttachmentDataItem
		if data, err = u.be.GetAttachmentData(m.AccountID, m.ID, a.AttachmentID); err != nil {
			return
		}
		err = os.WriteFile(path, data.Data, 0o644)
	}, func() {
		if err != nil {
			u.showError(err)
		}
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
	u.convScroll = nil // a different conversation starts at the top
	u.messages = nil
	u.msgCursor = 0
	u.collapsed = map[string]bool{}
	u.threadLoading = true
	u.refreshReader()

	var msgs []desktop.MessageItem
	var err error
	u.bg(func() {
		msgs, err = u.be.GetThread(t.AccountID, t.ID)
		// Fetch bodies concurrently: a long thread would otherwise pay one
		// sequential Gmail round trip per message.
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for i, m := range msgs {
			if m.Body == "" {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()
					if full, e := u.be.GetMessage(m.AccountID, m.ID); e == nil {
						msgs[i] = full
					}
				}()
			}
		}
		wg.Wait()
	}, func() {
		if gen != u.threadGen {
			return
		}
		u.threadLoading = false
		if err != nil {
			u.selected = nil // so clicking the row again retries
			u.list.UnselectAll()
			u.refreshReader()
			u.showError(err)
			return
		}
		u.messages = msgs
		u.msgCursor = max(len(msgs)-1, 0)
		u.refreshReader()
		for _, m := range msgs {
			if m.IsUnread {
				u.scheduleMarkRead(gen)
				break
			}
		}
	})
}

// markRead marks every unread message of the open thread read.
func (u *UI) markRead() {
	if u.selected == nil {
		return
	}
	t := *u.selected
	msgs := append([]desktop.MessageItem(nil), u.messages...)
	var first error
	u.bg(func() {
		for _, m := range msgs {
			if m.IsUnread {
				if err := u.be.MarkMessageRead(m.AccountID, m.ID); err != nil && first == nil {
					first = err
				}
			}
		}
	}, func() {
		if first != nil { // the thread is still unread on the server: do not show it read
			u.noteErr("could not mark as read", first)
			u.refreshSidebar()
			return
		}
		if u.selected != nil && u.selected.ID == t.ID && u.selected.AccountID == t.AccountID {
			for i := range u.messages {
				u.messages[i].IsUnread = false
			}
		}
		u.setThreadUnread(t.ID, t.AccountID, false)
		u.reloadLabels()
	})
}

// scheduleMarkRead marks the thread read after it has stayed open a few seconds.
func (u *UI) scheduleMarkRead(gen int) {
	if u.inline {
		return
	}
	u.readTimer = time.AfterFunc(markReadWait, func() {
		fyne.Do(func() {
			if gen != u.threadGen {
				return
			}
			u.markRead()
		})
	})
}

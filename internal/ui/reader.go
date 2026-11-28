package ui

import (
	"errors"
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
	"github.com/mailrat/mailrat/internal/mail"
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
	return topBar(container.New(layout.NewCustomPaddedLayout(0, 0, theme.Padding(), theme.Padding()), container.NewBorder(nil, nil, btns, right)))
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
	_, waiting := u.bodyWait[m.ID]
	if waiting || strings.TrimSpace(htmlToText(src)) == "" {
		if waiting || strings.TrimSpace(m.Snippet) == "" {
			return u.bodyPlaceholder(m)
		}
		src = m.Snippet // body parses to nothing: show the snippet
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
			q := widget.NewRichText(quotedNoBars(res.quoted)...)
			q.Wrapping = fyne.TextWrapWord
			// One solid bar for the whole block instead of a "▎" glyph per paragraph, which left gaps.
			quote := container.NewBorder(nil, nil, container.New(layout.NewCustomPaddedLayout(0, 0, quoteBarPad, quoteBarGap), quoteBar()), nil, q)
			items = append(items, container.New(layout.NewCustomPaddedLayout(0, 8, 52, 8), quote))
		}
	}
	if len(m.Attachments) > 0 {
		var cards []fyne.CanvasObject
		for _, a := range m.Attachments {
			a := a
			cards = append(cards, u.attachmentCard(a, th, func() { u.openAttachment(m, a) }, func(p fyne.Position) { u.attachmentMenu(m, a, p) }))
		}
		title := fmt.Sprintf("%d Attachments", len(m.Attachments))
		if len(m.Attachments) == 1 {
			title = "1 Attachment"
		}
		head := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		area := canvas.NewRectangle(th.Color(theme.ColorNameInputBackground, theme.VariantLight))
		area.CornerRadius = 10
		area.StrokeWidth = 1
		area.StrokeColor = th.Color(theme.ColorNameSeparator, theme.VariantLight)
		panel := container.NewStack(area, container.New(layout.NewCustomPaddedLayout(6, 8, 8, 8),
			container.NewVBox(head, container.NewGridWrap(fyne.NewSize(240, 58), cards...))))
		items = append(items, container.New(layout.NewCustomPaddedLayout(0, 10, 52, 8), panel))
	}
	return mailSurface(th, container.NewVBox(items...))
}

// Gmail quote bar: margin-left .8ex, 1px #ccc, padding-left 1ex. Shared by the
// expanded folded quote and inline quotes drawn by selBody.
const (
	quoteBarPad = 7 // left of the bar
	quoteBarGap = 8 // bar to text
)

func quoteBar() *canvas.Rectangle {
	line := canvas.NewRectangle(color.NRGBA{R: 204, G: 204, B: 204, A: 255})
	line.SetMinSize(fyne.NewSize(1, 1))
	return line
}

// quotedNoBars drops the per-paragraph "▎" glyph prefixes; the expanded quote
// draws a single continuous bar instead.
func quotedNoBars(segs []widget.RichTextSegment) []widget.RichTextSegment {
	out := make([]widget.RichTextSegment, 0, len(segs))
	for _, s := range segs {
		if t, ok := s.(*widget.TextSegment); ok && t.Style.ColorName == theme.ColorNamePlaceHolder && barRe.MatchString(t.Text) {
			continue
		}
		out = append(out, s)
	}
	return out
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

// attachmentCard is a Gmail-style tile: type badge, filename and size; tap opens, right-click saves.
func (u *UI) attachmentCard(a desktop.AttachmentItem, th mailTheme, onTap func(), onMenu func(fyne.Position)) fyne.CanvasObject {
	name := widget.NewLabel(a.Name)
	name.Truncation = fyne.TextTruncateEllipsis
	size := widget.NewLabel(humanSize(a.Size))
	size.Importance = widget.LowImportance
	k := attachmentKind(a.Name, a.MimeType)
	bg := canvas.NewRectangle(k.Color)
	bg.CornerRadius = 6
	lbl := canvas.NewText(k.Label, color.White)
	lbl.TextStyle = fyne.TextStyle{Bold: true}
	lbl.TextSize = 11
	lbl.Alignment = fyne.TextAlignCenter
	badge := container.NewGridWrap(fyne.NewSize(36, 36), container.NewStack(bg, container.NewCenter(lbl)))
	text := container.New(layout.NewCustomPaddedVBoxLayout(-6), layout.NewSpacer(), name, size, layout.NewSpacer())
	inner := container.NewBorder(nil, nil, container.NewCenter(badge), nil, text)
	border := canvas.NewRectangle(th.Color(theme.ColorNameBackground, theme.VariantLight))
	border.CornerRadius = 8
	border.StrokeWidth = 1
	border.StrokeColor = th.Color(theme.ColorNameSeparator, theme.VariantLight)
	tap := newTapRow(container.New(layout.NewCustomPaddedLayout(6, 6, 10, 8), inner), false, onTap)
	tap.onSecondary = onMenu
	return container.NewStack(border, tap)
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
	u.bodyWait = map[string]error{}
	u.threadLoading = true
	u.refreshReader()

	var msgs []desktop.MessageItem
	var err error
	u.bg(func() {
		msgs, err = u.be.GetThread(t.AccountID, t.ID)
	}, func() {
		if gen != u.threadGen {
			return
		}
		if err != nil {
			u.threadLoading = false
			u.selected = nil // so clicking the row again retries
			u.list.UnselectAll()
			u.refreshReader()
			u.showError(err)
			return
		}
		// Show what is stored at once (an uncached body renders as its snippet);
		// bodies still missing are fetched next without holding the reader back.
		u.messages = msgs
		u.msgCursor = max(len(msgs)-1, 0)
		u.threadLoading = false
		u.fetchBodies(gen, msgs)
		u.refreshReader()
		for _, m := range msgs {
			if m.IsUnread {
				u.scheduleMarkRead(gen)
				break
			}
		}
	})
}

// fetchBodies loads, in the background, the bodies of msgs that are not stored
// yet and redraws the reader when they arrive, showing the progress bar meanwhile.
// A message whose fetch fails is flagged in bodyWait so the reader shows the
// error with a Retry button instead of a blank body.
func (u *UI) fetchBodies(gen int, msgs []desktop.MessageItem) {
	var missing []desktop.MessageItem
	for _, m := range msgs {
		if m.Body == "" {
			missing = append(missing, m)
			u.bodyWait[m.ID] = nil
		}
	}
	if len(missing) == 0 {
		return
	}
	u.threadLoading = true
	got := make([]desktop.MessageItem, len(missing))
	errs := make([]error, len(missing))
	u.bg(func() {
		// Concurrent: a long thread would otherwise pay one sequential Gmail
		// round trip per message.
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for i, m := range missing {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				got[i], errs[i] = u.be.GetMessage(m.AccountID, m.ID)
			}()
		}
		wg.Wait()
	}, func() {
		if gen != u.threadGen {
			return
		}
		u.threadLoading = false
		gone := map[string]bool{} // deleted on the server
		fresh := map[string]desktop.MessageItem{}
		for i, m := range missing {
			switch {
			case errs[i] == nil:
				delete(u.bodyWait, m.ID)
				fresh[m.ID] = got[i]
			case errors.Is(errs[i], mail.ErrMessageGone):
				delete(u.bodyWait, m.ID)
				gone[m.ID] = true
			default:
				u.bodyWait[m.ID] = errs[i]
			}
		}
		kept := u.messages[:0:0]
		for _, m := range u.messages {
			if gone[m.ID] {
				continue
			}
			if f, ok := fresh[m.ID]; ok {
				f.IsUnread = m.IsUnread // keep the read state the user may have changed meanwhile
				m = f
			}
			kept = append(kept, m)
		}
		if len(kept) == 0 { // every message was deleted on the server
			u.clearSelection()
			u.reloadThreads()
			return
		}
		u.messages = kept
		u.msgCursor = min(u.msgCursor, len(kept)-1)
		u.refreshReader()
	})
}

// bodyPlaceholder is what an open message shows when it has no body text:
// "Loading", the fetch error with Retry, or an explicit note. Never blank.
func (u *UI) bodyPlaceholder(m desktop.MessageItem) fyne.CanvasObject {
	pad := func(o fyne.CanvasObject) fyne.CanvasObject {
		return container.New(layout.NewCustomPaddedLayout(4, 8, 52, 8), o)
	}
	lbl := func(s string) *widget.Label {
		l := widget.NewLabel(s)
		l.Importance = widget.LowImportance
		l.Wrapping = fyne.TextWrapWord
		return l
	}
	err, waiting := u.bodyWait[m.ID]
	switch {
	case waiting && err == nil:
		return pad(lbl("Loading message…"))
	case waiting:
		retry := widget.NewButton("Retry", func() { u.fetchBodies(u.threadGen, []desktop.MessageItem{m}); u.refreshReader() })
		return pad(container.NewVBox(lbl("Could not load this message: "+err.Error()), container.NewHBox(retry)))
	}
	return pad(lbl("(This message has no content.)"))
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

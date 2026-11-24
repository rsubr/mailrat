package ui

import (
	"fmt"
	"html"
	"image/color"
	netmail "net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/mail"
)

var tagRe = regexp.MustCompile(`<[^>]+>`)

// formatListDate renders a thread timestamp the way Mimestream's list does:
// time today, "Yesterday", weekday within a week, otherwise a short date.
func formatListDate(ms int64, now time.Time) string {
	if ms == 0 {
		return ""
	}
	d := time.UnixMilli(ms).In(now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	switch days := int(today.Sub(day).Hours() / 24); {
	case days <= 0:
		return d.Format("3:04 PM")
	case days == 1:
		return "Yesterday"
	case days < 7:
		return d.Format("Monday")
	case d.Year() == now.Year():
		return d.Format("Jan 2")
	default:
		return d.Format("1/2/06")
	}
}

// formatMessageDate renders a message timestamp: "Today at 10:58AM".
func formatMessageDate(ms int64, now time.Time) string {
	if ms == 0 {
		return ""
	}
	d := time.UnixMilli(ms).In(now.Location())
	clock := d.Format("3:04PM")
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	switch days := int(today.Sub(day).Hours() / 24); {
	case days <= 0:
		return "Today at " + clock
	case days == 1:
		return "Yesterday at " + clock
	case d.Year() == now.Year():
		return d.Format("Jan 2") + " at " + clock
	default:
		return d.Format("Jan 2, 2006") + " at " + clock
	}
}

// displayName extracts the human name from `Name <addr>`, falling back to the address.
func displayName(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.Index(addr, "<"); i > 0 {
		if n := strings.Trim(strings.TrimSpace(addr[:i]), `"`); n != "" {
			return n
		}
	}
	return strings.Trim(addr, "<>")
}

// initials returns up to two uppercase letters for an avatar.
func initials(s string) string {
	s = strings.TrimSpace(tagRe.ReplaceAllString(s, ""))
	var out []rune
	for _, p := range strings.Fields(s) {
		out = append(out, []rune(strings.ToUpper(p))[0])
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

var avatarPalette = []color.NRGBA{
	{0x0a, 0x84, 0xff, 0xff}, {0x30, 0xb0, 0x50, 0xff}, {0xff, 0x9f, 0x0a, 0xff},
	{0xbf, 0x5a, 0xf2, 0xff}, {0xff, 0x45, 0x3a, 0xff}, {0x64, 0xd2, 0xff, 0xff},
	{0xac, 0x8e, 0x68, 0xff}, {0xff, 0x37, 0x5f, 0xff},
}

func avatarColor(s string) color.NRGBA {
	h := 0
	for _, r := range s {
		h = int(r) + ((h << 5) - h)
	}
	if h < 0 {
		h = -h
	}
	return avatarPalette[h%len(avatarPalette)]
}

// parseHex parses "#rrggbb"; ok is false for anything else.
func parseHex(s string) (c color.NRGBA, ok bool) {
	var r, g, b uint8
	if len(s) == 7 && s[0] == '#' {
		if _, err := fmt.Sscanf(s[1:], "%02x%02x%02x", &r, &g, &b); err == nil {
			return color.NRGBA{r, g, b, 0xff}, true
		}
	}
	return color.NRGBA{}, false
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ── reply / forward drafting ────────────────────────────────────────────────

func prefixed(subject, prefix string) string {
	if strings.HasPrefix(strings.ToLower(subject), strings.ToLower(prefix)) {
		return subject
	}
	return prefix + subject
}

// quotedBody is the original message as sanitized HTML; plain text is escaped.
func quotedBody(m desktop.MessageItem) string {
	if isHTML(m.Body) {
		return mail.SanitizeHTML(m.Body)
	}
	body := strings.TrimSpace(m.Body)
	if body == "" {
		body = m.Snippet
	}
	return strings.ReplaceAll(html.EscapeString(body), "\n", "<br>")
}

func quoteDate(m desktop.MessageItem) string {
	return time.UnixMilli(m.Date).Format("Mon, Jan 2, 2006 at 3:04 PM")
}

// quoteHTML builds the Gmail-style quoted original used by reply.
func quoteHTML(m desktop.MessageItem) string {
	attr := html.EscapeString(m.FromAddr) + " wrote:"
	if m.Date != 0 {
		attr = "On " + quoteDate(m) + ", " + attr
	}
	return quoteOpen + `<div class="gmail_attr">` + attr + `</div><blockquote>` + quotedBody(m) + `</blockquote></div>`
}

// forwardHTML builds the "Forwarded message" block used by forward.
func forwardHTML(m desktop.MessageItem) string {
	var b strings.Builder
	b.WriteString(quoteOpen + `<div class="gmail_attr">---------- Forwarded message ---------<br>`)
	b.WriteString("From: " + html.EscapeString(m.FromAddr) + "<br>")
	if m.Date != 0 {
		b.WriteString("Date: " + quoteDate(m) + "<br>")
	}
	b.WriteString("Subject: " + html.EscapeString(m.Subject) + "<br>")
	b.WriteString("To: " + html.EscapeString(m.ToAddr))
	if m.CcAddr != "" {
		b.WriteString("<br>Cc: " + html.EscapeString(m.CcAddr))
	}
	b.WriteString(`</div><br>` + quotedBody(m) + `</div>`)
	return b.String()
}

// replyDraft prefills a reply. own are the user's addresses (accounts and
// send-as aliases): they are never replied to, and replying to a message the
// user sent goes to its original recipients. With all, the other recipients
// of the original (To and Cc) are copied. The reply goes to the message's
// Reply-To when it has a usable one, else to its sender, and carries the
// thread's References chain.
func replyDraft(m desktop.MessageItem, all bool, own ...string) desktop.OutboundMsg {
	d := desktop.OutboundMsg{
		AccountID: m.AccountID,
		To:        m.FromAddr,
		Subject:   prefixed(m.Subject, "Re: "),
		Body:      quoteHTML(m),
		InReplyTo: m.MessageID, // the RFC Message-ID, not Gmail's internal id; Gmail still threads by ThreadID
		ThreadID:  m.ThreadID,
	}
	d.References = strings.Join(mail.ReferencesFor(m.References, m.InReplyTo, m.MessageID), " ")
	mine := map[string]bool{}
	for _, o := range own {
		mine[addrKey(o)] = true
	}
	others := func(lists ...string) (out []string) {
		seen := map[string]bool{}
		for _, l := range lists {
			for _, a := range splitAddrs(l) {
				if k := addrKey(a); !mine[k] && !seen[k] {
					seen[k] = true
					out = append(out, a)
				}
			}
		}
		return out
	}
	cc := m.ToAddr + "," + m.CcAddr
	if sentByMe := len(others(m.FromAddr)) == 0; sentByMe {
		d.To = joinComma(others(m.ToAddr))
		cc = m.CcAddr
		if d.To == "" { // a message to myself
			d.To = m.ToAddr
		}
	} else if rt := others(mail.ParseAddressList(m.ReplyTo)...); len(rt) > 0 {
		d.To = joinComma(rt) // a missing, malformed or all-mine Reply-To falls back to From
	}
	if all {
		for _, a := range others(d.To) {
			mine[addrKey(a)] = true // already addressed in To
		}
		d.Cc = joinComma(others(cc))
	}
	return d
}

// addrKey is the lower-cased bare address of a header address.
func addrKey(a string) string {
	if p, err := netmail.ParseAddress(a); err == nil {
		return strings.ToLower(p.Address)
	}
	return strings.ToLower(strings.TrimSpace(a))
}

func forwardDraft(m desktop.MessageItem) desktop.OutboundMsg {
	return desktop.OutboundMsg{
		AccountID: m.AccountID,
		Subject:   prefixed(m.Subject, "Fwd: "),
		Body:      forwardHTML(m),
	}
}

// splitAddrs splits a header address list on commas that are outside quotes/brackets.
func splitAddrs(s string) []string {
	var out []string
	depth, quoted, start := 0, false, 0
	for i, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case quoted:
		case r == '<':
			depth++
		case r == '>':
			depth--
		case r == ',' && depth <= 0:
			if p := strings.TrimSpace(s[start:i]); p != "" {
				out = append(out, p)
			}
			start = i + 1
		}
	}
	if p := strings.TrimSpace(s[start:]); p != "" {
		out = append(out, p)
	}
	return out
}

func indexSpace(s string) int { return strings.IndexByte(s, ' ') }

func joinComma(p []string) string { return strings.Join(p, ", ") }

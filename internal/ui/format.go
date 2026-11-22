package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"time"

	"github.com/mailrat/mailrat/internal/desktop"
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

// quoteText builds the plain-text quoted original used by reply and forward.
func quoteText(m desktop.MessageItem) string {
	body := htmlToText(m.Body)
	if strings.TrimSpace(body) == "" {
		body = m.Snippet
	}
	attr := m.FromAddr + " wrote:"
	if m.Date != 0 {
		attr = "On " + time.UnixMilli(m.Date).Format("Mon, Jan 2, 2006 at 3:04 PM") + ", " + attr
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return "\n\n" + attr + "\n" + strings.Join(lines, "\n")
}

func replyDraft(m desktop.MessageItem, all bool) desktop.OutboundMsg {
	d := desktop.OutboundMsg{
		AccountID: m.AccountID,
		To:        m.FromAddr,
		Subject:   prefixed(m.Subject, "Re: "),
		Body:      quoteText(m),
		InReplyTo: m.ID,
		ThreadID:  m.ThreadID,
	}
	if all {
		d.Cc = m.ToAddr
	}
	return d
}

func forwardDraft(m desktop.MessageItem) desktop.OutboundMsg {
	return desktop.OutboundMsg{
		AccountID: m.AccountID,
		Subject:   prefixed(m.Subject, "Fwd: "),
		Body:      quoteText(m),
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

package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
)

// Outline icons on a 24×24 grid, drawn in the SF-Symbols style Mimestream uses.
// "currentColor" is substituted per use so a single path set serves every tint.
var iconPaths = map[string]string{
	"inbox":       `<path d="M3 13l3-8h12l3 8v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/><path d="M3 13h5l1 3h6l1-3h5"/>`,
	"star":        `<path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.9L12 16.9l-5.2 2.8 1-5.9L3.5 9.7l5.9-.8z"/>`,
	"draft":       `<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 12h6M9 16h6"/>`,
	"send":        `<path d="M21 3L3 10.5l7 3 3 7z"/><path d="M21 3L10 13.5"/>`,
	"archive":     `<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8M10 12h4"/>`,
	"allmail":     `<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8M10 12h4"/>`,
	"spam":        `<circle cx="12" cy="12" r="9"/><path d="M12 7v6M12 16.2v.3"/>`,
	"trash":       `<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/>`,
	"important":   `<path d="M4 5h11l5 7-5 7H4z"/>`,
	"starfill":    `<path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.9L12 16.9l-5.2 2.8 1-5.9L3.5 9.7l5.9-.8z" fill="currentColor"/>`,
	"tag":         `<path d="M3 12V4h8l10 10-8 8z"/><circle cx="7.5" cy="8.5" r="1"/>`,
	"unread":      `<rect x="3" y="6" width="16" height="13" rx="2"/><path d="M3.5 7.5L11 13l7.5-5.5"/><circle cx="20" cy="5" r="2.2" fill="currentColor"/>`,
	"reply":       `<path d="M9 7L4 12l5 5"/><path d="M4 12h10a6 6 0 0 1 6 6v1"/>`,
	"replyall":    `<path d="M7 7l-5 5 5 5"/><path d="M12 7l-5 5 5 5"/><path d="M7 12h7a6 6 0 0 1 6 6v1"/>`,
	"forward":     `<path d="M15 7l5 5-5 5"/><path d="M20 12H10a6 6 0 0 0-6 6v1"/>`,
	"compose":     `<path d="M11 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5"/><path d="M17.5 3.5a1.8 1.8 0 0 1 2.5 2.5L11 15l-3.5 1 1-3.5z"/>`,
	"search":      `<circle cx="10.5" cy="10.5" r="6.5"/><path d="M15.5 15.5L21 21"/>`,
	"settings":    `<path d="M9.60 5.21L9.86 2.44L14.14 2.44L14.40 5.21L15.10 5.50L17.25 3.73L20.27 6.75L18.50 8.90L18.79 9.60L21.56 9.86L21.56 14.14L18.79 14.40L18.50 15.10L20.27 17.25L17.25 20.27L15.10 18.50L14.40 18.79L14.14 21.56L9.86 21.56L9.60 18.79L8.90 18.50L6.75 20.27L3.73 17.25L5.50 15.10L5.21 14.40L2.44 14.14L2.44 9.86L5.21 9.60L5.50 8.90L3.73 6.75L6.75 3.73L8.90 5.50z"/><circle cx="12" cy="12" r="3"/>`,
	"moon":        `<path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"/>`,
	"sun":         `<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2.5M12 19v2.5M2.5 12H5M19 12h2.5M5.3 5.3l1.8 1.8M16.9 16.9l1.8 1.8M5.3 18.7l1.8-1.8M16.9 7.1l1.8-1.8"/>`,
	"sync":        `<path d="M20 11a8 8 0 0 0-14.5-3.5L4 9M4 4v5h5"/><path d="M4 13a8 8 0 0 0 14.5 3.5L20 15M20 20v-5h-5"/>`,
	"clip":        `<path d="M20 11.5l-8 8a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7L9.7 17.2a1.6 1.6 0 0 1-2.4-2.4L14.5 7.5"/>`,
	"menu":        `<path d="M4 6h16M4 12h16M4 18h16"/>`,
	"file":        `<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>`,
	"chevron":     `<path d="M9 6l6 6-6 6"/>`,
	"chevrondown": `<path d="M6 9l6 6 6-6"/>`,
	"filter":      `<circle cx="12" cy="12" r="9"/><path d="M7.5 9.5h9M9.5 12.5h5M11 15.5h2"/>`,
	"person":      `<circle cx="12" cy="8" r="4"/><path d="M4 20a8 8 0 0 1 16 0"/>`,
	"close":       `<path d="M6 6l12 12M18 6L6 18"/>`,
	"warning":     `<path d="M12 3l10 18H2z"/><path d="M12 10v5M12 17.8v.2"/>`,
	"image":       `<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="M21 16l-5-5-8 8"/>`,
	"read":        `<path d="M3 10l9-6 9 6v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/><path d="M3.5 10.5L12 16l8.5-5.5"/>`,
	"move":        `<path d="M3 6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/>`,
	"more":        `<circle cx="12" cy="12" r="9"/><path d="M8 12h.01M12 12h.01M16 12h.01" stroke-width="2.6"/>`,
}

// icon returns the named glyph tinted with c.
func icon(name string, c color.Color) fyne.Resource { return iconRot(name, c, 0) }

// iconRot is icon rotated deg degrees about its centre (spinner frames).
func iconRot(name string, c color.Color, deg int) fyne.Resource {
	body, ok := iconPaths[name]
	if !ok {
		body = iconPaths["tag"]
	}
	r, g, b, _ := c.RGBA()
	hex := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	if deg != 0 {
		body = fmt.Sprintf(`<g transform="rotate(%d 12 12)">%s</g>`, deg, body)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`
	svg = strings.ReplaceAll(svg, "currentColor", hex)
	return fyne.NewStaticResource(fmt.Sprintf("%s-%s-%d.svg", name, hex[1:], deg), []byte(svg))
}

// Mimestream tints each system mailbox icon with its own colour.
var labelIcons = map[string]struct {
	icon string
	col  color.Color
}{
	"INBOX":     {"inbox", color.NRGBA{0x0a, 0x84, 0xff, 0xff}},
	"STARRED":   {"star", color.NRGBA{0xff, 0xd6, 0x0a, 0xff}},
	"DRAFT":     {"draft", color.NRGBA{0x64, 0xd2, 0xff, 0xff}},
	"SENT":      {"send", color.NRGBA{0x30, 0xd1, 0x58, 0xff}},
	"ALL_MAIL":  {"allmail", color.NRGBA{0xbf, 0x5a, 0xf2, 0xff}},
	"IMPORTANT": {"important", color.NRGBA{0xff, 0x9f, 0x0a, 0xff}},
	"SPAM":      {"spam", color.NRGBA{0xff, 0x9f, 0x0a, 0xff}},
	"TRASH":     {"trash", color.NRGBA{0xff, 0x45, 0x3a, 0xff}},
}

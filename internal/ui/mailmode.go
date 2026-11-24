package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"github.com/mailrat/mailrat/internal/mail/darkmode"
)

// The mail view is the message body area of the reader (body, folded quote,
// "Load remote images" row, attachments) and the surface behind it. It has its
// own light/dark mode, independent of the app theme. Header, toolbar, list and
// sidebar always follow the app theme.
const prefMailMode = "mail.mode" // "", "light" or "dark"; empty follows the app theme

// mailTheme is the palette of the mail view: the app's light or dark palette
// with link blues that stay readable on that surface (>= 4.5:1).
type mailTheme struct{ mimeTheme }

func newMailTheme(light bool) mailTheme { return mailTheme{mimeTheme{forced: true, dark: !light}} }

func (t mailTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameHyperlink {
		if t.dark {
			return rgb(0x5a, 0xa9, 0xff)
		}
		return rgb(0x0b, 0x57, 0xd0)
	}
	return t.mimeTheme.Color(n, v)
}

// mailLight reports whether the mail view is in light mode.
func (u *UI) mailLight() bool {
	switch u.mailMode {
	case "light":
		return true
	case "dark":
		return false
	}
	return !isDark()
}

// toggleMailMode flips the mail view between light and dark, remembers the
// choice and redraws the open conversation, keeping the scroll position.
func (u *UI) toggleMailMode() {
	if u.mailLight() {
		u.mailMode = "dark"
	} else {
		u.mailMode = "light"
	}
	u.app.Preferences().SetString(prefMailMode, u.mailMode)
	var off fyne.Position
	if u.convScroll != nil {
		off = u.convScroll.Offset
	}
	u.refreshReader()
	if u.convScroll != nil {
		u.convScroll.ScrollToOffset(off)
	}
}

// mailToggle is the sun/moon button; it shows the mode a tap switches to.
func (u *UI) mailToggle() fyne.CanvasObject {
	glyph, tip := "sun", "Switch mail to light mode"
	if u.mailLight() {
		glyph, tip = "moon", "Switch mail to dark mode"
	}
	return iconButton(glyph, tip, u.toggleMailMode)
}

// mailSurface paints items on the mail view's own background and palette.
func mailSurface(th mailTheme, items fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(th.Color(theme.ColorNameBackground, theme.VariantLight))
	return container.NewStack(bg, container.NewThemeOverride(items, th))
}

// mailRender renders a message body for the mail view palette th. Both modes
// run darkmode.Adapt so <style> rules and legacy attributes become inline and
// author backgrounds reach the renderer as #rrggbb, which selBody paints as
// bands. Dark: colours are adapted Gmail-Android style, backgrounds darkened
// (text is >= 4.5:1 on its painted background). Light: KeepColors, the
// author's colours and backgrounds untouched; renderMail's readable() repairs
// only text under 3.5:1 against the background it is actually painted on.
func mailRender(src string, th mailTheme, images bool) renderResult {
	bg, text := th.Color(theme.ColorNameBackground, theme.VariantLight), th.Color(theme.ColorNameForeground, theme.VariantLight)
	link := th.Color(theme.ColorNameHyperlink, theme.VariantLight)
	if isHTML(src) {
		if r, err := darkmode.Adapt(src, darkmode.Theme{Background: bg, Foreground: text, Accent: link, KeepColors: !th.dark}); err == nil {
			src = r.HTML
		}
	}
	return renderMail(src, renderOpts{bg: bg, fg: text, images: images, link: link})
}

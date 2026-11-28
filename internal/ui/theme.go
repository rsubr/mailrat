package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Custom colour names for surfaces Fyne's theme has no slot for.
const (
	colorSidebar fyne.ThemeColorName = "mailratSidebar"
	colorSubtle  fyne.ThemeColorName = "mailratSubtle"
	colorChip    fyne.ThemeColorName = "mailratChip"
)

// mimeTheme mirrors Mimestream's macOS look: graphite panes in dark mode,
// light grey sidebar in light mode, system-blue accent.
type mimeTheme struct {
	forced bool // pin the variant instead of following the OS
	dark   bool
}

func rgb(r, g, b uint8) color.Color { return color.NRGBA{r, g, b, 0xff} }

func (t mimeTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.forced {
		v = theme.VariantLight
		if t.dark {
			v = theme.VariantDark
		}
	}
	if c, ok := parseHexName(n); ok { // author colour from rendered mail
		return c
	}
	dark := v == theme.VariantDark
	pick := func(d, l color.Color) color.Color {
		if dark {
			return d
		}
		return l
	}
	switch n {
	case theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground:
		return pick(rgb(0x38, 0x39, 0x3c), rgb(0xff, 0xff, 0xff))
	case theme.ColorNameBackground:
		return pick(rgb(0x2d, 0x2e, 0x30), rgb(0xff, 0xff, 0xff))
	case colorSidebar:
		return pick(rgb(0x38, 0x39, 0x3c), rgb(0xf1, 0xf1, 0xf3))
	case theme.ColorNameInputBackground:
		return pick(rgb(0x3f, 0x41, 0x44), rgb(0xf1, 0xf1, 0xf3))
	case theme.ColorNameButton:
		return pick(rgb(0x44, 0x46, 0x4a), rgb(0xe9, 0xe9, 0xeb))
	case theme.ColorNameForeground:
		return pick(rgb(0xe3, 0xe3, 0xe3), rgb(0x1c, 0x1c, 0x1e))
	case theme.ColorNamePlaceHolder, colorSubtle, theme.ColorNameDisabled:
		return pick(rgb(0xbd, 0xc1, 0xc6), rgb(0x76, 0x76, 0x7b))
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return rgb(0x0a, 0x84, 0xff)
	case theme.ColorNameSelection:
		return pick(rgb(0x43, 0x56, 0x72), rgb(0xdf, 0xdf, 0xe3))
	case theme.ColorNameHover:
		return pick(rgb(0x4a, 0x4c, 0x50), rgb(0xdf, 0xdf, 0xe3))
	case theme.ColorNamePressed:
		return pick(rgb(0x55, 0x58, 0x5c), rgb(0xdf, 0xdf, 0xe3))
	case colorChip:
		return pick(rgb(0x44, 0x46, 0x4a), rgb(0xe3, 0xe3, 0xe7))
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return pick(rgb(0x5f, 0x63, 0x68), rgb(0xd9, 0xd9, 0xdd))
	case theme.ColorNameShadow:
		return color.NRGBA{0, 0, 0, 0x40}
	}
	return theme.DefaultTheme().Color(n, v)
}

func (mimeTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }

func (mimeTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (mimeTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNameSubHeadingText:
		return 19
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameScrollBar:
		return 8
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 6
	}
	return theme.DefaultTheme().Size(n)
}

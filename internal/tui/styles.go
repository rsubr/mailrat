package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorBlue  = lipgloss.Color("#5C7CFF")
	colorGray  = lipgloss.Color("#888888")
	colorWhite = lipgloss.Color("#FFFFFF")
	colorRed   = lipgloss.Color("#FF5C5C")
	colorDark  = lipgloss.Color("#1A1A2E")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite).
			Background(colorDark).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2D2D44")).
			Foreground(colorWhite)

	unreadDotStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed)

	accentStyle = lipgloss.NewStyle().
			Foreground(colorBlue)

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444444"))
)

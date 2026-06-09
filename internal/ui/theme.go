package ui

import "github.com/charmbracelet/lipgloss"

var (
	Fg      = lipgloss.Color("#c0caf5")
	Comment = lipgloss.Color("#565f89")
	Blue    = lipgloss.Color("#7aa2f7")
	Cyan    = lipgloss.Color("#7dcfff")
	Magenta = lipgloss.Color("#bb9af7")
	Green   = lipgloss.Color("#9ece6a")
	Yellow  = lipgloss.Color("#e0af68")
	Red     = lipgloss.Color("#f7768e")
)

var (
	stepStyle  = lipgloss.NewStyle().Bold(true).Foreground(Blue)
	infoStyle  = lipgloss.NewStyle()
	errorStyle = lipgloss.NewStyle().Foreground(Red)
)

package dashboard

import "github.com/charmbracelet/lipgloss"

var (
	colorFg        = lipgloss.Color("#c0caf5")
	colorSelection = lipgloss.CompleteColor{TrueColor: "#283457", ANSI256: "237", ANSI: "8"}
	colorComment   = lipgloss.Color("#565f89")
	colorBlue      = lipgloss.Color("#7aa2f7")
	colorCyan      = lipgloss.Color("#7dcfff")
	colorMagenta   = lipgloss.Color("#bb9af7")
	colorGreen     = lipgloss.Color("#9ece6a")
	colorYellow    = lipgloss.Color("#e0af68")
	colorRed       = lipgloss.Color("#f7768e")
)

var (
	plainStyle = lipgloss.NewStyle()
	rowFill    = lipgloss.NewStyle().Background(colorSelection)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	modeMarker = lipgloss.NewStyle().Foreground(colorYellow)

	attachedMarker = lipgloss.NewStyle().Foreground(colorCyan)

	keyStyle       = lipgloss.NewStyle().Foreground(colorMagenta)
	footerStyle    = lipgloss.NewStyle().Foreground(colorComment)
	noticeStyle    = lipgloss.NewStyle().Foreground(colorYellow)
	labelStyle     = lipgloss.NewStyle().Foreground(colorComment)
	ruleStyle      = lipgloss.NewStyle().Foreground(colorBlue)
	detailLogStyle = lipgloss.NewStyle().Foreground(colorComment)
	valueStyle     = lipgloss.NewStyle().Foreground(colorFg)

	paneBorder      = lipgloss.NewStyle().Foreground(colorComment)
	paneBorderFocus = lipgloss.NewStyle().Foreground(colorCyan)
	paneTitle       = lipgloss.NewStyle().Foreground(colorComment)
	paneTitleFocus  = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)

	boxBorder = lipgloss.RoundedBorder()
	boxStyle  = lipgloss.NewStyle().
			Border(boxBorder).
			BorderForeground(colorComment).
			Padding(0, 1)
)

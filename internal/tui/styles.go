package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors assigned to apps in the sidebar
	appColors = []lipgloss.Color{
		lipgloss.Color("6"),  // cyan
		lipgloss.Color("3"),  // yellow
		lipgloss.Color("5"),  // magenta
		lipgloss.Color("2"),  // green
		lipgloss.Color("4"),  // blue
		lipgloss.Color("9"),  // bright red
		lipgloss.Color("14"), // bright cyan
		lipgloss.Color("11"), // bright yellow
	}

	colorRed    = lipgloss.Color("1")
	colorGreen  = lipgloss.Color("2")
	colorYellow = lipgloss.Color("3")
	colorDim    = lipgloss.Color("8")

	styleBold    = lipgloss.NewStyle().Bold(true)
	styleDim     = lipgloss.NewStyle().Faint(true)
	styleInverse = lipgloss.NewStyle().Reverse(true)

	styleStatusRunning  = lipgloss.NewStyle().Foreground(colorGreen)
	styleStatusCrashed  = lipgloss.NewStyle().Foreground(colorRed)
	styleStatusStopping = lipgloss.NewStyle().Foreground(colorYellow)
	styleStatusStopped  = lipgloss.NewStyle().Faint(true)
	styleError          = lipgloss.NewStyle().Foreground(colorRed)

	// Box drawing characters
	boxTL = "┌"
	boxTR = "┐"
	boxBL = "└"
	boxBR = "┘"
	boxH  = "─"
	boxV  = "│"
	boxML = "├"
	boxMR = "┤"
	boxMB = "┴"
)

func appColor(idx int) lipgloss.Color {
	if idx < 0 {
		idx = -idx
	}
	return appColors[idx%len(appColors)]
}

package tui

import "github.com/charmbracelet/lipgloss"

var (
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


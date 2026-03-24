package main

import "github.com/charmbracelet/lipgloss"

var (
	// Tree pane styles.
	styleNodeName   = lipgloss.NewStyle().Bold(true)
	styleType       = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	styleLiteral    = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
	styleDefault    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	styleCursorLine = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	styleSearchHit  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green bold

	// Pane borders.
	stylePaneBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	// Detail pane.
	styleDetailLabel = lipgloss.NewStyle().Bold(true).Width(12)

	// Status bar.
	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	styleStatusKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Background(lipgloss.Color("236")).
			Bold(true)
)

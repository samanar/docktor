package ui

import "github.com/charmbracelet/lipgloss"

// Theme holds all configurable colors for the application.
// No color values are hardcoded anywhere else in the app —
// every visual element references a Theme field.
type Theme struct {
	// Base
	Background lipgloss.Color
	Foreground lipgloss.Color

	// Pane borders
	BorderNormal   lipgloss.Color
	BorderFocused  lipgloss.Color
	BorderInactive lipgloss.Color

	// Tabs bar
	TabBarBackground lipgloss.Color
	TabActive        lipgloss.Color
	TabInactive      lipgloss.Color
	TabHighlight     lipgloss.Color

	// Action bar
	ActionBarBackground lipgloss.Color
	ActionKey           lipgloss.Color
	ActionLabel         lipgloss.Color
	ActionSeparator     lipgloss.Color

	// Table / content
	TableRowNormal   lipgloss.Color
	TableRowSelected lipgloss.Color
	TableHeader      lipgloss.Color
	RowSelected      lipgloss.Color // selected row background

	// Status colors
	StatusRunning  lipgloss.Color
	StatusStopped  lipgloss.Color
	StatusHealthy  lipgloss.Color
	StatusExited   lipgloss.Color

	// Misc
	ScrollbarTrack lipgloss.Color
	ScrollbarThumb lipgloss.Color
	DividerLine    lipgloss.Color
	TitleText      lipgloss.Color
}

// DefaultTheme returns the default color theme.
// This is the only place where "concrete" color values live.
func DefaultTheme() Theme {
	return Theme{
		Background: lipgloss.Color("0"), // terminal black
		Foreground: lipgloss.Color("7"), // terminal white

		BorderNormal:   lipgloss.Color("8"),  // bright black / grey
		BorderFocused:  lipgloss.Color("6"),  // cyan
		BorderInactive: lipgloss.Color("8"),

		TabBarBackground: lipgloss.Color("0"),
		TabActive:        lipgloss.Color("6"),  // cyan
		TabInactive:      lipgloss.Color("8"),  // grey
		TabHighlight:     lipgloss.Color("3"),  // yellow

		ActionBarBackground: lipgloss.Color("0"),
		ActionKey:           lipgloss.Color("3"), // yellow
		ActionLabel:         lipgloss.Color("7"), // white
		ActionSeparator:     lipgloss.Color("8"),

		TableRowNormal:   lipgloss.Color("7"),
		TableRowSelected: lipgloss.Color("0"), // will be used as bg
		TableHeader:      lipgloss.Color("6"),
		RowSelected:      lipgloss.Color("8"), // light gray

		StatusRunning: lipgloss.Color("2"),  // green
		StatusStopped: lipgloss.Color("1"),  // red
		StatusHealthy: lipgloss.Color("2"),  // green
		StatusExited:  lipgloss.Color("8"),  // grey

		ScrollbarTrack: lipgloss.Color("8"),
		ScrollbarThumb: lipgloss.Color("7"),
		DividerLine:    lipgloss.Color("8"),
		TitleText:      lipgloss.Color("6"),
	}
}

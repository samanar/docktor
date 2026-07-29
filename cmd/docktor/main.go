package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/samanar/docktor/internal/ui"
)

func main() {
	theme := ui.DefaultTheme()
	app, err := ui.NewApp(theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docktor: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "docktor: %v\n", err)
		os.Exit(1)
	}
}

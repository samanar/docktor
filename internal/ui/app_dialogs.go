package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/samanar/docktor/internal/docker"
)

func (m AppModel) renderGlobalActionBar(width int) string {
	type hint struct{ key, label string }
	hints := []hint{
		{"b", "Bulk Actions"},
		{"1", "List"},
		{"2", "Overview"},
		{"3", "Logs"},
		{"q", "Quit"},
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(m.theme.ActionKey).
		Bold(true).
		Background(m.theme.ActionBarBackground)

	labelStyle := lipgloss.NewStyle().
		Foreground(m.theme.ActionLabel).
		Background(m.theme.ActionBarBackground)

	sep := lipgloss.NewStyle().
		Foreground(m.theme.ActionSeparator).
		Background(m.theme.ActionBarBackground).
		Render("  ")

	var parts []string
	for _, h := range hints {
		parts = append(parts, keyStyle.Render(h.key)+":"+labelStyle.Render(h.label))
	}

	bar := strings.Join(parts, sep)

	return lipgloss.NewStyle().
		Width(width).
		Background(m.theme.ActionBarBackground).
		Padding(0, 1).
		Render(bar)
}

// ── Dialog overlay helper ────────────────────────────────────────

// overlayDialog splices the dialog string into the center of the main
// view without covering the entire screen.  It preserves the main view
// content around the dialog edges so the user can still see what is
// behind it (the panes are dimmed but visible).
func overlayDialog(mainView, dialog string, totalW, totalH int) string {
	mainLines := strings.Split(mainView, "\n")
	dialogLines := strings.Split(dialog, "\n")
	if len(dialogLines) == 0 {
		return mainView
	}

	dialogW := lipgloss.Width(dialogLines[0])
	dialogH := len(dialogLines)

	colOff := (totalW - dialogW) / 2
	if colOff < 0 {
		colOff = 0
	}
	rowOff := (totalH - dialogH) / 2
	if rowOff < 0 {
		rowOff = 0
	}

	// Pad mainLines so we have enough rows for the overlay.
	for len(mainLines) < totalH {
		mainLines = append(mainLines, "")
	}

	leftPad := strings.Repeat(" ", colOff)
	rightPadW := totalW - colOff - dialogW
	if rightPadW < 0 {
		rightPadW = 0
	}

	for i, dl := range dialogLines {
		targetRow := rowOff + i
		if targetRow >= len(mainLines) {
			break
		}
		// Build the overlaid row: spaces | dialog | truncated main content.
		right := mainLines[targetRow]
		right = ansiDrop(right, colOff+dialogW)
		if rightPadW > 0 {
			right = lipgloss.NewStyle().Width(rightPadW).Render(right)
		}
		mainLines[targetRow] = leftPad + dl + right
	}

	return strings.Join(mainLines, "\n")
}

// ansiDrop skips the first n visual columns of s, returning the
// remainder with any ANSI sequences preserved.  It also emits a
// reset sequence first so that colors from the skipped portion
// don't bleed into the result.
func ansiDrop(s string, n int) string {
	if n <= 0 {
		return s
	}
	visual := 0
	inEsc := false
	for i, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r >= '@' && r <= '~' {
				inEsc = false
			}
			continue
		}
		visual++
		if visual >= n {
			return s[i+len(string(r)):]
		}
	}
	return ""
}

// ── Bulk actions ─────────────────────────────────────────────────

// bulkActions is the ordered list of actions shown in the bulk
// actions dialog.
var bulkActions = []struct {
	label  string
	action func(docker.Client) (string, error)
}{
	{"Stop All Containers", func(dc docker.Client) (string, error) { return dc.StopAllContainers(context.Background()) }},
	{"Remove All Containers", func(dc docker.Client) (string, error) { return dc.RemoveAllContainers(context.Background()) }},
	{"Prune Exited Containers", func(dc docker.Client) (string, error) { return dc.PruneContainers(context.Background()) }},
}

// executeBulkAction runs the selected bulk action asynchronously
// and returns a tea.Cmd that delivers the result.
// Caller must set bulkActionLoading / bulkActionLabel / bulkSpinnerIdx
// before calling this method.
func (m AppModel) executeBulkAction(idx int) tea.Cmd {
	if idx < 0 || idx >= len(bulkActions) {
		return nil
	}
	a := bulkActions[idx]
	return tea.Batch(
		func() tea.Msg {
			output, err := a.action(m.dc)
			return bulkActionResultMsg{action: a.label, output: output, err: err}
		},
		spinnerTick(),
	)
}

// renderBulkDialog returns a styled modal dialog for bulk actions.
func (m AppModel) renderBulkDialog() string {
	dialogW := 40

	// ── Content ──────────────────────────────────────
	title := lipgloss.NewStyle().
		Foreground(m.theme.TabActive).
		Bold(true).
		Align(lipgloss.Center).
		Width(dialogW - 4).
		Render("Bulk Actions")

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", dialogW-4))

	var body string
	var footer string

	if m.bulkActionLoading {
		// ── Loading spinner ──────────────────────────
		spinner := spinnerFrames[m.bulkSpinnerIdx%len(spinnerFrames)]
		body = lipgloss.NewStyle().
			Foreground(m.theme.TabActive).
			Align(lipgloss.Center).
			Width(dialogW - 4).
			Render(spinner + " " + m.bulkActionLabel + "...")

		footer = lipgloss.NewStyle().
			Foreground(m.theme.TabInactive).
			Align(lipgloss.Center).
			Width(dialogW - 4).
			Render("please wait...")
	} else {
		// ── Build option lines ───────────────────────
		var options []string
		for i, a := range bulkActions {
			prefix := "  "
			if i == m.bulkDialogIdx {
				prefix = "▸ "
			}
			line := prefix + a.label
			if i == m.bulkDialogIdx {
				options = append(options, lipgloss.NewStyle().
					Foreground(m.theme.ActionKey).
					Bold(true).
					Render(line))
			} else {
				options = append(options, lipgloss.NewStyle().
					Foreground(m.theme.Foreground).
					Render(line))
			}
		}
		body = strings.Join(options, "\n")

		footer = lipgloss.NewStyle().
			Foreground(m.theme.TabInactive).
			Align(lipgloss.Center).
			Width(dialogW - 4).
			Render("↑↓ navigate  ↵ select  esc close")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		divider,
		body,
		"",
		footer,
	)

	return lipgloss.NewStyle().
		Width(dialogW).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.BorderFocused).
		Background(m.theme.Background).
		Render(content)
}

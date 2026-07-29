package ui

import (
	"fmt"
	"strings"
	"github.com/charmbracelet/lipgloss"
)

// ── View ─────────────────────────────────────────────────────────

func (m AppModel) View() string {
	if !m.ready {
		return "initializing..."
	}

	paneWidth := int(float64(m.width) * 0.70)
	rightWidth := m.width - paneWidth

	// Top panes (navigator + overview) take 1/3 of total height.
	// Bottom logs pane takes the remaining 2/3.
	paneHeight := m.height / 3
	if paneHeight < 5 {
		paneHeight = 5
	}

	// Ensure pane has correct dimensions
	m.pane.width = paneWidth
	m.pane.height = paneHeight

	paneView := m.pane.View()

	// ── Right pane (overview) ─────────────────────────
	rightBorder := m.theme.BorderInactive
	if m.focus == 2 {
		rightBorder = m.theme.BorderFocused
	}

	// Content area inside the border
	innerW := rightWidth - 2
	innerH := paneHeight - 2
	var rightContent string

	if m.pane.ActiveTabKey() == 'N' {
		// Network overview
		nw := m.pane.GetSelectedNetwork()
		if nw != nil {
			rightContent = m.renderNetworkOverview(innerW, innerH, nw)
		} else {
			rightContent = lipgloss.NewStyle().
				Width(innerW).Height(innerH).
				Align(lipgloss.Center, lipgloss.Center).
				Foreground(m.theme.TabInactive).
				Render("No network selected\n\n[2 to focus]")
		}
	} else if m.pane.ActiveTabKey() == 'i' {
		// Image overview
		img := m.pane.GetSelectedImage()
		if img != nil {
			rightContent = m.renderImageOverview(innerW, innerH, img)
		} else {
			rightContent = lipgloss.NewStyle().
				Width(innerW).Height(innerH).
				Align(lipgloss.Center, lipgloss.Center).
				Foreground(m.theme.TabInactive).
				Render("No image selected\n\n[2 to focus]")
		}
	} else if m.pane.ActiveTab() == 2 {
		// Volumes tab
		if m.selectedVolumeObj != nil {
			rightContent = m.renderVolumeOverview(innerW, innerH, m.selectedVolumeObj)
		} else {
			rightContent = lipgloss.NewStyle().
				Width(innerW).Height(innerH).
				Align(lipgloss.Center, lipgloss.Center).
				Foreground(m.theme.TabInactive).
				Render("No volume selected\n\n[2 to focus]")
		}
	} else {
		// Container overview — or group overview if a group header is selected.
		grp := m.pane.GetSelectedGroup()
		if grp != nil {
			rightContent = m.renderGroupOverview(innerW, innerH, grp)
		} else {
			ctr := m.pane.GetSelectedContainer()
			if ctr != nil {
				rightContent = m.renderOverview(innerW, innerH, ctr)
			} else {
				rightContent = lipgloss.NewStyle().
					Width(innerW).Height(innerH).
					Align(lipgloss.Center, lipgloss.Center).
					Foreground(m.theme.TabInactive).
					Render("No container selected\n\n[2 to focus]")
			}
		}
	}

	rightStyle := lipgloss.NewStyle().
		Width(rightWidth).
		Height(paneHeight).
		MaxHeight(paneHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(rightBorder).
		Background(m.theme.Background)

	rightView := rightStyle.Render(rightContent)

	// ── Bottom pane (logs / detail) ──────────────────
	// Reserve 1 line at the very bottom for the global action bar.
	globalBarH := 1
	bottomHeight := m.height - paneHeight - globalBarH
	var bottomView string
	if bottomHeight > 0 {
		bottomBorder := m.theme.BorderInactive
		if m.focus == 3 {
			bottomBorder = m.theme.BorderFocused
		}
		bottomStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(bottomHeight).
			MaxHeight(bottomHeight).
			Border(lipgloss.NormalBorder()).
			BorderForeground(bottomBorder).
			Background(m.theme.Background)

		bottomW := m.width - 2
		bottomH := bottomHeight - 2

		// Action bar at top when bottom pane is focused.
		actionBarH := 0
		if m.focus == 3 {
			actionBarH = 2 // action bar + divider
		}
		contentH := bottomH - actionBarH
		if contentH < 1 {
			contentH = 1
		}

		var bottomContent string
		if m.pane.ActiveTabKey() == 'N' {
			bottomContent = m.renderNetworkDetail(bottomW, contentH)
		} else if m.pane.ActiveTabKey() == 'i' && len(m.imageLayers) > 0 {
			bottomContent = m.renderImageLayers(bottomW, contentH)
		} else if m.pane.ActiveTab() == 2 && m.selectedVolume != "" {
			bottomContent = m.renderVolumeFileUsage(bottomW, contentH)
		} else {
			bottomContent = m.renderLogs(bottomW, contentH)
		}

		if m.focus == 3 {
			actionBar := m.renderLogActionBar(bottomW)
			divider := lipgloss.NewStyle().
				Foreground(m.theme.DividerLine).
				Render(strings.Repeat("─", bottomW))
			bottomContent = lipgloss.JoinVertical(lipgloss.Top, actionBar, divider, bottomContent)
		}

		bottomView = bottomStyle.Render(bottomContent)
	}

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, paneView, rightView)

	// ── Global action bar (always visible at the very bottom) ──
	globalBar := m.renderGlobalActionBar(m.width)

	var mainView string
	if bottomView != "" {
		mainView = lipgloss.JoinVertical(lipgloss.Top, topRow, bottomView, globalBar)
	} else {
		mainView = lipgloss.JoinVertical(lipgloss.Top, topRow, globalBar)
	}

	// ── Bulk actions dialog (centered overlay) ────────
	if m.bulkDialogOpen {
		dialog := m.renderBulkDialog()
		return overlayDialog(mainView, dialog, m.width, m.height)
	}

	return mainView
}

// ── Log viewer ───────────────────────────────────────────────────

func (m AppModel) renderLogs(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	if m.selectedName == "" {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.TabInactive).
			Render("Select a container to view logs")
	}

	// Reserve bottom row for status bar when follow or search is active.
	statusH := 0
	if m.followMode || m.logSearchQuery != "" {
		statusH = 1
	}
	logH := height - statusH
	if logH < 1 {
		logH = 1
	}

	// Clamp scroll offset
	maxOff := len(m.logLines) - logH
	if maxOff < 0 {
		maxOff = 0
	}
	if m.logScrollOff > maxOff {
		m.logScrollOff = maxOff
	}
	if m.logScrollOff < 0 {
		m.logScrollOff = 0
	}

	end := m.logScrollOff + logH
	if end > len(m.logLines) {
		end = len(m.logLines)
	}

	visible := m.logLines[m.logScrollOff:end]

	// Pad to full height
	for len(visible) < logH {
		visible = append(visible, "")
	}

	// Render visible lines with optional search highlighting
	lineStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width)

	matchStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("11")). // yellow highlight
		Bold(true)

	currentMatchStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("208")). // orange highlight
		Bold(true)

	var styled []string
	for i, line := range visible {
		styledLine := line
		lineIdx := m.logScrollOff + i

		// Highlight search matches within this line
		if m.logSearchQuery != "" {
			styledLine = highlightLine(line, m.logSearchQuery, lineIdx,
				m.logSearchMatches, m.logSearchMatchIdx,
				matchStyle, currentMatchStyle)
		}

		styled = append(styled, lineStyle.Render(styledLine))
	}

	result := strings.Join(styled, "\n")

	// ── Status bar (conditional) ─────────────────────
	if statusH > 0 {
		result += "\n" + m.renderLogStatusBar(width)
	}

	return result
}

// highlightLine highlights all occurrences of query in line using
// matchStyle. If lineIdx matches the current search match, it uses
// currentMatchStyle instead.
func highlightLine(
	line, query string,
	lineIdx int,
	matches []int,
	matchIdx int,
	matchStyle, currentMatchStyle lipgloss.Style,
) string {
	if query == "" {
		return line
	}

	lower := strings.ToLower(line)
	q := strings.ToLower(query)

	// Check if this line is the current match
	isCurrentMatch := false
	if matchIdx >= 0 && matchIdx < len(matches) && matches[matchIdx] == lineIdx {
		isCurrentMatch = true
	}

	var result strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], q)
		if idx < 0 {
			result.WriteString(line[pos:])
			break
		}
		absIdx := pos + idx
		// Text before the match
		result.WriteString(line[pos:absIdx])
		// The match itself
		if isCurrentMatch {
			result.WriteString(currentMatchStyle.Render(line[absIdx : absIdx+len(q)]))
		} else {
			result.WriteString(matchStyle.Render(line[absIdx : absIdx+len(q)]))
		}
		pos = absIdx + len(q)
	}

	return result.String()
}

// renderLogStatusBar draws the bottom bar of the log viewer showing
// follow-mode indicator and search match count.
func (m AppModel) renderLogStatusBar(width int) string {
	bg := lipgloss.NewStyle().
		Background(m.theme.ActionBarBackground).
		Width(width)

	var parts []string

	// Follow indicator
	if m.followMode {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Background(m.theme.ActionBarBackground).
			Render("● FOLLOWING"))
	} else {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(m.theme.TabInactive).
			Background(m.theme.ActionBarBackground).
			Render("○ paused"))
	}

	// Search match info
	if m.logSearchQuery != "" {
		info := fmt.Sprintf(" /%s", m.logSearchQuery)
		if len(m.logSearchMatches) > 0 {
			info += fmt.Sprintf(" [%d/%d]", m.logSearchMatchIdx+1, len(m.logSearchMatches))
		} else {
			info += " [no matches]"
		}
		parts = append(parts, lipgloss.NewStyle().
			Foreground(m.theme.Foreground).
			Background(m.theme.ActionBarBackground).
			Render(info))
	}

	// Line count
	countStr := fmt.Sprintf("lines: %d", len(m.logLines))
	parts = append(parts, lipgloss.NewStyle().
		Foreground(m.theme.TabInactive).
		Background(m.theme.ActionBarBackground).
		Render(countStr))

	return bg.Render(strings.Join(parts, " │ "))
}

// renderLogActionBar draws keybinding hints at the very bottom of
// the log pane, matching the style of the navigator's action bar.
func (m AppModel) renderLogActionBar(width int) string {
	type hint struct{ key, label string }
	hints := []hint{
		{"F", "Follow"},
		{"/", "Search"},
		{"n", "Next"},
		{"N", "Prev"},
		{"jk", "Scroll"},
		{"Esc", "Clear"},
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

// renderGlobalActionBar draws a persistent keybinding bar at the
// very bottom of the screen, visible regardless of which pane
// has focus.  Modeled after lazy docker's bottom status bar.

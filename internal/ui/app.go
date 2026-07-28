package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/samanar/lazycompose/internal/docker"
)

// AppModel is the root Bubble Tea model for the docktor application.
// It owns three panes: navigator (left), overview (right), logs (bottom).
type AppModel struct {
	theme Theme
	pane  Pane
	dc    *docker.Client

	width  int
	height int
	ready  bool

	// Focus: 1 = navigator, 2 = overview, 3 = logs
	focus int

	// Log / detail viewer state
	logLines      []string
	logScrollOff  int
	logAutoScroll bool
	selectedName  string

	// Image layer viewer state
	imageLayers     []docker.ImageLayer
	selectedImageID string

	// Network detail state
	selectedNetworkName     string
	networkDetailLines      []string
	networkDetailScrollOff  int
	networkDetailAutoScroll bool

	// Volume viewer state
	selectedVolume    string
	volumeFileUsage   []docker.VolumeFileUsage
	volumeUsageErr    error
	selectedVolumeObj *docker.Volume

	// ── Follow (streaming logs) ──────────────────────
	followMode bool               // true = streaming logs live
	logCancel  context.CancelFunc // cancels the docker logs -f subprocess
	logLineCh  chan string        // new log lines from the goroutine

	// ── Log search ───────────────────────────────────
	logSearchMode     bool   // true when / search is active in logs
	logSearchQuery    string // current search term
	logSearchMatches  []int  // indices into logLines that match
	logSearchMatchIdx int    // current position within matches
}

// NewApp creates the root application model with the given theme.
func NewApp(theme Theme) AppModel {
	dc := docker.NewClient()
	return AppModel{
		theme:         theme,
		pane:          NewPane(theme, dc),
		dc:            dc,
		focus:         1,
		logAutoScroll: true,
	}
}

// ── Custom messages ──────────────────────────────────────────────

// logsLoadedMsg is sent when docker logs have been fetched.
type logsLoadedMsg struct {
	containerName string
	logs          string
	err           error
}

// imageSizeLoadedMsg is sent when the image size for a container
// has been fetched.
type imageSizeLoadedMsg struct {
	containerName string
	imageSize     string
}

// imageLayersLoadedMsg is sent when image history (layers) has been
// fetched.
type imageLayersLoadedMsg struct {
	imageID string
	layers  []docker.ImageLayer
	err     error
}

// volumeUsageLoadedMsg is sent when the per-file/folder usage
// for a volume has been fetched.
type volumeUsageLoadedMsg struct {
	volumeName string
	entries    []docker.VolumeFileUsage
	err        error
}

// logLineMsg carries a single line from a running docker logs -f
// stream. Sent by the follow-logs goroutine.
type logLineMsg struct {
	containerName string
	line          string
}

// logStreamEndedMsg is sent when the docker logs -f subprocess exits.
type logStreamEndedMsg struct {
	containerName string
	err           error
}

// ── Bubble Tea Model ─────────────────────────────────────────────

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.pane.Init(),
		tea.EnterAltScreen,
	)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Set pane dimensions so it can compute correct table height.
		// Pane takes 70% width and shares top 1/3 of height with overview.
		m.pane.width = int(float64(msg.Width) * 0.70)
		paneH := msg.Height / 3
		if paneH < 5 {
			paneH = 5
		}
		m.pane.height = paneH
		var cmd tea.Cmd
		m.pane, cmd = m.pane.Update(msg)
		return m, cmd

	// ── Logs arrived ─────────────────────────────────
	case logsLoadedMsg:
		if msg.containerName == m.selectedName {
			if msg.err != nil {
				m.logLines = []string{"Error fetching logs: " + msg.err.Error()}
				m.logAutoScroll = false
			} else {
				m.logLines = strings.Split(msg.logs, "\n")
				if m.logAutoScroll {
					m.scrollLogsToEnd()
				}
			}
		}
		return m, nil

	// ── Streamed log line arrived ────────────────────
	case logLineMsg:
		m.logLines = append(m.logLines, msg.line)
		// Rebuild search matches if search is active
		if m.logSearchQuery != "" {
			m.rebuildLogSearchMatches()
		}
		if m.logAutoScroll {
			m.scrollLogsToEnd()
		}
		// Keep reading more lines
		return m, waitForLogLine(m.logLineCh)

	// ── Log stream ended ─────────────────────────────
	case logStreamEndedMsg:
		m.followMode = false
		return m, nil

	// ── Image size arrived ───────────────────────────
	case imageSizeLoadedMsg:
		m.pane.SetContainerImageSize(msg.containerName, msg.imageSize)
		return m, nil

	// ── Volume file usage arrived ────────────────────
	case volumeUsageLoadedMsg:
		if msg.volumeName == m.selectedVolume {
			if msg.err != nil {
				m.volumeUsageErr = msg.err
				m.volumeFileUsage = nil
			} else {
				m.volumeUsageErr = nil
				m.volumeFileUsage = msg.entries
			}
		}
		return m, nil

	// ── Container action completed ───────────────────
	case actionExecutedMsg:
		// Refresh container data after start/stop/restart/kill
		if msg.err != nil {
			// Show error briefly in the pane
			m.pane.lastError = fmt.Sprintf("Failed to %s %s: %v", msg.action, msg.name, msg.err)
		}
		// Compose actions clear the loading spinner immediately.
		if strings.HasPrefix(msg.action, "compose-") {
			m.pane.ClearComposeLoading()
		}
		return m, m.pane.Init()

	// ── Image layers arrived ─────────────────────────
	case imageLayersLoadedMsg:
		if msg.imageID == m.selectedImageID {
			if msg.err != nil {
				m.imageLayers = []docker.ImageLayer{
					{Command: "Error: " + msg.err.Error(), Size: ""},
				}
			} else {
				m.imageLayers = msg.layers
			}
			m.logLines = nil // clear container logs
		}
		return m, nil

	// ── Network inspect arrived ──────────────────────
	case networkInspectLoadedMsg:
		if msg.name == m.selectedNetworkName {
			if msg.err != nil {
				m.networkDetailLines = []string{"Error inspecting network: " + msg.err.Error()}
				m.networkDetailAutoScroll = false
			} else {
				m.buildNetworkDetail(msg.name, msg.json)
				if m.networkDetailAutoScroll {
					m.scrollNetworkDetailToEnd()
				}
			}
		}
		return m, nil

	// ── Network action completed ─────────────────────
	case networkActionExecutedMsg:
		if msg.err != nil {
			m.pane.lastError = fmt.Sprintf("Network %s failed: %v", msg.action, msg.err)
		}
		// Refresh pane after network prune
		return m, m.pane.Init()

	// ── Keyboard ─────────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

		// ── Focus switching ──────────────────────────
		switch msg.String() {
		case "1":
			m.focus = 1
			m.pane.focused = true
			return m, nil
		case "2":
			m.focus = 2
			m.pane.focused = false
			return m, nil
		case "3":
			m.focus = 3
			m.pane.focused = false
			return m, nil
		case "tab":
			m.focus = (m.focus % 3) + 1
			m.pane.focused = (m.focus == 1)
			return m, nil
		}

		// ── Pane navigation (focus 1) ─────────────────
		if m.focus == 1 {
			// Enter on a data row (not a group header) should
			// move focus to the logs / detail pane.
			isEnter := msg.Type == tea.KeyEnter

			prevContainer := m.selectedName
			prevNetwork := m.selectedNetworkName
			prevImgID := m.selectedImageID
			prevVol := m.selectedVolume
			var cmd tea.Cmd
			m.pane, cmd = m.pane.Update(msg)

			// Enter on a container / image / volume / network row
			// switches focus to the bottom detail pane.
			if isEnter {
				if m.pane.SelectedContainer() != "" ||
					m.pane.SelectedImage() != "" ||
					m.pane.SelectedVolume() != "" ||
					m.pane.SelectedNetwork() != "" {
					m.focus = 3
					m.pane.focused = false
					return m, cmd
				}
			}

			// Check for container selection change
			newContainer := m.pane.SelectedContainer()
			if newContainer != "" && newContainer != prevContainer {
				m.stopFollowLogs()
				m.clearLogSearch()
				m.selectedName = newContainer
				m.selectedNetworkName = ""
				m.selectedImageID = ""
				m.imageLayers = nil
				m.selectedVolume = ""
				m.selectedVolumeObj = nil
				m.volumeFileUsage = nil
				m.logAutoScroll = true
				return m, tea.Batch(cmd,
					fetchLogs(m.dc, newContainer),
					fetchDiskUsage(m.dc, newContainer),
				)
			}

			// Check for network selection change
			newNetwork := m.pane.SelectedNetwork()
			if newNetwork != "" && newNetwork != prevNetwork {
				m.selectedNetworkName = newNetwork
				m.selectedName = ""
				m.networkDetailAutoScroll = true
				return m, tea.Batch(cmd,
					fetchNetworkInspect(m.dc, newNetwork),
				)
			}

			// Check image selection
			newImgID := m.pane.SelectedImage()
			if newImgID != "" && newImgID != prevImgID {
				m.selectedImageID = newImgID
				m.selectedName = ""
				m.selectedNetworkName = ""
				m.selectedVolume = ""
				m.selectedVolumeObj = nil
				m.logLines = nil
				m.logAutoScroll = true
				return m, tea.Batch(cmd,
					fetchImageHistory(m.dc, newImgID),
				)
			}

			// Check for volume selection change
			newVol := m.pane.SelectedVolume()
			if newVol != "" && newVol != prevVol {
				m.selectedVolume = newVol
				m.selectedName = ""
				m.selectedImageID = ""
				m.selectedNetworkName = ""
				m.imageLayers = nil
				m.logAutoScroll = true
				m.logLines = nil
				m.volumeUsageErr = nil
				m.selectedVolumeObj = m.pane.FindVolume(newVol)
				return m, tea.Batch(cmd,
					fetchVolumeUsage(m.dc, newVol),
				)
			}

			return m, cmd
		}

		// ── Log / detail scrolling (focus 3) ───────────
		if m.focus == 3 {
			// Handle search mode in logs
			if m.logSearchMode {
				switch msg.Type {
				case tea.KeyEscape:
					m.logSearchMode = false
					return m, nil
				case tea.KeyEnter:
					m.logSearchMode = false
					return m, nil
				case tea.KeyBackspace:
					if len(m.logSearchQuery) > 0 {
						m.logSearchQuery = m.logSearchQuery[:len(m.logSearchQuery)-1]
						m.doLogSearch()
					}
					return m, nil
				case tea.KeyRunes:
					m.logSearchQuery += string(msg.Runes)
					m.doLogSearch()
					return m, nil
				}
				return m, nil
			}

			if m.pane.ActiveTabKey() == 'N' {
				// Network detail scrolling
				switch msg.String() {
				case "j", "down":
					m.scrollNetworkDetailDown()
				case "k", "up":
					m.scrollNetworkDetailUp()
				case "g":
					m.networkDetailScrollOff = 0
					m.networkDetailAutoScroll = false
				case "G":
					m.networkDetailAutoScroll = true
					m.scrollNetworkDetailToEnd()
				case "ctrl+d":
					m.scrollNetworkDetailDownHalf()
				case "ctrl+u":
					m.scrollNetworkDetailUpHalf()
				}
			} else {
				// Container log scrolling
				switch msg.String() {
				case "j", "down":
					m.scrollLogsDown()
				case "k", "up":
					m.scrollLogsUp()
				case "g":
					m.logScrollOff = 0
					m.logAutoScroll = false
				case "G":
					m.logAutoScroll = true
					m.scrollLogsToEnd()
				case "ctrl+d":
					m.scrollLogsDownHalf()
				case "ctrl+u":
					m.scrollLogsUpHalf()
				// ── Follow mode ──────────────────────
				case "F":
					if m.followMode {
						m.stopFollowLogs()
					} else {
						return m, m.startFollowLogs()
					}
				// ── Log search ──────────────────────
				case "/":
					m.logSearchMode = true
					m.logSearchQuery = ""
					m.logSearchMatches = nil
					m.logSearchMatchIdx = 0
					return m, nil
				case "n":
					m.nextLogSearchMatch()
				case "N":
					m.prevLogSearchMatch()
				case "esc":
					m.logLines = nil
					m.clearLogSearch()
				}
			}
			return m, nil
		}

		return m, nil
	}

	// ── All other messages → pane (stats, data, etc.) ─
	prevContainer := m.selectedName
	prevNetwork := m.selectedNetworkName
	prevImgID := m.selectedImageID
	prevVol := m.selectedVolume
	var cmd tea.Cmd
	m.pane, cmd = m.pane.Update(msg)

	newContainer := m.pane.SelectedContainer()
	if newContainer != "" && newContainer != prevContainer {
		m.stopFollowLogs()
		m.clearLogSearch()
		m.selectedName = newContainer
		m.selectedNetworkName = ""
		m.selectedImageID = ""
		m.imageLayers = nil
		m.selectedVolume = ""
		m.selectedVolumeObj = nil
		m.volumeFileUsage = nil
		m.logAutoScroll = true
		return m, tea.Batch(cmd,
			fetchLogs(m.dc, newContainer),
			fetchDiskUsage(m.dc, newContainer),
		)
	}

	newNetwork := m.pane.SelectedNetwork()
	if newNetwork != "" && newNetwork != prevNetwork {
		m.selectedNetworkName = newNetwork
		m.selectedName = ""
		m.networkDetailAutoScroll = true
		return m, tea.Batch(cmd,
			fetchNetworkInspect(m.dc, newNetwork),
		)
	}

	newImgID := m.pane.SelectedImage()
	if newImgID != "" && newImgID != prevImgID {
		m.selectedImageID = newImgID
		m.selectedName = ""
		m.logLines = nil
		m.logAutoScroll = true
		return m, tea.Batch(cmd,
			fetchImageHistory(m.dc, newImgID),
		)
	}

	// Check for volume selection change
	newVol := m.pane.SelectedVolume()
	if newVol != "" && newVol != prevVol {
		m.selectedVolume = newVol
		m.selectedName = ""
		m.logAutoScroll = true
		m.logLines = nil
		m.volumeUsageErr = nil
		m.selectedVolumeObj = m.pane.FindVolume(newVol)
		return m, tea.Batch(cmd,
			fetchVolumeUsage(m.dc, newVol),
		)
	}

	return m, cmd
}

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
	bottomHeight := m.height - paneHeight
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

	if bottomView != "" {
		return lipgloss.JoinVertical(lipgloss.Top, topRow, bottomView)
	}
	return topRow
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

// ── Network overview pane ────────────────────────────────────────

func (m AppModel) renderNetworkOverview(width, height int, nw *docker.Network) string {
	if width < 10 || height < 3 {
		return ""
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(m.theme.TabInactive).
		Width(14)

	valueStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width - 16)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true).
		Width(width)

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	row := func(label, value string) string {
		l := labelStyle.Render(label)
		v := valueStyle.Render(value)
		return l + v
	}

	// Boolean badge: green ✓ or grey ✗
	boolStyle := func(v bool) lipgloss.Style {
		if v {
			return lipgloss.NewStyle().Foreground(m.theme.StatusRunning)
		}
		return lipgloss.NewStyle().Foreground(m.theme.TabInactive)
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render(nw.Name))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Fields
	b.WriteString(row("ID:", truncateStr(nw.ID, 40)))
	b.WriteString("\n")
	b.WriteString(row("Driver:", nw.Driver))
	b.WriteString("\n")
	b.WriteString(row("Scope:", nw.Scope))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// IPAM section
	b.WriteString(titleStyle.Render("IPAM"))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	if nw.Subnet != "" {
		b.WriteString(row("Subnet:", nw.Subnet))
		b.WriteString("\n")
	}
	if nw.Gateway != "" {
		b.WriteString(row("Gateway:", nw.Gateway))
		b.WriteString("\n")
	}
	if nw.IPRange != "" {
		b.WriteString(row("IP Range:", nw.IPRange))
		b.WriteString("\n")
	}
	if nw.Subnet == "" && nw.Gateway == "" {
		b.WriteString(row("", "(no IPAM config)"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Flags section
	b.WriteString(titleStyle.Render("Flags"))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	internalStr := boolStyle(nw.Internal).Render("✗")
	if nw.Internal {
		internalStr = boolStyle(true).Render("✓")
	}
	b.WriteString(row("Internal:", internalStr))
	b.WriteString("\n")

	ipv6Str := boolStyle(nw.IPv6).Render("✗")
	if nw.IPv6 {
		ipv6Str = boolStyle(true).Render("✓")
	}
	b.WriteString(row("IPv6:", ipv6Str))
	b.WriteString("\n")

	attachStr := boolStyle(nw.Attachable).Render("✗")
	if nw.Attachable {
		attachStr = boolStyle(true).Render("✓")
	}
	b.WriteString(row("Attachable:", attachStr))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Summary
	b.WriteString(titleStyle.Render("Summary"))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	b.WriteString(row("Containers:", fmt.Sprintf("%d connected", len(nw.Containers))))
	b.WriteString("\n")
	if nw.Created != "" {
		b.WriteString(row("Created:", relativeTime(nw.Created)))
		b.WriteString("\n")
	}

	// Labels
	if len(nw.Labels) > 0 {
		b.WriteString("\n")
		b.WriteString(titleStyle.Render("Labels"))
		b.WriteString("\n")
		b.WriteString(divider)
		b.WriteString("\n\n")
		for k, v := range nw.Labels {
			b.WriteString(row(k+":", v))
			b.WriteString("\n")
		}
	}

	result := b.String()
	lines := strings.Split(result, "\n")

	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ── Network detail pane (bottom) ──────────────────────────────────

// buildNetworkDetail constructs the combined scrollable content
// for the bottom pane: connected containers table + JSON inspect.
func (m *AppModel) buildNetworkDetail(name string, rawJSON string) {
	var lines []string

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true)

	divider := strings.Repeat("─", 60)

	// Find the network to list connected containers
	var nw *docker.Network
	for _, g := range m.pane.networks {
		for i := range g.Networks {
			if g.Networks[i].Name == name {
				nw = &g.Networks[i]
				break
			}
		}
	}

	// ── Connected containers section ─────────────────
	lines = append(lines, titleStyle.Render("CONNECTED CONTAINERS"))
	lines = append(lines, divider)

	if nw != nil && len(nw.Containers) > 0 {
		// Mini table: NAME  |  IPv4  |  MAC
		header := fmt.Sprintf("%-30s %-20s %-18s", "NAME", "IPv4", "MAC")
		lines = append(lines, header)
		lines = append(lines, strings.Repeat("─", 68))

		for _, ctr := range nw.Containers {
			name := ctr.Name
			if len(name) > 30 {
				name = name[:29] + "…"
			}
			line := fmt.Sprintf("%-30s %-20s %-18s", name, ctr.IPv4Addr, ctr.MACAddr)
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, "(no containers connected)")
	}

	// ── JSON inspect section ─────────────────────────
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render("INSPECT JSON"))
	lines = append(lines, divider)

	// Append raw JSON lines
	for _, l := range strings.Split(rawJSON, "\n") {
		lines = append(lines, l)
	}

	m.networkDetailLines = lines
}

// renderNetworkDetail renders the scrollable network detail pane.
func (m AppModel) renderNetworkDetail(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	if m.selectedNetworkName == "" {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.TabInactive).
			Render("Select a network to view details")
	}

	if len(m.networkDetailLines) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.TabInactive).
			Render("Loading network details…")
	}

	// Clamp scroll offset
	maxOff := len(m.networkDetailLines) - height
	if maxOff < 0 {
		maxOff = 0
	}
	if m.networkDetailScrollOff > maxOff {
		m.networkDetailScrollOff = maxOff
	}
	if m.networkDetailScrollOff < 0 {
		m.networkDetailScrollOff = 0
	}

	end := m.networkDetailScrollOff + height
	if end > len(m.networkDetailLines) {
		end = len(m.networkDetailLines)
	}

	visible := m.networkDetailLines[m.networkDetailScrollOff:end]
	for len(visible) < height {
		visible = append(visible, "")
	}

	lineStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width)

	var styled []string
	for _, line := range visible {
		styled = append(styled, lineStyle.Render(line))
	}

	return strings.Join(styled, "\n")
}

// ── Network detail scrolling ──────────────────────────────────────

func (m *AppModel) scrollNetworkDetailToEnd() {
	visible := len(m.networkDetailLines)
	if visible == 0 {
		m.networkDetailScrollOff = 0
		return
	}
	avail := m.logViewHeight()
	m.networkDetailScrollOff = visible - avail
	if m.networkDetailScrollOff < 0 {
		m.networkDetailScrollOff = 0
	}
}

func (m *AppModel) scrollNetworkDetailDown() {
	maxOff := len(m.networkDetailLines) - 1
	if m.networkDetailScrollOff < maxOff {
		m.networkDetailScrollOff++
	}
	avail := m.logViewHeight()
	if m.networkDetailScrollOff >= len(m.networkDetailLines)-avail {
		m.networkDetailAutoScroll = true
	}
}

func (m *AppModel) scrollNetworkDetailUp() {
	m.networkDetailAutoScroll = false
	if m.networkDetailScrollOff > 0 {
		m.networkDetailScrollOff--
	}
}

func (m *AppModel) scrollNetworkDetailDownHalf() {
	avail := m.logViewHeight()
	if avail < 6 {
		avail = 6
	}
	half := avail / 2
	maxOff := len(m.networkDetailLines) - 1
	m.networkDetailScrollOff += half
	if m.networkDetailScrollOff > maxOff {
		m.networkDetailScrollOff = maxOff
	}
}

func (m *AppModel) scrollNetworkDetailUpHalf() {
	m.networkDetailAutoScroll = false
	avail := m.logViewHeight()
	if avail < 6 {
		avail = 6
	}
	half := avail / 2
	m.networkDetailScrollOff -= half
	if m.networkDetailScrollOff < 0 {
		m.networkDetailScrollOff = 0
	}
}

func (m AppModel) renderOverview(width, height int, ctr *docker.Container) string {
	if width < 10 || height < 3 {
		return ""
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(m.theme.TabInactive).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width - 14)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true).
		Width(width)

	// Status colour
	var stateStyle lipgloss.Style
	switch ctr.State {
	case "running", "healthy":
		stateStyle = lipgloss.NewStyle().Foreground(m.theme.StatusRunning).Bold(true)
	case "exited", "dead", "removing":
		stateStyle = lipgloss.NewStyle().Foreground(m.theme.StatusStopped).Bold(true)
	case "paused":
		stateStyle = lipgloss.NewStyle().Foreground(m.theme.TabHighlight).Bold(true)
	default:
		stateStyle = lipgloss.NewStyle().Foreground(m.theme.TabInactive)
	}

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	row := func(label, value string) string {
		l := labelStyle.Render(label)
		v := valueStyle.Render(value)
		return l + v
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render(shortenName(ctr.Name)))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Fields
	b.WriteString(row("ID:", truncateStr(ctr.ID, 16)))
	b.WriteString("\n")
	b.WriteString(row("Image:", ctr.Image))
	b.WriteString("\n")
	b.WriteString(row("State:", stateStyle.Render(ctr.State)))
	b.WriteString("\n")
	b.WriteString(row("Status:", ctr.Status))
	b.WriteString("\n")

	if ctr.Project != "" {
		b.WriteString(row("Project:", ctr.Project))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Resource usage
	b.WriteString(titleStyle.Render("Resources"))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	b.WriteString(row("CPU:", ctr.CPU))
	b.WriteString("\n")
	b.WriteString(row("Memory:", ctr.Memory))
	b.WriteString("\n")
	if ctr.NetIO != "" && ctr.NetIO != "0B / 0B" {
		b.WriteString(row("Network:", formatNetIO(ctr.NetIO)))
		b.WriteString("\n")
	}
	b.WriteString(row("Disk:", ctr.ImageSize))
	b.WriteString("\n")
	b.WriteString(row("Ports:", formatPorts(ctr.Ports)))
	b.WriteString("\n")

	// Times
	if ctr.CreatedAt != "" || ctr.StartedAt != "" {
		b.WriteString("\n")
		b.WriteString(divider)
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render("Timestamps"))
		b.WriteString("\n")
		b.WriteString(divider)
		b.WriteString("\n\n")
	}

	if ctr.CreatedAt != "" {
		b.WriteString(row("Created:", relativeTime(ctr.CreatedAt)))
		b.WriteString("\n")
	}
	if ctr.StartedAt != "" {
		b.WriteString(row("Started:", relativeTime(ctr.StartedAt)))
		b.WriteString("\n")
	}

	result := b.String()
	lines := strings.Split(result, "\n")

	// Trim or pad to fit the available height
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ── Group / Compose overview ──────────────────────────────────────

func (m AppModel) renderGroupOverview(width, height int, grp *docker.ContainerGroup) string {
	if width < 10 || height < 3 {
		return ""
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(m.theme.TabInactive).
		Width(10)

	valueStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width - 12)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true).
		Width(width)

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	row := func(label, value string) string {
		l := labelStyle.Render(label)
		v := valueStyle.Render(value)
		return l + v
	}

	// Status styles
	green := lipgloss.NewStyle().Foreground(m.theme.StatusRunning)
	red := lipgloss.NewStyle().Foreground(m.theme.StatusStopped)
	grey := lipgloss.NewStyle().Foreground(m.theme.TabInactive)

	var b strings.Builder

	// ── Title ────────────────────────────────────────
	label := grp.Project
	if label == "" {
		label = "Other"
	}
	b.WriteString(titleStyle.Render(label))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// ── Compose file ─────────────────────────────────
	if grp.ComposeFile != "" {
		b.WriteString(row("Compose:", grp.ComposeFile))
		b.WriteString("\n")
	}

	// ── Counts ───────────────────────────────────────
	running, healthy, exited, stopped := 0, 0, 0, 0
	for _, c := range grp.Containers {
		switch c.State {
		case "running":
			running++
		case "healthy":
			healthy++
		case "exited", "dead", "removing":
			exited++
		default:
			stopped++
		}
	}
	up := running + healthy
	total := len(grp.Containers)

	var countParts []string
	if up > 0 {
		countParts = append(countParts, green.Render(fmt.Sprintf("%d up", up)))
	}
	if exited > 0 {
		countParts = append(countParts, red.Render(fmt.Sprintf("%d exited", exited)))
	}
	if stopped > 0 {
		countParts = append(countParts, grey.Render(fmt.Sprintf("%d stopped", stopped)))
	}
	b.WriteString(row("Total:", fmt.Sprintf("%d containers", total)))
	b.WriteString("\n")
	if len(countParts) > 0 {
		b.WriteString(row("Status:", strings.Join(countParts, "  ")))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// ── Aggregate resources ──────────────────────────
	totalCPU := 0.0
	var totalMemUsed uint64
	for _, c := range grp.Containers {
		totalCPU += parseCPUPercent(c.CPU)
		used, _ := parseMemoryUsage(c.Memory)
		totalMemUsed += used
	}
	if totalCPU > 0 || totalMemUsed > 0 {
		b.WriteString(titleStyle.Render("Resources"))
		b.WriteString("\n")
		b.WriteString(divider)
		b.WriteString("\n\n")
		b.WriteString(row("CPU:", fmt.Sprintf("%.1f%% total", totalCPU)))
		b.WriteString("\n")
		b.WriteString(row("Memory:", formatBytes(totalMemUsed)))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(divider)
		b.WriteString("\n\n")
	}

	// ── Container list ───────────────────────────────
	b.WriteString(titleStyle.Render("Containers"))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Compact container rows: dot name  cpu  mem
	nameW := width - 30
	if nameW < 8 {
		nameW = 8
	}
	cpuW := 8
	memW := 16

	for _, c := range grp.Containers {
		// Status dot
		var dot string
		switch c.State {
		case "running", "healthy":
			dot = green.Render("●")
		case "exited", "dead", "removing":
			dot = red.Render("●")
		default:
			dot = grey.Render("●")
		}

		short := shortenName(c.Name)
		if len(short) > nameW {
			short = short[:nameW]
		}
		nameStr := lipgloss.NewStyle().Foreground(m.theme.Foreground).Width(nameW).Render(short)
		cpuStr := lipgloss.NewStyle().Foreground(m.theme.Foreground).Width(cpuW).Render(c.CPU)
		memStr := lipgloss.NewStyle().Foreground(m.theme.TabInactive).Width(memW).Render(memUsedPortion(c.Memory))

		b.WriteString(fmt.Sprintf(" %s %s %s %s\n", dot, nameStr, cpuStr, memStr))
	}

	// Pad to height
	result := b.String()
	lines := strings.Split(result, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ── Image overview ────────────────────────────────────────────────

func (m AppModel) renderImageOverview(width, height int, img *docker.Image) string {
	if width < 10 || height < 3 {
		return ""
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(m.theme.TabInactive).
		Width(10)

	valueStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width - 12)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true).
		Width(width)

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	row := func(label, value string) string {
		return labelStyle.Render(label) + valueStyle.Render(value)
	}

	var b strings.Builder

	// Title
	title := img.Repo
	if img.Tag != "<none>" && img.Tag != "" {
		title += ":" + img.Tag
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Fields
	b.WriteString(row("ID:", truncateStr(img.ID, 20)))
	b.WriteString("\n")
	b.WriteString(row("Size:", img.Size))
	b.WriteString("\n")
	b.WriteString(row("Created:", img.Created))
	b.WriteString("\n")
	b.WriteString(row("Repo:", img.Repo))
	b.WriteString("\n")
	b.WriteString(row("Tag:", img.Tag))
	b.WriteString("\n")

	result := b.String()
	lines := strings.Split(result, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// ── Image layers viewer ───────────────────────────────────────────

func (m AppModel) renderImageLayers(width, height int) string {
	if width < 10 || height < 1 {
		return ""
	}

	if len(m.imageLayers) == 0 {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.TabInactive).
			Render("Loading image layers...")
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(m.theme.TableHeader).
		Bold(true)

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	idW := 14
	createdW := 10
	sizeW := 8
	cmdW := width - idW - createdW - sizeW - 6
	if cmdW < 10 {
		cmdW = 10
	}

	// Header
	header := headerStyle.Render(
		fitStr("LAYER ID", idW) + "  " +
			fitStr("CREATED", createdW) + "  " +
			fitStr("SIZE", sizeW) + "  " +
			fitStr("COMMAND", cmdW))

	idStyle := lipgloss.NewStyle().Foreground(m.theme.TabHighlight)
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.TabInactive)
	bodyStyle := lipgloss.NewStyle().Foreground(m.theme.Foreground)

	// Clamp scroll offset
	maxOff := len(m.imageLayers) - height + 1
	if maxOff < 0 {
		maxOff = 0
	}
	if m.logScrollOff > maxOff {
		m.logScrollOff = maxOff
	}
	if m.logScrollOff < 0 {
		m.logScrollOff = 0
	}

	end := m.logScrollOff + height - 2 // -2 for header + divider
	if end > len(m.imageLayers) {
		end = len(m.imageLayers)
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n")

	for i := m.logScrollOff; i < end; i++ {
		l := m.imageLayers[i]
		shortID := l.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		line := idStyle.Render(fitStr(shortID, idW)) + "  " +
			dimStyle.Render(fitStr(l.Created, createdW)) + "  " +
			dimStyle.Render(fitStr(l.Size, sizeW)) + "  " +
			bodyStyle.Render(fitStr(l.Command, cmdW))
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Pad
	rendered := end - m.logScrollOff + 2
	for i := rendered; i < height; i++ {
		b.WriteString(strings.Repeat(" ", width))
		b.WriteString("\n")
	}

	return b.String()
}

func (m *AppModel) scrollLogsToEnd() {
	visible := len(m.logLines)
	if visible == 0 {
		m.logScrollOff = 0
		return
	}
	avail := m.logViewHeight()
	m.logScrollOff = visible - avail
	if m.logScrollOff < 0 {
		m.logScrollOff = 0
	}
}

func (m *AppModel) scrollLogsDown() {
	maxOff := len(m.logLines) - 1
	if m.logScrollOff < maxOff {
		m.logScrollOff++
	}
	// If we reach the bottom, re-enable auto-scroll
	avail := m.logViewHeight()
	if m.logScrollOff >= len(m.logLines)-avail {
		m.logAutoScroll = true
	}
}

func (m *AppModel) scrollLogsUp() {
	m.logAutoScroll = false
	if m.logScrollOff > 0 {
		m.logScrollOff--
	}
}

func (m *AppModel) scrollLogsDownHalf() {
	avail := m.logViewHeight()
	if avail < 6 {
		avail = 6
	}
	half := avail / 2
	maxOff := len(m.logLines) - 1
	m.logScrollOff += half
	if m.logScrollOff > maxOff {
		m.logScrollOff = maxOff
	}
}

func (m *AppModel) scrollLogsUpHalf() {
	m.logAutoScroll = false
	avail := m.logViewHeight()
	if avail < 6 {
		avail = 6
	}
	half := avail / 2
	m.logScrollOff -= half
	if m.logScrollOff < 0 {
		m.logScrollOff = 0
	}
}

// ── Volume overview ─────────────────────────────────────────────

// renderVolumeOverview draws the volume details pane (right side)
// when the volumes tab is active.
func (m AppModel) renderVolumeOverview(width, height int, vol *docker.Volume) string {
	if width < 10 || height < 3 {
		return ""
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(m.theme.TabInactive).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width - 14)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true).
		Width(width)

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	row := func(label, value string) string {
		l := labelStyle.Render(label)
		v := valueStyle.Render(value)
		return l + v
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render(truncateStr(vol.Name, 30)))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n\n")

	// Fields
	b.WriteString(row("Driver:", vol.Driver))
	b.WriteString("\n")
	b.WriteString(row("Mountpoint:", vol.Mountpoint))
	b.WriteString("\n")
	b.WriteString(row("Size:", vol.Size))
	b.WriteString("\n")

	// File count
	if len(m.volumeFileUsage) > 0 {
		b.WriteString(row("Files:", fmt.Sprintf("%d items", len(m.volumeFileUsage))))
		b.WriteString("\n")
	}

	result := b.String()
	lines := strings.Split(result, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ── Volume file usage (bottom pane) ─────────────────────────────

// renderVolumeFileUsage draws the per-file/folder disk usage inside
// a volume in the bottom pane.
func (m AppModel) renderVolumeFileUsage(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	if m.selectedVolume == "" {
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.TabInactive).
			Render("Select a volume to view file usage")
	}

	if len(m.volumeFileUsage) == 0 {
		if m.volumeUsageErr != nil {
			return lipgloss.NewStyle().
				Width(width).Height(height).
				Align(lipgloss.Center, lipgloss.Center).
				Foreground(m.theme.StatusStopped).
				Render(fmt.Sprintf("Error: %v", m.volumeUsageErr))
		}
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.TabInactive).
			Render("Loading file usage...")
	}

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.TitleText).
		Bold(true)

	header := titleStyle.Render(fmt.Sprintf("📁 %s", truncateStr(m.selectedVolume, width-4)))

	divider := lipgloss.NewStyle().
		Foreground(m.theme.DividerLine).
		Render(strings.Repeat("─", width))

	// Column headers
	nameHdr := lipgloss.NewStyle().
		Foreground(m.theme.TableHeader).
		Bold(true).
		Width(width - 12).
		Render("NAME")

	sizeHdr := lipgloss.NewStyle().
		Foreground(m.theme.TableHeader).
		Bold(true).
		Width(10).
		Render("SIZE")

	colHeader := nameHdr + "  " + sizeHdr

	// Render file entries
	dirStyle := lipgloss.NewStyle().Foreground(m.theme.TabActive).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(m.theme.Foreground)
	sizeStyle := lipgloss.NewStyle().Foreground(m.theme.TabInactive)
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.TabInactive)

	maxEntries := height - 3 // minus title, divider, col header
	if maxEntries < 1 {
		maxEntries = 1
	}

	var lines []string
	lines = append(lines, header)
	lines = append(lines, divider)
	lines = append(lines, colHeader)

	for i, entry := range m.volumeFileUsage {
		if i >= maxEntries {
			remaining := len(m.volumeFileUsage) - maxEntries
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... and %d more items", remaining)))
			break
		}

		name := entry.Name
		if entry.IsDir {
			name += "/"
		}

		nameCol := lipgloss.NewStyle().Width(width - 12)
		if entry.IsDir {
			nameCol = nameCol.Inherit(dirStyle)
		} else {
			nameCol = nameCol.Inherit(fileStyle)
		}

		szCol := sizeStyle.Width(10)

		nameRendered := nameCol.Render(truncateStr(name, width-14))
		szRendered := szCol.Render(entry.Size)

		lines = append(lines, nameRendered+"  "+szRendered)
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ── Helpers ──────────────────────────────────────────────────────

// truncateStr truncates a string to at most n characters, appending
// "…" if the string was shortened.
func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// formatNetIO parses a docker stats NetIO value (e.g. "1.2GB / 400MB")
// into a display string with arrows: "↑1.2GB ↓400MB".
func formatNetIO(raw string) string {
	tx, rx := splitIO(raw)
	if tx == "" {
		return raw
	}
	return "↑" + tx + " ↓" + rx
}

// formatDiskIO returns the raw BlockIO value as-is (e.g. "3.4GB / 1.2GB").
func formatDiskIO(raw string) string {
	return raw
}

// splitIO splits a "valueA / valueB" string into its two parts.
func splitIO(raw string) (string, string) {
	if idx := strings.Index(raw, " / "); idx >= 0 {
		return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx+3:])
	}
	return "", ""
}

// fitStr truncates or pads a string to exactly w runes.
func fitStr(s string, w int) string {
	runes := []rune(s)
	if len(runes) > w {
		return string(runes[:w])
	}
	return s + strings.Repeat(" ", w-len(runes))
}

// ── Follow-logs helpers ───────────────────────────────────────────

// startFollowLogs begins streaming logs from Docker for the currently
// selected container. Returns a tea.Cmd that starts the goroutine and
// begins reading lines.
func (m *AppModel) startFollowLogs() tea.Cmd {
	if m.selectedName == "" {
		return nil
	}

	// Stop any existing follow first.
	m.stopFollowLogs()

	ctx, cancel := context.WithCancel(context.Background())
	m.logCancel = cancel
	m.followMode = true
	m.logAutoScroll = true

	ch := make(chan string, 256)
	m.logLineCh = ch

	// Launch the goroutine that reads from docker logs -f and feeds
	// lines into the channel.
	go func() {
		defer close(ch)
		dc := docker.NewClient()
		stream := dc.FollowLogs(ctx, m.selectedName)
		for line := range stream {
			select {
			case ch <- line:
			case <-ctx.Done():
				return
			}
		}
	}()

	return waitForLogLine(ch)
}

// stopFollowLogs cancels the running docker logs -f subprocess and
// cleans up follow state.
func (m *AppModel) stopFollowLogs() {
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	m.followMode = false
	m.logLineCh = nil
}

// ── Log search helpers ────────────────────────────────────────────

// doLogSearch rebuilds the list of log line indices that match the
// current logSearchQuery. Resets the match position to 0.
func (m *AppModel) doLogSearch() {
	if m.logSearchQuery == "" {
		m.logSearchMatches = nil
		m.logSearchMatchIdx = 0
		return
	}

	q := strings.ToLower(m.logSearchQuery)
	m.logSearchMatches = nil
	for i, line := range m.logLines {
		if strings.Contains(strings.ToLower(line), q) {
			m.logSearchMatches = append(m.logSearchMatches, i)
		}
	}
	m.logSearchMatchIdx = 0

	// Jump to first match
	if len(m.logSearchMatches) > 0 {
		m.logAutoScroll = false
		m.logScrollOff = m.logSearchMatches[0]
		clampLogScroll(m)
	}
}

// rebuildLogSearchMatches rebuilds matches without resetting position.
func (m *AppModel) rebuildLogSearchMatches() {
	if m.logSearchQuery == "" {
		return
	}
	q := strings.ToLower(m.logSearchQuery)
	m.logSearchMatches = nil
	for i, line := range m.logLines {
		if strings.Contains(strings.ToLower(line), q) {
			m.logSearchMatches = append(m.logSearchMatches, i)
		}
	}
}

// nextLogSearchMatch moves to the next search match.
func (m *AppModel) nextLogSearchMatch() {
	if len(m.logSearchMatches) == 0 {
		return
	}
	m.logSearchMatchIdx++
	if m.logSearchMatchIdx >= len(m.logSearchMatches) {
		m.logSearchMatchIdx = 0
	}
	m.jumpToLogSearchMatch()
}

// prevLogSearchMatch moves to the previous search match.
func (m *AppModel) prevLogSearchMatch() {
	if len(m.logSearchMatches) == 0 {
		return
	}
	m.logSearchMatchIdx--
	if m.logSearchMatchIdx < 0 {
		m.logSearchMatchIdx = len(m.logSearchMatches) - 1
	}
	m.jumpToLogSearchMatch()
}

// jumpToLogSearchMatch scrolls to make the current search match visible.
func (m *AppModel) jumpToLogSearchMatch() {
	if m.logSearchMatchIdx < 0 || m.logSearchMatchIdx >= len(m.logSearchMatches) {
		return
	}
	m.logAutoScroll = false
	m.logScrollOff = m.logSearchMatches[m.logSearchMatchIdx]
	clampLogScroll(m)
}

// clearLogSearch clears the log search state.
func (m *AppModel) clearLogSearch() {
	m.logSearchQuery = ""
	m.logSearchMatches = nil
	m.logSearchMatchIdx = 0
}

// clampLogScroll ensures the log scroll offset is within valid bounds.
func clampLogScroll(m *AppModel) {
	if m.logScrollOff < 0 {
		m.logScrollOff = 0
	}
	maxOff := len(m.logLines) - m.logViewHeight()
	if maxOff < 0 {
		maxOff = 0
	}
	if m.logScrollOff > maxOff {
		m.logScrollOff = maxOff
	}
}

// ── Commands ─────────────────────────────────────────────────────

// logViewHeight returns the available height (in lines) for the
// log content area (excluding borders).
func (m AppModel) logViewHeight() int {
	avail := m.height - (m.height / 3) - 2
	if avail < 1 {
		avail = 1
	}
	return avail
}

// fetchLogs returns a command that fetches the last 200 lines of
// logs for the given container.
func fetchLogs(dc *docker.Client, containerName string) tea.Cmd {
	return func() tea.Msg {
		logs, err := dc.GetLogs(containerName)
		return logsLoadedMsg{containerName: containerName, logs: logs, err: err}
	}
}

// waitForLogLine reads the next line from the follow-logs channel.
// When the channel is closed, sends a logStreamEndedMsg.
func waitForLogLine(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return logStreamEndedMsg{}
		}
		return logLineMsg{containerName: "", line: line}
	}
}

// fetchDiskUsage returns a command that fetches the total disk usage
// (writable layer + mounted volumes) of the given container.
func fetchDiskUsage(dc *docker.Client, containerName string) tea.Cmd {
	return func() tea.Msg {
		size, err := dc.GetContainerDiskUsage(containerName)
		if err != nil {
			return imageSizeLoadedMsg{containerName: containerName, imageSize: "—"}
		}
		return imageSizeLoadedMsg{containerName: containerName, imageSize: size}
	}
}

// fetchImageHistory returns a command that fetches the layer history
// for the given image.
func fetchImageHistory(dc *docker.Client, imageID string) tea.Cmd {
	return func() tea.Msg {
		layers, err := dc.GetImageHistory(imageID)
		return imageLayersLoadedMsg{imageID: imageID, layers: layers, err: err}
	}
}

// fetchNetworkInspect returns a command that fetches the raw JSON
// output of `docker network inspect` for the given network.
func fetchNetworkInspect(dc *docker.Client, name string) tea.Cmd {
	return func() tea.Msg {
		raw, err := dc.InspectNetworkRaw(name)
		return networkInspectLoadedMsg{name: name, json: raw, err: err}
	}
}

// fetchVolumeUsage returns a command that fetches the per-file/folder
// disk usage inside a Docker volume.
func fetchVolumeUsage(dc *docker.Client, volumeName string) tea.Cmd {
	return func() tea.Msg {
		entries, err := dc.GetVolumeFileUsage(volumeName)
		return volumeUsageLoadedMsg{volumeName: volumeName, entries: entries, err: err}
	}
}

// ── Aggregate stats helpers ───────────────────────────────────────

// parseCPUPercent extracts a float64 from a CPU percentage string
// like "1.23%" or returns 0 for "—".
func parseCPUPercent(s string) float64 {
	s = strings.TrimSuffix(s, "%")
	if s == "—" || s == "" {
		return 0
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

// parseMemoryUsage splits a memory string like "80MiB / 1.5GiB"
// into used and limit bytes. Returns 0,0 for "—".
func parseMemoryUsage(s string) (used, limit uint64) {
	if s == "—" || s == "" {
		return 0, 0
	}
	parts := strings.SplitN(s, " / ", 2)
	if len(parts) == 2 {
		return parseBytes(strings.TrimSpace(parts[0])),
			parseBytes(strings.TrimSpace(parts[1]))
	}
	return 0, 0
}

// parseBytes converts a human-readable size string like "80MiB"
// or "1.5GiB" into raw bytes.
func parseBytes(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
		return 0
	}

	var value float64
	var unit string
	fmt.Sscanf(s, "%f%s", &value, &unit)

	switch strings.ToLower(unit) {
	case "b":
		return uint64(value)
	case "kb", "kib":
		return uint64(value * 1024)
	case "mb", "mib":
		return uint64(value * 1024 * 1024)
	case "gb", "gib":
		return uint64(value * 1024 * 1024 * 1024)
	case "tb", "tib":
		return uint64(value * 1024 * 1024 * 1024 * 1024)
	default:
		return uint64(value)
	}
}

// formatBytes converts a raw byte count to a human-readable string.
func formatBytes(b uint64) string {
	switch {
	case b >= 1024*1024*1024*1024:
		return fmt.Sprintf("%.1fTiB", float64(b)/(1024*1024*1024*1024))
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGiB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMiB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKiB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

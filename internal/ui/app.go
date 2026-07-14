package ui

import (
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

	// Log viewer state
	logLines      []string
	logScrollOff  int
	logAutoScroll bool
	selectedName  string

	// Volume viewer state
	selectedVolume    string
	volumeFileUsage   []docker.VolumeFileUsage
	volumeUsageErr    error
	selectedVolumeObj *docker.Volume
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

// volumeUsageLoadedMsg is sent when the per-file/folder usage
// for a volume has been fetched.
type volumeUsageLoadedMsg struct {
	volumeName string
	entries    []docker.VolumeFileUsage
	err        error
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
		var cmd tea.Cmd
		m.pane, cmd = m.pane.Update(msg)
		return m, cmd

	// ── Logs arrived ─────────────────────────────────
	case logsLoadedMsg:
		if msg.containerName == m.selectedName && msg.err == nil {
			m.logLines = strings.Split(msg.logs, "\n")
			if m.logAutoScroll {
				m.scrollLogsToEnd()
			}
		}
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
		return m, m.pane.Init()

	// ── Keyboard ─────────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
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
			prevName := m.selectedName
			prevVol := m.selectedVolume
			var cmd tea.Cmd
			m.pane, cmd = m.pane.Update(msg)

			// Check for container selection change
			newName := m.pane.SelectedContainer()
			if newName != "" && newName != prevName {
				m.selectedName = newName
				m.selectedVolume = ""
				m.selectedVolumeObj = nil
				m.volumeFileUsage = nil
				m.logAutoScroll = true
				return m, tea.Batch(cmd,
					fetchLogs(m.dc, newName),
					fetchDiskUsage(m.dc, newName),
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
				// Find the volume object
				m.selectedVolumeObj = m.pane.FindVolume(newVol)
				return m, tea.Batch(cmd,
					fetchVolumeUsage(m.dc, newVol),
				)
			}

			return m, cmd
		}

		// ── Log scrolling (focus 3) ───────────────────
		if m.focus == 3 {
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
			}
			return m, nil
		}

		return m, nil
	}

	// ── All other messages → pane (stats, data, etc.) ─
	prevName := m.selectedName
	prevVol := m.selectedVolume
	var cmd tea.Cmd
	m.pane, cmd = m.pane.Update(msg)

	// Check for container selection change
	newName := m.pane.SelectedContainer()
	if newName != "" && newName != prevName {
		m.selectedName = newName
		m.selectedVolume = ""
		m.selectedVolumeObj = nil
		m.volumeFileUsage = nil
		m.logAutoScroll = true
		return m, tea.Batch(cmd,
			fetchLogs(m.dc, newName),
			fetchDiskUsage(m.dc, newName),
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

	if m.pane.ActiveTab() == 2 {
		// Volumes tab — show volume details
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

	rightStyle := lipgloss.NewStyle().
		Width(rightWidth).
		Height(paneHeight).
		MaxHeight(paneHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(rightBorder).
		Background(m.theme.Background)

	rightView := rightStyle.Render(rightContent)

	// ── Bottom pane (logs) ────────────────────────────
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

		if m.pane.ActiveTab() == 2 && m.selectedVolume != "" {
			bottomView = bottomStyle.Render(m.renderVolumeFileUsage(m.width-2, bottomHeight-2))
		} else {
			bottomView = bottomStyle.Render(m.renderLogs(m.width-2, bottomHeight-2))
		}
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

	// Clamp scroll offset
	maxOff := len(m.logLines) - height
	if maxOff < 0 {
		maxOff = 0
	}
	if m.logScrollOff > maxOff {
		m.logScrollOff = maxOff
	}
	if m.logScrollOff < 0 {
		m.logScrollOff = 0
	}

	end := m.logScrollOff + height
	if end > len(m.logLines) {
		end = len(m.logLines)
	}

	visible := m.logLines[m.logScrollOff:end]

	// Pad to full height
	for len(visible) < height {
		visible = append(visible, "")
	}

	// Truncate long lines to width
	lineStyle := lipgloss.NewStyle().
		Foreground(m.theme.Foreground).
		Width(width)

	var styled []string
	for _, line := range visible {
		styled = append(styled, lineStyle.Render(line))
	}

	return strings.Join(styled, "\n")
}

// ── Overview pane ────────────────────────────────────────────────

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

// fetchVolumeUsage returns a command that fetches the per-file/folder
// disk usage inside a Docker volume.
func fetchVolumeUsage(dc *docker.Client, volumeName string) tea.Cmd {
	return func() tea.Msg {
		entries, err := dc.GetVolumeFileUsage(volumeName)
		return volumeUsageLoadedMsg{volumeName: volumeName, entries: entries, err: err}
	}
}

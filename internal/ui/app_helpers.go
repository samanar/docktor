package ui

import (
	"context"
	"fmt"
	"strings"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/samanar/docktor/internal/docker"
)

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
		stream, err := m.dc.FollowLogs(ctx, m.selectedName)
		if err != nil {
			return
		}
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
func fetchLogs(dc docker.Client, containerName string) tea.Cmd {
	return func() tea.Msg {
		logs, err := dc.GetLogs(context.Background(), containerName)
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
func fetchDiskUsage(dc docker.Client, containerName string) tea.Cmd {
	return func() tea.Msg {
		size, err := dc.GetContainerDiskUsage(context.Background(), containerName)
		if err != nil {
			return imageSizeLoadedMsg{containerName: containerName, imageSize: "—"}
		}
		return imageSizeLoadedMsg{containerName: containerName, imageSize: size}
	}
}

// fetchImageHistory returns a command that fetches the layer history
// for the given image.
func fetchImageHistory(dc docker.Client, imageID string) tea.Cmd {
	return func() tea.Msg {
		layers, err := dc.GetImageHistory(context.Background(), imageID)
		return imageLayersLoadedMsg{imageID: imageID, layers: layers, err: err}
	}
}

// fetchNetworkInspect returns a command that fetches the raw JSON
// output of `docker network inspect` for the given network.
func fetchNetworkInspect(dc docker.Client, name string) tea.Cmd {
	return func() tea.Msg {
		raw, err := dc.InspectNetworkRaw(context.Background(), name)
		return networkInspectLoadedMsg{name: name, json: raw, err: err}
	}
}

// fetchVolumeUsage returns a command that fetches the per-file/folder
// disk usage inside a Docker volume.
func fetchVolumeUsage(dc docker.Client, volumeName string) tea.Cmd {
	return func() tea.Msg {
		entries, err := dc.GetVolumeFileUsage(context.Background(), volumeName)
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

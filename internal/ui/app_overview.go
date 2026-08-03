package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/samanar/docktor/internal/docker"
)

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
	b.WriteString("\n")

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
	b.WriteString("\n")

	// CPU + inline sparkline
	sparkW := 14
	if width-30 < sparkW {
		sparkW = width - 30
	}
	if sparkW < 6 {
		sparkW = 6
	}
	valW := width - 16 - sparkW
	if valW < 4 {
		valW = 4
	}
	cpuSpark := renderSparkline(m.cpuHistory[ctr.Name], sparkW, m.theme.StatusRunning)
	b.WriteString(fmt.Sprintf("%s%-*s  %s\n",
		labelStyle.Render("CPU:"),
		valW, ctr.CPU,
		cpuSpark,
	))

	// Memory + inline sparkline
	memSpark := renderSparkline(m.memHistory[ctr.Name], sparkW, m.theme.TabHighlight)
	b.WriteString(fmt.Sprintf("%s%-*s  %s\n",
		labelStyle.Render("Memory:"),
		valW, ctr.Memory,
		memSpark,
	))

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
		b.WriteString("\n")
		b.WriteString(titleStyle.Render("Timestamps"))
		b.WriteString("\n")
		b.WriteString(divider)
		b.WriteString("\n")
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
		gid := grp.Project
		if gid == "" {
			gid = "Other"
		}
		sparkW := 14
		if width-26 < sparkW {
			sparkW = width - 26
		}
		if sparkW < 6 {
			sparkW = 6
		}
		valW := width - 14 - sparkW
		if valW < 4 {
			valW = 4
		}

		gCPUSpark := renderSparkline(m.groupCPUHistory[gid], sparkW, m.theme.StatusRunning)
		b.WriteString(fmt.Sprintf("%s%-*s  %s\n",
			labelStyle.Render("CPU:"),
			valW, fmt.Sprintf("%.1f%% total", totalCPU),
			gCPUSpark,
		))

		gMemSpark := renderSparkline(m.groupMemHistory[gid], sparkW, m.theme.TabHighlight)
		b.WriteString(fmt.Sprintf("%s%-*s  %s\n",
			labelStyle.Render("Memory:"),
			valW, formatBytes(totalMemUsed),
			gMemSpark,
		))
	}

	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString("\n")

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

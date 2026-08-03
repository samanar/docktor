package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"

	"github.com/samanar/docktor/internal/docker"
)

// ── Stats helpers ─────────────────────────────────────────────────

// memUsedPortion extracts the used part from a "used / total" memory
// string (e.g. "80MiB / 1.5GiB" → "80MiB").  Falls back to the
// original value when the format doesn't match.
func memUsedPortion(mem string) string {
	if idx := strings.Index(mem, " / "); idx >= 0 {
		return mem[:idx]
	}
	return mem
}

// ── Table construction ───────────────────────────────────────────

// containerColumns returns the column definitions for the container
// table.  Width 0 means "flex" — the column shares remaining space.
func containerColumns() []ColumnDef {
	return []ColumnDef{
		{Key: colIcon, Title: "", Width: 2},
		{Key: colName, Title: "NAME", Width: 20},
		{Key: colStatus, Title: "STATUS", Width: 8},
		{Key: colBuilt, Title: "BUILT", Width: 10},
		{Key: colRestarted, Title: "RESTARTED", Width: 10},
		{Key: colCPUMem, Title: "CPU/MEM", Width: 18},
		{Key: colPorts, Title: "PORTS", Width: 0}, // flex
	}
}

// imageColumns returns column definitions for the image table.
func imageColumns() []ColumnDef {
	return []ColumnDef{
		{Key: colName, Title: "REPOSITORY", Width: 25},
		{Key: "tag", Title: "TAG", Width: 15},
		{Key: colIcon, Title: "ID", Width: 14},
		{Key: "size", Title: "SIZE", Width: 10},
		{Key: "created", Title: "CREATED", Width: 0}, // flex
	}
}

// buildImageRows converts Docker images into table rows.
func buildImageRows(theme Theme, images []docker.Image) []Row {
	bodyStyle := lipgloss.NewStyle().Foreground(theme.Foreground)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TabInactive)
	idStyle := lipgloss.NewStyle().Foreground(theme.TabHighlight)

	var rows []Row
	for _, img := range images {
		shortID := img.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		row := Row{
			Cells: map[string]Cell{
				colName:   {Value: img.Repo, Style: bodyStyle},
				"tag":     {Value: img.Tag, Style: bodyStyle},
				colIcon:   {Value: shortID, Style: idStyle},
				"size":    {Value: img.Size, Style: dimStyle},
				"created": {Value: img.Created, Style: dimStyle},
			},
			Type: RowData,
		}
		rows = append(rows, row)
	}
	return rows
}

// networkColumns returns the column definitions for the network
// table.  All columns have fixed widths.
func networkColumns() []ColumnDef {
	return []ColumnDef{
		{Key: colName, Title: "NAME", Width: 20},
		{Key: colDriver, Title: "DRIVER", Width: 12},
		{Key: colScope, Title: "SCOPE", Width: 8},
		{Key: colSubnet, Title: "SUBNET", Width: 18},
		{Key: colGateway, Title: "GATEWAY", Width: 15},
		{Key: colContainers, Title: "CNT", Width: 5},
	}
}

// volumeColumns returns the column definitions for the volumes table.
func volumeColumns() []ColumnDef {
	return []ColumnDef{
		{Key: colIcon, Title: "", Width: 2},
		{Key: colName, Title: "VOLUME NAME", Width: 30},
		{Key: "driver", Title: "DRIVER", Width: 10},
		{Key: "mountpoint", Title: "MOUNTPOINT", Width: 0}, // flex
		{Key: "size", Title: "SIZE", Width: 10},
	}
}

// buildTableRows converts Docker container groups into table Rows
// with status dots, Docker-style coloring, collapsible groups, and
// visual separators between groups.
func buildTableRows(theme Theme, groups []docker.ContainerGroup, collapsed map[string]bool, composeLoadingID string, spinnerIdx int) []Row {
	// ── Pre-compute styles ────────────────────────────
	projectStyle := lipgloss.NewStyle().
		Foreground(theme.TabActive).
		Background(theme.TabBarBackground).
		Bold(true)

	bodyStyle := lipgloss.NewStyle().Foreground(theme.Foreground)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TabInactive)

	// Icon (status dot) styles
	iconRunning := lipgloss.NewStyle().Foreground(theme.StatusRunning)
	iconStopped := lipgloss.NewStyle().Foreground(theme.TabInactive)
	iconAmber := lipgloss.NewStyle().Foreground(theme.TabHighlight)
	iconRed := lipgloss.NewStyle().Foreground(theme.StatusStopped)

	// Status colors per user spec:
	//   running/healthy → green, stopped → grey, exited/error → red
	statusGreen := lipgloss.NewStyle().Foreground(theme.StatusRunning).Bold(true)
	statusGrey := lipgloss.NewStyle().Foreground(theme.TabInactive)
	statusRed := lipgloss.NewStyle().Foreground(theme.StatusStopped)
	statusAmber := lipgloss.NewStyle().Foreground(theme.TabHighlight)

	var rows []Row
	for _, g := range groups {
		label := g.Project
		if label == "" {
			label = "Other"
		}
		groupID := "group:" + label

		// Track short names used within this group to detect duplicates.
		// Maps short name → index of first row with that name (or -1 if
		// already renamed).
		firstNameIdx := make(map[string]int, len(g.Containers))
		nameCount := make(map[string]int, len(g.Containers))

		// ── Project header ─────────────────────────────
		count := len(g.Containers)
		toggle := "▸"
		if collapsed[groupID] {
			toggle = "▻"
		}
		// Show spinner when a compose action is running on this group.
		if composeLoadingID == groupID {
			toggle = spinnerFrames[spinnerIdx%len(spinnerFrames)]
		}

		// ── Health summary for compose groups ──────────
		var healthDot Cell
		title := fmt.Sprintf("%s %s (%d)", toggle, label, count)
		statusSummary := ""

		if g.ComposeFile != "" && label != "Other" {
			running, healthy, exited, stopped := 0, 0, 0, 0
			for _, c := range g.Containers {
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
			down := exited + stopped

			// Health dot: green = all up, amber = mixed, red = all down.
			switch {
			case up == count:
				healthDot = Cell{Value: "●", Style: iconRunning}
			case up > 0:
				healthDot = Cell{Value: "●", Style: iconAmber}
			case count > 0:
				healthDot = Cell{Value: "●", Style: iconRed}
			default:
				healthDot = Cell{Value: "●", Style: iconStopped}
			}

			title = fmt.Sprintf("%s %s (%d/%d up)", toggle, label, up, count)

			// Status breakdown for mixed states.
			if down > 0 {
				var parts []string
				if exited > 0 {
					parts = append(parts, fmt.Sprintf("%d exited", exited))
				}
				if stopped > 0 {
					parts = append(parts, fmt.Sprintf("%d stopped", stopped))
				}
				statusSummary = strings.Join(parts, " ")
			}
		}

		headerRow := Row{
			Cells: map[string]Cell{
				colIcon:      healthDot,
				colName:      {Value: title},
				colStatus:    {Value: statusSummary, Style: dimStyle},
				colCPUMem:    {Value: ""},
				colPorts:     {Value: ""},
				colBuilt:     {Value: ""},
				colRestarted: {Value: ""},
			},
			Style:   projectStyle,
			Type:    RowGroup,
			GroupID: groupID,
		}
		// ── Separator between groups ───────────────────
		if len(rows) > 0 {
			rows = append(rows, Row{
				Type: RowSeparator,
			})
		}

		rows = append(rows, headerRow)

		if collapsed[groupID] {
			continue
		}

		// ── Container rows ─────────────────────────────
		for _, c := range g.Containers {
			// Status dot
			var iconCell Cell
			switch c.State {
			case "running", "healthy":
				iconCell = Cell{Value: "●", Style: iconRunning}
			case "exited", "dead", "removing":
				iconCell = Cell{Value: "●", Style: iconRed}
			case "paused":
				iconCell = Cell{Value: "●", Style: iconAmber}
			default:
				iconCell = Cell{Value: "●", Style: iconStopped}
			}

			// Status text
			var statusCell Cell
			switch c.State {
			case "running":
				statusCell = Cell{Value: "running", Style: statusGreen}
			case "healthy":
				statusCell = Cell{Value: "healthy", Style: statusGreen}
			case "exited", "dead", "removing":
				statusCell = Cell{Value: c.State, Style: statusRed}
			case "paused":
				statusCell = Cell{Value: c.State, Style: statusAmber}
			default:
				statusCell = Cell{Value: c.State, Style: statusGrey}
			}

			shortName := shortenName(c.Name)

			// Detect duplicate short names within the same group
			// and disambiguate (e.g. two "app" containers become
			// "app (1)" and "app (2)").
			if idx, exists := firstNameIdx[shortName]; exists {
				nameCount[shortName]++
				// Rename the first occurrence on first collision
				if idx >= 0 {
					rows[idx].Cells[colName] = Cell{
						Value: shortName + " (1)",
						Style: rows[idx].Cells[colName].Style,
					}
					firstNameIdx[shortName] = -1 // mark as renamed
				}
				shortName = fmt.Sprintf("%s (%d)", shortName, nameCount[shortName])
			} else {
				firstNameIdx[shortName] = len(rows) // index of this row
				nameCount[shortName] = 1
			}

			// Combined CPU / Memory (compact for table column)
			memUsed := memUsedPortion(c.Memory)
			cpuMem := c.CPU + " / " + memUsed
			cpuMemCell := Cell{Value: cpuMem, Style: bodyStyle}
			if c.CPU == "—" && memUsed == "—" {
				cpuMemCell.Style = dimStyle
			}

			// Relative times
			builtCell := Cell{Value: relativeTime(c.CreatedAt), Style: dimStyle}
			restartedCell := Cell{Value: relativeTime(c.StartedAt), Style: dimStyle}

			row := Row{
				Cells: map[string]Cell{
					colIcon:         iconCell,
					colName:         {Value: shortName, Style: bodyStyle},
					colOriginalName: {Value: c.Name},
					colStatus:       statusCell,
					colCPUMem:       cpuMemCell,
					colPorts:        {Value: formatPorts(c.Ports), Style: bodyStyle},
					colBuilt:        builtCell,
					colRestarted:    restartedCell,
				},
				Type:    RowData,
				GroupID: groupID,
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// buildNetworkRows converts Docker network groups into table Rows
// with collapsible driver-group headers.
func buildNetworkRows(theme Theme, groups []docker.NetworkGroup, collapsed map[string]bool) []Row {
	groupStyle := lipgloss.NewStyle().
		Foreground(theme.TabActive).
		Background(theme.TabBarBackground).
		Bold(true)

	bodyStyle := lipgloss.NewStyle().Foreground(theme.Foreground)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TabInactive)

	var rows []Row
	for _, g := range groups {
		driver := g.Driver
		if driver == "" {
			driver = "unknown"
		}
		groupID := "netgroup:" + driver

		count := len(g.Networks)
		toggle := "▸"
		if collapsed[groupID] {
			toggle = "▻"
		}
		title := fmt.Sprintf("%s %s (%d)", toggle, driver, count)

		headerRow := Row{
			Cells: map[string]Cell{
				colName:       {Value: title},
				colDriver:     {Value: ""},
				colScope:      {Value: ""},
				colSubnet:     {Value: ""},
				colGateway:    {Value: ""},
				colContainers: {Value: ""},
			},
			Style:   groupStyle,
			Type:    RowGroup,
			GroupID: groupID,
		}
		rows = append(rows, headerRow)

		if collapsed[groupID] {
			continue
		}

		for _, n := range g.Networks {
			containerCount := fmt.Sprintf("%d", len(n.Containers))
			containerCell := Cell{Value: containerCount, Style: dimStyle}
			if len(n.Containers) > 0 {
				containerCell.Style = bodyStyle
			}

			row := Row{
				Cells: map[string]Cell{
					colName:         {Value: n.Name, Style: bodyStyle},
					colOriginalName: {Value: n.Name},
					colDriver:       {Value: n.Driver, Style: dimStyle},
					colScope:        {Value: n.Scope, Style: dimStyle},
					colSubnet:       {Value: n.Subnet, Style: bodyStyle},
					colGateway:      {Value: n.Gateway, Style: bodyStyle},
					colContainers:   containerCell,
				},
				Type:    RowData,
				GroupID: groupID,
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// buildVolumeRows converts Docker volumes into table rows.
func buildVolumeRows(theme Theme, volumes []docker.Volume) []Row {
	bodyStyle := lipgloss.NewStyle().Foreground(theme.Foreground)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TabInactive)
	iconVol := lipgloss.NewStyle().Foreground(theme.TabActive)

	var rows []Row
	for _, v := range volumes {
		row := Row{
			Cells: map[string]Cell{
				colIcon:         {Value: "⬡", Style: iconVol},
				colName:         {Value: v.Name, Style: bodyStyle},
				colOriginalName: {Value: v.Name},
				"driver":        {Value: v.Driver, Style: dimStyle},
				"mountpoint":    {Value: v.Mountpoint, Style: dimStyle},
				"size":          {Value: v.Size, Style: bodyStyle},
			},
			Type: RowData,
		}
		rows = append(rows, row)
	}
	return rows
}

// rebuildTableRows rebuilds rows from groups, filtering collapsed
// groups, and updates the table while preserving selection.
// The exact row set depends on the active tab.
func (p *Pane) rebuildTableRows() {
	var rows []Row
	if p.ActiveTabKey() == 'N' {
		rows = buildNetworkRows(p.theme, p.networks, p.networkCollapsed)
	} else if p.ActiveTabKey() == 'i' {
		rows = buildImageRows(p.theme, p.images)
	} else {
		rows = buildTableRows(p.theme, p.groups, p.collapsedGroups, p.composeLoadingID, p.spinnerIdx)
	}
	p.table = p.table.WithRows(rows)
	if len(rows) > 0 && p.table.HighlightedRow() < 0 {
		p.table.SelectFirst()
	}
}

// buildAndSetVolumeRows rebuilds the table with volume rows.
func (p *Pane) buildAndSetVolumeRows() {
	cols := volumeColumns()
	tbl := NewTable(cols).
		WithBaseStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.Foreground).
				Background(p.theme.Background),
		).
		WithHeaderStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.TableHeader).
				Background(p.theme.TabBarBackground).
				Bold(true),
		).
		WithSelectedStyle(
			lipgloss.NewStyle().
				Background(p.theme.RowSelected).
				Bold(true),
		).
		WithDividerStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.DividerLine),
		).
		WithSepStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.DividerLine),
		).
		WithHighlightColumn(1,
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true),
		).
		Focused(p.focused)

	rows := buildVolumeRows(p.theme, p.volumes)
	tbl = tbl.WithRows(rows)
	if len(rows) > 0 && tbl.HighlightedRow() < 0 {
		tbl.SelectFirst()
	}

	// Preserve width/height that were already set
	if p.table.width > 0 {
		tbl.SetWidth(p.table.width)
	}
	if p.table.height > 0 {
		tbl.SetHeight(p.table.height)
	}

	p.table = tbl
}

// buildContainerTable rebuilds the table with container columns and rows.
func (p *Pane) buildContainerTable() {
	cols := containerColumns()
	tbl := NewTable(cols).
		WithBaseStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.Foreground).
				Background(p.theme.Background),
		).
		WithHeaderStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.TableHeader).
				Background(p.theme.TabBarBackground).
				Bold(true),
		).
		WithSelectedStyle(
			lipgloss.NewStyle().
				Background(p.theme.RowSelected).
				Bold(true),
		).
		WithDividerStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.DividerLine),
		).
		WithSepStyle(
			lipgloss.NewStyle().
				Foreground(p.theme.DividerLine),
		).
		WithHighlightColumn(1,
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true),
		).
		Focused(true)

	// Preserve width/height
	w, h := p.table.width, p.table.height
	tbl.SetWidth(w)
	tbl.SetHeight(h)

	p.table = tbl
	p.rebuildTableRows()
}

// switchToTab switches the active tab and updates the table accordingly.
func (p *Pane) switchToTab(tabIdx int) {
	if tabIdx == p.activeTab {
		return
	}
	p.activeTab = tabIdx

	switch tabIdx {
	case 0: // Containers
		p.buildContainerTable()
	case 2: // Volumes
		p.volumesLoading = true
		p.table = p.table.WithRows(nil) // clear rows while loading
	default:
		// Images, Networks — placeholder for now, keep container table
	}
}

// restoreSelection attempts to restore the previously selected row
// after a table rebuild.  WithRows already preserves selected and
// yOffset for unchanged rows; we only intervene if the selection
// was clamped out of range or the group changed.
func (p *Pane) restoreSelection(prevIdx int, prevGID string) {
	// If selection is still valid (WithRows preserved it), keep scroll position
	if prevIdx >= 0 && prevIdx < p.table.RowCount() &&
		p.table.GroupIDAt(prevIdx) == prevGID && prevGID != "" {
		return
	}
	// Selection was lost — fall back to the first row
	p.table.SelectFirst()
}

// formatPorts parses Docker port mappings and returns a compact
// string of exposed ports only.
//
//	"0.0.0.0:80->80/tcp, :::443->443/tcp" → "80, 443"
//	"80/tcp, 443/udp"                     → "80, 443"
func formatPorts(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ", ")
	seen := map[string]bool{}
	var ports []string
	for _, p := range parts {
		// Extract the right side of "->" (exposed port)
		if idx := strings.Index(p, "->"); idx >= 0 {
			p = p[idx+2:]
		}
		// Strip protocol suffix "/tcp", "/udp"
		if idx := strings.IndexByte(p, '/'); idx >= 0 {
			p = p[:idx]
		}
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			ports = append(ports, p)
		}
	}
	return strings.Join(ports, ", ")
}

// relativeTime parses a Docker timestamp and returns a human-
// readable relative string like "2h ago", "3d ago", or "—".
func relativeTime(ts string) string {
	if ts == "" {
		return "—"
	}
	t, err := parseDockerTime(ts)
	if err != nil {
		return "—"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dM ago", int(d.Hours()/(24*30)))
	}
}

// parseDockerTime handles both docker ps (CreatedAt) and docker
// inspect (StartedAt) timestamp formats.
func parseDockerTime(s string) (time.Time, error) {
	// docker ps: "2026-07-11 17:56:33 +0330 +0330" (duplicate tz offset)
	if t, err := parseDockerPS(s); err == nil {
		return t, nil
	}
	// docker ps (legacy): "2024-01-15 10:30:00 +0000 UTC"
	if t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", s); err == nil {
		return t, nil
	}
	// docker ps (with nanos): "2024-01-15 10:30:00.999999999 -0700 MST"
	if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", s); err == nil {
		return t, nil
	}
	// docker inspect: "2026-07-14T07:44:19.407747938Z"
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	// unix timestamp (seconds): "1734567890"
	if t, err := parseUnixTimestamp(s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp: %s", s)
}

// parseDockerPS handles docker ps CreatedAt formats:
//
//	"2026-07-11 17:56:33 +0330 +0330" (duplicate tz)
//	"2026-07-11 17:56:33.123 +0330 +0330" (with sub-seconds)
func parseDockerPS(s string) (time.Time, error) {
	parts := strings.Fields(s)
	if len(parts) < 3 {
		return time.Time{}, fmt.Errorf("too few fields")
	}
	// Last part should look like a tz offset (±HHMM)
	last := parts[len(parts)-1]
	if len(last) != 5 || (last[0] != '+' && last[0] != '-') {
		return time.Time{}, fmt.Errorf("not a docker ps timestamp")
	}
	// Drop the trailing duplicate tz offset(s) — keep only the first one
	// "2026-07-11 17:56:33 +0330 +0330" → "2026-07-11 17:56:33 +0330"
	trimmed := strings.Join(parts[:len(parts)-1], " ")
	// Try with sub-seconds first
	if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700", trimmed); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05.999 -0700", trimmed); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05 -0700", trimmed); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognised docker ps format: %s", s)
}

// parseUnixTimestamp tries to parse a string as a Unix timestamp.
func parseUnixTimestamp(s string) (time.Time, error) {
	var sec int64
	if _, err := fmt.Sscanf(s, "%d", &sec); err != nil || sec <= 0 {
		return time.Time{}, fmt.Errorf("not a unix timestamp")
	}
	return time.Unix(sec, 0), nil
}

// shortenName strips the Docker Compose project prefix from a
// container name if one is present (e.g. "project_service_1" →
// "service").  Falls back to the original name.
func shortenName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) >= 3 {
		return parts[len(parts)-2]
	}
	if len(parts) == 2 {
		return parts[0]
	}
	return name
}

// ── Loading / empty renderers ────────────────────────────────────

// renderLoading draws a centred spinner + message while data
// is being fetched.
func (p Pane) renderLoading(width, height int, msg string) string {
	spinner := spinnerFrames[p.spinnerIdx]
	text := lipgloss.NewStyle().
		Foreground(p.theme.Foreground).
		Render(spinner + "  " + msg)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(p.theme.Background).
		Align(lipgloss.Center, lipgloss.Center).
		Render(text)
}

// renderError draws an error message with a retry hint.
func (p Pane) renderError(width, height int, msg string) string {
	errorStyle := lipgloss.NewStyle().
		Foreground(p.theme.StatusStopped).
		Bold(true)

	hintStyle := lipgloss.NewStyle().
		Foreground(p.theme.TabHighlight)

	errorText := errorStyle.Render("✖  " + msg)
	hintText := hintStyle.Render("\n\nPress r to retry")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(p.theme.Background).
		Align(lipgloss.Center, lipgloss.Center).
		Render(errorText + hintText)
}

// renderEmpty draws a centred message when no data was found.
func (p Pane) renderEmpty(width, height int, msg string) string {
	text := lipgloss.NewStyle().
		Foreground(p.theme.TabInactive).
		Render(msg)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(p.theme.Background).
		Align(lipgloss.Center, lipgloss.Center).
		Render(text)
}

// ── Bubble Tea commands ──────────────────────────────────────────

// padLines adds `pad` spaces of left padding to every line and
// ensures each line fills the target width.
func padLines(s string, targetWidth, pad int) string {
	lines := strings.Split(s, "\n")
	prefix := strings.Repeat(" ", pad)
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// fetchContainers returns a command that asynchronously fetches
// Docker containers and groups them by Compose project.
func fetchContainers(dc docker.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		groups, err := dc.ListContainers(ctx)
		if err == nil {
			started, _ := dc.GetStartedTimes(ctx)
			mergeStarted(groups, started)
		}
		return containersLoadedMsg{groups: groups, err: err}
	}
}

// fetchImages returns a command that asynchronously fetches Docker
// images.
func fetchImages(dc docker.Client) tea.Cmd {
	return func() tea.Msg {
		images, err := dc.ListImages(context.Background())
		return imagesLoadedMsg{images: images, err: err}
	}
}

// fetchNetworks returns a command that asynchronously fetches
// Docker networks and groups them by driver.
func fetchNetworks(dc docker.Client) tea.Cmd {
	return func() tea.Msg {
		groups, err := dc.ListNetworks(context.Background())
		return networksLoadedMsg{groups: groups, err: err}
	}
}

// mergeStarted copies StartedAt timestamps into matching containers.
func mergeStarted(groups []docker.ContainerGroup, started map[string]string) {
	for gi := range groups {
		for ci := range groups[gi].Containers {
			if t, ok := started[groups[gi].Containers[ci].Name]; ok {
				groups[gi].Containers[ci].StartedAt = t
			}
		}
	}
}

// spinnerTick returns a command that fires a spinnerTickMsg after
// 100 ms, driving the loading animation.
func spinnerTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// statsTick returns a command that fires a statsTickMsg after 1 s,
// driving the periodic stats refresh.
func statsTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return statsTickMsg{}
	})
}

// fetchStats returns a command that asynchronously fetches live
// container resource stats from the Docker daemon for the given
// container names (only running containers should be passed).
func fetchStats(dc docker.Client, names []string) tea.Cmd {
	return func() tea.Msg {
		stats, err := dc.GetStats(context.Background(), names)
		return statsRefreshMsg{stats: stats, err: err}
	}
}

// fetchVolumes returns a command that asynchronously fetches Docker
// volumes with their driver, mountpoint, and size information.
func fetchVolumes(dc docker.Client) tea.Cmd {
	return func() tea.Msg {
		volumes, err := dc.GetVolumes(context.Background())
		return volumesLoadedMsg{volumes: volumes, err: err}
	}
}

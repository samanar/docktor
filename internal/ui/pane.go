package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/samanar/lazycompose/internal/docker"
)

// ── Column keys ──────────────────────────────────────────────────

const (
	colIcon         = "icon"
	colName         = "name"
	colOriginalName = "originalName" // hidden — full Docker container name
	colStatus       = "status"
	colCPUMem       = "cpumem"
	colPorts        = "ports"
	colBuilt        = "built"
	colRestarted    = "restarted"
)

// ── Spinner ──────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ── Custom messages ──────────────────────────────────────────────

// containersLoadedMsg is sent when the Docker client finishes
// fetching and grouping containers.
type containersLoadedMsg struct {
	groups []docker.ContainerGroup
	err    error
}

// spinnerTickMsg is sent on every spinner animation frame.
type spinnerTickMsg struct{}

// statsTickMsg triggers a 1-second stats refresh cycle.
type statsTickMsg struct{}

// statsRefreshMsg carries updated container stats from an async
// docker stats fetch.
type statsRefreshMsg struct {
	stats map[string]docker.ContainerStats
	err   error
}

// ── Tab ──────────────────────────────────────────────────────────

// Tab represents a single tab in the pane's tab bar.
type Tab struct {
	Key   rune   // highlighted trigger character, e.g. 'c' for Containers
	Label string // full label, e.g. "Containers"
}

// ── Action ───────────────────────────────────────────────────────

// Action represents a keybinding shown in the action bar at the
// bottom of the pane.
type Action struct {
	Key   rune
	Label string
}

// ── Pane ─────────────────────────────────────────────────────────

// Pane is the primary navigator pane (70% width) containing:
//   - Tab bar at the top
//   - Scrollable table for container data
//   - Action bar at the bottom
//   - Borders around the entire pane
type Pane struct {
	theme Theme

	// Dimensions (set externally via Resize)
	width  int // total width including border
	height int // total height including border

	// Tabs
	tabs      []Tab
	activeTab int

	// Actions
	actions []Action

	// Focus
	focused bool

	// ── Custom table ─────────────────────────────────
	table Table

	// ── Loading & Docker ─────────────────────────────
	loading      bool
	spinnerIdx   int
	dockerClient *docker.Client

	// ── Raw grouped container data ───────────────────
	groups []docker.ContainerGroup

	// ── Collapsible groups ───────────────────────────
	collapsedGroups map[string]bool
}

// NewPane creates a new navigator pane with default tabs, actions,
// and a Docker client for live container data.
func NewPane(theme Theme, dc *docker.Client) Pane {
	tabs := []Tab{
		{Key: 'c', Label: "Containers"},
		{Key: 'i', Label: "Images"},
		{Key: 'v', Label: "Volumes"},
		{Key: 'n', Label: "Networks"},
	}

	actions := []Action{
		{Key: 's', Label: "Stop"},
		{Key: 'r', Label: "Restart"},
		{Key: 'd', Label: "Remove"},
		{Key: 'l', Label: "Logs"},
		{Key: '/', Label: "Filter"},
		{Key: 'q', Label: "Quit"},
	}

	tbl := NewTable(containerColumns()).
		WithBaseStyle(
			lipgloss.NewStyle().
				Foreground(theme.Foreground).
				Background(theme.Background),
		).
		WithHeaderStyle(
			lipgloss.NewStyle().
				Foreground(theme.TableHeader).
				Background(theme.TabBarBackground).
				Bold(true),
		).
		WithSelectedStyle(
			lipgloss.NewStyle().
				Background(theme.RowSelected).
				Bold(true),
		).
		WithDividerStyle(
			lipgloss.NewStyle().
				Foreground(theme.DividerLine),
		).
		WithSepStyle(
			lipgloss.NewStyle().
				Foreground(theme.DividerLine),
		).
		WithHighlightColumn(1,
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true),
		).
		Focused(true)

	return Pane{
		theme:           theme,
		tabs:            tabs,
		actions:         actions,
		activeTab:       0,
		focused:         true,
		table:           tbl,
		loading:         true,
		spinnerIdx:      0,
		dockerClient:    dc,
		collapsedGroups: make(map[string]bool),
	}
}

// ── Bubble Tea Model ─────────────────────────────────────────────

func (p Pane) Init() tea.Cmd {
	return tea.Batch(
		fetchContainers(p.dockerClient),
		spinnerTick(),
	)
}

func (p Pane) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Container data arrived ────────────────────────
	case containersLoadedMsg:
		p.loading = false
		if msg.err == nil {
			p.groups = msg.groups
			p.rebuildTableRows()
			p.recalcTable()
			return p, statsTick()
		}
		return p, nil

	// ── Stats refresh cycle ───────────────────────────
	case statsTickMsg:
		return p, fetchStats(p.dockerClient)

	case statsRefreshMsg:
		if msg.err == nil {
			docker.MergeStats(p.groups, msg.stats)
			sel := p.table.HighlightedRow()
			gid := p.table.GroupIDAt(sel)
			p.rebuildTableRows()
			// Try to restore previous selection
			p.restoreSelection(sel, gid)
		}
		return p, statsTick()

	// ── Spinner animation ─────────────────────────────
	case spinnerTickMsg:
		if p.loading {
			p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerFrames)
			return p, spinnerTick()
		}
		return p, nil

	case tea.WindowSizeMsg:
		p.width = int(float64(msg.Width) * 0.70)
		p.height = msg.Height
		p.recalcTable()

	case tea.KeyMsg:
		if !p.focused {
			return p, nil
		}

		// Block navigation while loading
		if p.loading {
			return p, nil
		}

		// ── Arrow keys + vim navigation ────────────────
		switch msg.Type {

		case tea.KeyUp:
			p.table.MoveSelection(-1)

		case tea.KeyDown:
			p.table.MoveSelection(1)

		case tea.KeyEnter:
			p.toggleGroup()

		// ── Character keys ─────────────────────────────
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "j":
				p.table.MoveSelection(1)
			case "k":
				p.table.MoveSelection(-1)

			// Tab switching
			case "c":
				p.activeTab = p.tabIndexByKey('c')
			case "i":
				p.activeTab = p.tabIndexByKey('i')
			case "v":
				p.activeTab = p.tabIndexByKey('v')
			case "n":
				p.activeTab = p.tabIndexByKey('n')

			// Toggle group collapse
			case " ":
				p.toggleGroup()

			// Actions (placeholder)
			case "s", "r", "d", "l":
			// will trigger container actions later

			// Filter (placeholder)
			case "/":
				// will trigger filter
			}
		}
	}

	return p, nil
}

func (p Pane) View() string {
	if p.width < 10 || p.height < 5 {
		return ""
	}

	// Recalculate table dimensions in case the pane was resized
	// externally (e.g. by AppModel).  This ensures the table
	// always matches the current pane size.
	p.recalcTable()

	innerW := p.width - 2  // minus left/right borders
	innerH := p.height - 2 // minus top/bottom borders

	// ── Tab bar ──────────────────────────────────────
	tabBar := p.renderTabBar(innerW)

	// ── Divider ──────────────────────────────────────
	divider := p.renderDivider(innerW)

	// ── Content area ─────────────────────────────────
	contentH := innerH - 4 // -4 = tabBar + 2 dividers + actionBar
	var contentArea string
	if p.loading {
		contentArea = p.renderLoading(innerW, contentH)
	} else if len(p.groups) == 0 {
		contentArea = p.renderEmpty(innerW, contentH)
	} else {
		contentArea = padLines(p.table.View(), innerW, 2)
	}

	// ── Action bar ───────────────────────────────────
	actionBar := p.renderActionBar(innerW)

	// ── Assemble inner content ───────────────────────
	inner := lipgloss.JoinVertical(
		lipgloss.Top,
		tabBar,
		divider,
		contentArea,
		divider,
		actionBar,
	)

	// ── Wrap with border ─────────────────────────────
	borderColor := p.theme.BorderFocused
	if !p.focused {
		borderColor = p.theme.BorderInactive
	}

	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		MaxHeight(p.height).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Background(p.theme.Background)

	return style.Render(inner)
}

// ── Rendering helpers ────────────────────────────────────────────

func (p Pane) renderTabBar(width int) string {
	var tabs []string
	for i, tab := range p.tabs {
		label := fmt.Sprintf("[%c]%s", tab.Key, tab.Label[1:]) // [C]ontainers
		if i == p.activeTab {
			tabs = append(tabs, lipgloss.NewStyle().
				Foreground(p.theme.TabActive).
				Bold(true).
				Render(label))
		} else {
			tabs = append(tabs, lipgloss.NewStyle().
				Foreground(p.theme.TabInactive).
				Render(label))
		}
	}

	bar := strings.Join(tabs, lipgloss.NewStyle().
		Foreground(p.theme.ActionSeparator).
		Render("  "))

	return lipgloss.NewStyle().
		Width(width).
		Background(p.theme.TabBarBackground).
		Padding(0, 1).
		Render(bar)
}

func (p Pane) renderDivider(width int) string {
	line := strings.Repeat("─", width)
	return lipgloss.NewStyle().
		Foreground(p.theme.DividerLine).
		Render(line)
}

func (p Pane) renderActionBar(width int) string {
	var parts []string
	for _, a := range p.actions {
		key := lipgloss.NewStyle().
			Foreground(p.theme.ActionKey).
			Bold(true).
			Render(string(a.Key))
		label := lipgloss.NewStyle().
			Foreground(p.theme.ActionLabel).
			Render(a.Label)
		parts = append(parts, key+":"+label)
	}

	sep := lipgloss.NewStyle().
		Foreground(p.theme.ActionSeparator).
		Render("  ")

	bar := strings.Join(parts, sep)

	return lipgloss.NewStyle().
		Width(width).
		Background(p.theme.ActionBarBackground).
		Padding(0, 1).
		Render(bar)
}

// ── Internal helpers ─────────────────────────────────────────────

// recalcTable updates the table's width and height based on the
// current pane dimensions.
// toggleGroup collapses or expands the group header at the currently
// selected row (no-op if the selected row is not a group header).
func (p *Pane) toggleGroup() {
	sel := p.table.HighlightedRow()
	if p.table.RowTypeAt(sel) == RowGroup {
		gid := p.table.GroupIDAt(sel)
		p.collapsedGroups[gid] = !p.collapsedGroups[gid]
		p.rebuildTableRows()
		p.recalcTable()
	}
}

func (p *Pane) recalcTable() {
	// Content area: total height - borders(2) - tabBar(1) -
	//   dividers(2) - actionBar(1)
	contentH := p.height - 2 - 1 - 2 - 1
	if contentH < 1 {
		contentH = 1
	}
	// Reserve 2 lines for header + divider
	dataH := contentH - 2
	if dataH < 1 {
		dataH = 1
	}

	// Width with 2-char horizontal padding on each side
	pad := 2
	contentW := p.width - 2 - pad*2
	if contentW < 1 {
		contentW = 1
	}

	p.table.SetWidth(contentW)
	p.table.SetHeight(dataH)
}

func (p Pane) tabIndexByKey(key rune) int {
	for i, t := range p.tabs {
		if t.Key == key {
			return i
		}
	}
	return p.activeTab
}

// SelectedContainer returns the full Docker name of the currently
// highlighted container, or empty string if a group header is selected.
func (p Pane) SelectedContainer() string {
	sel := p.table.HighlightedRow()
	if p.table.RowTypeAt(sel) != RowData {
		return ""
	}
	name, _ := p.table.GetCell(sel, colOriginalName)
	return name
}

// GetSelectedContainer returns the full Container object for the
// currently highlighted row, or nil if a group header is selected
// or the container is not found.
func (p Pane) GetSelectedContainer() *docker.Container {
	name := p.SelectedContainer()
	if name == "" {
		return nil
	}
	for _, g := range p.groups {
		for i := range g.Containers {
			if g.Containers[i].Name == name {
				return &g.Containers[i]
			}
		}
	}
	return nil
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

// buildTableRows converts Docker container groups into table Rows
// with status dots, Docker-style coloring, collapsible groups, and
// visual separators between groups.
func buildTableRows(theme Theme, groups []docker.ContainerGroup, collapsed map[string]bool) []Row {
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

		// ── Project header ─────────────────────────────
		count := len(g.Containers)
		toggle := "▸"
		if collapsed[groupID] {
			toggle = "▻"
		}
		title := fmt.Sprintf("%s %s (%d)", toggle, label, count)

		headerRow := Row{
			Cells: map[string]Cell{
				colIcon:      {Value: ""},
				colName:      {Value: title},
				colStatus:    {Value: ""},
				colCPUMem:    {Value: ""},
				colPorts:     {Value: ""},
				colBuilt:     {Value: ""},
				colRestarted: {Value: ""},
			},
			Style:   projectStyle,
			Type:    RowGroup,
			GroupID: groupID,
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

			// Combined CPU / Memory
			cpuMem := c.CPU + " / " + c.Memory
			cpuMemCell := Cell{Value: cpuMem, Style: bodyStyle}
			if c.CPU == "—" && c.Memory == "—" {
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

// rebuildTableRows rebuilds rows from groups, filtering collapsed
// groups, and updates the table while preserving selection.
func (p *Pane) rebuildTableRows() {
	rows := buildTableRows(p.theme, p.groups, p.collapsedGroups)
	p.table = p.table.WithRows(rows)
	if len(rows) > 0 && p.table.HighlightedRow() < 0 {
		p.table.SelectFirst()
	}
}

// restoreSelection attempts to restore the previously selected row
// after a table rebuild.  Falls back to SelectFirst if the previous
// row no longer exists or its GroupID changed.
func (p *Pane) restoreSelection(prevIdx int, prevGID string) {
	if prevIdx < 0 || prevIdx >= p.table.RowCount() {
		p.table.SelectFirst()
		return
	}
	// If the same group ID is at the same index, keep it
	if p.table.GroupIDAt(prevIdx) == prevGID && prevGID != "" {
		p.table.SelectFirst()
		for i := 0; i < prevIdx; i++ {
			p.table.MoveSelection(1)
		}
		return
	}
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

// renderLoading draws a centred spinner + message while containers
// are being fetched.
func (p Pane) renderLoading(width, height int) string {
	spinner := spinnerFrames[p.spinnerIdx]
	msg := lipgloss.NewStyle().
		Foreground(p.theme.Foreground).
		Render(spinner + "  Loading containers...")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(p.theme.Background).
		Align(lipgloss.Center, lipgloss.Center).
		Render(msg)
}

// renderEmpty draws a centred message when no containers were found.
func (p Pane) renderEmpty(width, height int) string {
	msg := lipgloss.NewStyle().
		Foreground(p.theme.TabInactive).
		Render("No containers found.")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(p.theme.Background).
		Align(lipgloss.Center, lipgloss.Center).
		Render(msg)
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
func fetchContainers(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		groups, err := dc.ListContainers()
		if err == nil {
			started, _ := dc.GetStartedTimes()
			mergeStarted(groups, started)
		}
		return containersLoadedMsg{groups: groups, err: err}
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
// container resource stats from the Docker daemon.
func fetchStats(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		stats, err := dc.GetStats()
		return statsRefreshMsg{stats: stats, err: err}
	}
}

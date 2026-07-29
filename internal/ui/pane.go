package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/samanar/docktor/internal/docker"
)

// ── Column keys ──────────────────────────────────────────────────

const (
	// Container columns
	colIcon         = "icon"
	colName         = "name"
	colOriginalName = "originalName" // hidden — full Docker container name
	colStatus       = "status"
	colCPUMem       = "cpumem"
	colPorts        = "ports"
	colBuilt        = "built"
	colRestarted    = "restarted"

	// Network columns
	colDriver     = "driver"
	colScope      = "scope"
	colSubnet     = "subnet"
	colGateway    = "gateway"
	colContainers = "containers"
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

// actionExecutedMsg is sent after a container action (start/stop/etc.)
// completes.  The app should refresh container data afterwards.
type actionExecutedMsg struct {
	action string // "start", "stop", "restart", "kill"
	name   string
	err    error
}

// imagesLoadedMsg is sent when docker image list has been fetched.
type imagesLoadedMsg struct {
	images []docker.Image
	err    error
}

// ── Network messages ─────────────────────────────────────────────

// networksLoadedMsg is sent when network data has been fetched.
type networksLoadedMsg struct {
	groups []docker.NetworkGroup
	err    error
}

// networkInspectLoadedMsg carries the raw JSON output of
// `docker network inspect` for the selected network.
type networkInspectLoadedMsg struct {
	name string
	json string
	err  error
}

// networkActionExecutedMsg is sent after a network action
// (inspect refresh, prune) completes.
type networkActionExecutedMsg struct {
	action string // "inspect", "prune"
	err    error
}

// volumesLoadedMsg is sent when Docker volumes have been fetched.
type volumesLoadedMsg struct {
	volumes []docker.Volume
	err     error
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
	actions    []Action
	allActions map[string][]Action // per-tab action sets

	// Focus
	focused bool

	// ── Custom table ─────────────────────────────────
	table Table

	// ── Loading & Docker ─────────────────────────────
	loading      bool
	spinnerIdx   int
	dockerClient docker.Client

	// ── Raw grouped container data ───────────────────
	groups []docker.ContainerGroup

	// ── Image data ───────────────────────────────────
	images []docker.Image

	// ── Volume data ──────────────────────────────────
	volumes        []docker.Volume
	volumesLoading bool

	// ── Collapsible groups ───────────────────────────
	collapsedGroups map[string]bool

	// ── Search ───────────────────────────────────────
	searchMode     bool   // true when search bar is active
	searchQuery    string // current search text
	searchMatches  []int  // table row indices matching the query
	searchMatchIdx int    // current position within searchMatches

	// ── Vim navigation ───────────────────────────────
	pendingG bool // waiting for second 'g' for gg (go to top)

	// ── Network state ────────────────────────────────
	networks         []docker.NetworkGroup
	networkLoading   bool
	networkCollapsed map[string]bool

	// ── Error state ─────────────────────────────────
	lastError string // non-empty when Docker is unreachable or an operation failed

	// ── Compose loading ─────────────────────────────
	composeLoadingID string // group ID of a compose action in progress
}

// ── Action sets per tab ──────────────────────────────────────────

var containerActions = []Action{
	{Key: 's', Label: "Start"},
	{Key: 'x', Label: "Stop"},
	{Key: 'r', Label: "Restart"},
	{Key: 'K', Label: "Kill"},
	{Key: 'e', Label: "Exec"},
	{Key: '/', Label: "Filter"},
	{Key: 'q', Label: "Quit"},
}

var imageActions = []Action{
	{Key: '/', Label: "Filter"},
	{Key: 'q', Label: "Quit"},
}

var networkActions = []Action{
	{Key: 'I', Label: "Inspect"},
	{Key: 'P', Label: "Prune"},
	{Key: '/', Label: "Filter"},
	{Key: 'q', Label: "Quit"},
}

var composeActions = []Action{
	{Key: 'u', Label: "Up"},
	{Key: 'd', Label: "Down"},
	{Key: 'p', Label: "Pull"},
	{Key: 'b', Label: "Build"},
	{Key: 'R', Label: "Restart"},
	{Key: '/', Label: "Filter"},
	{Key: 'q', Label: "Quit"},
}

// NewPane creates a new navigator pane with default tabs, actions,
// and a Docker client for live container data.
func NewPane(theme Theme, dc docker.Client) Pane {
	tabs := []Tab{
		{Key: 'c', Label: "Containers"},
		{Key: 'i', Label: "Images"},
		{Key: 'v', Label: "Volumes"},
		{Key: 'N', Label: "Networks"},
	}

	containerActions := []Action{
		{Key: 's', Label: "Start"},
		{Key: 'x', Label: "Stop"},
		{Key: 'r', Label: "Restart"},
		{Key: 'K', Label: "Kill"},
		{Key: 'e', Label: "Exec"},
		{Key: '/', Label: "Filter"},
		{Key: 'q', Label: "Quit"},
	}

	imageActions := []Action{
		{Key: '/', Label: "Filter"},
		{Key: 'q', Label: "Quit"},
	}

	allActions := map[string][]Action{
		"containers": containerActions,
		"images":     imageActions,
		"volumes":    {{Key: '/', Label: "Filter"}, {Key: 'q', Label: "Quit"}},
		"networks":   {{Key: '/', Label: "Filter"}, {Key: 'q', Label: "Quit"}},
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
		theme:            theme,
		tabs:             tabs,
		actions:          containerActions,
		allActions:       allActions,
		activeTab:        0,
		focused:          true,
		table:            tbl,
		loading:          true,
		spinnerIdx:       0,
		dockerClient:     dc,
		collapsedGroups:  make(map[string]bool),
		networkCollapsed: make(map[string]bool),
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
		if msg.err != nil {
			p.lastError = "Docker error: " + msg.err.Error()
			return p, nil
		}
		p.lastError = ""
		if p.activeTab == 0 {
			p.groups = msg.groups
			p.rebuildTableRows()
			p.recalcTable()
			// Only start stats loop if on containers tab
			if p.ActiveTabKey() == 'c' {
				return p, statsTick()
			}
		}
		return p, nil

	// ── Network data arrived ─────────────────────────
	case networksLoadedMsg:
		p.networkLoading = false
		p.loading = false
		if msg.err != nil {
			p.lastError = "Docker error: " + msg.err.Error()
			return p, nil
		}
		p.lastError = ""
		p.networks = msg.groups
		p.rebuildTableRows()
		p.recalcTable()
		return p, nil

	// ── Network action completed ─────────────────────
	case networkActionExecutedMsg:
		if msg.action == "prune" {
			// Refresh network list after pruning
			p.networkLoading = true
			p.loading = true
			return p, tea.Batch(
				fetchNetworks(p.dockerClient),
				spinnerTick(),
			)
		}
		return p, nil

	// ── Image data arrived ────────────────────────────
	case imagesLoadedMsg:
		p.loading = false
		if msg.err != nil {
			p.lastError = "Docker error: " + msg.err.Error()
			return p, nil
		}
		p.lastError = ""
		if p.activeTab == 1 {
			p.images = msg.images
			p.rebuildImageRows()
			p.recalcTable()
		}
		return p, nil

	// ── Volume data arrived ───────────────────────────
	case volumesLoadedMsg:
		p.volumesLoading = false
		if msg.err != nil {
			p.lastError = "Docker error: " + msg.err.Error()
			return p, nil
		}
		p.lastError = ""
		p.volumes = msg.volumes
		p.buildAndSetVolumeRows()
		p.recalcTable()
		return p, nil

	// ── Stats refresh cycle ───────────────────────────
	case statsTickMsg:
		if p.ActiveTabKey() != 'c' {
			return p, nil
		}
		return p, fetchStats(p.dockerClient)

	case statsRefreshMsg:
		if p.ActiveTabKey() != 'c' {
			return p, nil
		}
		if msg.err != nil {
			p.lastError = "Stats error: " + msg.err.Error()
			// Keep trying — daemon may come back
			return p, statsTick()
		}
		p.lastError = ""
		docker.MergeStats(p.groups, msg.stats)
		sel := p.table.HighlightedRow()
		gid := p.table.GroupIDAt(sel)
		p.rebuildTableRows()
		// Try to restore previous selection
		p.restoreSelection(sel, gid)
		// Ensure the selected row is visible after rebuild
		p.table.EnsureVisible()
		return p, statsTick()

	// ── Spinner animation ─────────────────────────────
	case spinnerTickMsg:
		if p.loading || p.volumesLoading || p.composeLoadingID != "" {
			p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerFrames)
			if p.composeLoadingID != "" && p.activeTab == 0 {
				p.rebuildTableRows()
			}
			return p, spinnerTick()
		}
		return p, nil

	case tea.WindowSizeMsg:
		// Dimensions are set by AppModel before passing the message.
		p.recalcTable()

	case tea.KeyMsg:
		if !p.focused {
			return p, nil
		}

		// ── Search mode: intercept all keys ────────────
		if p.searchMode {
			switch msg.Type {
			case tea.KeyEscape:
				p.searchMode = false
				p.searchQuery = ""
				return p, nil

			case tea.KeyEnter:
				p.searchMode = false
				p.doSearch()
				return p, nil

			case tea.KeyBackspace:
				if len(p.searchQuery) > 0 {
					p.searchQuery = p.searchQuery[:len(p.searchQuery)-1]
				}
				return p, nil

			case tea.KeyRunes:
				p.searchQuery += string(msg.Runes)
				return p, nil
			}
			return p, nil
		}

		// ── Not in search mode ─────────────────────────
		// Block navigation while loading
		if p.loading || p.volumesLoading {
			return p, nil
		}

		// Reset gg detection on any non-g key
		if msg.Type != tea.KeyRunes || string(msg.Runes) != "g" {
			p.pendingG = false
		}

		// ── Arrow keys + vim navigation ────────────────
		switch msg.Type {

		case tea.KeyUp:
			p.table.MoveSelection(-1)

		case tea.KeyDown:
			p.table.MoveSelection(1)

		case tea.KeyCtrlD:
			p.scrollHalfPage(1)

		case tea.KeyCtrlU:
			p.scrollHalfPage(-1)

		case tea.KeyEnter:
			p.toggleGroup()

		// ── Character keys ─────────────────────────────
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "j":
				p.pendingG = false
				p.table.MoveSelection(1)
			case "k":
				p.pendingG = false
				p.table.MoveSelection(-1)

			// Toggle group collapse (vim-style l / h)
			case "l", "h":
				p.pendingG = false
				p.toggleGroup()

			// Vim: gg → top, G → bottom
			case "g":
				if p.pendingG {
					p.pendingG = false
					p.table.SelectFirst()
				} else {
					p.pendingG = true
				}
				return p, nil
			case "G":
				p.pendingG = false
				p.goToLastRow()

			// Search
			case "/":
				p.searchMode = true
				p.searchQuery = ""
				return p, nil
			case "n":
				p.nextSearchMatch()

			// Tab switching
			case "c":
				return p, p.switchTab('c')
			case "i":
				return p, p.switchTab('i')
			case "v":
				return p, p.switchTab('v')
			case "N":
				return p, p.switchTab('N')

			// Toggle group collapse (containers only)
			case " ":
				if p.activeTab == 0 {
					p.toggleGroup()
				}

			// ── Actions ─────────────────────────────
			case "s":
				if p.activeTab == 0 {
					return p, p.doAction("start", p.dockerClient.StartContainer)
				}
			case "x":
				if p.activeTab == 0 {
					return p, p.doAction("stop", p.dockerClient.StopContainer)
				}
			case "r":
				if p.activeTab == 0 {
					return p, p.doAction("restart", p.dockerClient.RestartContainer)
				}
			case "K":
				if p.activeTab == 0 {
					return p, p.doAction("kill", p.dockerClient.KillContainer)
				}
			case "e":
				if p.activeTab == 0 {
					return p, p.doExec()
				}

			// ── Compose actions (containers tab, group selected) ──
			case "u", "d", "p", "b", "R":
				if p.activeTab == 0 {
					if cf := p.composeFileForSelectedGroup(); cf != "" {
						p.composeLoadingID = p.table.GroupIDAt(p.table.HighlightedRow())
						p.spinnerIdx = 0
						return p, tea.Batch(
							p.doComposeAction(string(msg.Runes), cf),
							spinnerTick(),
						)
					}
				}

			// ── Network actions ────────────────────
			case "I":
				if p.ActiveTabKey() == 'N' {
					return p, p.doNetworkAction("inspect")
				}
			case "P":
				if p.ActiveTabKey() == 'N' {
					return p, p.doNetworkAction("prune")
				}

			// Quit — delegated to AppModel
			case "q":
				return p, func() tea.Msg { return tea.Quit() }
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

	// Sync table focus state with pane focus — the table highlight
	// should dim when the pane is not focused (e.g., user pressed 2 or 3).
	p.table = p.table.Focused(p.focused)

	innerW := p.width - 2  // minus left/right borders
	innerH := p.height - 2 // minus top/bottom borders

	// ── Tab bar ──────────────────────────────────────
	tabBar := p.renderTabBar(innerW)

	// ── Divider ──────────────────────────────────────
	tabDivider := p.renderDivider(innerW)

	// ── Action bar (top, only when pane is focused) ──
	var actionBar string
	var actionDivider string
	actionBarH := 0
	if p.focused {
		actionBar = p.renderActionBar(innerW)
		actionDivider = p.renderDivider(innerW)
		actionBarH = 2
	}

	// ── Content area ─────────────────────────────────
	// Reserve space for search bar when active
	searchBarH := 0
	if p.searchMode {
		searchBarH = 1
	}
	// Fixed chrome: tabBar(1) + tabDivider(1) + [actionBar + actionDivider](0 or 2)
	contentH := innerH - 2 - actionBarH - searchBarH
	if contentH < 1 {
		contentH = 1
	}
	var contentArea string
	if p.lastError != "" {
		contentArea = p.renderError(innerW, contentH, p.lastError)
	} else if p.loading {
		contentArea = p.renderLoading(innerW, contentH, "Loading containers...")
	} else if p.volumesLoading {
		contentArea = p.renderLoading(innerW, contentH, "Loading volumes...")
	} else if p.activeTab == 2 && len(p.volumes) == 0 {
		contentArea = p.renderEmpty(innerW, contentH, "No volumes found.")
	} else if len(p.groups) == 0 && len(p.volumes) == 0 {
		contentArea = p.renderEmpty(innerW, contentH, "No containers found.")
	} else {
		contentArea = padLines(p.table.View(), innerW, 2)
	}

	// ── Search bar ───────────────────────────────────
	var searchBar string
	if p.searchMode {
		searchBar = p.renderSearchBar(innerW)
	}

	// ── Assemble inner content ───────────────────────
	parts := []string{tabBar, tabDivider}
	if p.focused {
		parts = append(parts, actionBar, actionDivider)
	}
	parts = append(parts, contentArea)
	if p.searchMode {
		parts = append(parts, searchBar)
	}
	inner := lipgloss.JoinVertical(lipgloss.Top, parts...)

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
	// Dynamically pick actions: compose actions when a compose group
	// header is selected on the containers tab.
	actions := p.actions
	if p.activeTab == 0 {
		sel := p.table.HighlightedRow()
		if p.table.RowTypeAt(sel) == RowGroup {
			if cf := p.composeFileForSelectedGroup(); cf != "" {
				actions = composeActions
			}
		}
	}

	var parts []string
	for _, a := range actions {
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

// renderSearchBar draws the search input bar with the current query
// and a blinking cursor indicator.
func (p Pane) renderSearchBar(width int) string {
	prompt := "/"
	query := p.searchQuery + "█" // cursor indicator

	text := lipgloss.NewStyle().
		Foreground(p.theme.Foreground).
		Render(prompt + query)

	count := ""
	if len(p.searchMatches) > 0 {
		count = fmt.Sprintf("[%d/%d]", p.searchMatchIdx+1, len(p.searchMatches))
		count = lipgloss.NewStyle().
			Foreground(p.theme.TabInactive).
			Render(count)
	}

	// Left-align the search text, right-align the match counter
	left := lipgloss.NewStyle().Width(width - 5).Render(text)
	right := lipgloss.NewStyle().Width(5).Align(lipgloss.Right).Render(count)

	return lipgloss.NewStyle().
		Width(width).
		Background(p.theme.ActionBarBackground).
		Padding(0, 1).
		Render(left + right)
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
		if p.ActiveTabKey() == 'N' {
			p.networkCollapsed[gid] = !p.networkCollapsed[gid]
		} else {
			p.collapsedGroups[gid] = !p.collapsedGroups[gid]
		}
		p.rebuildTableRows()
		p.recalcTable()
	}
}

func (p *Pane) recalcTable() {
	// Fixed chrome: borders(2) + tabBar(1) + tabDivider(1) +
	// [actionBar + actionDivider](0 or 2) = 4 or 6.
	// Table output adds header(1) + tableDivider(1) = 2 more.
	actionChrome := 0
	if p.focused {
		actionChrome = 2
	}
	searchH := 0
	if p.searchMode {
		searchH = 1
	}
	contentH := p.height - 4 - actionChrome - searchH
	if contentH < 1 {
		contentH = 1
	}
	// Table renders: header(1) + divider(1) + dataH rows = contentH
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

// composeFileForSelectedGroup returns the compose file path for the
// currently selected group header, or "" if the selection is not on
// a compose group or the group is "Other".
func (p Pane) composeFileForSelectedGroup() string {
	sel := p.table.HighlightedRow()
	gid := p.table.GroupIDAt(sel)
	for _, g := range p.groups {
		if "group:"+g.Project == gid && g.ComposeFile != "" {
			return g.ComposeFile
		}
	}
	return ""
}

// ClearComposeLoading clears the compose loading state and rebuilds
// the table rows so the spinner disappears immediately.
func (p *Pane) ClearComposeLoading() {
	p.composeLoadingID = ""
	p.rebuildTableRows()
}

// switchTab changes the active tab and performs setup: swapping
// column definitions, actions, and triggering data fetch if needed.
// Returns a tea.Cmd to kick off async data loading, or nil.
func (p *Pane) switchTab(key rune) tea.Cmd {
	p.activeTab = p.tabIndexByKey(key)

	switch key {
	case 'N':
		// Swap to network columns and actions
		p.table = p.table.WithColumns(networkColumns())
		p.actions = networkActions
		// Trigger network fetch on first visit or re-fetch
		if p.networks == nil {
			p.networkLoading = true
			p.loading = true
			return tea.Batch(
				fetchNetworks(p.dockerClient),
				spinnerTick(),
			)
		}
		// Already have data, just rebuild rows
		p.loading = false
		p.rebuildTableRows()
		p.recalcTable()
		return nil

	case 'i':
		// Swap to image columns and actions
		p.table = p.table.WithColumns(imageColumns())
		p.actions = imageActions
		p.loading = true
		return tea.Batch(
			fetchImages(p.dockerClient),
			spinnerTick(),
		)

	case 'v':
		// Swap to volume columns and actions
		p.volumesLoading = true
		p.table = p.table.WithRows(nil) // clear rows while loading
		return tea.Batch(
			fetchVolumes(p.dockerClient),
			spinnerTick(),
		)

	default:
		// Swap back to container columns and actions
		p.table = p.table.WithColumns(containerColumns())
		p.actions = containerActions
		p.loading = (p.groups == nil)
		p.rebuildTableRows()
		p.recalcTable()
		return nil
	}
}

// retryTab clears the error state and re-fetches data for the
// currently active tab.  Called when the user presses 'r' while
// an error is displayed.
func (p *Pane) retryTab() tea.Cmd {
	p.lastError = ""
	p.loading = true
	switch p.ActiveTabKey() {
	case 'N':
		p.networkLoading = true
		return tea.Batch(
			fetchNetworks(p.dockerClient),
			spinnerTick(),
		)
	case 'i':
		return tea.Batch(
			fetchImages(p.dockerClient),
			spinnerTick(),
		)
	case 'v':
		p.volumesLoading = true
		return tea.Batch(
			fetchVolumes(p.dockerClient),
			spinnerTick(),
		)
	default: // containers
		return tea.Batch(
			fetchContainers(p.dockerClient),
			spinnerTick(),
		)
	}
}

// SelectedContainer returns the full Docker name of the currently
// highlighted container, or empty string if a group header is selected
// or if the containers tab is not active.
func (p Pane) SelectedContainer() string {
	if p.activeTab != 0 {
		return ""
	}
	sel := p.table.HighlightedRow()
	if p.table.RowTypeAt(sel) != RowData {
		return ""
	}
	name, _ := p.table.GetCell(sel, colOriginalName)
	return name
}

// SelectedVolume returns the name of the currently highlighted volume,
// or empty string if nothing is selected or not on the volumes tab.
func (p Pane) SelectedVolume() string {
	if p.activeTab != 2 {
		return ""
	}
	sel := p.table.HighlightedRow()
	if p.table.RowTypeAt(sel) != RowData {
		return ""
	}
	name, _ := p.table.GetCell(sel, colOriginalName)
	return name
}

// ActiveTab returns the index of the currently active tab.
func (p Pane) ActiveTab() int { return p.activeTab }

// FindVolume returns a pointer to the Volume with the given name,
// or nil if not found.
func (p Pane) FindVolume(name string) *docker.Volume {
	for i := range p.volumes {
		if p.volumes[i].Name == name {
			return &p.volumes[i]
		}
	}
	return nil
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

// GetSelectedGroup returns the ContainerGroup for the currently
// highlighted group header, or nil if a container row is selected
// or the containers tab is not active.
func (p Pane) GetSelectedGroup() *docker.ContainerGroup {
	if p.activeTab != 0 {
		return nil
	}
	sel := p.table.HighlightedRow()
	if p.table.RowTypeAt(sel) != RowGroup {
		return nil
	}
	gid := p.table.GroupIDAt(sel)
	for i := range p.groups {
		if "group:"+p.groups[i].Project == gid {
			return &p.groups[i]
		}
	}
	return nil
}

// SelectedImage returns the repo:tag of the currently highlighted
// image, or empty string if none is selected or on a different tab.
func (p Pane) SelectedImage() string {
	if p.activeTab != 1 {
		return ""
	}
	sel := p.table.HighlightedRow()
	if sel < 0 || sel >= len(p.images) {
		return ""
	}
	img := p.images[sel]
	if img.Repo == "<none>" {
		return img.ID
	}
	return img.Repo + ":" + img.Tag
}

// GetSelectedImage returns the full Image object for the currently
// highlighted row, or nil if not on the images tab.
func (p Pane) GetSelectedImage() *docker.Image {
	if p.activeTab != 1 {
		return nil
	}
	sel := p.table.HighlightedRow()
	if sel < 0 || sel >= len(p.images) {
		return nil
	}
	return &p.images[sel]
}

// rebuildImageRows rebuilds the table rows from image data.
func (p *Pane) rebuildImageRows() {
	rows := buildImageRows(p.theme, p.images)
	p.table = p.table.WithRows(rows)
	if len(rows) > 0 && p.table.HighlightedRow() < 0 {
		p.table.SelectFirst()
	}
}

// ActiveTabKey returns the key of the currently active tab.
func (p Pane) ActiveTabKey() rune {
	if p.activeTab >= 0 && p.activeTab < len(p.tabs) {
		return p.tabs[p.activeTab].Key
	}
	return 0
}

// ── Network selection ────────────────────────────────────────────

// SelectedNetwork returns the full Docker name of the currently
// highlighted network, or empty string if the networks tab is not
// active or a group header is selected.
func (p Pane) SelectedNetwork() string {
	if p.ActiveTabKey() != 'N' {
		return ""
	}
	sel := p.table.HighlightedRow()
	if p.table.RowTypeAt(sel) != RowData {
		return ""
	}
	name, _ := p.table.GetCell(sel, colOriginalName)
	return name
}

// GetSelectedNetwork returns the full Network object for the
// currently highlighted row, or nil if not available.
func (p Pane) GetSelectedNetwork() *docker.Network {
	name := p.SelectedNetwork()
	if name == "" {
		return nil
	}
	for _, g := range p.networks {
		for i := range g.Networks {
			if g.Networks[i].Name == name {
				return &g.Networks[i]
			}
		}
	}
	return nil
}

// doNetworkAction returns a tea.Cmd that executes a network-level
// action. Returns nil if the action is not applicable.
func (p Pane) doNetworkAction(action string) tea.Cmd {
	ctx := context.Background()
	switch action {
	case "prune":
		return func() tea.Msg {
			_, err := p.dockerClient.PruneNetworks(ctx)
			return networkActionExecutedMsg{action: "prune", err: err}
		}
	case "inspect":
		name := p.SelectedNetwork()
		if name == "" {
			return nil
		}
		return func() tea.Msg {
			raw, err := p.dockerClient.InspectNetworkRaw(ctx, name)
			return networkInspectLoadedMsg{name: name, json: raw, err: err}
		}
	}
	return nil
}

// doAction returns a tea.Cmd that executes the given action function
// on the currently selected container.  Returns nil if no container
// is selected.
func (p Pane) doAction(action string, fn func(context.Context, string) error) tea.Cmd {
	name := p.SelectedContainer()
	if name == "" {
		return nil
	}
	return func() tea.Msg {
		err := fn(context.Background(), name)
		return actionExecutedMsg{action: action, name: name, err: err}
	}
}

// doExec suspends the TUI, spawns an interactive shell inside the
// currently selected container via `docker exec -it`, and resumes
// the TUI when the shell exits.
func (p Pane) doExec() tea.Cmd {
	name := p.SelectedContainer()
	if name == "" {
		return nil
	}
	return tea.Sequence(
		tea.ExitAltScreen,
		tea.ExecProcess(
			exec.Command("docker", "exec", "-it", name, "sh"),
			nil,
		),
		tea.EnterAltScreen,
	)
}

// doComposeAction returns a tea.Cmd that executes a docker-compose
// action on the specified compose file.
func (p Pane) doComposeAction(key string, composeFile string) tea.Cmd {
	var action string
	switch key {
	case "u":
		action = "up"
	case "d":
		action = "down"
	case "p":
		action = "pull"
	case "b":
		action = "build"
	case "R":
		action = "restart"
	default:
		return nil
	}

	return func() tea.Msg {
		var err error
		ctx := context.Background()
		switch key {
		case "u":
			err = p.dockerClient.ComposeUp(ctx, composeFile)
		case "d":
			err = p.dockerClient.ComposeDown(ctx, composeFile)
		case "p":
			err = p.dockerClient.ComposePull(ctx, composeFile)
		case "b":
			err = p.dockerClient.ComposeBuild(ctx, composeFile)
		case "R":
			err = p.dockerClient.ComposeRestart(ctx, composeFile)
		}
		return actionExecutedMsg{action: "compose-" + action, name: composeFile, err: err}
	}
}

// ── Search ────────────────────────────────────────────────────────

// doSearch finds all rows whose container name contains the current
// search query (case-insensitive) and jumps to the first match.
func (p *Pane) doSearch() {
	q := strings.ToLower(p.searchQuery)
	if q == "" {
		p.searchMatches = nil
		p.searchMatchIdx = 0
		return
	}

	p.searchMatches = nil
	for i := 0; i < p.table.RowCount(); i++ {
		if p.table.RowTypeAt(i) != RowData {
			continue
		}
		name, ok := p.table.GetCell(i, colName)
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(name), q) {
			p.searchMatches = append(p.searchMatches, i)
		}
	}

	if len(p.searchMatches) > 0 {
		p.searchMatchIdx = 0
		p.jumpToRow(p.searchMatches[0])
	}
}

// nextSearchMatch moves to the next search match, wrapping around
// after the last one.
func (p *Pane) nextSearchMatch() {
	if len(p.searchMatches) == 0 {
		return
	}
	p.searchMatchIdx = (p.searchMatchIdx + 1) % len(p.searchMatches)
	p.jumpToRow(p.searchMatches[p.searchMatchIdx])
}

// jumpToRow sets the table selection to the given row index and
// scrolls it into view.
func (p *Pane) jumpToRow(idx int) {
	if idx < 0 || idx >= p.table.RowCount() {
		return
	}
	// Move to first row, then step to target
	p.table.SelectFirst()
	for i := 0; i < idx; i++ {
		p.table.MoveSelection(1)
	}
}

// goToLastRow moves the selection to the last row in the table.
func (p *Pane) goToLastRow() {
	n := p.table.RowCount()
	if n == 0 {
		return
	}
	p.jumpToRow(n - 1)
}

// scrollHalfPage moves the selection by half the visible table height.
// Positive delta scrolls down, negative scrolls up.
func (p *Pane) scrollHalfPage(delta int) {
	half := p.table.VisibleHeight() / 2
	if half < 1 {
		half = 1
	}
	for i := 0; i < half; i++ {
		p.table.MoveSelection(delta)
	}
}

// SelectedContainerImage returns the image name of the currently
// highlighted container, or "" if none is selected.
func (p Pane) SelectedContainerImage() string {
	ctr := p.GetSelectedContainer()
	if ctr == nil {
		return ""
	}
	return ctr.Image
}

// SetContainerImageSize updates the ImageSize field for the
// container with the given name.  This is called asynchronously
// when the image size has been fetched.
func (p *Pane) SetContainerImageSize(name, size string) {
	for gi := range p.groups {
		for ci := range p.groups[gi].Containers {
			if p.groups[gi].Containers[ci].Name == name {
				p.groups[gi].Containers[ci].ImageSize = size
				return
			}
		}
	}
}

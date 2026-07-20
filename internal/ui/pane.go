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
	dockerClient *docker.Client

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
}

// ── Action sets per tab ──────────────────────────────────────────

var containerActions = []Action{
	{Key: 's', Label: "Start"},
	{Key: 'x', Label: "Stop"},
	{Key: 'r', Label: "Restart"},
	{Key: 'K', Label: "Kill"},
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

// NewPane creates a new navigator pane with default tabs, actions,
// and a Docker client for live container data.
func NewPane(theme Theme, dc *docker.Client) Pane {
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
		if p.loading || p.volumesLoading {
			p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerFrames)
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
	divider := p.renderDivider(innerW)

	// ── Content area ─────────────────────────────────
	// Reserve space for search bar when active
	searchBarH := 0
	if p.searchMode {
		searchBarH = 1
	}
	// tabBar(1) + content(header+data) + divider(1) + actionBar(1) = 3 fixed lines
	contentH := innerH - 3 - searchBarH
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

	// ── Action bar ───────────────────────────────────
	actionBar := p.renderActionBar(innerW)

	// ── Assemble inner content ───────────────────────
	parts := []string{tabBar, divider, contentArea}
	if p.searchMode {
		parts = append(parts, searchBar)
	}
	parts = append(parts, divider, actionBar)
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
	// Fixed chrome: borders(2) + tabBar(1) + divider(1) + actionBar(1)
	// The bottom divider is part of the content area (table header separator
	// doubles as the divider), so we save 1 line.
	searchH := 0
	if p.searchMode {
		searchH = 1
	}
	contentH := p.height - 2 - 1 - 1 - 1 - searchH
	if contentH < 1 {
		contentH = 1
	}
	// Table renders: header(1) + dataH rows = contentH
	// (the table's own divider line shares space with the UI divider)
	dataH := contentH - 1
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
	switch action {
	case "prune":
		return func() tea.Msg {
			_, err := p.dockerClient.PruneNetworks()
			return networkActionExecutedMsg{action: "prune", err: err}
		}
	case "inspect":
		name := p.SelectedNetwork()
		if name == "" {
			return nil
		}
		return func() tea.Msg {
			raw, err := p.dockerClient.InspectNetworkRaw(name)
			return networkInspectLoadedMsg{name: name, json: raw, err: err}
		}
	}
	return nil
}

// doAction returns a tea.Cmd that executes the given action function
// on the currently selected container.  Returns nil if no container
// is selected.
func (p Pane) doAction(action string, fn func(string) error) tea.Cmd {
	name := p.SelectedContainer()
	if name == "" {
		return nil
	}
	return func() tea.Msg {
		err := fn(name)
		return actionExecutedMsg{action: action, name: name, err: err}
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
		rows = buildTableRows(p.theme, p.groups, p.collapsedGroups)
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

// fetchImages returns a command that asynchronously fetches Docker
// images.
func fetchImages(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		images, err := dc.ListImages()
		return imagesLoadedMsg{images: images, err: err}
	}
}

// fetchNetworks returns a command that asynchronously fetches
// Docker networks and groups them by driver.
func fetchNetworks(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		groups, err := dc.ListNetworks()
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
// container resource stats from the Docker daemon.
func fetchStats(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		stats, err := dc.GetStats()
		return statsRefreshMsg{stats: stats, err: err}
	}
}

// fetchVolumes returns a command that asynchronously fetches Docker
// volumes with their driver, mountpoint, and size information.
func fetchVolumes(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		volumes, err := dc.GetVolumes()
		return volumesLoadedMsg{volumes: volumes, err: err}
	}
}

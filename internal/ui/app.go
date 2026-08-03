package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/samanar/docktor/internal/docker"
)

// AppModel is the root Bubble Tea model for the docktor application.
// It owns three panes: navigator (left), overview (right), logs (bottom).
type AppModel struct {
	theme Theme
	pane  Pane
	dc    docker.Client

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

	// ── Bulk actions dialog ──────────────────────────
	bulkDialogOpen    bool
	bulkDialogIdx     int
	bulkActionLoading bool
	bulkActionLabel   string
	bulkSpinnerIdx    int
	bulkActionResult  string
	bulkActionErr     error
}

// NewApp creates the root application model with the given theme.
func NewApp(theme Theme) (AppModel, error) {
	dc, err := docker.NewClient()
	if err != nil {
		return AppModel{}, fmt.Errorf("docker client: %w", err)
	}
	return AppModel{
		theme:         theme,
		pane:          NewPane(theme, dc),
		dc:            dc,
		focus:         1,
		logAutoScroll: true,
	}, nil
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

// bulkActionResultMsg is sent when a bulk action (stop all, remove
// all, prune) completes.
type bulkActionResultMsg struct {
	action string
	output string
	err    error
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

	// ── Bulk action completed ──────────────────────
	case bulkActionResultMsg:
		m.bulkActionLoading = false
		m.bulkDialogOpen = false
		if msg.err != nil {
			m.bulkActionErr = msg.err
			m.bulkActionResult = ""
		} else {
			m.bulkActionErr = nil
			m.bulkActionResult = msg.output
		}
		// Refresh containers after bulk action
		return m, m.pane.Init()

	// ── Mouse ────────────────────────────────────────
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}

		paneW := int(float64(m.width) * 0.70)
		paneH := m.height / 3
		if paneH < 5 {
			paneH = 5
		}

		inLeft := msg.X < paneW
		inTop := msg.Y < paneH

		// ── Wheel events ───────────────────────────
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			if inLeft && inTop {
				// Wheel in navigator pane → scroll table.
				newPane, cmd := m.pane.Update(msg)
				m.pane = newPane
				return m, cmd
			} else if m.focus == 3 {
				// Wheel in log / detail pane → scroll content.
				delta := 1
				if msg.Button == tea.MouseButtonWheelUp {
					delta = -1
				}
				for i := 0; i < 3; i++ {
					if m.pane.ActiveTabKey() == 'N' {
						if delta > 0 {
							m.scrollNetworkDetailDown()
						} else {
							m.scrollNetworkDetailUp()
						}
					} else {
						if delta > 0 {
							m.scrollLogsDown()
						} else {
							m.scrollLogsUp()
						}
					}
				}
				return m, nil
			}
			return m, nil
		}

		// ── Left-click events ──────────────────────
		if msg.Button != tea.MouseButtonLeft {
			return m, nil
		}

		if inLeft && inTop {
			// Clicked on the navigator pane.  Pass the
			// mouse event to the pane FIRST so it can
			// calculate row selection using the pane's
			// CURRENT chrome layout (focused or not).
			// Then set focus and propagate the selection
			// change.
			newPane, cmd := m.pane.Update(msg)
			m.pane = newPane
			m.focus = 1
			m.pane.focused = true
			return m.handleSelectionChanges(m.pane, cmd)
		} else if !inLeft && inTop {
			// Clicked on the overview pane.
			m.focus = 2
			m.pane.focused = false
			return m, nil
		} else {
			// Clicked on the logs / detail pane.
			m.focus = 3
			m.pane.focused = false
			return m, nil
		}

	// ── Keyboard ─────────────────────────────────────
	case tea.KeyMsg:
		// ── Bulk dialog navigation (takes priority) ──
		if m.bulkDialogOpen {
			// Block interaction while a bulk action is running.
			if m.bulkActionLoading {
				return m, nil
			}
			switch msg.String() {
			case "esc", "q":
				m.bulkDialogOpen = false
				m.bulkActionResult = ""
				m.bulkActionErr = nil
				return m, nil
			case "j", "down":
				m.bulkDialogIdx = (m.bulkDialogIdx + 1) % 3
				return m, nil
			case "k", "up":
				m.bulkDialogIdx = (m.bulkDialogIdx - 1 + 3) % 3
				return m, nil
			case "enter":
				a := bulkActions[m.bulkDialogIdx]
				m.bulkActionLoading = true
				m.bulkActionLabel = a.label
				m.bulkSpinnerIdx = 0
				return m, m.executeBulkAction(m.bulkDialogIdx)
			}
			return m, nil
		}
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
		case "b":
			m.bulkDialogOpen = true
			m.bulkDialogIdx = 0
			m.bulkActionResult = ""
			m.bulkActionErr = nil
			return m, nil
		}

		// ── Pane navigation (focus 1) ─────────────────
		if m.focus == 1 {
			// Enter on a data row (not a group header) should
			// move focus to the logs / detail pane.
			isEnter := msg.Type == tea.KeyEnter
			if isEnter {
				if m.pane.SelectedContainer() != "" ||
					m.pane.SelectedImage() != "" ||
					m.pane.SelectedVolume() != "" ||
					m.pane.SelectedNetwork() != "" {
					m.focus = 3
					m.pane.focused = false
					_, cmd := m.pane.Update(msg)
					return m, cmd
				}
			}

			return m.handleSelectionChanges(m.pane.Update(msg))
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

	// Intercept spinner ticks to animate the bulk-action spinner.
	if m.bulkActionLoading {
		if _, ok := msg.(spinnerTickMsg); ok {
			m.bulkSpinnerIdx = (m.bulkSpinnerIdx + 1) % len(spinnerFrames)
			return m, spinnerTick()
		}
		// While loading, drop all other messages except
		// bulkActionResultMsg (handled above) and WindowSize.
		if _, ok := msg.(tea.WindowSizeMsg); !ok {
			return m, nil
		}
	}
	return m.handleSelectionChanges(m.pane.Update(msg))
}

// handleSelectionChanges detects which entity (container, network,
// image, or volume) is selected and dispatches the appropriate
// async data-fetch commands.  This is called from both the focus=1
// KeyMsg handler and the catch-all message path to avoid duplication.
func (m AppModel) handleSelectionChanges(pane Pane, cmd tea.Cmd) (AppModel, tea.Cmd) {
	m.pane = pane

	// Snapshot current selection state so we can detect changes.
	prevContainer := m.selectedName
	prevNetwork := m.selectedNetworkName
	prevImgID := m.selectedImageID
	prevVol := m.selectedVolume

	// ── Container selection ─────────────────────────
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

	// ── Network selection ───────────────────────────
	newNetwork := m.pane.SelectedNetwork()
	if newNetwork != "" && newNetwork != prevNetwork {
		m.selectedNetworkName = newNetwork
		m.selectedName = ""
		m.networkDetailAutoScroll = true
		return m, tea.Batch(cmd,
			fetchNetworkInspect(m.dc, newNetwork),
		)
	}

	// ── Image selection ─────────────────────────────
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

	// ── Volume selection ────────────────────────────
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

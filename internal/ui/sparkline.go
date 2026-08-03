package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/samanar/docktor/internal/docker"
)

// ── Sparkline data ───────────────────────────────────────────────

const maxSparklinePoints = 60 // 60 seconds of data at 1s refresh

// sparklineHistory holds a ring buffer of float64 values for a single
// metric (CPU or memory) keyed by entity name (container or group).
type sparklineHistory struct {
	data  []float64
	head  int // write position
	count int // number of valid entries
}

// newSparklineHistory creates an empty ring buffer.
func newSparklineHistory() *sparklineHistory {
	return &sparklineHistory{
		data: make([]float64, maxSparklinePoints),
	}
}

// push appends a value to the ring buffer.
func (h *sparklineHistory) push(v float64) {
	h.data[h.head] = v
	h.head = (h.head + 1) % maxSparklinePoints
	if h.count < maxSparklinePoints {
		h.count++
	}
}

// slices returns the data in chronological order (oldest first).
func (h *sparklineHistory) slices() []float64 {
	if h.count == 0 {
		return nil
	}
	out := make([]float64, h.count)
	start := h.head - h.count
	if start < 0 {
		start += maxSparklinePoints
	}
	for i := 0; i < h.count; i++ {
		idx := (start + i) % maxSparklinePoints
		out[i] = h.data[idx]
	}
	return out
}

// ── Inline sparkline rendering (single-line, fits on same row) ───

// sparklineChars maps values [0,1] to 8-level Unicode block chars.
var sparklineChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// renderSparkline draws a single-line sparkline string of the given
// width, using Unicode block characters.  Values are auto-scaled
// against a sliding window of the most recent points (not all-time
// max), so the chart stays responsive even after a spike subsides.
// Returns a styled string that fits on one line.
func renderSparkline(h *sparklineHistory, width int, color lipgloss.Color) string {
	if h == nil || h.count < 2 {
		return lipgloss.NewStyle().
			Foreground(color).
			Render(strings.Repeat("▁", width))
	}

	values := h.slices()
	if len(values) < 2 {
		return lipgloss.NewStyle().
			Foreground(color).
			Render(strings.Repeat("▁", width))
	}

	// Down-sample to fit the target width.
	samples := downsample(values, width)
	if len(samples) == 0 {
		return lipgloss.NewStyle().
			Foreground(color).
			Render(strings.Repeat("▁", width))
	}

	// Use a sliding-window max (last 30 points) so the scale adapts
	// when spikes subside.
	windowLen := 30
	if len(values) < windowLen {
		windowLen = len(values)
	}
	recent := values[len(values)-windowLen:]
	ceil := recent[0]
	for _, v := range recent {
		if v > ceil {
			ceil = v
		}
	}
	if ceil <= 0 {
		ceil = 1
	}

	var out strings.Builder
	for _, v := range samples {
		norm := v / ceil
		if norm > 1.0 {
			norm = 1.0
		}
		if norm < 0 {
			norm = 0
		}
		idx := int(norm * 7) // 0..7
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		out.WriteRune(sparklineChars[idx])
	}

	return lipgloss.NewStyle().
		Foreground(color).
		Render(out.String())
}

// downsample reduces a slice of float64 to exactly n points by
// taking the max of each bucket (preserves peaks better than avg).
func downsample(vals []float64, n int) []float64 {
	if n <= 0 || len(vals) == 0 {
		return nil
	}
	if len(vals) <= n {
		out := make([]float64, len(vals))
		copy(out, vals)
		return out
	}

	bucketSize := float64(len(vals)) / float64(n)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		start := int(float64(i) * bucketSize)
		end := int(float64(i+1) * bucketSize)
		if end > len(vals) {
			end = len(vals)
		}
		if start >= end {
			start = end - 1
			if start < 0 {
				start = 0
			}
		}
		// Take the max in this bucket (preserves spike visibility).
		max := vals[start]
		for j := start + 1; j < end; j++ {
			if vals[j] > max {
				max = vals[j]
			}
		}
		out[i] = max
	}
	return out
}

// ── History recording helpers ────────────────────────────────────

// recordSparklineHistory updates CPU and memory sparkline history
// for every container in the groups based on a fresh stats snapshot.
func (m *AppModel) recordSparklineHistory(
	groups []docker.ContainerGroup,
	stats map[string]docker.ContainerStats,
) {
	if m.cpuHistory == nil {
		m.cpuHistory = make(map[string]*sparklineHistory)
	}
	if m.memHistory == nil {
		m.memHistory = make(map[string]*sparklineHistory)
	}
	if m.groupCPUHistory == nil {
		m.groupCPUHistory = make(map[string]*sparklineHistory)
	}
	if m.groupMemHistory == nil {
		m.groupMemHistory = make(map[string]*sparklineHistory)
	}

	for gi := range groups {
		grp := groups[gi]
		var groupCPUTotal float64
		var groupMemTotal float64 // MiB

		for ci := range grp.Containers {
			name := grp.Containers[ci].Name
			s, ok := stats[name]

			// ── Per-container CPU ──────────────────
			cpuH := m.cpuHistory[name]
			if cpuH == nil {
				cpuH = newSparklineHistory()
				m.cpuHistory[name] = cpuH
			}
			var cpuVal float64
			if ok {
				cpuVal = parseCPUPercent(s.CPUPerc)
			}
			cpuH.push(cpuVal)
			groupCPUTotal += cpuVal

			// ── Per-container memory (MiB) ─────────
			memH := m.memHistory[name]
			if memH == nil {
				memH = newSparklineHistory()
				m.memHistory[name] = memH
			}
			var memVal float64
			if ok {
				used, _ := parseMemoryUsage(s.MemUsage)
				memVal = float64(used) / (1024 * 1024) // bytes → MiB
			}
			memH.push(memVal)
			groupMemTotal += memVal
		}

		// ── Group-level history ───────────────────
		gid := grp.Project
		if gid == "" {
			gid = "Other"
		}

		gCPU := m.groupCPUHistory[gid]
		if gCPU == nil {
			gCPU = newSparklineHistory()
			m.groupCPUHistory[gid] = gCPU
		}
		gCPU.push(groupCPUTotal)

		gMem := m.groupMemHistory[gid]
		if gMem == nil {
			gMem = newSparklineHistory()
			m.groupMemHistory[gid] = gMem
		}
		gMem.push(groupMemTotal)
	}
}

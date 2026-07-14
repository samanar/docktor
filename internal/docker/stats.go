package docker

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ── Types ──────────────────────────────────────────────────────────

// ContainerStats holds real-time resource usage for one container.
type ContainerStats struct {
	Name     string
	CPUPerc  string // e.g. "1.23%"
	MemUsage string // e.g. "80MiB / 1.5GiB" (used / total)
	MemPerc  string // e.g. "2.45%"
	NetIO    string // e.g. "1.2GB / 400MB" (tx / rx)
	BlockIO  string // e.g. "3.4GB / 1.2GB" (read / write)
}

// statsLine mirrors the JSON object emitted by
//
//	docker stats --no-stream --format '{{json .}}'
type statsLine struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
}

// ── Stats fetching ─────────────────────────────────────────────────

// GetStats returns a map of container name → resource stats for all
// running containers.  Stopped / exited containers are omitted by
// the Docker daemon.
func (c *Client) GetStats() (map[string]ContainerStats, error) {
	raw, err := runDockerCLI("stats", "--no-stream",
		"--format", `{{json .}}`)
	if err != nil {
		return nil, fmt.Errorf("docker stats: %w", err)
	}

	result := make(map[string]ContainerStats)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var sl statsLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			continue // skip malformed lines
		}
		result[sl.Name] = ContainerStats{
			Name:     sl.Name,
			CPUPerc:  sl.CPUPerc,
			MemUsage: sl.MemUsage,
			MemPerc:  sl.MemPerc,
			NetIO:    sl.NetIO,
			BlockIO:  sl.BlockIO,
		}
	}
	return result, nil
}

// MergeStats copies CPU / Memory / Network / Disk values from the
// stats map into the matching containers in-place.  Containers not
// present in the map keep their previous values.
func MergeStats(groups []ContainerGroup, stats map[string]ContainerStats) {
	for gi := range groups {
		for ci := range groups[gi].Containers {
			name := groups[gi].Containers[ci].Name
			if s, ok := stats[name]; ok {
				groups[gi].Containers[ci].CPU = s.CPUPerc
				groups[gi].Containers[ci].Memory = s.MemUsage
				groups[gi].Containers[ci].NetIO = s.NetIO
				groups[gi].Containers[ci].BlockIO = s.BlockIO
			}
		}
	}
}

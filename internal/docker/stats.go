package docker

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

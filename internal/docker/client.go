package docker

import (
	"encoding/json"
	"strings"
)

// ── Types ──────────────────────────────────────────────────────────

// Volume represents a single Docker volume.
type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
	Size       string // human-readable size, e.g. "824.6kB"
	SizeBytes  int64  // size in bytes for sorting
	CreatedAt  string
}

// VolumeFileUsage represents file/folder disk usage inside a volume.
type VolumeFileUsage struct {
	Name  string // file or folder name
	Size  string // human-readable size, e.g. "12MB"
	IsDir bool
}

// Container represents a single Docker container with the fields the
// TUI needs to display.
type Container struct {
	ID        string
	Name      string
	Image     string
	State     string // running, exited, paused, etc.
	Status    string // human-readable status, e.g. "Up 2 hours"
	Ports     string
	Project   string // Docker Compose project name (empty if none)
	CPU       string // populated by stats refresh (e.g. "1.23%")
	Memory    string // populated by stats refresh (e.g. "80MiB / 1.5GiB")
	NetIO     string // populated by stats refresh (e.g. "1.2GB / 400MB" tx/rx)
	BlockIO   string // populated by stats refresh (e.g. "3.4GB / 1.2GB" r/w)
	ImageSize string // populated on selection (e.g. "142MB")
	CreatedAt string // ISO timestamp from docker ps
	StartedAt string // ISO timestamp from docker inspect
}

// ContainerGroup is a named group of containers (a Compose project
// or the "Other" catch-all).
type ContainerGroup struct {
	Project     string
	ComposeFile string // path to docker-compose.yml (empty for "Other")
	Containers  []Container
}

// Image represents a Docker image with the fields the TUI needs.
type Image struct {
	ID        string // short image ID
	Repo      string // repository name (e.g. "nginx")
	Tag       string // tag (e.g. "latest")
	Size      string // human-readable size (e.g. "142MB")
	CreatedAt string // ISO timestamp
	Created   string // human-readable (e.g. "2 weeks ago")
}

// ImageLayer represents a single layer in a Docker image history.
type ImageLayer struct {
	ID      string // layer ID
	Created string // e.g. "2 weeks ago"
	Command string // the command that created this layer
	Size    string // human-readable size
}

// ── Network types ──────────────────────────────────────────────────

// NetworkContainer describes a container attached to a Docker network.
type NetworkContainer struct {
	Name     string
	IPv4Addr string
	IPv6Addr string
	MACAddr  string
}

// Network represents a Docker network with the fields the TUI needs
// to display.
type Network struct {
	ID         string
	Name       string
	Driver     string // bridge, overlay, host, macvlan, etc.
	Scope      string // local, swarm, global
	Internal   bool
	IPv6       bool
	Attachable bool
	Created    string
	Subnet     string // primary IPAM subnet, e.g. "172.18.0.0/16"
	Gateway    string // primary IPAM gateway, e.g. "172.18.0.1"
	IPRange    string // primary IPAM IP range (may be empty)
	Containers []NetworkContainer
	Labels     map[string]string
}

// NetworkGroup is a named group of networks sharing the same driver.
type NetworkGroup struct {
	Driver   string
	Networks []Network
}

// ── Shared helpers ─────────────────────────────────────────────────

// parseDUOutput parses the output of "du -sh /path/*" into
// VolumeFileUsage entries.
func parseDUOutput(raw string) []VolumeFileUsage {
	var entries []VolumeFileUsage
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		size := fields[0]
		itemPath := fields[1]

		name := itemPath
		// Strip trailing slash before extracting basename so that
		// "/vol/subdir/" yields "subdir", not "".
		if strings.HasSuffix(name, "/") {
			name = name[:len(name)-1]
		}
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}

		isDir := strings.HasSuffix(itemPath, "/") || itemPath[len(itemPath)-1] == '/'

		entries = append(entries, VolumeFileUsage{
			Name:  name,
			Size:  size,
			IsDir: isDir,
		})
	}

	// Sort by name since du output order varies.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].Name > entries[j].Name {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	return entries
}

// formatLayerCommand shortens the layer creation command for display.
func formatLayerCommand(cmd string) string {
	cmd = strings.TrimPrefix(cmd, "/bin/sh -c ")
	cmd = strings.TrimPrefix(cmd, "#(nop) ")
	if len(cmd) > 60 {
		cmd = cmd[:57] + "..."
	}
	return cmd
}

// ── Grouping helpers ───────────────────────────────────────────────

// groupByProject buckets containers by Compose project. Any container
// without a project label is placed under "Other". Groups are ordered:
// named projects first (alphabetically), then "Other".
func groupByProject(containers []Container) []ContainerGroup {
	groups := map[string][]Container{}
	var projectOrder []string
	hasOther := false

	for _, c := range containers {
		key := c.Project
		if key == "" {
			key = "Other"
			hasOther = true
		}
		if _, ok := groups[key]; !ok {
			projectOrder = append(projectOrder, key)
		}
		groups[key] = append(groups[key], c)
	}

	var result []ContainerGroup
	for _, name := range projectOrder {
		if name == "Other" {
			continue
		}
		result = append(result, ContainerGroup{Project: name, Containers: groups[name]})
	}
	if hasOther {
		result = append(result, ContainerGroup{Project: "Other", Containers: groups["Other"]})
	}

	return result
}

// groupByDriver buckets networks by driver. Built-in drivers
// (bridge, host, none) come first; custom drivers follow
// alphabetically.
func groupByDriver(networks []Network) []NetworkGroup {
	groups := map[string][]Network{}
	var driverOrder []string

	builtinOrder := []string{"bridge", "host", "none"}

	for _, n := range networks {
		driver := n.Driver
		if driver == "" {
			driver = "unknown"
		}
		if _, ok := groups[driver]; !ok {
			driverOrder = append(driverOrder, driver)
		}
		groups[driver] = append(groups[driver], n)
	}

	var result []NetworkGroup
	for _, d := range builtinOrder {
		if nets, ok := groups[d]; ok {
			result = append(result, NetworkGroup{Driver: d, Networks: nets})
		}
	}

	var custom []string
	for _, d := range driverOrder {
		isBuiltin := false
		for _, b := range builtinOrder {
			if d == b {
				isBuiltin = true
				break
			}
		}
		if !isBuiltin {
			custom = append(custom, d)
		}
	}
	for i := 0; i < len(custom); i++ {
		for j := i + 1; j < len(custom); j++ {
			if custom[i] > custom[j] {
				custom[i], custom[j] = custom[j], custom[i]
			}
		}
	}
	for _, d := range custom {
		result = append(result, NetworkGroup{Driver: d, Networks: groups[d]})
	}

	return result
}

// fromJSON unmarshals a JSON string into a map for inspection display.
func fromJSON(data string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return m, nil
}

package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ── Types ──────────────────────────────────────────────────────────

// Volume represents a single Docker volume.
type Volume struct {
	Name      string
	Driver    string
	Mountpoint string
	Size      string // human-readable size, e.g. "824.6kB"
	SizeBytes int64  // size in bytes for sorting
	CreatedAt string
}

// VolumeFileUsage represents file/folder disk usage inside a volume.
type VolumeFileUsage struct {
	Name string // file or folder name
	Size string // human-readable size, e.g. "12MB"
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
	Project    string
	Containers []Container
}

// ── Client ─────────────────────────────────────────────────────────

// Client wraps calls to the local Docker daemon via the `docker` CLI.
type Client struct{}

// NewClient returns a ready-to-use Client.
func NewClient() *Client {
	return &Client{}
}

// ListContainers returns all containers (running + stopped) grouped
// by Docker Compose project. Containers without a Compose project
// label are placed in the "Other" group.
func (c *Client) ListContainers() ([]ContainerGroup, error) {
	raw, err := runDockerCLI("ps", "-a",
		"--format", `{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Status}}\t{{.Ports}}\t{{.Labels}}\t{{.CreatedAt}}`)
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}
		project := extractComposeProject(fields[6])
		containers = append(containers, Container{
			ID:        fields[0],
			Name:      fields[1],
			Image:     fields[2],
			State:     fields[3],
			Status:    fields[4],
			Ports:     fields[5],
			Project:   project,
			CPU:       "—",
			Memory:    "—",
			ImageSize: "—",
			CreatedAt: fields[7],
		})
	}

	return groupByProject(containers), nil
}

// GetStartedTimes returns a map of container name → StartedAt
// timestamp by running docker inspect on all containers.
func (c *Client) GetStartedTimes() (map[string]string, error) {
	// Get all container IDs first
	idsRaw, err := runDockerCLI("ps", "-aq")
	if err != nil {
		return nil, fmt.Errorf("docker ps -aq: %w", err)
	}
	idsRaw = strings.TrimSpace(idsRaw)
	if idsRaw == "" {
		return map[string]string{}, nil
	}

	ids := strings.Fields(idsRaw)
	// docker inspect --format does NOT interpret \t as a tab
	// (unlike docker ps), so use a delimiter that works everywhere.
	args := append([]string{"inspect", "--format", `{{.Name}}|||{{.State.StartedAt}}`}, ids...)
	raw, err := runDockerCLI(args...)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|||", 2)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[0], "/")
		result[name] = fields[1]
	}
	return result, nil
}

// GetLogs returns the last 200 lines of logs for a container.
func (c *Client) GetLogs(containerName string) (string, error) {
	return runDockerCLI("logs", "--tail", "200", containerName)
}

// ── Container actions ────────────────────────────────────────────

// StartContainer starts the given container.
func (c *Client) StartContainer(name string) error {
	_, err := runDockerCLI("start", name)
	return err
}

// StopContainer stops the given container.
func (c *Client) StopContainer(name string) error {
	_, err := runDockerCLI("stop", name)
	return err
}

// RestartContainer restarts the given container.
func (c *Client) RestartContainer(name string) error {
	_, err := runDockerCLI("restart", name)
	return err
}

// KillContainer forcefully kills the given container.
func (c *Client) KillContainer(name string) error {
	_, err := runDockerCLI("kill", name)
	return err
}

// ── Disk usage ───────────────────────────────────────────────────
// (writable layer + mounted volumes) in human-readable form, or
// "—" on error.
func (c *Client) GetContainerDiskUsage(containerName string) (string, error) {
	// 1. Get container writable layer size (in bytes)
	sizeRw, err := c.getContainerSizeRw(containerName)
	if err != nil {
		sizeRw = 0
	}

	// 2. Get mounted volume names
	volumes, err := c.getContainerVolumes(containerName)
	if err != nil {
		volumes = nil
	}

	// 3. Get volume sizes from docker system df -v
	volSizes := make(map[string]int64)
	if len(volumes) > 0 {
		volSizes, _ = c.getVolumeSizes()
	}

	// 4. Sum everything
	total := sizeRw
	for _, vName := range volumes {
		if sz, ok := volSizes[vName]; ok {
			total += sz
		}
	}

	if total == 0 {
		return "—", nil
	}
	return humanSize(total), nil
}

// getContainerSizeRw returns SizeRw (writable layer) in bytes.
func (c *Client) getContainerSizeRw(name string) (int64, error) {
	raw, err := runDockerCLI("inspect", "--format", `{{.SizeRw}}`, name)
	if err != nil {
		return 0, err
	}
	return parseInt64(strings.TrimSpace(raw))
}

// getContainerVolumes returns the names of volumes mounted to a container.
func (c *Client) getContainerVolumes(name string) ([]string, error) {
	raw, err := runDockerCLI("inspect", "--format",
		`{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}}|||{{end}}{{end}}`, name)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var vols []string
	for _, v := range strings.Split(raw, "|||") {
		v = strings.TrimSpace(v)
		if v != "" {
			vols = append(vols, v)
		}
	}
	return vols, nil
}

// getVolumeSizes parses docker system df -v and returns a map of
// volume name → size in bytes.
func (c *Client) getVolumeSizes() (map[string]int64, error) {
	raw, err := runDockerCLI("system", "df", "-v")
	if err != nil {
		return nil, err
	}

	result := make(map[string]int64)
	inVolumesSection := false

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)

		// Detect start of volumes section
		if strings.Contains(line, "Local Volumes space usage") {
			inVolumesSection = true
			continue
		}
		if !inVolumesSection {
			continue
		}
		// Skip the column header line
		if strings.HasPrefix(line, "VOLUME NAME") || line == "" {
			continue
		}
		// Stop at the next section (empty line after volumes)
		if !strings.Contains(line, " ") {
			continue
		}

		// Parse: "<volume name>    <links>    <size>"
		// The volume name is the first field, size is the last.
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		volName := fields[0]
		sizeStr := fields[len(fields)-1]

		bytes := parseHumanSize(sizeStr)
		if bytes > 0 {
			result[volName] = bytes
		}
	}
	return result, nil
}

// parseHumanSize converts a human-readable size string (e.g. "824.6kB",
// "1.5MB", "2.3GB") to bytes. Returns 0 on parse failure.
func parseHumanSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
		return 0
	}

	// Split numeric part from unit suffix
	var numPart string
	var unit string
	for i, ch := range s {
		if (ch < '0' || ch > '9') && ch != '.' {
			numPart = s[:i]
			unit = strings.ToUpper(s[i:])
			break
		}
	}
	if numPart == "" {
		return 0
	}

	val := 0.0
	fmt.Sscanf(numPart, "%f", &val)

	switch unit {
	case "B", "":
		return int64(val)
	case "KB", "K":
		return int64(val * 1024)
	case "MB", "M":
		return int64(val * 1024 * 1024)
	case "GB", "G":
		return int64(val * 1024 * 1024 * 1024)
	case "TB", "T":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	default:
		return 0
	}
}

// humanSize converts bytes to a human-readable string (e.g. 1048576 → "1.0MB").
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// parseInt64 parses a decimal string into int64.
func parseInt64(s string) (int64, error) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

// ── Volumes ─────────────────────────────────────────────────────

// GetVolumes returns all Docker volumes with their driver, mountpoint,
// and size information.
func (c *Client) GetVolumes() ([]Volume, error) {
	// 1. Get volume names, drivers, and mountpoints from docker volume ls
	// Use ||| as delimiter (docker volume ls --format may not interpret \t)
	raw, err := runDockerCLI("volume", "ls",
		"--format", `{{.Name}}|||{{.Driver}}|||{{.Mountpoint}}`)
	if err != nil {
		return nil, fmt.Errorf("docker volume ls: %w", err)
	}

	// 2. Get volume sizes from docker system df -v
	volSizes, _ := c.getVolumeSizes()

	var volumes []Volume
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|||")
		if len(fields) < 3 {
			continue
		}
		name := fields[0]
		driver := fields[1]
		mountpoint := fields[2]

		sizeBytes := volSizes[name]
		sizeStr := "—"
		if sizeBytes > 0 {
			sizeStr = humanSize(sizeBytes)
		}

		volumes = append(volumes, Volume{
			Name:       name,
			Driver:     driver,
			Mountpoint: mountpoint,
			Size:       sizeStr,
			SizeBytes:  sizeBytes,
		})
	}

	// Sort by size descending, then by name
	sortVolumes(volumes)

	return volumes, nil
}

// GetVolumeFileUsage returns per-file/folder disk usage inside a
// Docker volume. Uses the volume's host mountpoint to run du
// directly, which is fast and requires no image pulls.
// Falls back to a docker-run approach if the host path is
// inaccessible.
func (c *Client) GetVolumeFileUsage(name string) ([]VolumeFileUsage, error) {
	// Primary: use host filesystem via volume mountpoint.
	entries, err := c.getVolumeFileUsageHost(name)
	if err == nil && len(entries) > 0 {
		return entries, nil
	}

	// Fallback: use a lightweight container to inspect the volume.
	// Try busybox first (tiny, commonly cached), then alpine.
	// Use sh -c so the glob * is expanded by the shell inside the container.
	raw, err2 := runDockerCLI("run", "--rm",
		"-v", name+":/vol:ro",
		"busybox", "sh", "-c", "du -sh /vol/*")
	if err2 != nil {
		raw, err2 = runDockerCLI("run", "--rm",
			"-v", name+":/vol:ro",
			"alpine", "sh", "-c", "du -sh /vol/*")
		if err2 != nil {
			// Return the original host-path error if everything failed
			if err != nil {
				return nil, err
			}
			return nil, err2
		}
	}
	return parseDUOutput(raw), nil
}

// getVolumeFileUsageHost is a fallback that reads the volume
// mountpoint directly from the host filesystem.
func (c *Client) getVolumeFileUsageHost(name string) ([]VolumeFileUsage, error) {
	raw, err := runDockerCLI("volume", "inspect", name,
		"--format", `{{.Mountpoint}}`)
	if err != nil {
		return nil, fmt.Errorf("docker volume inspect: %w", err)
	}
	mountpoint := strings.TrimSpace(raw)
	if mountpoint == "" {
		return nil, fmt.Errorf("empty mountpoint for volume %s", name)
	}
	return getDirUsage(mountpoint)
}

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

		// Just the basename
		name := itemPath
		if idx := strings.LastIndex(itemPath, "/"); idx >= 0 {
			name = itemPath[idx+1:]
		}

		// du output: directories end with / in the path
		isDir := strings.HasSuffix(itemPath, "/") || itemPath[len(itemPath)-1] == '/'

		entries = append(entries, VolumeFileUsage{
			Name:  name,
			Size:  size,
			IsDir: isDir,
		})
	}

	// Sort by size descending
	sortFileUsage(entries)

	return entries
}

// getDirUsage runs du on a host path and returns per-item sizes.
// Uses a shell to correctly expand the glob pattern.
func getDirUsage(path string) ([]VolumeFileUsage, error) {
	// Use sh -c so the glob * is expanded by the shell.
	// Redirect stderr to capture permission errors separately.
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("du -sh -- '%s'/* 2>/dev/null", path))
	out, err := cmd.Output()
	if err != nil {
		// Check if the directory is readable at all
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf("cannot access volume path %s: %w", path, statErr)
		}
		// Directory exists but du failed (empty dir, or all items denied)
		return nil, nil
	}
	return parseDUOutput(string(out)), nil
}

// isDirectory returns true if the given path is a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// sortVolumes sorts volumes by size (descending), then alphabetically.
func sortVolumes(vols []Volume) {
	for i := 0; i < len(vols); i++ {
		for j := i + 1; j < len(vols); j++ {
			if vols[j].SizeBytes > vols[i].SizeBytes ||
				(vols[j].SizeBytes == vols[i].SizeBytes &&
					vols[j].Name < vols[i].Name) {
				vols[i], vols[j] = vols[j], vols[i]
			}
		}
	}
}

// sortFileUsage sorts volume file usage entries by size (descending).
func sortFileUsage(entries []VolumeFileUsage) {
	// Parse sizes and sort
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if parseHumanSize(entries[j].Size) > parseHumanSize(entries[i].Size) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// ── Helpers ────────────────────────────────────────────────────────

func runDockerCLI(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// extractComposeProject parses Docker container labels (comma-
// separated "key=value" pairs) looking for the Compose project label.
func extractComposeProject(labels string) string {
	if labels == "" {
		return ""
	}
	for _, pair := range strings.Split(labels, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] == "com.docker.compose.project" {
			return kv[1]
		}
	}
	return ""
}

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

	// Simple sort: move "Other" to the end.
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

// ── JSON helpers (for future docker-socket use) ───────────────────

func fromJSON(data string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return m, nil
}

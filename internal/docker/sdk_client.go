package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-units"
)

// ── SDK Client ────────────────────────────────────────────────────

// sdkClient implements Client using the official Docker Engine API SDK.
type sdkClient struct {
	cli *dockerclient.Client
}

// NewClient creates a Client connected to the local Docker daemon.
// Uses the DOCKER_HOST environment variable or the default Unix socket.
func NewClient() (Client, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &sdkClient{cli: cli}, nil
}

// Ping checks that the Docker daemon is reachable.
func (c *sdkClient) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// ── Containers ─────────────────────────────────────────────────────

func (c *sdkClient) ListContainers(ctx context.Context) ([]ContainerGroup, error) {
	ctrs, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	var containers []Container
	for _, ctr := range ctrs {
		containers = append(containers, Container{
			ID:      ctr.ID[:12],
			Name:    strings.TrimPrefix(ctr.Names[0], "/"),
			Image:   ctr.Image,
			State:   ctr.State,
			Status:  ctr.Status,
			Ports:   formatPortsFromSDK(ctr.Ports),
			Project: ctr.Labels["com.docker.compose.project"],
			CPU:     "—",
			Memory:  "—",
		})
	}

	groups := groupByProject(containers)

	// Resolve compose file path for each group.
	for i := range groups {
		if groups[i].Project == "Other" || len(groups[i].Containers) == 0 {
			continue
		}
		groups[i].ComposeFile = c.resolveComposeFile(ctx, groups[i].Containers[0].ID)
	}

	return groups, nil
}

// GetStartedTimes returns a map of container name → StartedAt
// timestamp from the Docker daemon.
func (c *sdkClient) GetStartedTimes(ctx context.Context) (map[string]string, error) {
	ctrs, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	result := make(map[string]string, len(ctrs))
	for _, ctr := range ctrs {
		name := strings.TrimPrefix(ctr.Names[0], "/")
		if ctr.Created > 0 {
			result[name] = time.Unix(ctr.Created, 0).Format("2006-01-02T15:04:05")
		}
	}
	return result, nil
}

// ── Container actions ──────────────────────────────────────────────

func (c *sdkClient) StartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStart(ctx, name, container.StartOptions{})
}

func (c *sdkClient) StopContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStop(ctx, name, container.StopOptions{})
}

func (c *sdkClient) RestartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerRestart(ctx, name, container.StopOptions{})
}

func (c *sdkClient) KillContainer(ctx context.Context, name string) error {
	return c.cli.ContainerKill(ctx, name, "SIGKILL")
}

// ── Logs ───────────────────────────────────────────────────────────
// Docker multiplexes stdout/stderr into a single stream with an
// 8-byte header per frame: [stream(1)][pad(3)][size(4 BE)].
// ContainerLogsReader handles the demultiplexing for us.

func (c *sdkClient) GetLogs(ctx context.Context, containerName string) (string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "200",
	}
	rc, err := c.cli.ContainerLogs(ctx, containerName, opts)
	if err != nil {
		return "", fmt.Errorf("container logs %s: %w", containerName, err)
	}
	defer rc.Close()

	// Demux the multiplexed Docker log stream into plain text.
	var buf strings.Builder
	_, err = stdcopy.StdCopy(&buf, &buf, rc)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (c *sdkClient) FollowLogs(ctx context.Context, containerName string) (<-chan string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "500",
		Follow:     true,
	}
	rc, err := c.cli.ContainerLogs(ctx, containerName, opts)
	if err != nil {
		return nil, fmt.Errorf("follow logs %s: %w", containerName, err)
	}

	// Demux into a pipe, then read lines from it.
	pr, pw := io.Pipe()
	go func() {
		_, _ = stdcopy.StdCopy(pw, pw, rc)
		pw.Close()
		rc.Close()
	}()

	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case ch <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// ── Stats ──────────────────────────────────────────────────────────

func (c *sdkClient) GetStats(ctx context.Context, names []string) (map[string]ContainerStats, error) {
	if len(names) == 0 {
		return map[string]ContainerStats{}, nil
	}

	// Resolve container names to IDs with a single, filtered API call.
	// We pass names explicitly to avoid listing ALL containers every tick.
	args := filters.NewArgs(filters.Arg("status", "running"))
	for _, n := range names {
		args.Add("name", n)
	}
	ctrs, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: args,
	})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	if len(ctrs) == 0 {
		return map[string]ContainerStats{}, nil
	}

	// Fetch stats for all containers concurrently.
	type statResult struct {
		name  string
		stats ContainerStats
	}
	ch := make(chan statResult, len(ctrs))

	for _, ctr := range ctrs {
		go func(id string, containerName string) {
			cs := ContainerStats{Name: containerName}
			resp, err := c.cli.ContainerStats(ctx, id, false)
			if err != nil {
				ch <- statResult{name: containerName, stats: cs}
				return
			}

			var stats container.StatsResponse
			if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
				resp.Body.Close()
				ch <- statResult{name: containerName, stats: cs}
				return
			}
			resp.Body.Close()

			// CPU %
			if prevCPU := stats.PreCPUStats.CPUUsage.TotalUsage; prevCPU > 0 {
				cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - prevCPU)
				sysDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
				if sysDelta > 0 {
					cs.CPUPerc = fmt.Sprintf("%.2f%%", (cpuDelta/sysDelta)*float64(stats.CPUStats.OnlineCPUs)*100)
				}
			}

			// Memory
			if stats.MemoryStats.Usage > 0 {
				cs.MemUsage = fmt.Sprintf("%s / %s",
					units.BytesSize(float64(stats.MemoryStats.Usage)),
					units.BytesSize(float64(stats.MemoryStats.Limit)),
				)
				if stats.MemoryStats.Limit > 0 {
					cs.MemPerc = fmt.Sprintf("%.2f%%",
						float64(stats.MemoryStats.Usage)/float64(stats.MemoryStats.Limit)*100)
				}
			}

			// Network I/O
			var rx, tx uint64
			for _, net := range stats.Networks {
				rx += net.RxBytes
				tx += net.TxBytes
			}
			cs.NetIO = fmt.Sprintf("%s / %s", units.BytesSize(float64(rx)), units.BytesSize(float64(tx)))

			// Block I/O
			var readBytes, writeBytes uint64
			if len(stats.BlkioStats.IoServiceBytesRecursive) > 0 {
				readBytes = stats.BlkioStats.IoServiceBytesRecursive[0].Value
			}
			if len(stats.BlkioStats.IoServiceBytesRecursive) > 1 {
				writeBytes = stats.BlkioStats.IoServiceBytesRecursive[1].Value
			}
			cs.BlockIO = fmt.Sprintf("%s / %s",
				units.BytesSize(float64(readBytes)),
				units.BytesSize(float64(writeBytes)),
			)

			ch <- statResult{name: containerName, stats: cs}
		}(ctr.ID, strings.TrimPrefix(ctr.Names[0], "/"))
	}

	// Collect results with a timeout.
	result := make(map[string]ContainerStats, len(ctrs))
	deadline := time.After(3 * time.Second)
	for i := 0; i < len(ctrs); i++ {
		select {
		case r := <-ch:
			result[r.name] = r.stats
		case <-deadline:
			return result, nil
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}

	return result, nil
}

// ── Images ─────────────────────────────────────────────────────────

func (c *sdkClient) ListImages(ctx context.Context) ([]Image, error) {
	imgs, err := c.cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	var result []Image
	for _, img := range imgs {
		repo, tag := "<none>", "latest"
		if len(img.RepoTags) > 0 {
			parts := strings.SplitN(img.RepoTags[0], ":", 2)
			repo = parts[0]
			if len(parts) > 1 {
				tag = parts[1]
			}
		}

		shortID := strings.TrimPrefix(img.ID, "sha256:")
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		created := time.Unix(img.Created, 0)

		result = append(result, Image{
			ID:        shortID,
			Repo:      repo,
			Tag:       tag,
			Size:      units.BytesSize(float64(img.Size)),
			CreatedAt: created.Format(time.RFC3339),
			Created:   formatTimeAgo(created),
		})
	}

	return result, nil
}

func (c *sdkClient) GetImageHistory(ctx context.Context, imageID string) ([]ImageLayer, error) {
	history, err := c.cli.ImageHistory(ctx, imageID)
	if err != nil {
		return nil, fmt.Errorf("image history %s: %w", imageID, err)
	}

	var layers []ImageLayer
	for _, h := range history {
		shortID := h.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		layers = append(layers, ImageLayer{
			ID:      shortID,
			Created: formatTimeAgo(time.Unix(h.Created, 0)),
			Command: formatLayerCommand(h.CreatedBy),
			Size:    units.BytesSize(float64(h.Size)),
		})
	}
	return layers, nil
}

// ── Networks ───────────────────────────────────────────────────────

func (c *sdkClient) ListNetworks(ctx context.Context) ([]NetworkGroup, error) {
	nets, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}

	var networks []Network
	for _, n := range nets {
		nw := Network{
			ID:         n.ID[:12],
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			IPv6:       n.EnableIPv6,
			Attachable: n.Attachable,
			Created:    n.Created.Format(time.RFC3339),
			Labels:     n.Labels,
		}

		if len(n.IPAM.Config) > 0 {
			nw.Subnet = n.IPAM.Config[0].Subnet
			nw.Gateway = n.IPAM.Config[0].Gateway
			nw.IPRange = n.IPAM.Config[0].IPRange
		}

		for ctrName, ctr := range n.Containers {
			nw.Containers = append(nw.Containers, NetworkContainer{
				Name:     ctrName,
				IPv4Addr: ctr.IPv4Address,
				IPv6Addr: ctr.IPv6Address,
				MACAddr:  ctr.MacAddress,
			})
		}

		networks = append(networks, nw)
	}

	return groupByDriver(networks), nil
}

func (c *sdkClient) InspectNetworkRaw(ctx context.Context, name string) (string, error) {
	nw, err := c.cli.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspect network %s: %w", name, err)
	}

	data, err := json.MarshalIndent(nw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *sdkClient) PruneNetworks(ctx context.Context) (string, error) {
	report, err := c.cli.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		return "", fmt.Errorf("prune networks: %w", err)
	}
	return fmt.Sprintf("Pruned %d networks", len(report.NetworksDeleted)), nil
}

// ── Volumes ────────────────────────────────────────────────────────

func (c *sdkClient) GetVolumes(ctx context.Context) ([]Volume, error) {
	resp, err := c.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}

	// Get disk usage to populate sizes.
	du, _ := c.cli.DiskUsage(ctx, types.DiskUsageOptions{})

	volSizes := make(map[string]int64)
	for _, v := range du.Volumes {
		volSizes[v.Name] = v.UsageData.Size
	}

	var volumes []Volume
	for _, v := range resp.Volumes {
		sizeBytes := volSizes[v.Name]
		sizeStr := "—"
		if sizeBytes > 0 {
			sizeStr = units.BytesSize(float64(sizeBytes))
		}

		volumes = append(volumes, Volume{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			Size:       sizeStr,
			SizeBytes:  sizeBytes,
		})
	}

	sort.Slice(volumes, func(i, j int) bool {
		if volumes[i].SizeBytes != volumes[j].SizeBytes {
			return volumes[i].SizeBytes > volumes[j].SizeBytes
		}
		return volumes[i].Name < volumes[j].Name
	})

	return volumes, nil
}

func (c *sdkClient) GetVolumeFileUsage(ctx context.Context, name string) ([]VolumeFileUsage, error) {
	vol, err := c.cli.VolumeInspect(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("inspect volume %s: %w", name, err)
	}

	// Read the host mountpoint directly.
	entries, err := os.ReadDir(vol.Mountpoint)
	if err != nil {
		// Fallback: use a docker container to read the volume.
		return c.getVolumeFileUsageFallback(ctx, name)
	}

	var result []VolumeFileUsage
	for _, entry := range entries {
		info, err := entry.Info()
		size := "—"
		if err == nil {
			size = units.BytesSize(float64(info.Size()))
		}
		result = append(result, VolumeFileUsage{
			Name:  entry.Name(),
			Size:  size,
			IsDir: entry.IsDir(),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (c *sdkClient) GetContainerDiskUsage(ctx context.Context, containerName string) (string, error) {
	info, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "—", fmt.Errorf("inspect container %s: %w", containerName, err)
	}

	var total int64
	if info.SizeRw != nil {
		total = *info.SizeRw
	}

	if total == 0 {
		return "—", nil
	}
	return units.BytesSize(float64(total)), nil
}

// ── Compose (still CLI-based, docker-compose is not part of the engine API) ──

func (c *sdkClient) ComposeUp(ctx context.Context, composeFile string) error {
	dir := strings.TrimSuffix(composeFile, "/docker-compose.yml")
	return runDocker(ctx, "compose", "-f", composeFile, "--project-directory", dir, "up", "-d")
}

func (c *sdkClient) ComposeDown(ctx context.Context, composeFile string) error {
	dir := strings.TrimSuffix(composeFile, "/docker-compose.yml")
	return runDocker(ctx, "compose", "-f", composeFile, "--project-directory", dir, "down")
}

func (c *sdkClient) ComposePull(ctx context.Context, composeFile string) error {
	dir := strings.TrimSuffix(composeFile, "/docker-compose.yml")
	return runDocker(ctx, "compose", "-f", composeFile, "--project-directory", dir, "pull")
}

func (c *sdkClient) ComposeBuild(ctx context.Context, composeFile string) error {
	dir := strings.TrimSuffix(composeFile, "/docker-compose.yml")
	return runDocker(ctx, "compose", "-f", composeFile, "--project-directory", dir, "build")
}

func (c *sdkClient) ComposeRestart(ctx context.Context, composeFile string) error {
	dir := strings.TrimSuffix(composeFile, "/docker-compose.yml")
	return runDocker(ctx, "compose", "-f", composeFile, "--project-directory", dir, "restart")
}

// ── Bulk actions ──────────────────────────────────────────────────

// StopAllContainers stops all running containers.
func (c *sdkClient) StopAllContainers(ctx context.Context) (string, error) {
	ctrs, err := c.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	for _, ctr := range ctrs {
		if err := c.cli.ContainerStop(ctx, ctr.ID, container.StopOptions{}); err != nil {
			return "", fmt.Errorf("stop %s: %w", ctr.ID[:12], err)
		}
	}
	return fmt.Sprintf("Stopped %d containers", len(ctrs)), nil
}

// RemoveAllContainers removes all containers (uses -f to force).
func (c *sdkClient) RemoveAllContainers(ctx context.Context) (string, error) {
	ctrs, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	for _, ctr := range ctrs {
		if err := c.cli.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: true}); err != nil {
			return "", fmt.Errorf("remove %s: %w", ctr.ID[:12], err)
		}
	}
	return fmt.Sprintf("Removed %d containers", len(ctrs)), nil
}

func (c *sdkClient) PruneContainers(ctx context.Context) (string, error) {
	out, err := runDockerOutput(ctx, "container", "prune", "-f")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ── Helpers ────────────────────────────────────────────────────────

// resolveComposeFile uses the SDK to get container labels and find the
// docker-compose.yml path.
func (c *sdkClient) resolveComposeFile(ctx context.Context, containerID string) string {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return ""
	}
	if wd, ok := info.Config.Labels["com.docker.compose.project.working_dir"]; ok {
		return wd + "/docker-compose.yml"
	}
	return ""
}

// formatPortsFromSDK converts Docker SDK port bindings to display strings.
func formatPortsFromSDK(ports []types.Port) string {
	if len(ports) == 0 {
		return ""
	}
	var parts []string
	for _, p := range ports {
		if p.PublicPort > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	return strings.Join(parts, ", ")
}

// formatTimeAgo returns a human-readable relative time string.
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	if d < 0 {
		return "—"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dM ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

// ── Minimal CLI helpers (only for compose and bulk actions) ────────

func runDocker(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func runDockerOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// getVolumeFileUsageFallback runs du inside a temporary container.
func (c *sdkClient) getVolumeFileUsageFallback(ctx context.Context, name string) ([]VolumeFileUsage, error) {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", name+":/vol:ro",
		"busybox", "sh", "-c", "du -sh /vol/*")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("volume usage %s: %w", name, err)
	}
	return parseDUOutput(string(out)), nil
}

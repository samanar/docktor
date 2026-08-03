package docker

import "context"

// Client is the interface for all Docker daemon interactions.
// Implementations must be safe for concurrent use by the Bubble Tea
// event loop and background goroutines.
type Client interface {
	// ── Containers ───────────────────────────────────
	ListContainers(ctx context.Context) ([]ContainerGroup, error)
	GetStartedTimes(ctx context.Context) (map[string]string, error)
	StartContainer(ctx context.Context, name string) error
	StopContainer(ctx context.Context, name string) error
	RestartContainer(ctx context.Context, name string) error
	KillContainer(ctx context.Context, name string) error

	// ── Logs ─────────────────────────────────────────
	GetLogs(ctx context.Context, containerName string) (string, error)
	FollowLogs(ctx context.Context, containerName string) (<-chan string, error)

	// ── Stats ────────────────────────────────────────
	// GetStats fetches live resource stats for the given container
	// names.  Only running containers should be passed — stopped
	// containers will fail and be skipped.
	GetStats(ctx context.Context, names []string) (map[string]ContainerStats, error)

	// ── Images ───────────────────────────────────────
	ListImages(ctx context.Context) ([]Image, error)
	GetImageHistory(ctx context.Context, imageID string) ([]ImageLayer, error)

	// ── Networks ─────────────────────────────────────
	ListNetworks(ctx context.Context) ([]NetworkGroup, error)
	InspectNetworkRaw(ctx context.Context, name string) (string, error)
	PruneNetworks(ctx context.Context) (string, error)

	// ── Volumes ──────────────────────────────────────
	GetVolumes(ctx context.Context) ([]Volume, error)
	GetVolumeFileUsage(ctx context.Context, name string) ([]VolumeFileUsage, error)
	GetContainerDiskUsage(ctx context.Context, containerName string) (string, error)

	// ── Compose ──────────────────────────────────────
	ComposeUp(ctx context.Context, composeFile string) error
	ComposeDown(ctx context.Context, composeFile string) error
	ComposePull(ctx context.Context, composeFile string) error
	ComposeBuild(ctx context.Context, composeFile string) error
	ComposeRestart(ctx context.Context, composeFile string) error

	// ── Bulk actions ─────────────────────────────────
	StopAllContainers(ctx context.Context) (string, error)
	RemoveAllContainers(ctx context.Context) (string, error)
	PruneContainers(ctx context.Context) (string, error)

	// ── Health ───────────────────────────────────────
	Ping(ctx context.Context) error
}

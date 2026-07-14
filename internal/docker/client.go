package docker

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ── Types ──────────────────────────────────────────────────────────

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
	CPU       string // populated by stats refresh
	Memory    string // populated by stats refresh
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

package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
)

// ── groupByProject ─────────────────────────────────────────────────

func TestGroupByProject_SingleProject(t *testing.T) {
	containers := []Container{
		{Name: "web-1", Project: "myapp", State: "running"},
		{Name: "db-1", Project: "myapp", State: "running"},
	}
	groups := groupByProject(containers)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Project != "myapp" {
		t.Fatalf("expected project 'myapp', got %q", groups[0].Project)
	}
	if len(groups[0].Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(groups[0].Containers))
	}
}

func TestGroupByProject_MultipleProjects(t *testing.T) {
	containers := []Container{
		{Name: "app1", Project: "proj-a"},
		{Name: "app2", Project: "proj-b"},
		{Name: "app3", Project: "proj-a"},
	}
	groups := groupByProject(containers)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// proj-a comes first (alphabetically), proj-b second
	if groups[0].Project != "proj-a" || len(groups[0].Containers) != 2 {
		t.Fatalf("expected proj-a with 2 containers, got %s with %d", groups[0].Project, len(groups[0].Containers))
	}
	if groups[1].Project != "proj-b" || len(groups[1].Containers) != 1 {
		t.Fatalf("expected proj-b with 1 container, got %s with %d", groups[1].Project, len(groups[1].Containers))
	}
}

func TestGroupByProject_OtherAlwaysLast(t *testing.T) {
	containers := []Container{
		{Name: "orphan", Project: ""},
		{Name: "app", Project: "zapp"},
	}
	groups := groupByProject(containers)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Project != "zapp" {
		t.Fatalf("expected zapp first, got %s", groups[0].Project)
	}
	if groups[1].Project != "Other" {
		t.Fatalf("expected Other last, got %s", groups[1].Project)
	}
}

func TestGroupByProject_Empty(t *testing.T) {
	groups := groupByProject(nil)
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
	}
}

// ── groupByDriver ──────────────────────────────────────────────────

func TestGroupByDriver_BuiltinsFirst(t *testing.T) {
	networks := []Network{
		{Name: "custom-net", Driver: "overlay"},
		{Name: "bridge-net", Driver: "bridge"},
		{Name: "host-net", Driver: "host"},
	}
	groups := groupByDriver(networks)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0].Driver != "bridge" {
		t.Fatalf("expected bridge first, got %s", groups[0].Driver)
	}
	if groups[1].Driver != "host" {
		t.Fatalf("expected host second, got %s", groups[1].Driver)
	}
	if groups[2].Driver != "overlay" {
		t.Fatalf("expected overlay last, got %s", groups[2].Driver)
	}
}

func TestGroupByDriver_EmptyDriver(t *testing.T) {
	networks := []Network{
		{Name: "weird-net", Driver: ""},
	}
	groups := groupByDriver(networks)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Driver != "unknown" {
		t.Fatalf("expected 'unknown' driver, got %q", groups[0].Driver)
	}
}

// ── MergeStats ─────────────────────────────────────────────────────

func TestMergeStats_UpdatesFields(t *testing.T) {
	groups := []ContainerGroup{
		{
			Project: "test",
			Containers: []Container{
				{Name: "ctr-1", CPU: "—", Memory: "—", NetIO: "—", BlockIO: "—"},
			},
		},
	}

	stats := map[string]ContainerStats{
		"ctr-1": {CPUPerc: "5.0%", MemUsage: "100MiB / 1GiB", NetIO: "1GB / 500MB", BlockIO: "2GB / 1GB"},
	}

	MergeStats(groups, stats)

	c := groups[0].Containers[0]
	if c.CPU != "5.0%" {
		t.Fatalf("expected CPU 5.0%%, got %q", c.CPU)
	}
	if c.Memory != "100MiB / 1GiB" {
		t.Fatalf("expected Memory 100MiB / 1GiB, got %q", c.Memory)
	}
	if c.NetIO != "1GB / 500MB" {
		t.Fatalf("expected NetIO 1GB / 500MB, got %q", c.NetIO)
	}
}

func TestMergeStats_MissingContainer(t *testing.T) {
	groups := []ContainerGroup{
		{
			Project: "test",
			Containers: []Container{
				{Name: "ctr-1", CPU: "old", Memory: "old"},
			},
		},
	}

	stats := map[string]ContainerStats{
		"ctr-2": {CPUPerc: "10%", MemUsage: "1GiB"},
	}

	MergeStats(groups, stats)

	// ctr-1 should keep old values since it's not in stats.
	c := groups[0].Containers[0]
	if c.CPU != "old" {
		t.Fatalf("expected CPU to stay 'old', got %q", c.CPU)
	}
}

// ── formatTimeAgo ──────────────────────────────────────────────────

func TestFormatTimeAgo_Zero(t *testing.T) {
	if s := formatTimeAgo(time.Time{}); s != "—" {
		t.Fatalf("expected '—', got %q", s)
	}
}

func TestFormatTimeAgo_JustNow(t *testing.T) {
	s := formatTimeAgo(time.Now())
	if s != "just now" {
		t.Fatalf("expected 'just now', got %q", s)
	}
}

func TestFormatTimeAgo_Future(t *testing.T) {
	s := formatTimeAgo(time.Now().Add(24 * time.Hour))
	if s != "—" {
		t.Fatalf("expected '—' for future time, got %q", s)
	}
}

func TestFormatTimeAgo_HoursAgo(t *testing.T) {
	s := formatTimeAgo(time.Now().Add(-5 * time.Hour))
	if s != "5h ago" {
		t.Fatalf("expected '5h ago', got %q", s)
	}
}

func TestFormatTimeAgo_DaysAgo(t *testing.T) {
	s := formatTimeAgo(time.Now().Add(-72 * time.Hour))
	if s != "3d ago" {
		t.Fatalf("expected '3d ago', got %q", s)
	}
}

// ── formatLayerCommand ─────────────────────────────────────────────

func TestFormatLayerCommand_StripsPrefix(t *testing.T) {
	cmd := formatLayerCommand("/bin/sh -c #(nop) ADD file:abc123 in /")
	if cmd != "ADD file:abc123 in /" {
		t.Fatalf("expected 'ADD file:abc123 in /', got %q", cmd)
	}
}

func TestFormatLayerCommand_Truncates(t *testing.T) {
	long := "RUN echo " + string(make([]byte, 200))
	cmd := formatLayerCommand(long)
	if len(cmd) > 60 {
		t.Fatalf("expected truncation to 60 chars, got %d: %q", len(cmd), cmd)
	}
	if !contains(cmd, "...") {
		t.Fatalf("expected '...' suffix, got %q", cmd)
	}
}

func TestFormatLayerCommand_Short(t *testing.T) {
	cmd := formatLayerCommand("RUN echo hello")
	if cmd != "RUN echo hello" {
		t.Fatalf("expected 'RUN echo hello', got %q", cmd)
	}
}

// ── parseDUOutput ──────────────────────────────────────────────────

func TestParseDUOutput_Normal(t *testing.T) {
	raw := "4.0K\t/vol/file.txt" + "\n" + "8.0K\t/vol/subdir/" + "\n"
	entries := parseDUOutput(raw)
	t.Logf("raw=%q", raw)
	for i, e := range entries {
		t.Logf("entry[%d]: name=%q size=%q isDir=%v", i, e.Name, e.Size, e.IsDir)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	foundFile, foundSubdir := false, false
	for _, e := range entries {
		switch {
		case e.Name == "file.txt":
			if e.IsDir {
				t.Errorf("file.txt should not be a directory")
			}
			foundFile = true
		case e.Name == "subdir":
			if !e.IsDir {
				t.Errorf("subdir should be a directory")
			}
			foundSubdir = true
		}
	}
	if !foundFile {
		t.Error("missing file.txt entry")
	}
	if !foundSubdir {
		t.Error("missing subdir entry")
	}
}

func TestParseDUOutput_Empty(t *testing.T) {
	entries := parseDUOutput("")
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

// ── formatPortsFromSDK ─────────────────────────────────────────────

func TestFormatPortsFromSDK_Empty(t *testing.T) {
	s := formatPortsFromSDK(nil)
	if s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
}

func TestFormatPortsFromSDK_PublicPort(t *testing.T) {
	ports := []types.Port{
		{IP: "0.0.0.0", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
	}
	s := formatPortsFromSDK(ports)
	if s != "0.0.0.0:8080->80/tcp" {
		t.Fatalf("expected '0.0.0.0:8080->80/tcp', got %q", s)
	}
}

func TestFormatPortsFromSDK_PrivateOnly(t *testing.T) {
	ports := []types.Port{
		{PrivatePort: 80, Type: "tcp"},
	}
	s := formatPortsFromSDK(ports)
	if s != "80/tcp" {
		t.Fatalf("expected '80/tcp', got %q", s)
	}
}

// ── helpers ────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

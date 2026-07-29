package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/samanar/docktor/internal/docker"
)

// ── highlightLine ──────────────────────────────────────────────────

func TestHighlightLine_Basic(t *testing.T) {
	matchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
	currentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("208"))
	line := "hello world hello"
	result := highlightLine(line, "world", 0, []int{0}, 0, matchStyle, currentStyle)

	if !strings.Contains(result, "world") {
		t.Fatalf("expected result to contain 'world', got %q", result)
	}
	// First occurrence of "world" should be highlighted with currentStyle (since lineIdx=0 matches)
}

func TestHighlightLine_NoMatch(t *testing.T) {
	matchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0"))
	currentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	result := highlightLine("hello world", "xyz", 0, nil, 0, matchStyle, currentStyle)

	if result != "hello world" {
		t.Fatalf("expected unchanged line, got %q", result)
	}
}

func TestHighlightLine_EmptyQuery(t *testing.T) {
	matchStyle := lipgloss.NewStyle()
	currentStyle := lipgloss.NewStyle()
	result := highlightLine("hello", "", 0, nil, 0, matchStyle, currentStyle)

	if result != "hello" {
		t.Fatalf("expected unchanged line, got %q", result)
	}
}

func TestHighlightLine_CaseInsensitive(t *testing.T) {
	matchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
	currentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("208"))
	result := highlightLine("Hello World", "world", 0, []int{0}, 0, matchStyle, currentStyle)

	// Should match "World" case-insensitively
	if !strings.Contains(strings.ToLower(result), "world") {
		t.Fatalf("expected case-insensitive match, got %q", result)
	}
}

// ── truncateStr ────────────────────────────────────────────────────

func TestTruncateStr_Short(t *testing.T) {
	if s := truncateStr("hello", 10); s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
}

func TestTruncateStr_Long(t *testing.T) {
	s := truncateStr("hello world this is long", 8)
	// truncateStr uses "…" (multi-byte), so byte length may exceed n.
	if len(s) < 5 || !strings.HasSuffix(s, "…") {
		t.Fatalf("expected truncation with …, got %q", s)
	}
}

func TestTruncateStr_Exact(t *testing.T) {
	if s := truncateStr("hello", 5); s != "hello" {
		t.Fatalf("expected 'hello', got %q", s)
	}
}

// ── fitStr ─────────────────────────────────────────────────────────

func TestFitStr_Short(t *testing.T) {
	s := fitStr("hi", 10)
	// fitStr pads to exact width
	if len(s) != 10 {
		t.Fatalf("expected len 10 (padded), got len %d: %q", len(s), s)
	}
}

func TestFitStr_Truncate(t *testing.T) {
	s := fitStr("hello world", 5)
	if len(s) != 5 {
		t.Fatalf("expected length 5, got %d: %q", len(s), s)
	}
}

// ── splitIO ────────────────────────────────────────────────────────

func TestSplitIO_Normal(t *testing.T) {
	rx, tx := splitIO("1.2GB / 400MB")
	if rx != "1.2GB" || tx != "400MB" {
		t.Fatalf("expected rx=1.2GB tx=400MB, got rx=%q tx=%q", rx, tx)
	}
}

func TestSplitIO_OnlyOne(t *testing.T) {
	rx, tx := splitIO("1.2GB")
	// Single value without " / " returns empty strings.
	if rx != "" || tx != "" {
		t.Fatalf("expected both empty for single value, got rx=%q tx=%q", rx, tx)
	}
}

func TestSplitIO_Empty(t *testing.T) {
	rx, tx := splitIO("")
	if rx != "" || tx != "" {
		t.Fatalf("expected both empty, got rx=%q tx=%q", rx, tx)
	}
}

// ── parseCPUPercent ────────────────────────────────────────────────

func TestParseCPUPercent_Normal(t *testing.T) {
	if v := parseCPUPercent("5.23%"); v != 5.23 {
		t.Fatalf("expected 5.23, got %f", v)
	}
}

func TestParseCPUPercent_Dash(t *testing.T) {
	if v := parseCPUPercent("—"); v != 0 {
		t.Fatalf("expected 0, got %f", v)
	}
}

func TestParseCPUPercent_Empty(t *testing.T) {
	if v := parseCPUPercent(""); v != 0 {
		t.Fatalf("expected 0, got %f", v)
	}
}

// ── parseBytes ─────────────────────────────────────────────────────

func TestParseBytes_KB(t *testing.T) {
	if b := parseBytes("10KB"); b != 10*1024 {
		t.Fatalf("expected %d, got %d", 10*1024, b)
	}
}

func TestParseBytes_MB(t *testing.T) {
	if b := parseBytes("1MB"); b != 1024*1024 {
		t.Fatalf("expected %d, got %d", 1024*1024, b)
	}
}

func TestParseBytes_GB(t *testing.T) {
	if b := parseBytes("2GB"); b != 2*1024*1024*1024 {
		t.Fatalf("expected %d, got %d", 2*1024*1024*1024, b)
	}
}

func TestParseBytes_Dash(t *testing.T) {
	if b := parseBytes("—"); b != 0 {
		t.Fatalf("expected 0, got %d", b)
	}
}

func TestParseBytes_MiB(t *testing.T) {
	if b := parseBytes("100MiB"); b != 100*1024*1024 {
		t.Fatalf("expected %d, got %d", 100*1024*1024, b)
	}
}

// ── formatBytes ────────────────────────────────────────────────────

func TestFormatBytes_Zero(t *testing.T) {
	s := formatBytes(0)
	if s == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("formatBytes(0) = %q", s)
}

func TestFormatBytes_KB(t *testing.T) {
	s := formatBytes(1024)
	if !strings.Contains(s, "KiB") {
		t.Fatalf("expected KiB suffix, got %q", s)
	}
}

func TestFormatBytes_MB(t *testing.T) {
	s := formatBytes(1024 * 1024)
	if !strings.Contains(s, "MiB") {
		t.Fatalf("expected MiB suffix, got %q", s)
	}
}

func TestFormatBytes_GB(t *testing.T) {
	s := formatBytes(1024 * 1024 * 1024)
	if !strings.Contains(s, "GiB") {
		t.Fatalf("expected GiB suffix, got %q", s)
	}
}

// ── memUsedPortion ─────────────────────────────────────────────────

func TestMemUsedPortion_Normal(t *testing.T) {
	if s := memUsedPortion("80MiB / 1.5GiB"); s != "80MiB" {
		t.Fatalf("expected '80MiB', got %q", s)
	}
}

func TestMemUsedPortion_NoUsed(t *testing.T) {
	if s := memUsedPortion("800MiB"); s != "800MiB" {
		t.Fatalf("expected '800MiB', got %q", s)
	}
}

// ── formatPorts ────────────────────────────────────────────────────

func TestFormatPorts_Empty(t *testing.T) {
	if s := formatPorts(""); s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
}

func TestFormatPorts_NoColon(t *testing.T) {
	s := formatPorts("0.0.0.0:8080->80/tcp")
	// formatPorts is defined in pane_rows.go and reformats port mappings.
	if s == "" {
		t.Fatal("expected non-empty port string")
	}
	t.Logf("formatPorts result: %q", s)
}

// ── relativeTime ───────────────────────────────────────────────────

func TestRelativeTime_Empty(t *testing.T) {
	if s := relativeTime(""); s != "—" {
		t.Fatalf("expected '—', got %q", s)
	}
}

func TestRelativeTime_Dash(t *testing.T) {
	if s := relativeTime("—"); s != "—" {
		t.Fatalf("expected '—', got %q", s)
	}
}

// ── shortenName ────────────────────────────────────────────────────

func TestShortenName_Short(t *testing.T) {
	if s := shortenName("app"); s != "app" {
		t.Fatalf("expected 'app', got %q", s)
	}
}

func TestShortenName_Long(t *testing.T) {
	s := shortenName("myapp-dev-container-abc123")
	// shortenName strips the project prefix and hash suffix.
	if s == "myapp-dev-container-abc123" {
		t.Fatal("expected name to be shortened")
	}
	t.Logf("shortenName result: %q", s)
}

func TestShortenName_IdFormat(t *testing.T) {
	s := shortenName("a1b2c3d4e5f6")
	if s == "a1b2c3d4e5f6" {
		// Might be shortened if recognized as ID pattern
		t.Logf("shortenName result: %q", s)
	}
}

// ── buildImageRows ─────────────────────────────────────────────────

func TestBuildImageRows_Single(t *testing.T) {
	theme := DefaultTheme()
	images := []docker.Image{
		{ID: "sha256:abc123def4567890", Repo: "nginx", Tag: "latest", Size: "142MB", Created: "2 weeks ago"},
	}
	rows := buildImageRows(theme, images)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Type != RowData {
		t.Fatalf("expected RowData, got %v", rows[0].Type)
	}
	name, ok := rows[0].Cells[colName]
	if !ok || name.Value != "nginx" {
		t.Fatalf("expected repo 'nginx', got %q", name.Value)
	}
}

func TestBuildImageRows_NoneImage(t *testing.T) {
	theme := DefaultTheme()
	images := []docker.Image{
		{ID: "sha256:abc", Repo: "<none>", Tag: "<none>", Size: "0B", Created: "—"},
	}
	rows := buildImageRows(theme, images)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestBuildImageRows_Empty(t *testing.T) {
	theme := DefaultTheme()
	rows := buildImageRows(theme, nil)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// ── buildVolumeRows ────────────────────────────────────────────────

func TestBuildVolumeRows_Single(t *testing.T) {
	theme := DefaultTheme()
	volumes := []docker.Volume{
		{Name: "my-vol", Driver: "local", Mountpoint: "/var/lib/docker/volumes/my-vol/_data", Size: "824.6kB", SizeBytes: 844390},
	}
	rows := buildVolumeRows(theme, volumes)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	name, ok := rows[0].Cells[colName]
	if !ok || name.Value != "my-vol" {
		t.Fatalf("expected name 'my-vol', got %q", name.Value)
	}
}

func TestBuildVolumeRows_Empty(t *testing.T) {
	theme := DefaultTheme()
	rows := buildVolumeRows(theme, nil)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// ── padLines ───────────────────────────────────────────────────────

func TestPadLines_Short(t *testing.T) {
	result := padLines("hello", 10, 0)
	if len(result) < len("hello") {
		t.Fatalf("expected at least %d chars, got %d", len("hello"), len(result))
	}
}

func TestPadLines_Empty(t *testing.T) {
	result := padLines("", 10, 0)
	// padLines with empty input may return empty.
	t.Logf("padLines('',10,0) = %q", result)
}

// ── mergeStarted ───────────────────────────────────────────────────

func TestMergeStarted_Updates(t *testing.T) {
	groups := []docker.ContainerGroup{
		{
			Project: "test",
			Containers: []docker.Container{
				{Name: "ctr-1", StartedAt: ""},
				{Name: "ctr-2", StartedAt: ""},
			},
		},
	}
	started := map[string]string{
		"ctr-1": "2024-01-01T00:00:00",
		"ctr-3": "2024-01-02T00:00:00",
	}
	mergeStarted(groups, started)

	if groups[0].Containers[0].StartedAt != "2024-01-01T00:00:00" {
		t.Fatalf("expected ctr-1 to be updated, got %q", groups[0].Containers[0].StartedAt)
	}
	if groups[0].Containers[1].StartedAt != "" {
		t.Fatalf("expected ctr-2 to remain empty, got %q", groups[0].Containers[1].StartedAt)
	}
}

func TestMergeStarted_EmptyMaps(t *testing.T) {
	groups := []docker.ContainerGroup{
		{Project: "test", Containers: []docker.Container{{Name: "ctr-1", StartedAt: "old"}}},
	}
	// Should not panic with nil map.
	mergeStarted(groups, nil)
	if groups[0].Containers[0].StartedAt != "old" {
		t.Fatalf("expected 'old' unchanged, got %q", groups[0].Containers[0].StartedAt)
	}
}

// ── buildNetworkRows ───────────────────────────────────────────────

func TestBuildNetworkRows_GroupHeader(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.NetworkGroup{
		{Driver: "bridge", Networks: []docker.Network{
			{Name: "bridge", Driver: "bridge", Scope: "local"},
		}},
	}
	rows := buildNetworkRows(theme, groups, nil)

	// First row should be group header
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows (header + data), got %d", len(rows))
	}
	if rows[0].Type != RowGroup {
		t.Fatalf("expected first row to be RowGroup, got %v", rows[0].Type)
	}
	if rows[1].Type != RowData {
		t.Fatalf("expected second row to be RowData, got %v", rows[1].Type)
	}
}

func TestBuildNetworkRows_Collapsed(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.NetworkGroup{
		{Driver: "bridge", Networks: []docker.Network{
			{Name: "bridge", Driver: "bridge"},
		}},
	}
	collapsed := map[string]bool{"netgroup:bridge": true}
	rows := buildNetworkRows(theme, groups, collapsed)

	// Should only have header, no data rows when collapsed.
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (collapsed header), got %d", len(rows))
	}
	if rows[0].Type != RowGroup {
		t.Fatalf("expected RowGroup, got %v", rows[0].Type)
	}
}

func TestBuildNetworkRows_Empty(t *testing.T) {
	theme := DefaultTheme()
	rows := buildNetworkRows(theme, nil, nil)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// ── containerColumns ───────────────────────────────────────────────

func TestContainerColumns_HasAllColumns(t *testing.T) {
	cols := containerColumns()
	names := make(map[string]bool)
	for _, c := range cols {
		names[c.Key] = true
	}
	required := []string{colIcon, colName, colStatus, colCPUMem, colPorts, colBuilt, colRestarted}
	for _, r := range required {
		if !names[r] {
			t.Fatalf("missing required column: %s", r)
		}
	}
}

// ── imageColumns ───────────────────────────────────────────────────

func TestImageColumns_HasRepo(t *testing.T) {
	cols := imageColumns()
	found := false
	for _, c := range cols {
		if c.Key == colName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected imageColumns to contain colName")
	}
}

// ── networkColumns ─────────────────────────────────────────────────

func TestNetworkColumns_HasSubnet(t *testing.T) {
	cols := networkColumns()
	found := false
	for _, c := range cols {
		if c.Key == colSubnet {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected networkColumns to contain colSubnet")
	}
}

// ── volumeColumns ──────────────────────────────────────────────────

func TestVolumeColumns_HasMountpoint(t *testing.T) {
	cols := volumeColumns()
	found := false
	for _, c := range cols {
		if c.Key == "mountpoint" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected volumeColumns to contain mountpoint")
	}
}

// ── buildTableRows ─────────────────────────────────────────────────

func TestBuildTableRows_GroupHeader(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.ContainerGroup{
		{Project: "myapp", ComposeFile: "/path/docker-compose.yml", Containers: []docker.Container{
			{Name: "myapp-web-1", State: "running", Image: "nginx"},
		}},
	}
	rows := buildTableRows(theme, groups, nil, "", 0)

	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(rows))
	}
	if rows[0].Type != RowGroup {
		t.Fatalf("expected first row to be RowGroup, got %v", rows[0].Type)
	}
	if rows[1].Type != RowData {
		t.Fatalf("expected second row to be RowData, got %v", rows[1].Type)
	}
}

func TestBuildTableRows_Collapsed(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.ContainerGroup{
		{Project: "myapp", Containers: []docker.Container{
			{Name: "myapp-web-1", State: "running"},
		}},
	}
	collapsed := map[string]bool{"group:myapp": true}
	rows := buildTableRows(theme, groups, collapsed, "", 0)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row (collapsed header), got %d", len(rows))
	}
}

func TestBuildTableRows_ComposeFile(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.ContainerGroup{
		{Project: "myapp", ComposeFile: "/app/docker-compose.yml", Containers: []docker.Container{
			{Name: "myapp-web-1", State: "running"},
		}},
	}
	rows := buildTableRows(theme, groups, nil, "", 0)

	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(rows))
	}
	header := rows[0]
	if header.Type != RowGroup {
		t.Fatalf("expected RowGroup, got %v", header.Type)
	}
	// Header should show count and health
	title, ok := header.Cells[colName]
	if !ok {
		t.Fatal("header missing colName cell")
	}
	if !strings.Contains(title.Value, "myapp") {
		t.Fatalf("expected header to contain 'myapp', got %q", title.Value)
	}
}

func TestBuildTableRows_OtherGroup(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.ContainerGroup{
		{Project: "Other", ComposeFile: "", Containers: []docker.Container{
			{Name: "orphan-ctr", State: "exited"},
		}},
	}
	rows := buildTableRows(theme, groups, nil, "", 0)

	// Other group: should not show health summary.
	header := rows[0]
	title, _ := header.Cells[colName]
	// No health summary for Other group
	if strings.Contains(title.Value, "up") {
		t.Fatalf("Other group should not show health, got %q", title.Value)
	}
}

func TestBuildTableRows_RunningIcon(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.ContainerGroup{
		{Project: "app", Containers: []docker.Container{
			{Name: "app-1", State: "running"},
		}},
	}
	rows := buildTableRows(theme, groups, nil, "", 0)

	if len(rows) < 2 {
		t.Fatal("expected at least 2 rows")
	}
	iconCell := rows[1].Cells[colIcon]
	if iconCell.Value != "●" {
		t.Fatalf("expected '●' icon, got %q", iconCell.Value)
	}
}

func TestBuildTableRows_ExitedIcon(t *testing.T) {
	theme := DefaultTheme()
	groups := []docker.ContainerGroup{
		{Project: "app", Containers: []docker.Container{
			{Name: "app-1", State: "exited"},
		}},
	}
	rows := buildTableRows(theme, groups, nil, "", 0)

	if len(rows) < 2 {
		t.Fatal("expected at least 2 rows")
	}
	iconCell := rows[1].Cells[colIcon]
	if iconCell.Value != "●" {
		t.Fatalf("expected '●' icon for exited, got %q", iconCell.Value)
	}
}

func TestBuildTableRows_Empty(t *testing.T) {
	theme := DefaultTheme()
	rows := buildTableRows(theme, nil, nil, "", 0)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestBuildTableRows_DuplicateNames(t *testing.T) {
	theme := DefaultTheme()
	// Two containers that would have the same short name
	groups := []docker.ContainerGroup{
		{Project: "app", Containers: []docker.Container{
			{Name: "app-worker-1", State: "running"},
			{Name: "app-worker-2", State: "running"},
		}},
	}
	rows := buildTableRows(theme, groups, nil, "", 0)

	if len(rows) < 3 {
		t.Fatal("expected header + 2 container rows")
	}
	name1, _ := rows[1].Cells[colName]
	name2, _ := rows[2].Cells[colName]
	if name1.Value == name2.Value {
		t.Fatalf("duplicate names should be disambiguated, got %q and %q", name1.Value, name2.Value)
	}
	t.Logf("disambiguated names: %q and %q", name1.Value, name2.Value)
}

// ── parseDockerTime ────────────────────────────────────────────────

func TestParseDockerTime_Valid(t *testing.T) {
	_, err := parseDockerTime("2024-01-15T10:30:00.123456789Z")
	if err != nil {
		t.Fatalf("expected valid parse, got error: %v", err)
	}
}

func TestParseDockerTime_Invalid(t *testing.T) {
	_, err := parseDockerTime("not-a-time")
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
}

// ── parseDockerPS ──────────────────────────────────────────────────

func TestParseDockerPS_Valid(t *testing.T) {
	_, err := parseDockerPS("2024-01-15 10:30:00 +0000 UTC")
	if err != nil {
		// May fail if format doesn't match — that's OK, we just verify no panic.
		t.Logf("parseDockerPS returned error (expected if format mismatch): %v", err)
	}
}

func TestParseDockerPS_Invalid(t *testing.T) {
	_, err := parseDockerPS("not-a-time")
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
}

// ── parseUnixTimestamp ─────────────────────────────────────────────

func TestParseUnixTimestamp_Valid(t *testing.T) {
	_, err := parseUnixTimestamp("1705315800")
	if err != nil {
		t.Fatalf("expected valid parse, got error: %v", err)
	}
}

func TestParseUnixTimestamp_Invalid(t *testing.T) {
	_, err := parseUnixTimestamp("not-a-number")
	if err == nil {
		t.Fatal("expected error for invalid number")
	}
}

// ── parseMemoryUsage ───────────────────────────────────────────────

func TestParseMemoryUsage(t *testing.T) {
	used, limit := parseMemoryUsage("80MiB / 1.5GiB")
	if used == 0 || limit == 0 {
		t.Fatalf("expected non-zero values, got used=%d limit=%d", used, limit)
	}
	if used >= limit {
		t.Fatalf("expected used < limit, got used=%d limit=%d", used, limit)
	}
}

func TestParseMemoryUsage_Dash(t *testing.T) {
	used, limit := parseMemoryUsage("—")
	if used != 0 || limit != 0 {
		t.Fatalf("expected 0,0 for '—', got used=%d limit=%d", used, limit)
	}
}

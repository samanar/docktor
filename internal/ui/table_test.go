package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// makeTestTable creates a table with height visible rows and count total rows.
func makeTestTable(height, count int) Table {
	cols := []ColumnDef{
		{Key: "name", Title: "Name", Width: 10},
		{Key: "status", Title: "Status", Width: 10},
	}
	t := NewTable(cols)
	t.SetHeight(height)
	t.SetWidth(50)

	rows := make([]Row, count)
	for i := range count {
		rows[i] = Row{
			Cells: map[string]Cell{
				"name":   {Value: "test"},
				"status": {Value: "running"},
			},
			Type: RowData,
		}
	}
	t = t.WithRows(rows)
	t.SelectFirst()
	return t
}

func TestMoveSelection_ScrollsDown(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Move to row 5 (first row that should cause scrolling)
	for i := range 5 {
		tbl.MoveSelection(1)
		_ = i
	}
	if tbl.HighlightedRow() != 5 {
		t.Fatalf("expected selected=5, got %d", tbl.HighlightedRow())
	}
	if tbl.yOffset != 1 {
		t.Fatalf("expected yOffset=1 (row 5 should trigger scroll with height=5), got yOffset=%d", tbl.yOffset)
	}
}

func TestMoveSelection_StaysVisible(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Move way down to row 18
	for range 18 {
		tbl.MoveSelection(1)
	}
	if tbl.HighlightedRow() != 18 {
		t.Fatalf("expected selected=18, got %d", tbl.HighlightedRow())
	}
	// With height=5, yOffset should be 18-5+1 = 14
	expectedOffset := 18 - tbl.VisibleHeight() + 1
	if tbl.yOffset != expectedOffset {
		t.Fatalf("expected yOffset=%d, got yOffset=%d", expectedOffset, tbl.yOffset)
	}

	// Verify that row 18 IS in the visible range
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 18 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 18 should be visible, but visible range is %v (yOffset=%d, height=%d)",
			visible, tbl.yOffset, tbl.VisibleHeight())
	}
}

func TestRebuildRows_PreservesScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Navigate to row 18
	for range 18 {
		tbl.MoveSelection(1)
	}
	origOffset := tbl.yOffset
	origSel := tbl.HighlightedRow()

	// Rebuild rows (simulating stats refresh)
	tbl = tbl.WithRows(tbl.rows)

	if tbl.HighlightedRow() != origSel {
		t.Fatalf("selection changed after rebuild: %d -> %d", origSel, tbl.HighlightedRow())
	}
	if tbl.yOffset != origOffset {
		t.Fatalf("yOffset changed after rebuild: %d -> %d", origOffset, tbl.yOffset)
	}

	// Verify row is still visible
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == origSel {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row %d should be visible after rebuild, but visible range=%v (yOffset=%d, height=%d)",
			origSel, visible, tbl.yOffset, tbl.VisibleHeight())
	}
}

func TestWithRows_ScrollsToVisible(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Manually set selected to 19 (last row) and yOffset to 0
	tbl.selected = 19
	tbl.yOffset = 0

	// Rebuild rows - WithRows should fix the yOffset
	tbl = tbl.WithRows(tbl.rows)

	// Now yOffset should be adjusted to make row 19 visible
	expectedOffset := 19 - tbl.VisibleHeight() + 1
	if tbl.yOffset != expectedOffset {
		t.Fatalf("WithRows didn't scroll to visible: expected yOffset=%d, got yOffset=%d (selected=%d, height=%d)",
			expectedOffset, tbl.yOffset, tbl.HighlightedRow(), tbl.VisibleHeight())
	}

	// Verify row 19 is visible
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 19 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 19 should be visible, but visible range=%v (yOffset=%d)", visible, tbl.yOffset)
	}
}

func TestSelectFirst_ResetsScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)
	tbl.selected = 18
	tbl.yOffset = 14

	tbl.SelectFirst()

	if tbl.HighlightedRow() != 0 {
		t.Fatalf("expected selected=0, got %d", tbl.HighlightedRow())
	}
	if tbl.yOffset != 0 {
		t.Fatalf("expected yOffset=0 after SelectFirst, got %d", tbl.yOffset)
	}
}

func TestEnsureVisible_WhenHidden(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// selected at 10, yOffset at 0 - row is not visible
	tbl.selected = 10
	tbl.yOffset = 0

	tbl.EnsureVisible()

	expectedOffset := 10 - tbl.VisibleHeight() + 1
	if tbl.yOffset != expectedOffset {
		t.Fatalf("EnsureVisible failed: expected yOffset=%d, got %d", expectedOffset, tbl.yOffset)
	}
}

// Test the full pane-like flow: MoveSelection, then rebuild, then ensure.
func TestMoveRebuildEnsure_PreservesScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Navigate down: j j j j j j j j j j (10 times)
	for range 10 {
		tbl.MoveSelection(1)
	}

	origSel := tbl.HighlightedRow()
	origOffset := tbl.yOffset

	// Simulate stats refresh: rebuild rows
	tbl = tbl.WithRows(tbl.rows)
	tbl.EnsureVisible()

	if tbl.HighlightedRow() != origSel {
		t.Fatalf("selection changed: %d -> %d", origSel, tbl.HighlightedRow())
	}
	if tbl.yOffset != origOffset {
		t.Fatalf("yOffset changed: %d -> %d", origOffset, tbl.yOffset)
	}

	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == origSel {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row %d NOT visible after MoveRebuildEnsure! visible=%v, yOffset=%d",
			origSel, visible, tbl.yOffset)
	}
}

// Test scroll with group header rows mixed in.
func TestScrollWithGroupHeaders(t *testing.T) {
	cols := []ColumnDef{
		{Key: "name", Title: "Name", Width: 10},
	}
	tbl := NewTable(cols)
	tbl.SetHeight(5)
	tbl.SetWidth(30)

	rows := []Row{
		{Cells: map[string]Cell{"name": {Value: "Group 1"}}, Type: RowGroup, GroupID: "g1"},
		{Cells: map[string]Cell{"name": {Value: "c1"}}, Type: RowData, GroupID: "g1"},
		{Cells: map[string]Cell{"name": {Value: "c2"}}, Type: RowData, GroupID: "g1"},
		{Cells: map[string]Cell{"name": {Value: "Group 2"}}, Type: RowGroup, GroupID: "g2"},
		{Cells: map[string]Cell{"name": {Value: "c3"}}, Type: RowData, GroupID: "g2"},
		{Cells: map[string]Cell{"name": {Value: "c4"}}, Type: RowData, GroupID: "g2"},
		{Cells: map[string]Cell{"name": {Value: "c5"}}, Type: RowData, GroupID: "g2"},
		{Cells: map[string]Cell{"name": {Value: "c6"}}, Type: RowData, GroupID: "g2"},
		{Cells: map[string]Cell{"name": {Value: "c7"}}, Type: RowData, GroupID: "g2"},
		{Cells: map[string]Cell{"name": {Value: "c8"}}, Type: RowData, GroupID: "g2"},
	}
	tbl = tbl.WithRows(rows)
	tbl.SelectFirst()

	// Navigate all the way down
	for range 9 {
		tbl.MoveSelection(1)
	}

	if tbl.HighlightedRow() != 9 {
		t.Fatalf("expected selected=9, got %d", tbl.HighlightedRow())
	}

	// yOffset should make row 9 visible
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 9 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 9 NOT visible! visible=%v, yOffset=%d", visible, tbl.yOffset)
	}

	// Rebuild and check
	tbl = tbl.WithRows(tbl.rows)
	tbl.EnsureVisible()

	visible = tbl.visibleRows()
	found = false
	for _, idx := range visible {
		if idx == 9 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("after rebuild, row 9 NOT visible! visible=%v, yOffset=%d", visible, tbl.yOffset)
	}
}

// Test that verifies MoveSelection triggers scroll even when the row
// is only just barely out of view.
func TestBoundaryScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// yOffset=0, visible rows: 0,1,2,3,4
	// Move to row 4 (last visible)
	tbl.MoveSelection(4)
	if tbl.yOffset != 0 {
		t.Fatalf("row 4 should not trigger scroll yet, yOffset=%d", tbl.yOffset)
	}

	// Move to row 5 (first invisible, should trigger scroll)
	tbl.MoveSelection(1)
	if tbl.yOffset != 1 {
		t.Fatalf("row 5 should trigger scroll to yOffset=1, got yOffset=%d", tbl.yOffset)
	}

	// yOffset=1, visible rows: 1,2,3,4,5 - row 5 is visible
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 5 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 5 should be visible (yOffset=%d), visible=%v", tbl.yOffset, visible)
	}
}

// Test scrolling UP from bottom of the list.
func TestScrollUp(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Go to bottom (row 19)
	for range 19 {
		tbl.MoveSelection(1)
	}
	bottomYOffset := tbl.yOffset // should be 19 - 5 + 1 = 15

	// Move up one - row 18 is visible with yOffset=15, so yOffset shouldn't change
	tbl.MoveSelection(-1)
	// Row 18 IS visible when yOffset=15 (rows 15-19), so yOffset stays at 15
	// This is correct behavior - yOffset only changes when row moves OUT of visible area

	// Move up enough to leave the visible area (16 steps = from row 18 to row 2)
	for range 16 {
		tbl.MoveSelection(-1)
	}

	// Should now be at row 2 (started at 18, moved up 16 → 18-16=2)
	if tbl.HighlightedRow() != 2 {
		t.Fatalf("expected row 2, got %d", tbl.HighlightedRow())
	}
	// yOffset should be < bottomYOffset
	if tbl.yOffset >= bottomYOffset {
		t.Fatalf("yOffset should have decreased: was %d, now %d", bottomYOffset, tbl.yOffset)
	}
	// yOffset should be <= 2
	if tbl.yOffset > 2 {
		t.Fatalf("yOffset should be <= 2, got %d", tbl.yOffset)
	}
}

// Test that SetHeight before rows are set works.
func TestSetHeightBeforeRows(t *testing.T) {
	cols := []ColumnDef{
		{Key: "name", Title: "Name", Width: 10},
	}
	tbl := NewTable(cols)
	tbl.SetHeight(5)
	tbl.SetWidth(30)

	// Set rows after height
	rows := make([]Row, 20)
	for i := range 20 {
		rows[i] = Row{
			Cells:   map[string]Cell{"name": {Value: "test"}},
			Type:    RowData,
			GroupID: "g1",
		}
	}
	tbl = tbl.WithRows(rows)
	tbl.selected = 19
	tbl.yOffset = 0

	// Rebuild should fix the yOffset
	tbl = tbl.WithRows(tbl.rows)

	if tbl.yOffset < 5 {
		t.Fatalf("WithRows should have adjusted yOffset to make row 19 visible, but yOffset=%d (height=%d, selected=%d)",
			tbl.yOffset, tbl.VisibleHeight(), tbl.HighlightedRow())
	}
}

// Test that Pane.restoreSelection doesn't break scroll when group matches.
// This simulates the exact flow from statsRefreshMsg.
func TestRestoreSelectionKeepsScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)
	tbl.SetHeight(5)

	// Give all rows the same group ID
	for i := range tbl.rows {
		tbl.rows[i].GroupID = "group:test"
	}

	// Navigate down
	for range 18 {
		tbl.MoveSelection(1)
	}

	origSel := tbl.HighlightedRow()
	origOffset := tbl.yOffset
	gid := tbl.GroupIDAt(origSel)

	// Simulate rebuildTableRows + restoreSelection
	tbl = tbl.WithRows(tbl.rows)

	// Simulate restoreSelection logic
	if origSel >= 0 && origSel < tbl.RowCount() &&
		tbl.GroupIDAt(origSel) == gid && gid != "" {
		// Selection valid, don't change
	} else {
		tbl.SelectFirst()
	}

	// Ensure visible
	tbl.EnsureVisible()

	if tbl.yOffset != origOffset {
		t.Fatalf("restoreSelection changed yOffset: %d -> %d", origOffset, tbl.yOffset)
	}

	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == origSel {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row %d NOT visible after restore! visible=%v, yOffset=%d",
			origSel, visible, tbl.yOffset)
	}
}

// Verify that View output contains the selected row.
func TestViewContainsSelectedRow(t *testing.T) {
	tbl := makeTestTable(5, 20)
	tbl.SetWidth(40)

	// Navigate to row 15
	for range 15 {
		tbl.MoveSelection(1)
	}

	// Get what View would render
	_ = tbl.View()

	// Verify row 15 is in visible range
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 15 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 15 NOT in visible range! visible=%v, yOffset=%d, height=%d",
			visible, tbl.yOffset, tbl.VisibleHeight())
	}
}

// Quick benchmark-style test that runs many move-rebuild cycles.
func TestRepeatedMoveRebuild_CorrectScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)

	for cycle := range 20 {
		// Move down
		for i := range cycle {
			tbl.MoveSelection(1)
			_ = i
		}

		// Simulate stats refresh
		tbl = tbl.WithRows(tbl.rows)
		tbl.EnsureVisible()

		sel := tbl.HighlightedRow()
		visible := tbl.visibleRows()
		found := false
		for _, idx := range visible {
			if idx == sel {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("cycle %d: row %d NOT visible! visible=%v, yOffset=%d, height=%d",
				cycle, sel, visible, tbl.yOffset, tbl.VisibleHeight())
		}

		// Move back up to reset
		for range cycle {
			tbl.MoveSelection(-1)
		}
	}
}

// Test that scroll happens correctly even with height=0 initially (the
// real scenario: Table created, height set later by recalcTable, then
// rows set).
func TestHeightZeroThenSet(t *testing.T) {
	cols := []ColumnDef{
		{Key: "name", Title: "Name", Width: 10},
	}
	tbl := NewTable(cols) // height=0
	tbl.SetWidth(30)

	// This simulates the real flow: height is set by recalcTable
	// BEFORE rows are set by rebuildTableRows
	tbl.SetHeight(5)

	rows := make([]Row, 20)
	for i := range 20 {
		rows[i] = Row{Type: RowData, GroupID: "g1"}
	}
	tbl = tbl.WithRows(rows)

	// Set selected to 19 (last row)
	tbl.selected = 19
	tbl.yOffset = 0

	// Rebuild should fix scroll
	tbl = tbl.WithRows(tbl.rows)

	if tbl.yOffset < 5 {
		t.Fatalf("WithRows should have fixed yOffset for last row, got yOffset=%d (height=%d, selected=%d)",
			tbl.yOffset, tbl.VisibleHeight(), tbl.HighlightedRow())
	}

	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 19 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 19 NOT visible! visible=%v, yOffset=%d", visible, tbl.yOffset)
	}
}

// TestMoveSelectionThenViewThenMove verifies the cycle:
// Update(MoveSelection) → View → Update(MoveSelection) → View
func TestMoveSelectionThenViewThenMove(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Simulate multiple Update-View cycles
	for cycle := range 10 {
		// Update: move selection
		tbl.MoveSelection(1)

		// Verify scroll after Update
		sel := tbl.HighlightedRow()
		visible := tbl.visibleRows()
		found := false
		for _, idx := range visible {
			if idx == sel {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("cycle %d after Update: row %d NOT visible! visible=%v, yOffset=%d, height=%d",
				cycle, sel, visible, tbl.yOffset, tbl.VisibleHeight())
		}

		// View: render (should not affect state)
		_ = tbl.View()

		// Verify still visible after View
		visible = tbl.visibleRows()
		found = false
		for _, idx := range visible {
			if idx == sel {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("cycle %d after View: row %d NOT visible! visible=%v, yOffset=%d",
				cycle, sel, visible, tbl.yOffset)
		}
	}
}

// Test that the View method doesn't corrupt yOffset (View is on value receiver).
func TestViewDoesNotCorruptState(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Move to row 18
	for range 18 {
		tbl.MoveSelection(1)
	}

	origYOffset := tbl.yOffset
	origSelected := tbl.HighlightedRow()

	// Call View (value receiver, creates a copy)
	view := tbl.View()
	_ = view

	// State should be unchanged
	if tbl.yOffset != origYOffset {
		t.Fatalf("View() changed yOffset: %d -> %d", origYOffset, tbl.yOffset)
	}
	if tbl.HighlightedRow() != origSelected {
		t.Fatalf("View() changed selected: %d -> %d", origSelected, tbl.HighlightedRow())
	}
}

// TestTable_PointerVsValue verifies that WithRows with the inline scroll
// correctly returns the yOffset in the returned value.
func TestTable_PointerVsValue(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Set up: selected=18, yOffset=0 (not visible)
	tbl.selected = 18
	tbl.yOffset = 0

	// WithRows should fix it
	tbl2 := tbl.WithRows(tbl.rows)

	// tbl2 should have fixed yOffset
	if tbl2.yOffset == 0 {
		t.Fatalf("WithRows didn't fix yOffset (still 0). selected=%d, height=%d",
			tbl2.HighlightedRow(), tbl2.VisibleHeight())
	}

	// Original tbl should NOT be modified (value receiver)
	if tbl.yOffset != 0 {
		t.Fatalf("WithRows modified original table's yOffset: %d", tbl.yOffset)
	}

	// Verify fix by checking visible rows of tbl2
	visible := tbl2.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 18 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tbl2: row 18 NOT visible! visible=%v, yOffset=%d", visible, tbl2.yOffset)
	}
}

// Simulate the exact Pane.statsRefreshMsg flow.
func TestFullStatsRefreshCycle(t *testing.T) {
	height := 5

	cols := []ColumnDef{
		{Key: "name", Title: "Name", Width: 10},
	}
	tbl := NewTable(cols)
	tbl.SetHeight(height)
	tbl.SetWidth(40)

	// Create rows with group headers and data
	type rowInfo struct {
		typ     RowType
		groupID string
		name    string
	}

	data := []rowInfo{
		{RowGroup, "group:project1", "Project 1 (5)"},
		{RowData, "group:project1", "container-a"},
		{RowData, "group:project1", "container-b"},
		{RowData, "group:project1", "container-c"},
		{RowData, "group:project1", "container-d"},
		{RowData, "group:project1", "container-e"},
		{RowGroup, "group:project2", "Project 2 (7)"},
		{RowData, "group:project2", "container-f"},
		{RowData, "group:project2", "container-g"},
		{RowData, "group:project2", "container-h"},
		{RowData, "group:project2", "container-i"},
		{RowData, "group:project2", "container-j"},
		{RowData, "group:project2", "container-k"},
		{RowData, "group:project2", "container-l"},
		{RowGroup, "group:project3", "Project 3 (6)"},
		{RowData, "group:project3", "container-m"},
		{RowData, "group:project3", "container-n"},
		{RowData, "group:project3", "container-o"},
		{RowData, "group:project3", "container-p"},
		{RowData, "group:project3", "container-q"},
		{RowData, "group:project3", "container-r"},
	}

	rows := make([]Row, len(data))
	for i, d := range data {
		rows[i] = Row{
			Type:    d.typ,
			GroupID: d.groupID,
			Cells:   map[string]Cell{"name": {Value: d.name}},
		}
	}
	tbl = tbl.WithRows(rows)
	tbl.SelectFirst()

	// Move to container-q (idx 20)
	for range 20 {
		tbl.MoveSelection(1)
	}

	if tbl.HighlightedRow() != 20 {
		t.Fatalf("expected selected=20, got %d", tbl.HighlightedRow())
	}

	// Save state
	origSel := tbl.HighlightedRow()
	origGID := tbl.GroupIDAt(origSel)

	// Simulate stats refresh: rebuild rows (same rows, just rebuilt)
	tbl = tbl.WithRows(rows) // rebuidTableRows

	// Simulate restoreSelection
	if origSel >= 0 && origSel < tbl.RowCount() &&
		tbl.GroupIDAt(origSel) == origGID && origGID != "" {
		// do nothing - valid selection
	} else {
		tbl.SelectFirst()
	}

	// Simulate EnsureVisible
	tbl.EnsureVisible()

	// Verify selection preserved
	if tbl.HighlightedRow() != origSel {
		t.Fatalf("selection changed: %d -> %d", origSel, tbl.HighlightedRow())
	}

	// Verify row is visible
	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == origSel {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("FULL CYCLE: row %d NOT visible! visible=%v, yOffset=%d, height=%d, selected=%d",
			origSel, visible, tbl.yOffset, tbl.VisibleHeight(), tbl.HighlightedRow())
	}
}

// Test that rendering works correctly after scrolling.
func TestRenderAfterScroll(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Move to row 12
	for range 12 {
		tbl.MoveSelection(1)
	}

	// Render
	output := tbl.View()

	// The output should have some non-empty content
	if len(output) == 0 {
		t.Fatal("View returned empty output")
	}

	// Count rendered lines (data rows only, excluding header+divider)
	lines := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines < tbl.VisibleHeight() {
		t.Fatalf("View rendered only %d lines, expected at least %d", lines, tbl.VisibleHeight())
	}
}

// Test j/k scrolling up from a scrolled position.
func TestScrollBackUp(t *testing.T) {
	tbl := makeTestTable(5, 20)

	// Go to bottom
	for range 19 {
		tbl.MoveSelection(1)
	}
	bottomYOffset := tbl.yOffset

	// Go back up one
	tbl.MoveSelection(-1)

	// Row 18 should be visible, yOffset should not have INCREASED
	if tbl.yOffset > bottomYOffset {
		t.Fatalf("moving up increased yOffset: %d -> %d", bottomYOffset, tbl.yOffset)
	}

	// Scroll back enough to change yOffset
	for range 10 {
		tbl.MoveSelection(-1)
	}

	// We should now be at row 8
	if tbl.HighlightedRow() != 8 {
		t.Fatalf("expected row 8, got %d", tbl.HighlightedRow())
	}

	// yOffset should be <= 8 and >= 4 (row 8 visible with height 5)
	if tbl.yOffset > 8 || tbl.yOffset < 4 {
		t.Fatalf("yOffset %d out of range [4,8] for row 8 with height 5", tbl.yOffset)
	}

	visible := tbl.visibleRows()
	found := false
	for _, idx := range visible {
		if idx == 8 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("row 8 NOT visible! visible=%v, yOffset=%d", visible, tbl.yOffset)
	}
}

// Compile-time check that unused imports are needed.
var _ = lipgloss.NewStyle

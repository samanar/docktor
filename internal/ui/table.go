package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Column definition ─────────────────────────────────────────────

// ColumnDef defines a single column in the table.
//   - Width > 0  → fixed-width column (in characters)
//   - Width == 0 → flex column (shares remaining space equally)
type ColumnDef struct {
	Key   string
	Title string
	Width int
}

// ── Cell ──────────────────────────────────────────────────────────

// Cell holds a value with optional cell-level styling.
// If Style is the zero-value, the table's default body style is used.
type Cell struct {
	Value string
	Style lipgloss.Style
}

// ── Row types ─────────────────────────────────────────────────────

// RowType distinguishes group headers, data rows, and separators.
type RowType int

const (
	RowData      RowType = iota // normal data row
	RowGroup                    // collapsible group header
	RowSeparator                // visual spacer between sections
)

// Row is a single row in the table.
//   - Type / GroupID are used for collapsible groups (set by the caller).
//   - When Type == RowSeparator, Cells and Style are ignored.
type Row struct {
	Cells   map[string]Cell
	Style   lipgloss.Style
	Type    RowType
	GroupID string // ties container rows to their group header
}

// ── Table ─────────────────────────────────────────────────────────

// Table is a scrollable, stylable table renderer.  It is NOT a
// full Bubble Tea model — the owning component (Pane) drives it
// directly by calling its methods.
type Table struct {
	cols []ColumnDef
	rows []Row

	width  int // total layout width available to the table
	height int // number of visible data rows (excluding header)

	selected int  // currently highlighted row, -1 = none
	yOffset  int  // first visible data row
	focused  bool // dims the highlight when false

	// ── Styles ───────────────────────────────────────
	baseStyle     lipgloss.Style // default cell style
	headerStyle   lipgloss.Style // column title row
	selectedStyle lipgloss.Style // highlighted row overlay
	dividerStyle  lipgloss.Style // horizontal rule under header
	sepStyle      lipgloss.Style // column separator

	// ── Highlight column ─────────────────────────────
	highlightCol      int            // column index for extra selection styling, -1 = none
	highlightColStyle lipgloss.Style // applied to highlightCol when row is selected

	// ── Computed ─────────────────────────────────────
	resolved []int // resolved pixel width per column
}

// NewTable creates a table with the given columns. Call SetWidth /
// SetHeight before rendering.
func NewTable(cols []ColumnDef) Table {
	return Table{
		cols:     cols,
		selected: -1,
		yOffset:  0,
		focused:  true,

		baseStyle:         lipgloss.NewStyle(),
		headerStyle:       lipgloss.NewStyle().Bold(true),
		selectedStyle:     lipgloss.NewStyle(),
		dividerStyle:      lipgloss.NewStyle(),
		sepStyle:          lipgloss.NewStyle(),
		highlightCol:      -1,
		highlightColStyle: lipgloss.NewStyle(),
	}
}

// ── Configuration (fluent) ────────────────────────────────────────

// WithRows replaces all data rows.
func (t Table) WithRows(rows []Row) Table {
	t.rows = rows
	// Clamp selection
	if t.selected >= len(t.rows) {
		t.selected = len(t.rows) - 1
	}
	if t.selected < -1 {
		t.selected = -1
	}
	return t
}

// WithBaseStyle sets the default cell style.
func (t Table) WithBaseStyle(s lipgloss.Style) Table {
	t.baseStyle = s
	return t
}

// WithHeaderStyle sets the header row style.
func (t Table) WithHeaderStyle(s lipgloss.Style) Table {
	t.headerStyle = s
	return t
}

// WithSelectedStyle sets the highlight style for the selected row.
func (t Table) WithSelectedStyle(s lipgloss.Style) Table {
	t.selectedStyle = s
	return t
}

// WithDividerStyle sets the horizontal-rule style.
func (t Table) WithDividerStyle(s lipgloss.Style) Table {
	t.dividerStyle = s
	return t
}

// WithSepStyle sets the column-separator style.
func (t Table) WithSepStyle(s lipgloss.Style) Table {
	t.sepStyle = s
	return t
}

// Focused sets whether the table looks focused (selection visible).
func (t Table) Focused(f bool) Table {
	t.focused = f
	return t
}

// WithHighlightColumn makes the given column index receive an extra
// style overlay when its row is selected (e.g. for bolding the name).
func (t Table) WithHighlightColumn(col int, s lipgloss.Style) Table {
	t.highlightCol = col
	t.highlightColStyle = s
	return t
}

// ── Sizing ────────────────────────────────────────────────────────

// SetWidth updates the total layout width and recalculates column
// widths.  Should be called before every render when the pane resizes.
func (t *Table) SetWidth(w int) {
	if w < 1 {
		w = 1
	}
	t.width = w
	t.resolveWidths()
}

// SetHeight sets the number of visible data rows (excluding header).
// The table will render at most this many rows.
func (t *Table) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	t.height = h
}

// resolveWidths computes column widths. Fixed columns get their
// declared width; flex columns split the remainder.  Flex columns
// get at least 1 char; total never exceeds t.width.
func (t *Table) resolveWidths() {
	n := len(t.cols)
	if n == 0 {
		return
	}
	t.resolved = make([]int, n)

	fixed := 0
	flexN := 0
	for _, c := range t.cols {
		if c.Width > 0 {
			fixed += c.Width
		} else {
			flexN++
		}
	}

	gaps := (n - 1) * 2 // 2-char column gap
	available := t.width - fixed - gaps
	if available < 0 {
		available = 0
	}

	flexEach := 0
	if flexN > 0 {
		flexEach = available / flexN
		if flexEach < 1 {
			flexEach = 1
		}
	}

	// Clamp down if minimum flex widths would overflow
	for fixed+flexEach*flexN+gaps > t.width && flexEach > 1 {
		flexEach--
	}

	for i, c := range t.cols {
		if c.Width > 0 {
			t.resolved[i] = c.Width
		} else {
			t.resolved[i] = flexEach
		}
	}
}

// ── Navigation ────────────────────────────────────────────────────

// MoveSelection shifts the highlighted row by delta (+1 / -1).
func (t *Table) MoveSelection(delta int) {
	if len(t.rows) == 0 {
		t.selected = -1
		return
	}

	newSel := t.selected + delta
	if newSel < 0 {
		newSel = 0
	}
	if newSel >= len(t.rows) {
		newSel = len(t.rows) - 1
	}
	t.selected = newSel
	t.scrollToVisible()
}

// SelectFirst moves selection to the first row.
func (t *Table) SelectFirst() {
	if len(t.rows) > 0 {
		t.selected = 0
		t.yOffset = 0
	}
}

// HighlightedRow returns the index of the selected row, or -1.
func (t Table) HighlightedRow() int { return t.selected }

// RowCount returns the number of rows currently in the table.
func (t Table) RowCount() int { return len(t.rows) }

// RowTypeAt returns the type of the row at the given index.
func (t Table) RowTypeAt(idx int) RowType {
	if idx < 0 || idx >= len(t.rows) {
		return RowData
	}
	return t.rows[idx].Type
}

// GroupIDAt returns the GroupID of the row at the given index.
func (t Table) GroupIDAt(idx int) string {
	if idx < 0 || idx >= len(t.rows) {
		return ""
	}
	return t.rows[idx].GroupID
}

// GetCell returns the value of a cell by row index and column key.
func (t Table) GetCell(rowIdx int, colKey string) (string, bool) {
	if rowIdx < 0 || rowIdx >= len(t.rows) {
		return "", false
	}
	cell, ok := t.rows[rowIdx].Cells[colKey]
	return cell.Value, ok
}

func (t *Table) scrollToVisible() {
	if t.selected < 0 || t.height < 1 {
		return
	}
	if t.selected < t.yOffset {
		t.yOffset = t.selected
	} else if t.selected >= t.yOffset+t.height {
		t.yOffset = t.selected - t.height + 1
	}
	if t.yOffset < 0 {
		t.yOffset = 0
	}
}

// ── Rendering ─────────────────────────────────────────────────────

// View renders the table as a string suitable for lipgloss placement.
// It includes the header row + divider + up to `height` data rows.
func (t Table) View() string {
	if t.width < 1 {
		return ""
	}

	var b strings.Builder

	// ── Header ───────────────────────────────────────
	b.WriteString(t.renderHeader())
	b.WriteString("\n")

	// ── Divider ──────────────────────────────────────
	b.WriteString(t.renderDivider())
	b.WriteString("\n")

	// ── Data rows ────────────────────────────────────
	visible := t.visibleRows()
	for _, idx := range visible {
		b.WriteString(t.renderRow(idx))
		b.WriteString("\n")
	}

	// ── Pad to target height ─────────────────────────
	rendered := len(visible)
	for i := rendered; i < t.height; i++ {
		b.WriteString(strings.Repeat(" ", t.width))
		b.WriteString("\n")
	}

	return b.String()
}

// visibleRows returns the slice of row indices currently in view.
func (t Table) visibleRows() []int {
	if len(t.rows) == 0 || t.height < 1 {
		return nil
	}

	start := t.yOffset
	end := start + t.height
	if end > len(t.rows) {
		end = len(t.rows)
	}

	indices := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		indices = append(indices, i)
	}
	return indices
}

// ── Row rendering ─────────────────────────────────────────────────

func (t Table) renderHeader() string {
	var cells []string
	for i, col := range t.cols {
		w := t.resolved[i]
		cells = append(cells, t.headerStyle.Render(fit(col.Title, w)))
	}
	return strings.Join(cells, "  ")
}

func (t Table) renderDivider() string {
	var parts []string
	for i := range t.cols {
		parts = append(parts, strings.Repeat("─", t.resolved[i]))
	}
	return t.dividerStyle.Render(strings.Join(parts, "──"))
}

func (t Table) renderRow(idx int) string {
	row := t.rows[idx]

	// ── Separator row ────────────────────────────────
	if row.Type == RowSeparator {
		return t.dividerStyle.Render(strings.Repeat("─", t.width))
	}

	isSelected := t.focused && idx == t.selected

	var cells []string
	for i, col := range t.cols {
		w := t.resolved[i]

		cell, ok := row.Cells[col.Key]
		value := ""
		style := lipgloss.NewStyle()
		if ok {
			value = cell.Value
			style = cell.Style
		}

		// Layer: cell style > row style > base style
		// (most specific wins — cell colours override defaults)
		s := style.Inherit(row.Style).Inherit(t.baseStyle)

		// Selected row: overlay highlight background + bold,
		// preserving per-cell foreground (e.g. green status).
		if isSelected {
			s = t.selectedStyle.Inherit(s)
		}

		// Highlight column (name): extra bright foreground when
		// the row is selected.
		if isSelected && i == t.highlightCol {
			s = t.highlightColStyle.Inherit(s)
		}

		// Render cell and include the 2-char column gap in the
		// styled output so the background covers the full row.
		rendered := s.Render(fit(value, w))
		if i < len(t.cols)-1 {
			rendered += s.Render("  ")
		}
		cells = append(cells, rendered)
	}

	return strings.Join(cells, "")
}

// ── Helpers ───────────────────────────────────────────────────────

// fit truncates or right-pads a string to exactly `w` runes.
func fit(s string, w int) string {
	runes := []rune(s)
	if len(runes) > w {
		return string(runes[:w])
	}
	return s + strings.Repeat(" ", w-len(runes))
}

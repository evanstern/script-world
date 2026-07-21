package tui

// Layout math for the widescreen composite (TASK-34). Pure functions over
// terminal width/height — see docs/design/tui/patterns/layout.md. No panel
// measures the terminal itself; Update computes budgets once per
// tea.WindowSizeMsg and View hands each region its exact (width, height).

// widescreenBreakpoint is the width at/above which the composite home page
// (map ‖ dock) renders instead of today's single-pane narrow fallback.
const widescreenBreakpoint = 112

// Column budget constants (widescreen). The map and dock split the
// terminal 50/50 (patterns/layout.md "Column budget") — the map takes the
// odd leftover column when (width - gutter) doesn't divide evenly.
const gutterCols = 1

// Row budget constants (widescreen).
const (
	headerRows     = 1
	minibufferRows = 3
	footerRows     = 1
)

// isWidescreen reports whether width is enough for the composite home page.
func isWidescreen(width int) bool { return width >= widescreenBreakpoint }

// columnBudget is the widescreen composite's horizontal split.
type columnBudget struct {
	MapCols  int
	Gutter   int
	DockCols int
}

// computeColumns splits totalCols 50/50 between the map and the dock (a
// planning decision superseding the earlier fixed-44-col dock — see
// docs/design/tui/patterns/layout.md "Column budget"); the map takes the
// odd column when (totalCols - gutter) is odd.
func computeColumns(totalCols int) columnBudget {
	avail := totalCols - gutterCols
	if avail < 0 {
		avail = 0
	}
	dock := avail / 2
	mapCols := avail - dock
	return columnBudget{MapCols: mapCols, Gutter: gutterCols, DockCols: dock}
}

// rowBudget is the widescreen composite's vertical split.
type rowBudget struct {
	Header     int
	Body       int
	Minibuffer int
	Footer     int
}

// computeRows splits totalRows between the fixed-height chrome (header,
// minibuffer, footer) and the body (map ‖ dock), which takes whatever is
// left. Body never goes negative — panels shed rows before that happens.
func computeRows(totalRows int) rowBudget {
	body := totalRows - headerRows - minibufferRows - footerRows
	if body < 0 {
		body = 0
	}
	return rowBudget{Header: headerRows, Body: body, Minibuffer: minibufferRows, Footer: footerRows}
}

// mapViewportTiles converts a panel's (cols, rows) into terrain tiles: 2
// terminal columns per tile (same family as today's vw/vh computation),
// minus room for the panel's border+padding and legend row.
func mapViewportTiles(panelCols, panelRows int) (tilesW, tilesH int) {
	tilesW = (panelCols - 4) / 2 // border (2) + the box style's Padding(0,1) (2) — B1
	if tilesW < 1 {
		tilesW = 1
	}
	tilesH = panelRows - 3 // border + legend row
	if tilesH < 1 {
		tilesH = 1
	}
	return tilesW, tilesH
}

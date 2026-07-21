package tui

// Layout math for the widescreen composite (TASK-34). Pure functions over
// terminal width/height — see docs/design/tui/patterns/layout.md. No panel
// measures the terminal itself; Update computes budgets once per
// tea.WindowSizeMsg and View hands each region its exact (width, height).

// widescreenBreakpoint is the width at/above which the composite home page
// (map ‖ dock) renders instead of today's single-pane narrow fallback.
const widescreenBreakpoint = 112

// Column budget constants (widescreen). The dock holds steady at
// dockWidthMax until the terminal narrows past the point where the map
// would otherwise shrink below mapWidthTarget; below that, the dock gives
// up columns down to dockWidthMin before the map gives up any.
const (
	dockWidthMax   = 44
	dockWidthMin   = 36
	gutterCols     = 1
	mapWidthTarget = 118 - dockWidthMax - gutterCols // 73: map width once dock is at max
)

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

// computeColumns splits totalCols between the map and the dock. The dock
// shrinks (down to dockWidthMin) before the map gives up its target width;
// see docs/design/tui/patterns/layout.md "Column budget".
func computeColumns(totalCols int) columnBudget {
	dock := totalCols - gutterCols - mapWidthTarget
	if dock > dockWidthMax {
		dock = dockWidthMax
	}
	if dock < dockWidthMin {
		dock = dockWidthMin
	}
	mapCols := totalCols - gutterCols - dock
	if mapCols < 0 {
		mapCols = 0
	}
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
// minus room for the panel's border and legend row.
func mapViewportTiles(panelCols, panelRows int) (tilesW, tilesH int) {
	tilesW = (panelCols - 2) / 2 // border eats ~2 cols
	if tilesW < 1 {
		tilesW = 1
	}
	tilesH = panelRows - 3 // border + legend row
	if tilesH < 1 {
		tilesH = 1
	}
	return tilesW, tilesH
}

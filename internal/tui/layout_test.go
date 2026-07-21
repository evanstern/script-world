package tui

import "testing"

// TestIsWidescreen is the layout.md breakpoint: >=112 widescreen, <112 narrow.
func TestIsWidescreen(t *testing.T) {
	cases := []struct {
		width int
		want  bool
	}{
		{80, false},
		{111, false},
		{112, true},
		{113, true},
		{200, true},
	}
	for _, c := range cases {
		if got := isWidescreen(c.width); got != c.want {
			t.Errorf("isWidescreen(%d) = %v, want %v", c.width, got, c.want)
		}
	}
}

// TestComputeColumns is layout.md's "Column budget": the dock holds 44
// cols until the map would otherwise shrink below its 73-col target, then
// the dock gives up columns (down to a 36-col floor) before the map does.
func TestComputeColumns(t *testing.T) {
	cases := []struct {
		total        int
		wantDock     int
		wantMap      int
		wantMapFixed bool // map should equal 73 (unaffected by the shrink)
	}{
		{200, dockWidthMax, 200 - 1 - dockWidthMax, false},
		{118, dockWidthMax, mapWidthTarget, true},
		{112, 38, mapWidthTarget, true}, // dock shrinks, map holds its target
		{100, dockWidthMin, 100 - 1 - dockWidthMin, false},
	}
	for _, c := range cases {
		got := computeColumns(c.total)
		if got.DockCols != c.wantDock {
			t.Errorf("computeColumns(%d).DockCols = %d, want %d", c.total, got.DockCols, c.wantDock)
		}
		if got.MapCols != c.wantMap {
			t.Errorf("computeColumns(%d).MapCols = %d, want %d", c.total, got.MapCols, c.wantMap)
		}
		if got.MapCols+got.Gutter+got.DockCols != c.total {
			t.Errorf("computeColumns(%d) columns don't sum to total: %+v", c.total, got)
		}
		if got.DockCols < dockWidthMin || got.DockCols > dockWidthMax {
			t.Errorf("computeColumns(%d).DockCols = %d out of [%d,%d]", c.total, got.DockCols, dockWidthMin, dockWidthMax)
		}
	}
}

// TestComputeRows is layout.md's "Row budget": header 1, minibuffer 3,
// footer 1, body takes the remainder (never negative).
func TestComputeRows(t *testing.T) {
	cases := []struct {
		total    int
		wantBody int
	}{
		{40, 40 - 1 - 3 - 1},
		{30, 30 - 1 - 3 - 1},
		{3, 0}, // starved: body floors at 0, never negative
	}
	for _, c := range cases {
		got := computeRows(c.total)
		if got.Header != 1 || got.Minibuffer != 3 || got.Footer != 1 {
			t.Errorf("computeRows(%d) chrome rows wrong: %+v", c.total, got)
		}
		if got.Body != c.wantBody {
			t.Errorf("computeRows(%d).Body = %d, want %d", c.total, got.Body, c.wantBody)
		}
		if got.Body < 0 {
			t.Errorf("computeRows(%d).Body went negative: %d", c.total, got.Body)
		}
	}
}

// TestMapViewportTiles: 2 terminal columns per tile, minus border/legend
// overhead; never below 1x1.
func TestMapViewportTiles(t *testing.T) {
	w, h := mapViewportTiles(74, 30)
	if w != (74-2)/2 {
		t.Errorf("tilesW = %d, want %d", w, (74-2)/2)
	}
	if h != 30-3 {
		t.Errorf("tilesH = %d, want %d", h, 30-3)
	}
	if w2, h2 := mapViewportTiles(1, 1); w2 < 1 || h2 < 1 {
		t.Errorf("tiny panel must floor at 1x1, got %dx%d", w2, h2)
	}
}

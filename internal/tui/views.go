package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/worldmap"
)

var (
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleTabOn  = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	styleTabOff = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	styleDim    = lipgloss.NewStyle().Faint(true)
	styleErr    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	stylePaused = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	styleNight  = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleAgent  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	styleAsleep = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)

	// Style tokens (patterns/layout.md "Style tokens") — one named style per
	// role, panels refer to the role never a raw color.
	stylePanelFocus  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1) // amber, same hue as PAUSED
	styleTabActive   = lipgloss.NewStyle().Bold(true).Underline(true)
	styleTabInactive = lipgloss.NewStyle().Faint(true)
	styleTabBadge    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	styleFeedType    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleFeedName    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	styleFeedSpeech  = lipgloss.NewStyle().Bold(true)
	styleFeedClock   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleFeedSelect  = lipgloss.NewStyle().Reverse(true)
)

func (m Model) View() string {
	if m.quitting {
		if m.fatalErr != "" {
			return styleErr.Render("detached: "+m.fatalErr) + "\n"
		}
		return "detached (the world keeps running)\n"
	}
	if isWidescreen(m.width) {
		return m.widescreenView()
	}
	return m.narrowView()
}

// --- narrow fallback (pages/solo-views.md "Narrow fallback") ---
// Today's single-pane UI, unchanged.

func (m Model) narrowView() string {
	var b strings.Builder
	b.WriteString(m.headerView() + "\n")
	b.WriteString(m.tabsView() + "\n\n")
	switch m.active {
	case paneMap:
		b.WriteString(m.mapView())
	case paneChronicle:
		b.WriteString(m.chronicleView())
	case paneMetatron:
		b.WriteString(m.metatronView())
	case paneSouls:
		b.WriteString(m.soulsView())
	}
	b.WriteString("\n" + m.footerView())
	return b.String()
}

func (m Model) headerView() string {
	name := m.w.Manifest.Name
	if !m.connected {
		msg := fmt.Sprintf("%s — disconnected", name)
		if m.lastErr != "" {
			msg += ": " + m.lastErr
		}
		return styleErr.Render(msg + " (retrying…)")
	}
	if m.status == nil {
		return styleHeader.Render(name)
	}
	c := m.status.Clock
	state := "running"
	if c.Paused {
		state = stylePaused.Render("PAUSED")
	}
	line := fmt.Sprintf("%s — tick %d · %s · %s · speed %s (%.1f t/s)",
		name, c.Tick, c.GameTime, state, c.Speed, c.EffectiveRate)
	if c.Degraded {
		line += " " + styleErr.Render("[degraded]")
	}
	return styleHeader.Render(line)
}

func (m Model) tabsView() string {
	var tabs []string
	for i := pane(0); i < paneCount; i++ {
		label := fmt.Sprintf("%d %s", i+1, paneNames[i])
		if i == m.active {
			tabs = append(tabs, styleTabOn.Render(label))
		} else {
			tabs = append(tabs, styleTabOff.Render(label))
		}
	}
	return strings.Join(tabs, " ")
}

func (m Model) footerView() string {
	switch {
	case m.mbFocused:
		return styleDim.Render("esc release · ⏎ send · ↑↓ history")
	case m.inspecting():
		return styleDim.Render("j/k select · ⏎ expand · space resume · m ask")
	case isWidescreen(m.width) && m.solo:
		return styleDim.Render(fmt.Sprintf("%s back to map · space resume · q quit", dockTabKey[m.dockTab]))
	case isWidescreen(m.width):
		return styleDim.Render("2 chronicle 3 metatron 4 souls (again: solo) · m ask · space pause · q quit")
	default:
		return styleDim.Render("1-4 panes · space pause · q quit")
	}
}

// --- widescreen composite (pages/home.md, pages/solo-views.md "Solo zoom") ---

func (m Model) widescreenView() string {
	cols := computeColumns(m.width)
	rows := computeRows(m.height)

	var body string
	if m.solo {
		body = m.soloPanelView(cols.MapCols+cols.Gutter+cols.DockCols, rows.Body)
	} else {
		mapPanel := m.mapPanelView(cols.MapCols, rows.Body)
		dockPanel := m.dockPanelView(cols.DockCols, rows.Body)
		body = lipgloss.JoinHorizontal(lipgloss.Top, mapPanel, strings.Repeat(" ", cols.Gutter), dockPanel)
	}

	var b strings.Builder
	b.WriteString(m.headerView() + "\n")
	b.WriteString(body + "\n")
	b.WriteString(m.minibufferView(m.width) + "\n")
	b.WriteString(m.footerView())
	return b.String()
}

// mapPanelView is the widescreen MAP region — same glyph rendering as the
// narrow mapView (map.md: "content unchanged"), sized from the column
// budget instead of the full terminal width.
func (m Model) mapPanelView(cols, rows int) string {
	if rows < 5 { // B5: never let a starved resize drive Height() negative
		rows = 5
	}
	title := "MAP · following centroid"
	if m.panX != 0 || m.panY != 0 {
		title = "MAP · panned (c to recenter)"
	}
	vw, vh := mapViewportTiles(cols, rows-1) // -1: title row lives outside the grid box
	grid, legend := m.renderMapGrid(vw, vh)
	content := styleHeader.Render(title) + "\n" + grid
	if legend != "" {
		content += "\n" + legend
	}
	// clipContent is the load-bearing part here (B1): the legend line is
	// prose and routinely wider than the panel — without a hard per-line
	// cap, lipgloss's Width()-driven soft-wrap turns it into two rendered
	// lines, growing the panel past its Height() budget (Height only
	// pads short content, it never truncates tall content) and pushing
	// the header off the top of a real terminal. See clipContent's doc
	// for why a style-level MaxWidth() does not reliably substitute for
	// this. Every panel must render to exactly its handed (width,
	// height) — layout.md's composition contract.
	return styleBox.Width(cols - 2).Height(rows - 2).Render(clipContent(content, cols-2))
}

// dockPanelView is the widescreen DOCK region: tab row + active tab body
// (dock.md "Structure").
func (m Model) dockPanelView(cols, rows int) string {
	if rows < 5 { // B5: never let a starved resize drive Height() negative
		rows = 5
	}
	inner := cols - 4
	if inner < 10 {
		inner = 10
	}
	tabRow := m.dockTabsRow()
	divider := styleDim.Render(strings.Repeat("─", inner))
	content := m.dockTabContent(inner, rows-6)
	body := tabRow + "\n" + divider + "\n" + content
	// clipContent: see mapPanelView — never let a too-wide content line
	// soft-wrap and grow the panel past its Height() budget.
	return styleBox.Width(cols - 2).Height(rows - 2).Render(clipContent(body, cols-2))
}

// soloPanelView renders the same dock content full-width — "one
// implementation, two widths" (pages/solo-views.md "Solo rules").
func (m Model) soloPanelView(cols, rows int) string {
	if rows < 5 { // B5: never let a starved resize drive Height() negative
		rows = 5
	}
	inner := cols - 4
	if inner < 10 {
		inner = 10
	}
	title := styleHeader.Render(m.soloTitle())
	content := m.dockTabContent(inner, rows-4)
	body := title + "\n" + content
	// clipContent: see mapPanelView.
	return styleBox.Width(cols - 2).Height(rows - 2).Render(clipContent(body, cols-2))
}

func (m Model) soloTitle() string {
	name := strings.ToUpper(paneNames[m.dockTab])
	if m.dockTab == paneChronicle {
		if m.inspecting() {
			mode := "raw"
			if !m.chronRaw && m.replica != nil && len(m.replica.Chronicle) > 0 {
				mode = "narrated"
			}
			return fmt.Sprintf("%s · %s · paused — j/k select · ⏎ expand · r narrated", name, mode)
		}
		return name + " · r narrated ↔ raw · a/t filter"
	}
	return name
}

// dockTabsRow is the tab row that "doubles as the panel title" (dock.md).
func (m Model) dockTabsRow() string {
	tabs := []struct {
		p     pane
		label string
	}{
		{paneChronicle, "chronicle"},
		{paneMetatron, "metatron"},
		{paneSouls, "souls"},
	}
	var parts []string
	for _, t := range tabs {
		style := styleTabInactive
		if t.p == m.dockTab {
			style = styleTabActive
		}
		label := t.label
		if t.p == m.dockTab {
			label = strings.ToUpper(label)
		}
		rendered := style.Render(label)
		if t.p == paneMetatron && m.metatronUnseen {
			rendered += " " + styleTabBadge.Render("•")
		}
		parts = append(parts, rendered)
	}
	return strings.Join(parts, styleDim.Render(" │ "))
}

// dockTabContent renders just the active tab's body — shared verbatim by
// the dock panel and the solo view.
func (m Model) dockTabContent(width, height int) string {
	if height < 3 {
		height = 3
	}
	switch m.dockTab {
	case paneChronicle:
		maxWrap := 1
		if width < 60 {
			maxWrap = 3
		}
		return m.chronicleBody(width, height, maxWrap)
	case paneMetatron:
		return m.metatronTranscriptBody(width, height)
	case paneSouls:
		return m.soulsBody(width, height)
	}
	return ""
}

// --- map (panels/map.md: "Rendering is unchanged") ---

// renderMapGrid draws the terrain+agents grid at exactly vw x vh tiles,
// returning the grid block and legend line separately — the shared core
// behind both the narrow mapView (today's vw/vh formula) and the
// widescreen mapPanelView (layout.md's column-budget formula). Only the
// sizing input differs; the glyphs themselves never change.
func (m Model) renderMapGrid(vw, vh int) (grid, legend string) {
	gm := m.gameMap
	if gm == nil {
		return styleDim.Render("no terrain (world manifest missing?)"), ""
	}
	if vw > gm.W {
		vw = gm.W
	}
	if vh > gm.H {
		vh = gm.H
	}
	if vw < 1 {
		vw = 1
	}
	if vh < 1 {
		vh = 1
	}

	// Camera center: wanderer centroid + pan offset, clamped to the map.
	cx, cy := gm.W/2, gm.H/2
	if m.replica != nil {
		sx, sy, n := 0, 0, 0
		for _, a := range m.replica.Agents {
			if a.Dead {
				continue
			}
			sx += a.X
			sy += a.Y
			n++
		}
		if n > 0 {
			cx, cy = sx/n, sy/n
		}
	}
	cx += m.panX
	cy += m.panY
	x0 := clampInt(cx-vw/2, 0, gm.W-vw)
	y0 := clampInt(cy-vh/2, 0, gm.H-vh)

	agents := map[[2]int]string{}
	structures := map[[2]int]string{}
	// Quarried (spec 012, US1): depleted rock outcrops are dynamic overlay
	// state (never part of the static gm.At tile), so the set comes from the
	// replica just like structures/dens below.
	quarried := map[[2]int]bool{}
	// Piles (spec 013 US2): ground piles are dynamic overlay state, same
	// treatment as Quarried/Structures — never part of the static gm.At tile.
	piles := map[[2]int]bool{}
	if m.replica != nil {
		for _, st := range m.replica.Structures {
			switch st.Kind {
			case "fire":
				// Lit vs cold (spec 012 T019/T024): lit iff current tick <
				// FuelUntil. A cold fire shows a hollow, faint glyph so the
				// player can tell a dead fire from a burning one (SC-006).
				if m.replica.Tick < st.FuelUntil {
					structures[[2]int{st.X, st.Y}] = styleFire.Render("▲")
				} else {
					structures[[2]int{st.X, st.Y}] = styleFireCold.Render("△")
				}
			case "shelter":
				structures[[2]int{st.X, st.Y}] = styleShelter.Render("⌂")
			case "oven":
				structures[[2]int{st.X, st.Y}] = styleOven.Render("▣")
			case "chest":
				structures[[2]int{st.X, st.Y}] = styleChest.Render("☐")
			}
		}
		for _, q := range m.replica.Quarried {
			quarried[[2]int{q.X, q.Y}] = true
		}
		// Piles (spec 013 US2): a dedicated map, not folded into
		// structures — build-site validation (FR-007) keeps piles and
		// structures off the same tile, but keeping them separate means a
		// coincidental overlap loses neither glyph's priority silently.
		for _, p := range m.replica.Piles {
			piles[[2]int{p.X, p.Y}] = true
		}
		for _, a := range m.replica.Agents {
			g := strings.ToUpper(a.Name[:1])
			switch {
			case a.Dead:
				g = styleErr.Render("†")
			case a.Asleep:
				g = styleAsleep.Render(strings.ToLower(g))
			default:
				g = styleAgent.Render(g)
			}
			agents[[2]int{a.X, a.Y}] = g
		}
	}
	dens := map[[2]int]bool{}
	for _, d := range gm.Dens {
		dens[[2]int{d.X, d.Y}] = true
	}

	gruX, gruY := -1, -1
	if m.replica != nil && m.replica.Gru != nil {
		gruX, gruY = m.replica.Gru.X, m.replica.Gru.Y
	}

	night := m.replica != nil && m.replica.Night
	tile := func(x, y int) string {
		if x == gruX && y == gruY {
			return styleGru.Render("G")
		}
		if g, ok := agents[[2]int{x, y}]; ok {
			return g
		}
		if g, ok := structures[[2]int{x, y}]; ok {
			return g
		}
		if piles[[2]int{x, y}] {
			return stylePile.Render("%")
		}
		if dens[[2]int{x, y}] {
			return styleDen.Render("ᴥ")
		}
		var s string
		var st lipgloss.Style
		switch {
		case quarried[[2]int{x, y}]:
			// Depleted outcrop (effective-kind path, worldmap.Depleted):
			// passable dug-out ground, distinct from both intact rock and
			// plain grass (research R8).
			s, st = ",", styleDepleted
		case gm.At(x, y) == worldmap.Water:
			s, st = "~", styleWater
		case gm.At(x, y) == worldmap.Tree:
			s, st = "♠", styleTree
		case gm.At(x, y) == worldmap.Forage:
			s, st = "\"", styleForage
		case gm.At(x, y) == worldmap.Rock:
			s, st = "^", styleRock
		default:
			s, st = "·", styleDim
		}
		if night {
			st = st.Faint(true)
		}
		return st.Render(s)
	}

	var rows []string
	for y := y0; y < y0+vh; y++ {
		var row strings.Builder
		for x := x0; x < x0+vw; x++ {
			row.WriteString(tile(x, y) + " ")
		}
		rows = append(rows, strings.TrimRight(row.String(), " "))
	}
	grid = strings.Join(rows, "\n")

	phase := "day"
	if night {
		phase = styleNight.Render("night")
	}
	// Stockpile inspection (spec 013 T021, US2-AS5, SC-006): piles currently
	// in view are grouped into zones by 4-neighbor Manhattan adjacency — a
	// render-side-only computation (no zone state; data-model.md, spec.md
	// "Stockpile zone") — and each zone's aggregate contents (non-food
	// counts + food batch totals) are appended to the legend line, the
	// map panel's one designated inspection surface (map.md: "legend stays
	// pinned as the panel's last row" — content grows the line, never a
	// second row; clipContent already clips an over-wide legend, so this is
	// safe the same way the existing key text is).
	pilesInfo := ""
	if m.replica != nil && len(m.replica.Piles) > 0 {
		var visible []sim.Pile
		for _, p := range m.replica.Piles {
			if p.X >= x0 && p.X < x0+vw && p.Y >= y0 && p.Y < y0+vh {
				visible = append(visible, p)
			}
		}
		if len(visible) > 0 {
			var bits []string
			for _, zone := range pileZones(visible) {
				bits = append(bits, describePileZone(zone))
			}
			pilesInfo = " · " + strings.Join(bits, " · ")
		}
	}
	// Chest inspection (spec 013 T026, SC-006): chests currently in view get
	// an owner + contents entry appended to the same legend line, following
	// the pile inspection precedent above (T021) — the map panel's one
	// designated inspection surface, content grows the line rather than
	// adding a second row.
	chestsInfo := ""
	if m.replica != nil {
		var visible []sim.Structure
		for _, st := range m.replica.Structures {
			if st.Kind != "chest" {
				continue
			}
			if st.X >= x0 && st.X < x0+vw && st.Y >= y0 && st.Y < y0+vh {
				visible = append(visible, st)
			}
		}
		if len(visible) > 0 {
			names := m.agentNames()
			var bits []string
			for _, ch := range visible {
				bits = append(bits, describeChest(ch, names))
			}
			chestsInfo = " · " + strings.Join(bits, " · ")
		}
	}
	legend = styleDim.Render(fmt.Sprintf(
		"%s · [%d,%d–%d,%d of %d×%d] · ~water ♠wood \"forage ^rock ,quarried ᴥden ▲fire △cold ⌂shelter ▣oven %%pile ☐chest · agents by initial (lowercase asleep, †dead) · arrows pan, c center%s%s",
		phase, x0, y0, x0+vw-1, y0+vh-1, gm.W, gm.H, pilesInfo, chestsInfo))
	return grid, legend
}

// chestCapDisplay mirrors internal/sim's private chestCap (spec 013
// data-model.md: per-chest stored bulk ceiling, 48) for the TUI's fullness
// hint. Not exported from internal/sim by this task (T026 is TUI-only, no
// internal/sim edits) — the same "mirrored, single source of truth stays in
// sim" caveat as BulkCap/Bulk applies in spirit, but this constant is
// display-only and never used to clamp or gate anything, so a private
// mirror is safe here.
const chestCapDisplay = 48

// describeChest renders one chest's inspection entry (spec 013 T026,
// SC-006): "chest(x,y) [Owner] <contents> <bulk>/<cap>" — owner resolved to
// the agent's Name via the same agentName helper the chronicle grammar uses
// (grammar.go), contents via summarizeInventoryContents (mirroring the pile
// zone summary's "non-food counts + food batch totals" shape, T021), and a
// fullness hint so "is the chest full" is answerable without opening state.
func describeChest(ch sim.Structure, names []string) string {
	owner := agentName(names, ch.Owner)
	contents := "empty"
	full := 0
	if ch.Store != nil {
		full = sim.Bulk(*ch.Store)
		contents = summarizeInventoryContents(*ch.Store)
	}
	return fmt.Sprintf("chest(%d,%d) [%s] %s %d/%d", ch.X, ch.Y, owner, contents, full, chestCapDisplay)
}

// summarizeInventoryContents renders a chest's Store the same way
// summarizePileContents renders a pile's aggregate contents (T021): each
// non-zero resource count, a spear count, and the food triplet as one
// "food Nr/Nc/Nm" entry when any food is held. A chest's Store is a plain
// sim.Inventory (counts, not FoodBatch — chests preserve food forever, no
// spoilage deadlines to track, FR-010), so this reads the counts directly
// rather than summing batches.
func summarizeInventoryContents(inv sim.Inventory) string {
	var parts []string
	if inv.Wood > 0 {
		parts = append(parts, fmt.Sprintf("%dw", inv.Wood))
	}
	if inv.Stone > 0 {
		parts = append(parts, fmt.Sprintf("%dst", inv.Stone))
	}
	if inv.Water > 0 {
		parts = append(parts, fmt.Sprintf("%dwt", inv.Water))
	}
	if inv.Planks > 0 {
		parts = append(parts, fmt.Sprintf("%dpl", inv.Planks))
	}
	if inv.RefinedStone > 0 {
		parts = append(parts, fmt.Sprintf("%drs", inv.RefinedStone))
	}
	if n := len(inv.Spears); n > 0 {
		parts = append(parts, fmt.Sprintf("%dspear", n))
	}
	if inv.FoodRaw+inv.FoodCooked+inv.Meals > 0 {
		parts = append(parts, fmt.Sprintf("food %dr/%dc/%dm", inv.FoodRaw, inv.FoodCooked, inv.Meals))
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, " ")
}

// pileZones groups piles into stockpile zones by 4-neighbor Manhattan
// adjacency (spec.md "Stockpile zone": "an observability grouping of
// adjacent piles — a rendering concept, not a state entity"). Purely a
// render-side flood fill: it reads only the piles handed to it and
// produces no state. Deterministic given a deterministic input order —
// zones are discovered in `piles` order, and each zone's members are
// visited in a fixed 4-neighbor order (N, E, S, W), matching the sim
// package's own Manhattan-adjacency convention (internal/sim/state.go
// pileOnOrAdjacent's neighborOrder).
func pileZones(piles []sim.Pile) [][]sim.Pile {
	byTile := make(map[[2]int]sim.Pile, len(piles))
	for _, p := range piles {
		byTile[[2]int{p.X, p.Y}] = p
	}
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	visited := make(map[[2]int]bool, len(piles))
	var zones [][]sim.Pile
	for _, p := range piles {
		start := [2]int{p.X, p.Y}
		if visited[start] {
			continue
		}
		visited[start] = true
		queue := [][2]int{start}
		var zone []sim.Pile
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			zone = append(zone, byTile[cur])
			for _, d := range dirs {
				nb := [2]int{cur[0] + d[0], cur[1] + d[1]}
				if _, ok := byTile[nb]; ok && !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		zones = append(zones, zone)
	}
	return zones
}

// describePileZone renders one pile-content inspection entry: a single pile
// as "pile(x,y) contents", a multi-pile zone as its bounding box + pile
// count. Contents = non-food counts (wood/stone/water/planks/refined stone/
// spears) + food batch totals per kind, matching T021's spec wording and the
// souls pane's carried-inventory phrasing (SC-006 consistency).
func describePileZone(zone []sim.Pile) string {
	contents := summarizePileContents(zone)
	if len(zone) == 1 {
		return fmt.Sprintf("pile(%d,%d) %s", zone[0].X, zone[0].Y, contents)
	}
	minX, minY, maxX, maxY := zone[0].X, zone[0].Y, zone[0].X, zone[0].Y
	for _, p := range zone[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return fmt.Sprintf("zone[%d](%d,%d)-(%d,%d) %s", len(zone), minX, minY, maxX, maxY, contents)
}

// summarizePileContents aggregates one or more piles' contents into the
// same "non-food counts + food batch totals" shape T021 calls for: raw
// resource counts, a spear count, and the food triplet raw/cooked/meals
// (batch totals, deadlines omitted — this is a contents summary, not a rot
// countdown).
func summarizePileContents(piles []sim.Pile) string {
	var wood, stone, water, planks, refined, spears int
	var foodRaw, foodCooked, foodMeals int
	for _, p := range piles {
		wood += p.Wood
		stone += p.Stone
		water += p.Water
		planks += p.Planks
		refined += p.RefinedStone
		spears += len(p.Spears)
		for _, f := range p.Food {
			switch f.Kind {
			case "food_raw":
				foodRaw += f.N
			case "food_cooked":
				foodCooked += f.N
			case "meals":
				foodMeals += f.N
			}
		}
	}
	var parts []string
	if wood > 0 {
		parts = append(parts, fmt.Sprintf("%dw", wood))
	}
	if stone > 0 {
		parts = append(parts, fmt.Sprintf("%dst", stone))
	}
	if water > 0 {
		parts = append(parts, fmt.Sprintf("%dwt", water))
	}
	if planks > 0 {
		parts = append(parts, fmt.Sprintf("%dpl", planks))
	}
	if refined > 0 {
		parts = append(parts, fmt.Sprintf("%drs", refined))
	}
	if spears > 0 {
		parts = append(parts, fmt.Sprintf("%dspear", spears))
	}
	if foodRaw+foodCooked+foodMeals > 0 {
		parts = append(parts, fmt.Sprintf("food %dr/%dc/%dm", foodRaw, foodCooked, foodMeals))
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, " ")
}

// Terrain glyphs. Night dims the palette rather than hiding the world.
var (
	styleWater    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleTree     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleForage   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleRock     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleDepleted = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))
	styleDen      = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleFire     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
	styleFireCold = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))
	styleShelter  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("130"))
	styleOven     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("166"))
	// Pile (spec 013 US2): "%" is the roguelike convention for a ground
	// item/goods stash, distinct from every existing glyph; tan/gold (178)
	// reads as "cache" without colliding with fire's orange (208), oven's
	// burnt orange (166), or shelter's brown (130).
	stylePile = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("178"))
	// Chest (spec 013 US3): "☐" (empty box) reads as a container distinct
	// from every existing glyph — unlike a pile's loose "%", a chest is a
	// built structure with a lid. Dark goldenrod (136) sits between pile's
	// tan (178) and shelter's brown (130) without matching either, so a
	// chest never gets mistaken for a stockpile or a house at a glance.
	styleChest = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("136"))
	styleGru   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

// mapView is the narrow-fallback map pane: today's vw/vh formula,
// unchanged (pages/solo-views.md "Narrow fallback" — "today's single-pane
// UI renders unchanged").
func (m Model) mapView() string {
	vw, vh := 32, 18
	if m.width > 8 {
		if w := (m.width - 6) / 2; w < vw || m.width >= 80 {
			vw = w
		}
	}
	if m.height > 12 {
		vh = m.height - 10
	}
	grid, legend := m.renderMapGrid(vw, vh)
	return styleBox.Render(grid) + "\n" + legend
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clipLine crops a single line (ANSI-safe, via lipgloss.Style.MaxWidth) to
// at most width visible columns; a line that already fits is returned
// unchanged (MaxWidth alone would pad it, which clipContent doesn't want).
func clipLine(s string, width int) string {
	if width < 1 {
		width = 1
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// clipContent crops every line of a multi-line block to fit inside a
// styleBox/stylePanelFocus-family panel whose Width() is set to boxWidth —
// B1. Two lipgloss facts combine into a bug otherwise: (1) Height() only
// *pads* short content, it never truncates tall content, so one overlong
// line silently grows the whole panel past its row budget instead of
// erroring; (2) a style's own Padding(0,1) eats 2 of boxWidth's columns
// *before* text renders, so the true usable width is boxWidth-2, not
// boxWidth. A style-level .MaxWidth() does not reliably substitute for
// this: empirically (see TASK-34 investigation notes), MaxWidth combined
// with Height on multi-line content whose line count already meets the
// Height budget can still double-wrap every line instead of cropping —
// pre-clipping each line before Render() is the only combination that
// held up under test. Callers pass the same boxWidth given to .Width().
func clipContent(content string, boxWidth int) string {
	usable := boxWidth - 2 // Padding(0,1)
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = clipLine(l, usable)
	}
	return strings.Join(lines, "\n")
}

// --- chronicle (panels/chronicle.md, patterns/chronicle-grammar.md) ---
// One body renderer shared by the narrow pane, the dock tab, and the solo
// view — differing only in (width, height, maxWrap).

func (m Model) chronicleFilterHint() string {
	agentName := "all"
	if m.replica != nil && m.chronAgent >= 0 && m.chronAgent < len(m.replica.Agents) {
		agentName = m.replica.Agents[m.chronAgent].Name
	}
	thread := m.chronThread
	if thread == "" {
		thread = "all"
	}
	return fmt.Sprintf("agent %s · thread %s · a/t filter, r raw feed", agentName, thread)
}

// chronicleBody dispatches to inspect / narrated / raw per panels/chronicle.md.
func (m Model) chronicleBody(width, height, maxWrap int) string {
	if m.inspecting() {
		return m.chronicleInspectBody(width, height)
	}
	narrated := m.replica != nil && len(m.replica.Chronicle) > 0
	if !m.chronRaw && narrated {
		return m.chronicleNarratedBody(width, height)
	}
	return m.chronicleRawBody(width, height, maxWrap)
}

// chronicleNarratedBody is TASK-11's narrated feed — content unchanged.
func (m Model) chronicleNarratedBody(width, rows int) string {
	header := styleDim.Render(m.chronicleFilterHint())
	var lines []string
	for _, c := range m.replica.Chronicle {
		if m.chronAgent >= 0 && !c.Mentions(m.chronAgent) {
			continue
		}
		if m.chronThread != "" && c.Thread != m.chronThread {
			continue
		}
		stamp := fmt.Sprintf("day %d", c.Day)
		if c.Thread != "" {
			stamp += " · " + c.Thread
		}
		lines = append(lines, styleDim.Render(stamp)+" "+chronNames(m.replica, c))
		lines = append(lines, wrapText(c.Text, width)...)
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		return header + "\n\n" + styleDim.Render("no entries match these filters yet")
	}
	// B1/B5: `rows` is this body's *entire* row budget, but header+blank
	// above already spend 2 of it — reserve those before capping the
	// entry list, or the returned string can run 2 lines over budget.
	entryRows := rows - 2
	if entryRows < 3 {
		entryRows = 3
	}
	if len(lines) > entryRows {
		lines = lines[len(lines)-entryRows:]
	}
	return header + "\n\n" + strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// chronicleRawBody is the raw event feed formatted by the chronicle
// grammar (patterns/chronicle-grammar.md), auto-following the tail.
func (m Model) chronicleRawBody(width, rows, maxWrap int) string {
	narrated := m.replica != nil && len(m.replica.Chronicle) > 0
	hint := "raw feed · no narrated entries yet — the narrator writes at day and night boundaries"
	if narrated {
		hint = "raw feed · r narrated view"
	}
	if len(m.events) == 0 {
		return styleDim.Render(hint) + "\n\n" +
			styleDim.Render("no events yet this session — the chronicle fills as the world moves")
	}
	names := m.agentNames()
	var out []string
	for _, e := range m.events {
		l := formatChronicleLine(e, names)
		out = append(out, renderChronicleRow(l, width, maxWrap, false))
	}
	all := strings.Split(strings.Join(out, "\n"), "\n")
	// B1/B5: `rows` is this body's *entire* row budget; hint+blank above
	// already spend 2 of it (see chronicleNarratedBody).
	entryRows := rows - 2
	if entryRows < 3 {
		entryRows = 3
	}
	if len(all) > entryRows {
		all = all[len(all)-entryRows:]
	}
	return styleDim.Render(hint) + "\n\n" + strings.Join(all, "\n")
}

// chronicleInspectBody is Mode 2 (paused) — selection, expansion, the
// stored event verbatim (patterns/chronicle-grammar.md "Inspector").
// chronicleInspectBody windows the raw feed around the selection, bounded
// to exactly `rows` total lines whether or not something is expanded (B2 /
// B1: the expansion block's line count is reserved out of the row budget
// *before* windowing the marker rows, so an expanded event can never push
// the composite past its handed height — which was the actual cause of
// "j/k looks like a no-op while expanded": the selection moved correctly,
// but an unbounded expansion could grow the panel past the terminal's
// visible rows, scrolling the moved marker out of view).
func (m Model) chronicleInspectBody(width, rows int) string {
	if len(m.events) == 0 {
		return styleDim.Render("paused — no events recorded yet")
	}
	if rows < 3 {
		rows = 3
	}
	names := m.agentNames()
	n := len(m.events)
	sel := m.chronSelectionBase()

	baseBudget := rows
	var expBlock string
	if m.chronExpanded && m.chronExpIdx >= 0 && m.chronExpIdx < n {
		expBlock = indentBlock(formatInspector(m.events[m.chronExpIdx], names), "  ")
		expLines := len(strings.Split(expBlock, "\n"))
		baseBudget = rows - expLines
		if baseBudget < 1 {
			baseBudget = 1
		}
	}

	start := sel - baseBudget/2
	if start < 0 {
		start = 0
	}
	end := start + baseBudget
	if end > n {
		end = n
		start = end - baseBudget
		if start < 0 {
			start = 0
		}
	}
	var out []string
	for i := start; i < end; i++ {
		e := m.events[i]
		l := formatChronicleLine(e, names)
		selected := i == sel
		marker := "  "
		if selected {
			marker = styleFeedSelect.Render("▌") + " "
		}
		out = append(out, marker+renderChronicleRow(l, width-2, 1, selected))
		if m.chronExpanded && m.chronExpIdx == i {
			out = append(out, expBlock)
		}
	}
	return strings.Join(out, "\n")
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = styleDim.Render(prefix) + lines[i]
	}
	return strings.Join(lines, "\n")
}

// renderChronicleRow styles+wraps/truncates one formatted line to width.
func renderChronicleRow(l chronicleLine, width, maxWrap int, selected bool) string {
	plain := plainChronicleLine(l)
	lines := wrapOrTruncatePlain(plain, width, maxWrap)
	style := styleDim
	switch l.Class {
	case classClock:
		style = styleFeedClock
	case classSpeech:
		style = styleFeedSpeech
	}
	if selected {
		style = style.Reverse(true)
	}
	for i, ln := range lines {
		lines[i] = style.Render(ln)
	}
	return strings.Join(lines, "\n")
}

// chronicleView is the narrow-fallback chronicle pane (today's TASK-11
// behavior, header/footer chrome unchanged; body now shares the grammar
// formatter with the dock/solo renderers).
func (m Model) chronicleView() string {
	width := m.width - 4
	if width < 30 {
		width = 30
	}
	rows := m.height - 9
	return m.chronicleBody(width, rows, 1)
}

// chronNames renders an entry's cast, styled like agents elsewhere.
func chronNames(s *sim.State, c sim.ChronicleEntry) string {
	var names []string
	for _, a := range c.Agents {
		if a >= 0 && a < len(s.Agents) {
			names = append(names, s.Agents[a].Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return styleAgent.Render(strings.Join(names, ", "))
}

// nextThread cycles "" → each distinct thread in ring order → "".
func nextThread(s *sim.State, cur string) string {
	if s == nil {
		return ""
	}
	var threads []string
	seen := map[string]bool{}
	for _, c := range s.Chronicle {
		if c.Thread != "" && !seen[c.Thread] {
			seen[c.Thread] = true
			threads = append(threads, c.Thread)
		}
	}
	if len(threads) == 0 {
		return ""
	}
	if cur == "" {
		return threads[0]
	}
	for i, t := range threads {
		if t == cur && i+1 < len(threads) {
			return threads[i+1]
		}
	}
	return ""
}

// wrapText greedy-wraps prose to the given width.
func wrapText(text string, width int) []string {
	var lines []string
	var cur strings.Builder
	for _, w := range strings.Fields(text) {
		if cur.Len() > 0 && cur.Len()+1+len(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

// --- metatron (panels/dock.md "metatron", panels/minibuffer.md) ---

// metatronTranscriptBody is the dock/solo metatron tab: history only —
// input lives in the minibuffer (minibuffer.md).
func (m Model) metatronTranscriptBody(width, rows int) string {
	if rows < 3 {
		rows = 3
	}
	var lines []string
	if len(m.transcript) == 0 && !m.mbBusy {
		lines = append(lines, styleDim.Render("you   ask the angel anything — press m to focus the minibuffer"))
	}
	for _, l := range m.transcript {
		lines = append(lines, transcriptRowLines(l, width)...)
	}
	if m.mbBusy {
		lines = append(lines, styleAgent.Render("angel ⋮ thinking…"))
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:] // newest at bottom; opens scrolled to bottom
	}
	return strings.Join(lines, "\n")
}

// transcriptRowLines renders one stored transcript line as you/angel rows
// (dock.md mockup), wrapping the text to width.
func transcriptRowLines(l string, width int) []string {
	label, text, style := classifyTranscriptLine(l)
	if label == "" {
		return []string{style.Render(l)}
	}
	wrapped := wrapText(text, width-6)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	var out []string
	for i, w := range wrapped {
		prefix := "      "
		if i == 0 {
			prefix = fmt.Sprintf("%-5s ", label)
		}
		out = append(out, styleDim.Render(prefix)+style.Render(w))
	}
	return out
}

func classifyTranscriptLine(l string) (label, text string, style lipgloss.Style) {
	switch {
	case strings.HasPrefix(l, "you: "):
		return "you", strings.TrimPrefix(l, "you: "), lipgloss.NewStyle()
	case strings.HasPrefix(l, "angel: "):
		return "angel", strings.TrimPrefix(l, "angel: "), lipgloss.NewStyle()
	default:
		return "", l, styleAgent
	}
}

// metatronView is the narrow-fallback metatron pane: transcript + the
// focus-contract-governed input line (replaces the old always-typing
// console pane — the exact bug at tui.go:305-309).
func (m Model) metatronView() string {
	width := m.width - 6
	if width < 30 {
		width = 30
	}
	charges := 0
	if m.status != nil {
		charges = m.status.Clock.MetatronCharges
	}
	header := fmt.Sprintf("METATRON · charges %s%s",
		strings.Repeat("⚡", charges), strings.Repeat("·", clampInt(sim.MetatronChargeCap-charges, 0, sim.MetatronChargeCap)))
	if m.consoleCharter != "" {
		header += styleDim.Render(" · " + m.consoleCharter + " (charter.md)")
	}

	body := m.metatronTranscriptBody(width, clampInt(m.height-14, 4, 200))
	if m.mbErr != "" {
		body += "\n" + styleErr.Render("the angel is unreachable: "+m.mbErr)
	}

	content := header + "\n\n" + body
	if m.status != nil && m.status.LLM != nil {
		l := m.status.LLM
		up := func(b bool) string {
			if b {
				return styleAgent.Render("up")
			}
			return styleErr.Render("down")
		}
		content += "\n" + styleDim.Render(fmt.Sprintf("cloud %s %s · spend $%.2f of $%.0f", l.Cloud.Model, up(l.Cloud.Up), l.Spent, l.Budget))
		if l.Spent >= l.Budget {
			content += "\n" + styleErr.Render("budget exhausted — the angel's voice is stilled")
		}
	}
	// Sized to the same content width as the transcript above it (not the
	// full terminal width) — this box nests inside metatronView's own
	// bordered pane, which adds its own chrome on top.
	content += "\n\n" + m.minibufferView(width)
	return styleBox.Render(content)
}

// minibufferView renders the one-line Metatron input at its three states
// (minibuffer.md): dormant, focused (amber border + hint), busy.
func (m Model) minibufferView(width int) string {
	// Total rendered width = inner + 2 (border) — Width()'s own
	// Padding(0,1) eats 2 *more* columns before any text renders, so the
	// true usable text width is inner-2, not inner (B1/B3: this was the
	// off-by-2 that let a long focused input's hint wrap the box to 4
	// rows instead of the fixed 3).
	inner := width - 2
	if inner < 12 {
		inner = 12
	}
	usable := inner - 2
	switch {
	case m.mbFocused:
		hint := "esc release · ⏎ send"
		hintW := lipgloss.Width(hint)
		cursor := "▌"
		// B3: the input text + right-aligned hint must always fit
		// `usable` without wrapping — a wrapped hint silently grows the
		// minibuffer past its fixed 3-row budget (and, combined with
		// B1, is what pushed the header off the top of the terminal).
		// The input display truncates to its visible tail (cursor
		// glued to the right edge, like a normal terminal input line)
		// so the box never needs to wrap; if there's no room for the
		// hint at all, it's dropped rather than ever truncated into
		// illegibility.
		showHint := usable-hintW-1 >= 4
		avail := usable
		if showHint {
			avail = usable - hintW - 1
		}
		left := truncateTail(m.mbInput, avail-lipgloss.Width(cursor)) + cursor
		if !showHint {
			return stylePanelFocus.Width(inner).Render(clipContent(left, inner))
		}
		pad := usable - lipgloss.Width(left) - hintW
		if pad < 1 {
			pad = 1
		}
		line := left + strings.Repeat(" ", pad) + styleDim.Render(hint)
		return stylePanelFocus.Width(inner).Render(clipContent(line, inner))
	case m.mbBusy:
		hint := "esc to background"
		left := "⋮ the angel is answering…"
		pad := usable - lipgloss.Width(left) - lipgloss.Width(hint) - 1
		if pad < 1 {
			pad = 1
		}
		line := styleDim.Render(left) + strings.Repeat(" ", pad) + styleDim.Render(hint)
		return styleBox.Width(inner).Render(clipContent(line, inner))
	case m.mbFlash != "":
		return styleBox.Width(inner).Render(clipContent(styleDim.Render(m.mbFlash), inner))
	default:
		return styleBox.Width(inner).Render(clipContent(styleDim.Render("⏎ m — speak with the angel…"), inner))
	}
}

// truncateTail keeps at most max runes of s, from the end — the visible
// window once a minibuffer input outgrows the display width, cursor glued
// to the right edge (normal terminal input-line behavior).
func truncateTail(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[len(r)-max:])
}

// --- souls (panels/dock.md "souls": "content unchanged", width-aware) ---

func (m Model) soulsView() string {
	body := m.soulsBody(clampInt(m.width-6, 20, 500), clampInt(m.height-6, 4, 500))
	return styleBox.Render(body)
}

// soulsBody renders the roster, dropping the least important column first
// as width narrows (dock.md "souls": "wrap/condense columns; drop the
// least important column first when narrow").
func (m Model) soulsBody(width, height int) string {
	if m.replica == nil || len(m.replica.Agents) == 0 {
		return styleHeader.Render("SOUL READER") + "\n\n" + styleDim.Render("waiting for world state…")
	}
	wide := width >= 40
	var lines []string
	for _, a := range m.replica.Agents {
		status := "awake"
		switch {
		case a.Dead:
			status = styleErr.Render("dead")
		case a.Asleep:
			status = styleAsleep.Render("asleep")
		}
		if wide {
			goal := "idle"
			if a.Intent != nil {
				goal = a.Intent.Goal
			}
			lines = append(lines, fmt.Sprintf("%-8s %s · %s · (%d,%d)", a.Name, status, goal, a.X, a.Y))
			lines = append(lines, styleDim.Render(fmt.Sprintf(
				"         health %s food %s rest %s warmth %s morale %s",
				bar(a.Needs.Health), bar(a.Needs.Food), bar(a.Needs.Rest),
				bar(a.Needs.Warmth), bar(a.Needs.Morale))))
			// Carried inventory (spec 012 T043, SC-006): the full raw/refined
			// surface — wood/stone/water/planks/refined stone, the food
			// triplet, and spear count (with the most-worn spear's
			// remaining uses when at least one is carried — Spears is kept
			// sorted ascending by the reducer, so Spears[0] is the one
			// closest to breaking). Leading bulk n/24 (spec 013 T015,
			// SC-006) answers "how full are this villager's hands" from
			// the TUI alone — sim.Bulk is the same derived-load function
			// the reducer/executor clamp against, so the number never
			// drifts from what a gather/craft/give will actually do.
			carry := fmt.Sprintf("bulk %d/%d · carry %dw %dst %dwt %dpl %drs · food %dr/%dc/%dm",
				sim.Bulk(a.Inv), sim.BulkCap,
				a.Inv.Wood, a.Inv.Stone, a.Inv.Water, a.Inv.Planks, a.Inv.RefinedStone,
				a.Inv.FoodRaw, a.Inv.FoodCooked, a.Inv.Meals)
			if n := len(a.Inv.Spears); n > 0 {
				carry += fmt.Sprintf(" · spear %d(%d)", n, a.Inv.Spears[0])
			}
			lines = append(lines, styleDim.Render("         "+carry))
			lines = append(lines, "")
		} else {
			// Narrow dock width: drop goal/position/memory, keep name + status + health.
			lines = append(lines, fmt.Sprintf("%-8s %s health %s", a.Name, status, bar(a.Needs.Health)))
		}
	}
	// B1/B5: "SOUL READER" + blank above spend 2 of `height`'s budget;
	// drop trailing agents (rather than partial rows) if the roster
	// doesn't fit, the same "shed content, never overflow" rule the
	// chronicle and minibuffer follow.
	budget := height - 2
	if budget < 1 {
		budget = 1
	}
	if len(lines) > budget {
		lines = lines[:budget]
	}
	body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	return styleHeader.Render("SOUL READER") + "\n\n" + body
}

// bar renders a 0..1000 need as a compact five-cell gauge.
func bar(v int) string {
	filled := v / 200
	if v > 0 && filled == 0 {
		filled = 1
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 5-filled)
}

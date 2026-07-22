package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
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

	// Family color roles (contracts/digest-grammar.md §4, TASK-60 Phase 5):
	// applied to the type column for natural-phrase families, and to the
	// whole line for labeled-voice families (clock/cog/daemon — §2). The
	// palette (recorded in patterns/chronicle-grammar.md's Color roles
	// section): clock keeps its existing yellow (contract §4 says so
	// explicitly); the rest are chosen to stay distinguishable from each
	// other and from the name/speech/emphasis/alert roles below.
	styleFamilyWorld      = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))            // blue — foundational/world
	styleFamilySim        = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green — environment (plain, vs. name's bold green)
	styleFamilyAgent      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // cyan — the plurality of events (today's default type color)
	styleFamilySocial     = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))            // magenta — relationships/conversation
	styleFamilyGovernance = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))          // amber — meeting/norm proceedings
	styleFamilyGru        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")) // bold red — predator threat
	styleFamilyChronicle  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))           // bright magenta — the narrator's voice
	styleFamilyMetatron   = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))           // violet — the angel, otherworldly
	styleFamilyCog        = lipgloss.NewStyle().Faint(true)                                // muted — telemetry noise
	// daemon has no distinct tint in data-model.md's token list (process
	// bookkeeping, low salience) — familyTint falls back to styleDim.

	styleFeedEmphasis = lipgloss.NewStyle().Underline(true)                            // amounts/kinds/causes/coords
	styleFeedAlert    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")) // whole-line: died/attacked/chest_taken/violated
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
	case paneVillagers:
		b.WriteString(m.villagersView())
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
		return styleDim.Render("j/k select · J/K scroll detail · space resume · m ask")
	case m.villagersVisible() && m.villDetail:
		return styleDim.Render("esc back · space pause · q quit")
	case m.villagersVisible():
		return styleDim.Render("j/k select · ⏎ inspect · space pause · q quit")
	case isWidescreen(m.width) && m.solo:
		return styleDim.Render(fmt.Sprintf("%s back to map · space resume · q quit", dockTabKey[m.dockTab]))
	case isWidescreen(m.width):
		return styleDim.Render("2 chronicle 3 metatron 4 villagers (again: solo) · m ask · space pause · q quit")
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
			return fmt.Sprintf("%s · %s · paused — j/k select · J/K scroll detail · r narrated", name, mode)
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
		{paneVillagers, "villagers"},
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
	case paneVillagers:
		return m.villagersBody(width, height)
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
	return fmt.Sprintf("chest(%d,%d) [%s] %s %d/%d", ch.X, ch.Y, owner, contents, full, sim.ChestCap)
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

// chronicleRawBody is the raw event feed formatted by the chronicle digest
// grammar (contracts/digest-grammar.md), auto-following the tail. R8:
// window first, then format — only the tail slice of events that could
// possibly land in the visible budget is digested, not the whole 256-event
// ring, so per-frame cost stays O(visible rows) even at max time
// compression (SC-005).
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
	// B1/B5: `rows` is this body's *entire* row budget; hint+blank above
	// already spend 2 of it (see chronicleNarratedBody).
	entryRows := rows - 2
	if entryRows < 3 {
		entryRows = 3
	}
	// Each event contributes at least one physical line, so the tail
	// `entryRows` events are always enough to fill (and, once wrapped,
	// potentially overfill) the budget — the physical-line slice below
	// trims any overshoot from dock-mode wrapping.
	events := m.events
	if len(events) > entryRows {
		events = events[len(events)-entryRows:]
	}
	names := m.agentNames()
	dock := maxWrap > 1
	lines := make([]chronicleLine, len(events))
	for i, e := range events {
		lines[i] = formatChronicleLine(e, names)
	}
	cols := computeChronicleColumns(lines, dock)
	var out []string
	for _, l := range lines {
		out = append(out, renderChronicleRow(l, cols, width, maxWrap, false))
	}
	all := strings.Split(strings.Join(out, "\n"), "\n")
	if len(all) > entryRows {
		all = all[len(all)-entryRows:]
	}
	return styleDim.Render(hint) + "\n\n" + strings.Join(all, "\n")
}

// chronicleInspectBody is Mode 2 (paused) — panels/chronicle.md "Mode 2",
// contracts/digest-grammar.md §5. The body splits into the entry list (top)
// and an always-on detail pane (bottom) separated by a rule line: no
// keypress required to see the selected event's verbatim payload (FR-008,
// R6) — the ⏎-triggered inline inspector this replaced is gone (R7).
// Bounded to exactly `rows` total lines regardless of payload size (B1/B2):
// the pane's row budget is reserved *before* windowing the list, and the
// pane itself windows the annotated payload by chronDetailScroll rather
// than ever emitting it in full — the actual cause of the old inline
// inspector's unbounded-growth bug (see the historical comment this
// replaced) for oversized payloads like world.migrated (FR-011).
func (m Model) chronicleInspectBody(width, rows int) string {
	if len(m.events) == 0 {
		return styleDim.Render("paused — no events recorded yet")
	}
	// Minimum viable split: list(5) + rule(1) — the floor every other body
	// in this package clamps to (B5); paneRows may shrink to 0 below this
	// only in a starved-terminal degenerate case.
	if rows < 6 {
		rows = 6
	}
	names := m.agentNames()
	n := len(m.events)
	sel := m.chronSelectionBase()

	// R6: paneRows = min(rows/2, 14); list keeps the remainder, floored at 5.
	paneRows := rows / 2
	if paneRows > 14 {
		paneRows = 14
	}
	const ruleRows = 1
	listRows := rows - paneRows - ruleRows
	if listRows < 5 {
		listRows = 5
		paneRows = rows - listRows - ruleRows
		if paneRows < 0 {
			paneRows = 0
		}
	}

	// --- entry list (unchanged windowing discipline, minus expansion) ---
	start := sel - listRows/2
	if start < 0 {
		start = 0
	}
	end := start + listRows
	if end > n {
		end = n
		start = end - listRows
		if start < 0 {
			start = 0
		}
	}
	lines := make([]chronicleLine, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, formatChronicleLine(m.events[i], names))
	}
	cols := computeChronicleColumns(lines, false) // inspect is always tick-shown (solo-style)
	listOut := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		l := lines[i-start]
		selected := i == sel
		marker := "  "
		if selected {
			marker = styleFeedSelect.Render("▌") + " "
		}
		listOut = append(listOut, marker+renderChronicleRow(l, cols, width-2, 1, selected))
	}

	// --- rule + detail pane (contract §5) ---
	e := m.events[sel]
	rule := styleDim.Render(fmt.Sprintf("DETAIL · seq %d", e.Seq))
	out := append([]string{}, listOut...)
	out = append(out, rule)
	if paneRows > 0 {
		out = append(out, chronicleDetailPane(e, names, m.chronDetailScroll, width, paneRows)...)
	}
	return strings.Join(out, "\n")
}

// chronicleDetailPane windows formatInspector's verbatim-payload output to
// exactly paneRows lines (contract §5): scroll clamps to content so J past
// the end (or K before the start) is a no-op rather than drifting the view
// blank; a footer replaces the last row with a remaining-line count plus
// the [future: actions] slot (FR-009) whenever content overflows the pane.
// Oversized payloads (world.migrated) are never processed beyond this
// slice — the annotated string is built once, then only the visible lines
// are touched, satisfying FR-011 structurally rather than by a size cap.
func chronicleDetailPane(e store.Event, names []string, scroll, width, paneRows int) []string {
	content := strings.Split(formatInspector(e, names), "\n")

	contentRows := paneRows
	footerNeeded := len(content) > paneRows
	if footerNeeded {
		contentRows = paneRows - 1
		if contentRows < 1 {
			contentRows = 1
		}
	}
	maxScroll := len(content) - contentRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	visEnd := scroll + contentRows
	if visEnd > len(content) {
		visEnd = len(content)
	}

	out := make([]string, 0, paneRows)
	for _, ln := range content[scroll:visEnd] {
		out = append(out, indentBlock(ln, "  "))
	}
	if footerNeeded {
		remaining := len(content) - visEnd
		footer := fmt.Sprintf("… (+%d more — J to scroll)", remaining)
		actions := "[future: actions]" // detailActions' attachment slot (FR-009)
		gap := width - len([]rune(footer)) - len([]rune(actions)) - 4
		if gap < 2 {
			gap = 2
		}
		out = append(out, styleDim.Render("  "+footer+strings.Repeat(" ", gap)+actions))
	}
	for len(out) < paneRows { // pad so the composite height is fixed (B1)
		out = append(out, "")
	}
	return out
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = styleDim.Render(prefix) + lines[i]
	}
	return strings.Join(lines, "\n")
}

// familyTint resolves a family to its color-role token (contract §4).
// Roles, never raw colors, at the call site — this is the one place a
// family maps to an actual lipgloss.Style.
func familyTint(f eventFamily) lipgloss.Style {
	switch f {
	case familyWorld:
		return styleFamilyWorld
	case familyClock:
		return styleFeedClock // existing token; contract §4: "clock keeps yellow"
	case familySim:
		return styleFamilySim
	case familyAgent:
		return styleFamilyAgent
	case familySocial:
		return styleFamilySocial
	case familyGovernance:
		return styleFamilyGovernance
	case familyGru:
		return styleFamilyGru
	case familyChronicle:
		return styleFamilyChronicle
	case familyMetatron:
		return styleFamilyMetatron
	case familyCog:
		return styleFamilyCog
	default: // familyDaemon, familyUnknown — no distinct tint (see token block)
		return styleDim
	}
}

// styleForRole maps one styled rune's paint-time role to a style, given the
// row's family tint (used for styleRoleFamily — the prefix — since it's the
// one role whose color varies per line rather than being fixed).
func styleForRole(role styleRole, fam lipgloss.Style) lipgloss.Style {
	switch role {
	case styleRoleFamily:
		return fam
	case styleRoleName:
		return styleFeedName
	case styleRoleSpeech:
		return styleFeedSpeech
	case styleRoleEmphasis:
		return styleFeedEmphasis
	default:
		return lipgloss.NewStyle() // default terminal foreground
	}
}

// paintStyledLine renders one already-wrapped/truncated styledLine
// (grammar.go's styleWrapLine) by walking its per-rune roles and emitting
// one Render() call per contiguous same-role run — R4's "style segment-wise
// after wrap": the wrapping/truncation that produced l already happened on
// plain runes, so this can never split an ANSI escape.
func paintStyledLine(l styledLine, fam lipgloss.Style, selected bool) string {
	var b strings.Builder
	i := 0
	for i < len(l.Runes) {
		role := styleRoleText
		if i < len(l.Roles) {
			role = l.Roles[i]
		}
		j := i + 1
		for j < len(l.Runes) {
			r := styleRoleText
			if j < len(l.Roles) {
				r = l.Roles[j]
			}
			if r != role {
				break
			}
			j++
		}
		st := styleForRole(role, fam)
		if selected {
			st = st.Reverse(true)
		}
		b.WriteString(st.Render(string(l.Runes[i:j])))
		i = j
	}
	return b.String()
}

// renderChronicleRow styles+wraps/truncates one formatted line to width,
// given the shared window's column layout (R5) — contract §2/§4/T021:
//   - alert types (agent.died, gru.attacked, social.chest_taken,
//     norm.violated) render the whole line in the alert role, regardless
//     of family, so they pop without reading.
//   - labeled-voice families (cog, clock, daemon) tint the whole line with
//     the family color — the summary IS already "key=value", no further
//     per-segment treatment applies.
//   - every other family tints only the type column, and the summary
//     renders segment-wise (name/speech/emphasis roles pop against
//     default-color connective prose) via styleWrapLine + paintStyledLine.
//
// Selection reverse is preserved in all three paths.
func renderChronicleRow(l chronicleLine, cols chronicleColumns, width, maxWrap int, selected bool) string {
	if isAlertType(l.Type) {
		return styleWholeLine(plainChronicleLine(l, cols), width, maxWrap, styleFeedAlert, selected)
	}
	if isLabeledVoiceFamily(l.Family) {
		return styleWholeLine(plainChronicleLine(l, cols), width, maxWrap, familyTint(l.Family), selected)
	}
	prefix := chronicleLinePrefix(l, cols)
	fam := familyTint(l.Family)
	styledLines := styleWrapLine(prefix, l.Summary, width, maxWrap)
	out := make([]string, len(styledLines))
	for i, sl := range styledLines {
		out[i] = paintStyledLine(sl, fam, selected)
	}
	return strings.Join(out, "\n")
}

// styleWholeLine wraps/truncates the plain line then renders every
// physical line with one uniform style — the alert and labeled-voice paths
// don't need per-segment attribution (contract §2).
func styleWholeLine(plain string, width, maxWrap int, style lipgloss.Style, selected bool) string {
	lines := wrapOrTruncatePlain(plain, width, maxWrap)
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

// --- villagers (panels/dock.md "Tab: villagers"; TASK-56 roster + per-
// villager detail, width- and height-aware) ---

// villagersView is the narrow-fallback pane — same body renderer the dock
// tab and solo view share (dockTabContent), boxed like every narrow pane.
func (m Model) villagersView() string {
	body := m.villagersBody(clampInt(m.width-6, 20, 500), clampInt(m.height-6, 4, 500))
	return styleBox.Render(body)
}

// villagersBody dispatches to the roster or the selected villager's detail
// view (data-model.md "New TUI model state": villDetail).
func (m Model) villagersBody(width, height int) string {
	if m.replica == nil || len(m.replica.Agents) == 0 {
		return styleHeader.Render("VILLAGERS") + "\n\n" + styleDim.Render("waiting for world state…")
	}
	if m.villDetail {
		return m.villagerDetailBody(width, height)
	}
	return m.villagerRosterBody(width, height)
}

// villagerRosterBody renders the roster with a selection cursor, dropping
// the least important column first as width narrows (dock.md "wrap/condense
// columns; drop the least important column first when narrow") — the same
// columns and drop-trailing-agents height rule as before TASK-56, plus the
// cursor glyph on the selected row.
func (m Model) villagerRosterBody(width, height int) string {
	sel := clampInt(m.villSelected, 0, len(m.replica.Agents)-1)
	wide := width >= 40
	var lines []string
	for i, a := range m.replica.Agents {
		cursor := "  "
		if i == sel {
			cursor = styleFeedSelect.Render("▌") + " "
		}
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
			lines = append(lines, cursor+fmt.Sprintf("%-8s %s · %s · (%d,%d)", a.Name, status, goal, a.X, a.Y))
			lines = append(lines, styleDim.Render(fmt.Sprintf(
				"           health %s food %s rest %s warmth %s morale %s",
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
			lines = append(lines, styleDim.Render("           "+carry))
			lines = append(lines, "")
		} else {
			// Narrow dock width: drop goal/position/memory, keep cursor + name + status + health.
			lines = append(lines, cursor+fmt.Sprintf("%-8s %s health %s", a.Name, status, bar(a.Needs.Health)))
		}
	}
	// B1/B5: "VILLAGERS" + blank above spend 2 of `height`'s budget;
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
	return styleHeader.Render("VILLAGERS") + "\n\n" + body
}

// villagerDetailBody renders the selected villager within the given
// (width, height) budget. Sections in fixed priority order
// (contracts/state-and-keys.md "Rendering contract"): identity/vitals →
// objective → inventory → beliefs/narrative → memories (most recent
// first). The section list truncates from the bottom — memories shed
// first — so identity/objective/inventory are never pushed off-screen
// (spec "Very short pane height" edge case); every line is width-clipped
// so a long belief/memory/narrative line can never push the panel past its
// column budget either (SC-004).
func (m Model) villagerDetailBody(width, height int) string {
	if height < 1 {
		height = 1
	}
	sel := clampInt(m.villSelected, 0, len(m.replica.Agents)-1)
	a := m.replica.Agents[sel]
	wide := width >= 40

	lines := []string{strings.ToUpper(a.Name), ""}
	lines = append(lines, strings.Split(villagerIdentitySection(a, wide), "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(villagerObjectiveSection(a), "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(villagerInventorySection(a), "\n")...)
	if s := villagerBeliefsSection(a); s != "" {
		lines = append(lines, "")
		lines = append(lines, strings.Split(s, "\n")...)
	}
	lines[0] = styleHeader.Render(lines[0])

	if len(lines) > height {
		// Pathological height: the fixed sections themselves don't fit.
		// Shed from the bottom like everywhere else rather than ever
		// emitting more than `height` lines.
		lines = lines[:height]
	} else if remaining := height - len(lines); remaining > 1 {
		if mem := villagerMemoriesLines(a, remaining-1); len(mem) > 0 { // -1: blank separator
			lines = append(lines, "")
			lines = append(lines, mem...)
		}
	}

	for i, l := range lines {
		lines[i] = clipLine(l, width)
	}
	return strings.Join(lines, "\n")
}

// villagerIdentitySection is FR-003: name, awake/asleep/dead status,
// position, and needs.
func villagerIdentitySection(a sim.Agent, wide bool) string {
	status := "awake"
	switch {
	case a.Dead:
		status = styleErr.Render("dead")
	case a.Asleep:
		status = styleAsleep.Render("asleep")
	}
	line := fmt.Sprintf("%s · %s · (%d,%d)", a.Name, status, a.X, a.Y)
	needs := fmt.Sprintf("health %s food %s rest %s warmth %s morale %s",
		bar(a.Needs.Health), bar(a.Needs.Food), bar(a.Needs.Rest), bar(a.Needs.Warmth), bar(a.Needs.Morale))
	if !wide {
		return line + "\n" + needs
	}
	return line + "\n" + styleDim.Render(needs)
}

// villagerObjectiveSection is FR-005/FR-006 and US2's three display states
// (data-model.md "Derived display state: objective"): active (Intent !=
// nil), past (LastGoal survives Intent clearing, marked "last"), or "no
// objective yet" when neither has ever been set.
func villagerObjectiveSection(a sim.Agent) string {
	switch {
	case a.Intent != nil:
		return fmt.Sprintf("objective: %s → (%d,%d) (current)", a.Intent.Goal, a.Intent.TargetX, a.Intent.TargetY)
	case a.LastGoal != "":
		return fmt.Sprintf("objective: %s (last, %s)", a.LastGoal, clock.Format(a.LastGoalTick))
	default:
		return "objective: no objective yet"
	}
}

// villagerInventorySection is FR-004: every carried kind itemized with
// counts (spear wear included); empty kinds omitted; an entirely empty
// pack stated plainly rather than rendering nothing.
func villagerInventorySection(a sim.Agent) string {
	var items []string
	add := func(label string, n int) {
		if n > 0 {
			items = append(items, fmt.Sprintf("%s %d", label, n))
		}
	}
	add("wood", a.Inv.Wood)
	add("stone", a.Inv.Stone)
	add("water", a.Inv.Water)
	add("planks", a.Inv.Planks)
	add("refined stone", a.Inv.RefinedStone)
	add("raw food", a.Inv.FoodRaw)
	add("cooked food", a.Inv.FoodCooked)
	add("meals", a.Inv.Meals)
	if n := len(a.Inv.Spears); n > 0 {
		wear := make([]string, n)
		for i, w := range a.Inv.Spears {
			wear[i] = fmt.Sprintf("%d", w)
		}
		items = append(items, fmt.Sprintf("spears %d (uses left: %s)", n, strings.Join(wear, ",")))
	}
	if len(items) == 0 {
		return "inventory: empty pack"
	}
	return "inventory:\n  " + strings.Join(items, "\n  ")
}

// villagerBeliefsSection is FR-008: consolidated beliefs and narrative,
// shown only when present (empty string omits the section silently).
func villagerBeliefsSection(a sim.Agent) string {
	if len(a.Beliefs) == 0 && a.Narrative == "" {
		return ""
	}
	var b strings.Builder
	if a.Narrative != "" {
		b.WriteString("narrative: " + a.Narrative)
	}
	if len(a.Beliefs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("beliefs:")
		for _, belief := range a.Beliefs {
			b.WriteString(fmt.Sprintf("\n  %s (%d%%)", belief.Statement, belief.Confidence))
		}
	}
	return b.String()
}

// villagerMemoriesLines is FR-007: episodic memories most-recent-first
// (Memories accretes oldest-first, so this walks it in reverse),
// bounded to at most `budget` lines total including its own header — the
// section this detail view sheds first when height runs short. budget < 1
// omits the section entirely; a villager with no memories yet says so
// plainly rather than showing nothing.
func villagerMemoriesLines(a sim.Agent, budget int) []string {
	if budget < 1 {
		return nil
	}
	if len(a.Memories) == 0 {
		return []string{styleHeader.Render("memories") + " " + styleDim.Render("· no memories yet")}
	}
	lines := []string{styleHeader.Render("memories")}
	for i := len(a.Memories) - 1; i >= 0 && len(lines) < budget; i-- {
		lines = append(lines, sim.FormatMemory(a.Memories[i]))
	}
	return lines
}

// bar renders a 0..1000 need as a compact five-cell gauge.
func bar(v int) string {
	filled := v / 200
	if v > 0 && filled == 0 {
		filled = 1
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 5-filled)
}

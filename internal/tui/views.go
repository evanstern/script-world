package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
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
)

func (m Model) View() string {
	if m.quitting {
		return "detached (the world keeps running)\n"
	}
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

// Terrain glyphs. Night dims the palette rather than hiding the world.
var (
	styleWater   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleTree    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleForage  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDen     = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleFire    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
	styleShelter = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("130"))
)

// mapView renders a camera window over the generated terrain with the live
// replica's wanderers on top. The camera follows the wanderer centroid;
// arrow keys pan (panX/panY), 'c' recenters.
func (m Model) mapView() string {
	gm := m.gameMap
	if gm == nil {
		return styleDim.Render("no terrain (world manifest missing?)")
	}

	// Viewport size from the terminal, 2 columns per tile.
	vw, vh := 32, 18
	if m.width > 8 {
		if w := (m.width - 6) / 2; w < vw || m.width >= 80 {
			vw = w
		}
	}
	if m.height > 12 {
		vh = m.height - 10
	}
	if vw > gm.W {
		vw = gm.W
	}
	if vh > gm.H {
		vh = gm.H
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
	if m.replica != nil {
		for _, st := range m.replica.Structures {
			switch st.Kind {
			case "fire":
				structures[[2]int{st.X, st.Y}] = styleFire.Render("▲")
			case "shelter":
				structures[[2]int{st.X, st.Y}] = styleShelter.Render("⌂")
			}
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

	night := m.replica != nil && m.replica.Night
	tile := func(x, y int) string {
		if g, ok := agents[[2]int{x, y}]; ok {
			return g
		}
		if g, ok := structures[[2]int{x, y}]; ok {
			return g
		}
		if dens[[2]int{x, y}] {
			return styleDen.Render("ᴥ")
		}
		var s string
		var st lipgloss.Style
		switch gm.At(x, y) {
		case worldmap.Water:
			s, st = "~", styleWater
		case worldmap.Tree:
			s, st = "♠", styleTree
		case worldmap.Forage:
			s, st = "\"", styleForage
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
	grid := styleBox.Render(strings.Join(rows, "\n"))

	phase := "day"
	if night {
		phase = styleNight.Render("night")
	}
	legend := styleDim.Render(fmt.Sprintf(
		"%s · [%d,%d–%d,%d of %d×%d] · ~water ♠wood \"forage ᴥden ▲fire ⌂shelter · agents by initial (lowercase asleep, †dead) · arrows pan, c center",
		phase, x0, y0, x0+vw-1, y0+vh-1, gm.W, gm.H))
	return grid + "\n" + legend
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

// chronicleView is the raw event feed until TASK-11 narrates it.
func (m Model) chronicleView() string {
	if len(m.events) == 0 {
		return styleDim.Render("no events yet this session — the chronicle fills as the world moves")
	}
	rows := m.height - 8
	if rows < 5 {
		rows = 5
	}
	start := len(m.events) - rows
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	for _, e := range m.events[start:] {
		b.WriteString(eventRow(e) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func eventRow(e store.Event) string {
	return fmt.Sprintf("%s %s  %-18s %s",
		styleDim.Render(fmt.Sprintf("#%-7d", e.Seq)),
		clock.Format(e.Tick), e.Type, styleDim.Render(string(e.Payload)))
}

func (m Model) metatronView() string {
	lines := []string{
		"METATRON CONSOLE",
		"",
		"The angel has not yet been summoned.",
		"",
		styleDim.Render("Conversation with Metatron — judging your intents, shaping"),
		styleDim.Render("dreams and omens — arrives with TASK-12. Its charter will be"),
		styleDim.Render("the only player-editable prompt in the game."),
	}
	if m.status != nil && m.status.LLM != nil {
		l := m.status.LLM
		up := func(b bool) string {
			if b {
				return styleAgent.Render("up")
			}
			return styleErr.Render("down")
		}
		lines = append(lines, "",
			"THE ANGEL'S VOICE (llm orchestrator)",
			fmt.Sprintf("  local  %-14s %s · queue %d", l.Local.Model, up(l.Local.Up), l.Local.Queue),
			fmt.Sprintf("  cloud  %-14s %s · queue %d", l.Cloud.Model, up(l.Cloud.Up), l.Cloud.Queue),
			fmt.Sprintf("  spend  $%.2f of $%.0f (%s)", l.Spent, l.Budget, l.Month),
		)
		if l.Spent >= l.Budget {
			lines = append(lines, styleErr.Render("  budget exhausted — cloud calls refused"))
		}
	}
	return styleBox.Render(strings.Join(lines, "\n"))
}

func (m Model) soulsView() string {
	var b strings.Builder
	b.WriteString("SOUL READER\n\n")
	if m.replica == nil || len(m.replica.Agents) == 0 {
		b.WriteString(styleDim.Render("waiting for world state…"))
		return styleBox.Render(strings.TrimRight(b.String(), "\n"))
	}
	for _, a := range m.replica.Agents {
		status := "awake"
		switch {
		case a.Dead:
			status = styleErr.Render("dead")
		case a.Asleep:
			status = styleAsleep.Render("asleep")
		}
		goal := "idle"
		if a.Intent != nil {
			goal = a.Intent.Goal
		}
		b.WriteString(fmt.Sprintf("%-8s %s · %s · (%d,%d)\n",
			a.Name, status, goal, a.X, a.Y))
		b.WriteString(styleDim.Render(fmt.Sprintf(
			"         health %s food %s rest %s warmth %s morale %s · wood %d, meals %d",
			bar(a.Needs.Health), bar(a.Needs.Food), bar(a.Needs.Rest),
			bar(a.Needs.Warmth), bar(a.Needs.Morale), a.Inv.Wood, a.Inv.Food)) + "\n\n")
	}
	b.WriteString(styleDim.Render("bodies only — persona.md / soul.md arrive with TASK-7"))
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

// bar renders a 0..1000 need as a compact five-cell gauge.
func bar(v int) string {
	filled := v / 200
	if v > 0 && filled == 0 {
		filled = 1
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 5-filled)
}

func (m Model) footerView() string {
	return styleDim.Render("1-4/tab panes · space pause/resume · [ ] speed · q detach")
}

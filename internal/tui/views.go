package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/sim"
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
	styleGru     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
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

// chronicleView (TASK-11) renders the narrated story from the replica's
// chronicle ring — snapshot-carried, so an attaching client reads days of
// history immediately (the catch-up mechanism). 'a'/'t' filter by agent and
// thread; 'r' (or a world with no narrated entries yet) shows the raw feed.
func (m Model) chronicleView() string {
	narrated := m.replica != nil && len(m.replica.Chronicle) > 0
	if m.chronRaw || !narrated {
		return m.rawFeedView(narrated)
	}

	agentName := "all"
	if m.chronAgent >= 0 && m.chronAgent < len(m.replica.Agents) {
		agentName = m.replica.Agents[m.chronAgent].Name
	}
	thread := m.chronThread
	if thread == "" {
		thread = "all"
	}
	header := styleDim.Render(fmt.Sprintf(
		"agent %s · thread %s · a/t filter, r raw feed", agentName, thread))

	width := m.width - 4
	if width < 30 {
		width = 30
	}
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
	rows := m.height - 9
	if rows < 5 {
		rows = 5
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	return header + "\n\n" + strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// rawFeedView is the unfiltered event stream — the pre-narrator chronicle,
// kept as the fallback for worlds without a narrator and as the 'r' toggle.
func (m Model) rawFeedView(narrated bool) string {
	hint := "raw feed · no narrated entries yet — the narrator writes at day and night boundaries"
	if narrated {
		hint = "raw feed · r narrated view"
	}
	if len(m.events) == 0 {
		return styleDim.Render(hint) + "\n\n" +
			styleDim.Render("no events yet this session — the chronicle fills as the world moves")
	}
	rows := m.height - 10
	if rows < 5 {
		rows = 5
	}
	start := len(m.events) - rows
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	b.WriteString(styleDim.Render(hint) + "\n\n")
	for _, e := range m.events[start:] {
		b.WriteString(eventRow(e) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
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

func eventRow(e store.Event) string {
	return fmt.Sprintf("%s %s  %-18s %s",
		styleDim.Render(fmt.Sprintf("#%-7d", e.Seq)),
		clock.Format(e.Tick), e.Type, styleDim.Render(string(e.Payload)))
}

// metatronView is the console (TASK-12): transcript + input line, the ⚡
// bank in the header, tier health at the foot.
func (m Model) metatronView() string {
	width := m.width - 6
	if width < 30 {
		width = 30
	}
	charges := 0
	if m.status != nil {
		charges = m.status.Clock.MetatronCharges
	}
	header := fmt.Sprintf("METATRON CONSOLE · charges %s%s",
		strings.Repeat("⚡", charges), strings.Repeat("·", clampInt(sim.MetatronChargeCap-charges, 0, sim.MetatronChargeCap)))
	if m.consoleCharter != "" {
		header += styleDim.Render(" · " + m.consoleCharter + " (charter.md)")
	}

	var lines []string
	if len(m.consoleLines) == 0 {
		lines = append(lines, styleDim.Render("The angel awaits. Type, then Enter. (Its charter — charter.md in the"),
			styleDim.Render("save directory — is the one prompt in this game that is yours to edit.)"))
	}
	for _, l := range m.consoleLines {
		style := styleDim
		if strings.HasPrefix(l, "metatron:") {
			style = lipgloss.NewStyle()
		} else if strings.HasPrefix(l, "⚡") || strings.HasPrefix(l, "!") {
			style = styleAgent
		}
		for _, w := range wrapText(l, width) {
			lines = append(lines, style.Render(w))
		}
	}
	if m.consoleErr != "" {
		lines = append(lines, styleErr.Render("the angel is unreachable: "+m.consoleErr))
	}

	rows := m.height - 14
	if rows < 4 {
		rows = 4
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}

	input := "> " + m.consoleInput + "█"
	if m.consoleBusy {
		input = styleDim.Render("> … the angel considers …")
	}

	body := header + "\n\n" + strings.Join(lines, "\n") + "\n\n" + input
	if m.status != nil && m.status.LLM != nil {
		l := m.status.LLM
		up := func(b bool) string {
			if b {
				return styleAgent.Render("up")
			}
			return styleErr.Render("down")
		}
		body += "\n" + styleDim.Render(fmt.Sprintf("cloud %s %s · spend $%.2f of $%.0f", l.Cloud.Model, up(l.Cloud.Up), l.Spent, l.Budget))
		if l.Spent >= l.Budget {
			body += "\n" + styleErr.Render("budget exhausted — the angel's voice is stilled")
		}
	}
	body += "\n" + styleDim.Render("Enter send · Esc map pane")
	return styleBox.Render(body)
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
		if n := len(a.Memories); n > 0 {
			m := a.Memories[n-1]
			b.WriteString(styleDim.Render(fmt.Sprintf("         ~ %s", sim.FormatMemory(m))) + "\n")
		}
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

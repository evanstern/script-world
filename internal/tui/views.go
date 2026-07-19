package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
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

// mapView renders the village grid from the live replica.
func (m Model) mapView() string {
	if m.replica == nil {
		return styleDim.Render("waiting for world state…")
	}
	glyphs := map[[2]int]string{}
	for i, w := range m.replica.Wanderers {
		g := string(rune('A' + i))
		if w.Asleep {
			g = styleAsleep.Render(strings.ToLower(g))
		} else {
			g = styleAgent.Render(g)
		}
		glyphs[[2]int{w.X, w.Y}] = g
	}
	dot := "·"
	if m.replica.Night {
		dot = styleNight.Render("·")
	}
	var rows []string
	for y := 0; y < sim.GridSize; y++ {
		var row strings.Builder
		for x := 0; x < sim.GridSize; x++ {
			if g, ok := glyphs[[2]int{x, y}]; ok {
				row.WriteString(g + " ")
			} else {
				row.WriteString(dot + " ")
			}
		}
		rows = append(rows, strings.TrimRight(row.String(), " "))
	}
	grid := styleBox.Render(strings.Join(rows, "\n"))

	phase := "day"
	if m.replica.Night {
		phase = styleNight.Render("night")
	}
	legend := styleDim.Render(fmt.Sprintf("%s · A/B wanderers (lowercase = asleep)", phase))
	return grid + "\n" + legend
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
	return styleBox.Render(strings.Join([]string{
		"METATRON CONSOLE",
		"",
		"The angel has not yet been summoned.",
		"",
		styleDim.Render("Conversation with Metatron — judging your intents, shaping"),
		styleDim.Render("dreams and omens — arrives with TASK-12. Its charter will be"),
		styleDim.Render("the only player-editable prompt in the game."),
	}, "\n"))
}

func (m Model) soulsView() string {
	agentsDir := filepath.Join(m.w.Dir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil || len(entries) == 0 {
		return styleBox.Render(strings.Join([]string{
			"SOUL READER",
			"",
			"No souls inhabit this world yet.",
			"",
			styleDim.Render("persona.md / soul.md files appear under agents/ when the"),
			styleDim.Render("agent mind lands (TASK-7). This pane will read them."),
		}, "\n"))
	}
	var b strings.Builder
	b.WriteString("SOUL READER — agents/\n\n")
	for _, e := range entries {
		b.WriteString("  " + e.Name() + "\n")
	}
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) footerView() string {
	return styleDim.Render("1-4/tab panes · space pause/resume · [ ] speed · q detach")
}

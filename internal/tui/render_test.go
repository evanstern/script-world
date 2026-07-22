package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/ipc"
	"github.com/evanstern/script-world/internal/sim"
)

// TestWidescreenViewExactHeight is the B1 regression: the widescreen
// composite must render to EXACTLY m.height lines in every mode layered on
// top of it. Bubble Tea scrolls the top of the frame off-screen when a
// View() overflows the terminal height, which is what pushed the header
// row off-screen in the live tmux repro. Covers home, solo, and
// paused/inspect (both in the dock and solo'd), at several sizes straddling
// the breakpoint and the 50/50 column split.
func TestWidescreenViewExactHeight(t *testing.T) {
	sizes := []struct{ w, h int }{
		{112, 20}, {112, 30}, {113, 30}, {118, 30}, {140, 40}, {160, 50}, {200, 24},
	}
	for _, sz := range sizes {
		for _, state := range []string{"home", "solo", "inspect", "inspect-solo", "villagers-solo", "villagers-detail-solo", "metatron-solo"} {
			t.Run(fmt.Sprintf("%dx%d/%s", sz.w, sz.h, state), func(t *testing.T) {
				m := widescreenModel(t)
				m.width, m.height = sz.w, sz.h
				seedEvents(&m, 20)
				switch state {
				case "solo":
					m.solo = true
				case "inspect":
					m.connected = true
					m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
					m.chronSelected = 5
				case "inspect-solo":
					m.connected = true
					m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
					m.chronSelected = 5
					m.solo = true
				case "villagers-solo":
					m.dockTab = paneVillagers
					m.solo = true
				case "villagers-detail-solo":
					m.dockTab = paneVillagers
					m.solo = true
					m.villDetail = true
					m.replica.Agents[0].Beliefs = []sim.Belief{{Statement: "the fire needs tending", Confidence: 80}}
					m.replica.Agents[0].Narrative = "a long night watching the fire die and reviving it by hand."
					for i := 0; i < 20; i++ {
						m.replica.Agents[0].Memories = append(m.replica.Agents[0].Memories,
							sim.Memory{Text: "chopped wood at the treeline", Salience: 3, Tick: int64(i) * 60})
					}
				case "metatron-solo":
					m.dockTab = paneMetatron
					m.solo = true
					m.transcript = []string{"you: why is Rowan hoarding wood?", "angel: three cold nights, and Ash let the fire die each time."}
				}
				v := m.View()
				lines := strings.Split(v, "\n")
				if len(lines) != sz.h {
					t.Errorf("View() = %d lines, want exactly %d — a taller frame scrolls the header off the top of a real terminal:\n%s",
						len(lines), sz.h, v)
				}
			})
		}
	}
}

// TestWidescreenViewExactHeightWithLongMinibufferInput is B3's regression:
// a long focused-minibuffer input must never wrap the box past its fixed
// 3-row budget.
func TestWidescreenViewExactHeightWithLongMinibufferInput(t *testing.T) {
	m := widescreenModel(t)
	m.width, m.height = 140, 40
	m.mbFocused = true
	m.mbInput = strings.Repeat("why does Rowan keep hoarding wood when the fire needs tending ", 5)
	v := m.View()
	lines := strings.Split(v, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() with a long focused input = %d lines, want exactly %d:\n%s", len(lines), m.height, v)
	}
}

// TestWidescreenViewExactHeightDenseChronicleExpanded is B1+B2+B5 together:
// a chronicle with far more events than fit (600 in a 30-row terminal),
// paused with a mid-ring event expanded — the composite must still render
// to exactly m.height lines, and the map panel (unrelated to how busy the
// dock is) must stay at its budgeted height.
func TestWidescreenViewExactHeightDenseChronicleExpanded(t *testing.T) {
	m := widescreenModel(t)
	m.width, m.height = 112, 30
	seedEvents(&m, 600)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
	m.chronSelected = 300
	m.chronExpanded = true
	m.chronExpIdx = 300

	v := m.View()
	lines := strings.Split(v, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() with 600 events + an expansion = %d lines, want exactly %d:\n%s", len(lines), m.height, v)
	}

	cols := computeColumns(m.width)
	rows := computeRows(m.height)
	mapPanel := m.mapPanelView(cols.MapCols, rows.Body)
	if got := len(strings.Split(mapPanel, "\n")); got != rows.Body {
		t.Errorf("map panel = %d lines while the dock is dense+expanded, want exactly its budgeted %d — "+
			"the dock's content must never bleed into the map's row budget", got, rows.Body)
	}
}

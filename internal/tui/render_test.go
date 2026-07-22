package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
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

// TestWidescreenViewExactHeightDenseChronicle is B1+B2+B5 together (spec
// 018: the always-on detail pane replaces the old ⏎-triggered expansion,
// but the row-budget invariant it guarded is the same one): a chronicle
// with far more events than fit (600 in a 30-row terminal), paused with a
// mid-ring selection — the composite must still render to exactly
// m.height lines, and the map panel (unrelated to how busy the dock is)
// must stay at its budgeted height.
func TestWidescreenViewExactHeightDenseChronicle(t *testing.T) {
	m := widescreenModel(t)
	m.width, m.height = 112, 30
	seedEvents(&m, 600)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
	m.chronSelected = 300

	v := m.View()
	lines := strings.Split(v, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() with 600 events, paused mid-ring = %d lines, want exactly %d:\n%s", len(lines), m.height, v)
	}

	cols := computeColumns(m.width)
	rows := computeRows(m.height)
	mapPanel := m.mapPanelView(cols.MapCols, rows.Body)
	if got := len(strings.Split(mapPanel, "\n")); got != rows.Body {
		t.Errorf("map panel = %d lines while the dock is dense+expanded, want exactly its budgeted %d — "+
			"the dock's content must never bleed into the map's row budget", got, rows.Body)
	}
}

// --- style-role tests (TASK-60 Phase 5, T022) ---
//
// go test's stdout isn't a TTY, so lipgloss's default renderer auto-detects
// no color and Render() becomes a no-op — these tests force a color
// profile for their duration (lipgloss.SetColorProfile is documented as
// existing "mostly for testing purposes") and restore it afterward so
// other tests in the package are unaffected.

func withColorProfile(t *testing.T, p termenv.Profile) {
	t.Helper()
	orig := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(p)
	t.Cleanup(func() { lipgloss.SetColorProfile(orig) })
}

// TestFamilyTintDistinctPerFamily: every family color role (contract §4)
// renders to a distinguishable style — the "roles, never raw colors"
// requirement means each named token must actually differ, not just exist.
func TestFamilyTintDistinctPerFamily(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	families := []eventFamily{
		familyWorld, familyClock, familySim, familyAgent, familySocial,
		familyGovernance, familyGru, familyChronicle, familyMetatron, familyCog,
	}
	seen := map[string]eventFamily{}
	for _, f := range families {
		rendered := familyTint(f).Render("TYPE")
		if !strings.Contains(rendered, "\x1b") {
			t.Errorf("family %v: tint produced no ANSI under a forced color profile: %q", f, rendered)
		}
		if prior, ok := seen[rendered]; ok {
			t.Errorf("family %v renders identically to family %v (%q) — tints must be distinguishable", f, prior, rendered)
		}
		seen[rendered] = f
	}
}

// TestPaintStyledLineRoleMapping: paintStyledLine maps each seg role to its
// documented style token (contract §4: name, speech, emphasis) and the
// prefix to the family tint passed in — role→token mapping, not just "some
// style got applied".
func TestPaintStyledLineRoleMapping(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	summary := []seg{
		{Text: "Ash", Role: segName},
		{Text: " said ", Role: segText},
		{Text: `"hi"`, Role: segSpeech},
		{Text: " x2", Role: segEmphasis},
	}
	lines := styleWrapLine("TYPE  ", summary, 60, 1)
	if len(lines) != 1 {
		t.Fatalf("want 1 styled line, got %d", len(lines))
	}
	out := paintStyledLine(lines[0], styleFamilyAgent, false)

	for _, want := range []string{
		styleFamilyAgent.Render("TYPE  "),
		styleFeedName.Render("Ash"),
		styleFeedSpeech.Render(`"hi"`),
		styleFeedEmphasis.Render(" x2"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("styled output missing expected fragment %q in:\n%q", want, out)
		}
	}
}

// TestRenderChronicleRowAlertWholeLine: the four alert types (contract §2)
// render the entire line in styleFeedAlert regardless of family.
func TestRenderChronicleRowAlertWholeLine(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	e := store.Event{Seq: 1, Tick: 60, Type: "agent.died", Payload: json.RawMessage(`{"agent":0,"cause":"starvation"}`)}
	l := formatChronicleLine(e, []string{"Ash"})
	cols := computeChronicleColumns([]chronicleLine{l}, false)
	got := renderChronicleRow(l, cols, 60, 1, false)
	want := styleFeedAlert.Render(plainChronicleLine(l, cols))
	if got != want {
		t.Errorf("alert row should be styled whole-line with styleFeedAlert:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestRenderChronicleRowLabeledVoiceWholeLine: labeled-voice families
// (cog/clock/daemon, contract §2) tint the whole line, not per-segment.
func TestRenderChronicleRowLabeledVoiceWholeLine(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	e := store.Event{Seq: 1, Tick: 60, Type: "clock.speed_set", Payload: json.RawMessage(`{"speed":"4x"}`)}
	l := formatChronicleLine(e, nil)
	cols := computeChronicleColumns([]chronicleLine{l}, false)
	got := renderChronicleRow(l, cols, 60, 1, false)
	want := styleFeedClock.Render(plainChronicleLine(l, cols))
	if got != want {
		t.Errorf("labeled-voice row should be styled whole-line with the family tint:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestRenderChronicleRowSelectionReverse: selection reverse survives the
// segment-wise styling rework (R4/T021) for both alert and phrase-voice rows.
func TestRenderChronicleRowSelectionReverse(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	cases := []struct {
		typ     string
		payload string
	}{
		{"agent.died", `{"agent":0,"cause":"starvation"}`},   // alert path
		{"clock.speed_set", `{"speed":"4x"}`},                 // labeled-voice path
		{"agent.moved", `{"agent":0,"x":1,"y":1}`},             // phrase-voice, segment-wise path
	}
	for _, c := range cases {
		e := store.Event{Seq: 1, Tick: 60, Type: c.typ, Payload: json.RawMessage(c.payload)}
		l := formatChronicleLine(e, []string{"Ash"})
		cols := computeChronicleColumns([]chronicleLine{l}, false)
		unselected := renderChronicleRow(l, cols, 60, 1, false)
		selected := renderChronicleRow(l, cols, 60, 1, true)
		if selected == unselected {
			t.Errorf("%s: selected row should render differently (reverse video) than unselected", c.typ)
		}
	}
}

// TestPureLayerEmitsNoANSI: the pure formatting layer (grammar.go/digest.go)
// never touches lipgloss (R4) — sweeps the full catalog fixture plus the
// fallback path for any stray ESC byte, regardless of color profile (this
// invariant must hold even when no profile is forced).
func TestPureLayerEmitsNoANSI(t *testing.T) {
	names := []string{"Ash", "Birch", "Cedar", "Rowan"}
	for typ, fx := range catalogFixture {
		e := store.Event{Seq: 1, Tick: 1, Type: typ, Payload: json.RawMessage(fx.payload)}
		l := formatChronicleLine(e, names)
		if strings.Contains(plainSegs(l.Summary), "\x1b") {
			t.Errorf("%s: digest summary contains an ESC byte — the pure layer must never emit ANSI", typ)
		}
		cols := computeChronicleColumns([]chronicleLine{l}, false)
		if strings.Contains(plainChronicleLine(l, cols), "\x1b") {
			t.Errorf("%s: plainChronicleLine contains an ESC byte", typ)
		}
	}
	fallback := store.Event{Seq: 1, Tick: 1, Type: "future.unknown_type", Payload: json.RawMessage(`{"agent":0}`)}
	l := formatChronicleLine(fallback, names)
	if strings.Contains(plainSegs(l.Summary), "\x1b") {
		t.Error("fallback summary contains an ESC byte")
	}
}

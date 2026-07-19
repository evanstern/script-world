package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
)

func testModel(t *testing.T) Model {
	t.Helper()
	w, err := world.Create(t.TempDir()+"/w", "test", 42)
	if err != nil {
		t.Fatal(err)
	}
	m := New(w)
	m.replica = sim.NewState(42)
	m.width, m.height = 80, 30
	return m
}

func key(s string) tea.KeyMsg {
	if s == "tab" {
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestPaneNavigation(t *testing.T) {
	m := testModel(t)
	if m.active != paneMap {
		t.Fatal("default pane must be the map (AC#1)")
	}
	cases := []struct {
		key  string
		want pane
	}{
		{"2", paneChronicle},
		{"3", paneMetatron},
		{"4", paneSouls},
		{"1", paneMap},
		{"tab", paneChronicle},
		{"tab", paneMetatron},
		{"tab", paneSouls},
		{"tab", paneMap}, // wraps
	}
	var mdl tea.Model = m
	for _, c := range cases {
		mdl, _ = mdl.(Model).Update(key(c.key))
		if got := mdl.(Model).active; got != c.want {
			t.Errorf("after %q active = %s, want %s", c.key, paneNames[got], paneNames[c.want])
		}
	}
	// Every pane renders without a live connection (stubs included).
	for i := pane(0); i < paneCount; i++ {
		mm := mdl.(Model)
		mm.active = i
		if v := mm.View(); v == "" {
			t.Errorf("pane %s rendered empty", paneNames[i])
		}
	}
}

func TestMapRendersWanderers(t *testing.T) {
	m := testModel(t)
	m.replica.Wanderers = []sim.Wanderer{{X: 3, Y: 4}, {X: 10, Y: 2, Asleep: true}}
	view := m.mapView()
	if !strings.Contains(view, "A") {
		t.Error("awake wanderer A missing from map")
	}
	if !strings.Contains(view, "b") {
		t.Error("asleep wanderer should render lowercase b")
	}
	rows := strings.Count(view, "\n")
	if rows < sim.GridSize {
		t.Errorf("map has %d rows, want at least %d", rows, sim.GridSize)
	}
}

func TestApplyEventUpdatesReplicaAndChronicle(t *testing.T) {
	m := testModel(t)
	m.lastSeq = 10

	// At-or-before the snapshot seq: already reflected, must be skipped.
	stale := store.Event{Seq: 10, Tick: 5, Type: "agent.moved",
		Payload: json.RawMessage(`{"agent":0,"x":9,"y":9}`)}
	m.applyEvent(stale)
	if len(m.events) != 0 || m.replica.Wanderers[0].X == 9 {
		t.Fatal("stale event must not apply")
	}

	fresh := store.Event{Seq: 11, Tick: 60, Type: "agent.moved",
		Payload: json.RawMessage(`{"agent":0,"x":7,"y":8}`)}
	m.applyEvent(fresh)
	if m.replica.Wanderers[0].X != 7 || m.replica.Wanderers[0].Y != 8 {
		t.Errorf("replica not updated: %+v", m.replica.Wanderers[0])
	}
	if m.replica.Tick != 60 {
		t.Errorf("replica tick = %d, want 60", m.replica.Tick)
	}
	if m.lastSeq != 11 || len(m.events) != 1 {
		t.Errorf("chronicle/cursor wrong: lastSeq=%d events=%d", m.lastSeq, len(m.events))
	}

	night := store.Event{Seq: 12, Tick: 16 * 3600, Type: "sim.night_started",
		Payload: json.RawMessage(`{"day":1}`)}
	m.applyEvent(night)
	if !m.replica.Night {
		t.Error("night event did not flip replica to night")
	}
}

func TestChronicleRingCap(t *testing.T) {
	m := testModel(t)
	for i := int64(1); i <= chronicleCap+50; i++ {
		m.applyEvent(store.Event{Seq: i, Tick: i, Type: "sim.day_started",
			Payload: json.RawMessage(`{"day":1}`)})
	}
	if len(m.events) != chronicleCap {
		t.Errorf("ring size = %d, want %d", len(m.events), chronicleCap)
	}
	if m.events[0].Seq != 51 {
		t.Errorf("ring dropped wrong end: oldest seq %d", m.events[0].Seq)
	}
}

func TestQuitDetaches(t *testing.T) {
	m := testModel(t)
	mdl, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q must produce tea.Quit")
	}
	if v := mdl.(Model).View(); !strings.Contains(v, "keeps running") {
		t.Errorf("quit view should reassure the world keeps running: %q", v)
	}
}

func TestDisconnectedHeaderShowsRetry(t *testing.T) {
	m := testModel(t)
	m.connected = false
	m.lastErr = "daemon not running"
	if v := m.headerView(); !strings.Contains(v, "disconnected") {
		t.Errorf("header should show disconnected state: %q", v)
	}
}

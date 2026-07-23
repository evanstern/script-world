package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/metatron"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/world"
)

// testModel defaults to a narrow (80-col) terminal — the widescreen tests
// set width explicitly (>= widescreenBreakpoint) where they need it.
func testModel(t *testing.T) Model {
	t.Helper()
	w, err := world.Create(t.TempDir()+"/w", "test", 42)
	if err != nil {
		t.Fatal(err)
	}
	m := New(w)
	m.replica = sim.NewState(42, w.Map())
	m.width, m.height = 80, 30
	return m
}

func widescreenModel(t *testing.T) Model {
	t.Helper()
	m := testModel(t)
	m.width, m.height = 140, 40
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func update(mdl tea.Model, k string) tea.Model {
	next, _ := mdl.Update(key(k))
	return next
}

func TestMapRendersWanderers(t *testing.T) {
	m := testModel(t)
	m.replica.Agents = []sim.Agent{
		{Name: "Ash", X: 3, Y: 4},
		{Name: "Birch", X: 10, Y: 2, Asleep: true},
	}
	view := m.mapView()
	lines := strings.Split(view, "\n")
	gridOnly := strings.Join(lines[:len(lines)-1], "\n") // drop the legend line
	if !strings.Contains(gridOnly, "A") {
		t.Error("awake wanderer A missing from map grid")
	}
	if !strings.Contains(gridOnly, "b") {
		t.Error("asleep wanderer should render lowercase b in map grid")
	}
	if len(lines) < 15 {
		t.Errorf("map viewport has %d lines, want a real window", len(lines))
	}
	if !strings.Contains(view, "~") {
		t.Error("terrain (water) missing from rendered window")
	}
}

// TestMapRendersPilesAndStockpileZones covers spec 013 T021 (US2-AS5,
// SC-006): the pile glyph appears on the map, adjacent piles are grouped
// into one stockpile zone by the render-side flood fill, and the legend
// (the map panel's one inspection surface — map.md "legend stays pinned as
// the panel's last row") reports each zone's/lone pile's contents as
// non-food counts + food batch totals.
func TestMapRendersPilesAndStockpileZones(t *testing.T) {
	m := testModel(t)
	cx, cy := m.gameMap.W/2, m.gameMap.H/2
	m.replica.Agents = []sim.Agent{{Name: "Ash", X: cx, Y: cy}}
	m.replica.Piles = []sim.Pile{
		{X: cx, Y: cy, Wood: 3, Stone: 1},
		{X: cx + 1, Y: cy, Planks: 2}, // Manhattan-adjacent to the pile above → one zone
		{X: cx - 6, Y: cy - 6, Food: []sim.FoodBatch{{Kind: "food_raw", N: 5, SpoilAt: 100}}}, // isolated
	}
	view := m.mapView()
	lines := strings.Split(view, "\n")
	gridOnly := strings.Join(lines[:len(lines)-1], "\n")
	legend := lines[len(lines)-1]

	if !strings.Contains(gridOnly, "%") {
		t.Error("pile glyph % missing from map grid")
	}
	if !strings.Contains(legend, "zone[2]") {
		t.Errorf("legend should report the 2-pile adjacent zone, got: %s", legend)
	}
	if !strings.Contains(legend, "3w") || !strings.Contains(legend, "1st") || !strings.Contains(legend, "2pl") {
		t.Errorf("legend should summarize the zone's non-food counts, got: %s", legend)
	}
	if !strings.Contains(legend, "food 5r/0c/0m") {
		t.Errorf("legend should summarize the isolated pile's food batch totals, got: %s", legend)
	}
	if !strings.Contains(legend, "%pile") {
		t.Error("legend key should explain the % pile glyph")
	}
}

// TestPileZonesGroupsOnlyManhattanAdjacentPiles is a focused unit test on
// the flood fill itself (spec 013 T021): diagonal neighbors must NOT merge
// (data-model.md / spec.md restrict adjacency to the sim package's own
// Manhattan convention), while a chain of orthogonal drops does.
func TestPileZonesGroupsOnlyManhattanAdjacentPiles(t *testing.T) {
	piles := []sim.Pile{
		{X: 0, Y: 0, Wood: 1},
		{X: 1, Y: 0, Wood: 1}, // adjacent to (0,0)
		{X: 2, Y: 1, Wood: 1}, // diagonal to (1,0) only — not adjacent
		{X: 9, Y: 9, Wood: 1}, // far away, its own zone
	}
	zones := pileZones(piles)
	if len(zones) != 3 {
		t.Fatalf("want 3 zones (2-chain, diagonal-isolated, far-isolated), got %d: %+v", len(zones), zones)
	}
	if len(zones[0]) != 2 {
		t.Errorf("first zone should merge the two orthogonally adjacent piles, got %d piles", len(zones[0]))
	}
	if len(zones[1]) != 1 || zones[1][0].X != 2 {
		t.Errorf("diagonal neighbor must not merge into the chain, got %+v", zones[1])
	}
	if len(zones[2]) != 1 || zones[2][0].X != 9 {
		t.Errorf("far pile should be its own zone, got %+v", zones[2])
	}
}

// TestMapRendersChestGlyphAndInspection covers spec 013 T026 (SC-006): the
// chest glyph appears on the map, and the legend (the map panel's one
// inspection surface, T021's precedent) reports each visible chest's owner
// name, contents, and a fullness hint.
func TestMapRendersChestGlyphAndInspection(t *testing.T) {
	m := testModel(t)
	cx, cy := m.gameMap.W/2, m.gameMap.H/2
	m.replica.Agents = []sim.Agent{{Name: "Ash", X: cx, Y: cy}, {Name: "Birch", X: cx + 2, Y: cy}}
	m.replica.Structures = []sim.Structure{
		// Off the agents' own tiles: the agent glyph outranks the structure
		// glyph on a shared tile (tile()'s priority order), so a chest at an
		// agent's own position would hide the glyph the test asserts on.
		{Kind: "chest", X: cx + 1, Y: cy, Owner: 1, Store: &sim.Inventory{Wood: 3, Planks: 2, FoodRaw: 5}},
	}
	view := m.mapView()
	lines := strings.Split(view, "\n")
	gridOnly := strings.Join(lines[:len(lines)-1], "\n")
	legend := lines[len(lines)-1]

	if !strings.Contains(gridOnly, "☐") {
		t.Error("chest glyph ☐ missing from map grid")
	}
	if !strings.Contains(legend, "☐chest") {
		t.Errorf("legend key should explain the ☐ chest glyph, got: %s", legend)
	}
	wantOwner := fmt.Sprintf("chest(%d,%d) [Birch]", cx+1, cy)
	if !strings.Contains(legend, wantOwner) {
		t.Errorf("legend should name the chest's owner by agent Name, got: %s", legend)
	}
	if !strings.Contains(legend, "3w") || !strings.Contains(legend, "2pl") || !strings.Contains(legend, "food 5r/0c/0m") {
		t.Errorf("legend should summarize the chest's contents, got: %s", legend)
	}
	if !strings.Contains(legend, "10/48") {
		t.Errorf("legend should show a fullness hint (3+2+5 = 10 of 48), got: %s", legend)
	}
}

// TestDescribeChestEmptyStore covers the empty-chest and out-of-range-owner
// edges of T026's inspection line: an empty Store reads "empty" rather than
// a blank/zero-padded contents string, and an owner index outside the
// roster (a defensive case, not one the sim package should ever produce)
// renders via agentName's "#N" fallback instead of panicking.
func TestDescribeChestEmptyStore(t *testing.T) {
	got := describeChest(sim.Structure{X: 1, Y: 2, Owner: 5, Store: &sim.Inventory{}}, []string{"Ash"})
	want := "chest(1,2) [#5] empty 0/48"
	if got != want {
		t.Errorf("describeChest empty store: got %q, want %q", got, want)
	}
}

// TestVillagersRosterShowsFullInventory covers SC-006 (spec 012 T043): the
// villagers roster must surface every carried resource kind —
// wood/stone/water/planks/refined stone, the food triplet, and the
// most-worn spear's remaining uses.
func TestVillagersRosterShowsFullInventory(t *testing.T) {
	m := widescreenModel(t)
	m.replica.Agents = []sim.Agent{
		{Name: "Ash", X: 3, Y: 4, Inv: sim.Inventory{
			Wood: 1, Stone: 2, Water: 3, Planks: 4, RefinedStone: 5,
			FoodRaw: 6, FoodCooked: 7, Meals: 8, Spears: []int{1, 3},
		}},
	}
	body := m.villagerRosterBody(m.width-6, m.height-6)
	want := "carry 1w 2st 3wt 4pl 5rs · food 6r/7c/8m · spear 2(1)"
	if !strings.Contains(body, want) {
		t.Errorf("villagers roster missing full inventory line %q, got:\n%s", want, body)
	}
}

func TestApplyEventUpdatesReplicaAndChronicle(t *testing.T) {
	m := testModel(t)
	m.lastSeq = 10

	// At-or-before the snapshot seq: already reflected, must be skipped.
	stale := store.Event{Seq: 10, Tick: 5, Type: "agent.moved",
		Payload: json.RawMessage(`{"agent":0,"x":9,"y":9}`)}
	m.applyEvent(stale)
	if len(m.events) != 0 || m.replica.Agents[0].X == 9 {
		t.Fatal("stale event must not apply")
	}

	fresh := store.Event{Seq: 11, Tick: 60, Type: "agent.moved",
		Payload: json.RawMessage(`{"agent":0,"x":7,"y":8}`)}
	m.applyEvent(fresh)
	if m.replica.Agents[0].X != 7 || m.replica.Agents[0].Y != 8 {
		t.Errorf("replica not updated: %+v", m.replica.Agents[0])
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

// TestCtrlCQuitsFromAnyState is focus-contract.md rule 3: "ctrl+c quits the
// app from any state whatsoever" — including while the minibuffer is
// focused and mid-input.
func TestCtrlCQuitsFromAnyState(t *testing.T) {
	m := testModel(t)
	var mdl tea.Model = m
	mdl = update(mdl, "m")
	mdl = update(mdl, "h")
	mdl = update(mdl, "i")
	mdl, cmd := mdl.(Model).Update(key("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c while focused must still produce tea.Quit")
	}
	if !mdl.(Model).quitting {
		t.Fatal("ctrl+c while focused must set quitting")
	}
}

// TestReplyTooLargeQuitsInsteadOfRetrying is TASK-19 AC#1 at the TUI: a
// reply over the protocol cap used to feed the 2s retry loop forever; now it
// is fatal — quit, with the actionable reason in the final view (and in
// cmdUI's exit error via FatalErr).
func TestReplyTooLargeQuitsInsteadOfRetrying(t *testing.T) {
	m := testModel(t)
	mdl, cmd := m.Update(disconnectedMsg{err: fmt.Errorf("state: %w", ipc.ErrReplyTooLarge)})
	mm := mdl.(Model)
	if !mm.quitting || mm.FatalErr() == "" {
		t.Fatalf("oversized reply must be fatal: quitting=%v fatal=%q", mm.quitting, mm.FatalErr())
	}
	if cmd == nil {
		t.Fatal("fatal disconnect must produce tea.Quit, not a retry")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("want tea.QuitMsg, got %T", cmd())
	}
	if v := mm.View(); !strings.Contains(v, "reply cap") {
		t.Errorf("final view should carry the reason: %q", v)
	}

	// Transient failures keep the old behavior: not fatal, schedule a retry.
	m2 := testModel(t)
	mdl2, cmd2 := m2.Update(disconnectedMsg{err: errors.New("daemon not running")})
	mm2 := mdl2.(Model)
	if mm2.quitting || mm2.FatalErr() != "" {
		t.Fatal("transient disconnect must not be fatal")
	}
	if cmd2 == nil {
		t.Fatal("transient disconnect should schedule a retry")
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

// chronEntry appends a narrated entry to the test replica's ring.
func chronEntry(m *Model, day int64, text, thread string, agents ...int) {
	m.replica.Chronicle = append(m.replica.Chronicle, sim.ChronicleEntry{
		Tick: day * 86400, Day: day, FromTick: (day - 1) * 86400, ToTick: day * 86400,
		Text: text, Thread: thread, Agents: agents,
	})
}

// TestChronicleNarratedView is TASK-11 AC#1/#2 at the pane: narrated entries
// render, and the a/t keys filter by agent and thread.
func TestChronicleNarratedView(t *testing.T) {
	m := testModel(t)
	m.active = paneChronicle
	chronEntry(&m, 1, "Ash lit the first fire.", "cold-start", 0)
	chronEntry(&m, 2, "The gru circled Sage in the dark.", "gru", 7)

	view := m.chronicleView()
	if !strings.Contains(view, "Ash lit the first fire.") || !strings.Contains(view, "gru circled Sage") {
		t.Fatalf("narrated entries missing: %q", view)
	}

	// 'a' cycles to agent 0 (Ash): only entries mentioning Ash remain.
	var mdl tea.Model = m
	mdl = update(mdl, "a")
	view = mdl.(Model).chronicleView()
	if !strings.Contains(view, "first fire") || strings.Contains(view, "gru circled") {
		t.Errorf("agent filter leaked: %q", view)
	}

	// Back to all, then 't' cycles to the first thread (cold-start).
	for i := 0; i < len(m.replica.Agents); i++ {
		mdl = update(mdl, "a")
	}
	mdl = update(mdl, "t")
	mm := mdl.(Model)
	if mm.chronAgent != -1 || mm.chronThread != "cold-start" {
		t.Fatalf("filter state: agent=%d thread=%q", mm.chronAgent, mm.chronThread)
	}
	view = mm.chronicleView()
	if !strings.Contains(view, "first fire") || strings.Contains(view, "gru circled") {
		t.Errorf("thread filter leaked: %q", view)
	}

	// 't' again reaches "gru", once more wraps to all.
	mdl = update(mm, "t")
	if mdl.(Model).chronThread != "gru" {
		t.Errorf("thread cycle: %q", mdl.(Model).chronThread)
	}
	mdl = update(mdl, "t")
	if mdl.(Model).chronThread != "" {
		t.Errorf("thread cycle should wrap to all: %q", mdl.(Model).chronThread)
	}
}

// TestChronicleRawFallback: no narrated entries -> raw feed automatically;
// 'r' toggles back to raw even when narration exists.
func TestChronicleRawFallback(t *testing.T) {
	m := testModel(t)
	m.active = paneChronicle
	m.applyEvent(store.Event{Seq: 1, Tick: 60, Type: "agent.moved",
		Payload: json.RawMessage(`{"agent":0,"x":7,"y":8}`)})

	view := m.chronicleView()
	if !strings.Contains(view, "agent.moved") || !strings.Contains(view, "raw feed") {
		t.Fatalf("empty ring must fall back to the raw feed: %q", view)
	}

	chronEntry(&m, 1, "Ash lit the first fire.", "cold-start", 0)
	if view := m.chronicleView(); strings.Contains(view, "agent.moved") {
		t.Fatalf("narrated view should replace raw once entries exist: %q", view)
	}
	var mdl tea.Model = m
	mdl = update(mdl, "r")
	if view := mdl.(Model).chronicleView(); !strings.Contains(view, "agent.moved") {
		t.Errorf("'r' should show the raw feed: %q", view)
	}
}

// TestChronicleKeysScopedToPane: a/t/r do nothing outside the chronicle pane.
func TestChronicleKeysScopedToPane(t *testing.T) {
	m := testModel(t)
	m.active = paneMap
	chronEntry(&m, 1, "x", "cold-start", 0)
	var mdl tea.Model = m
	mdl = update(mdl, "a")
	mdl = update(mdl, "t")
	mdl = update(mdl, "r")
	mm := mdl.(Model)
	if mm.chronAgent != -1 || mm.chronThread != "" || mm.chronRaw {
		t.Errorf("filters changed outside the pane: %+v", mm)
	}
}

func TestWrapText(t *testing.T) {
	lines := wrapText("one two three four five", 9)
	if len(lines) != 3 || lines[0] != "one two" {
		t.Errorf("wrap: %v", lines)
	}
	if got := wrapText("", 10); got != nil {
		t.Errorf("empty wrap: %v", got)
	}
}

// TestMinibufferReply: a turn's reply, nudge, and moments land in the
// transcript and the busy flag clears; errors render honestly.
func TestMinibufferReply(t *testing.T) {
	m := testModel(t)
	m.active = paneMetatron
	m.dockTab = paneMetatron
	m.mbBusy = true
	var mdl tea.Model = m
	mdl, _ = mdl.(Model).Update(consoleReplyMsg{result: &metatron.TurnResult{
		Reply:   "It is done.",
		Nudge:   &metatron.Nudge{Form: "dream", Targets: []string{"Fern"}, Text: "a river of light"},
		Moments: []string{"day 3 — Ash died"},
		Charges: 0,
	}})
	mm := mdl.(Model)
	if mm.mbBusy {
		t.Fatal("busy flag not cleared")
	}
	view := mm.metatronView()
	for _, want := range []string{"It is done.", "dream", "Fern", "Ash died"} {
		if !strings.Contains(view, want) {
			t.Errorf("console view missing %q", want)
		}
	}
	mdl, _ = mm.Update(consoleReplyMsg{err: fmt.Errorf("tier is down")})
	if v := mdl.(Model).metatronView(); !strings.Contains(v, "unreachable") {
		t.Errorf("error not rendered honestly: %q", v)
	}
}

// TestMetatronBadgeWhenTabNotVisible is minibuffer.md's reply-arrival rule:
// stream in place if the metatron tab/pane is visible, otherwise badge the
// dock tab and flash the minibuffer once — never steal the selected tab.
func TestMetatronBadgeWhenTabNotVisible(t *testing.T) {
	m := widescreenModel(t)
	m.dockTab = paneChronicle // metatron not visible
	mdl, _ := m.Update(consoleReplyMsg{result: &metatron.TurnResult{Reply: "the wood is dry"}})
	mm := mdl.(Model)
	if !mm.metatronUnseen {
		t.Error("tab should badge when metatron tab is not the visible one")
	}
	if mm.dockTab != paneChronicle {
		t.Error("arriving reply must not steal the selected tab")
	}
	if mm.mbFlash == "" {
		t.Error("minibuffer should flash once when the reply lands off-tab")
	}

	// Selecting the metatron tab clears the badge and flash.
	mdl2, _ := mm.selectTab(paneMetatron)
	mm2 := mdl2.(Model)
	if mm2.metatronUnseen || mm2.mbFlash != "" {
		t.Error("selecting the metatron tab should clear the badge/flash")
	}
}

// TestConsoleToolsSummary (spec 021 T021, SC-005): the console header's
// granted-tool summary is quiet for a full-grant default world, "none" for a
// conversation-only world, and the short-form set otherwise, carrying any
// miracle-kind restriction through.
func TestConsoleToolsSummary(t *testing.T) {
	cases := []struct {
		name string
		s    metatron.Status
		want string
	}{
		{"default is quiet", metatron.Status{ManifestDefault: true,
			GrantedTools: []string{"nudge_dream", "nudge_omen", "work_miracle"}}, ""},
		{"conversation-only", metatron.Status{ManifestDefault: false, GrantedTools: nil}, "tools: none"},
		{"subset short form", metatron.Status{ManifestDefault: false,
			GrantedTools: []string{"nudge_dream", "nudge_omen"}}, "tools: dream, omen"},
		{"restricted miracle kinds", metatron.Status{ManifestDefault: false,
			GrantedTools: []string{"nudge_dream", "work_miracle(move,give_item)"}},
			"tools: dream, miracles(move,give_item)"},
		{"unrestricted miracles", metatron.Status{ManifestDefault: false,
			GrantedTools: []string{"work_miracle"}}, "tools: miracles"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.s
			if got := consoleToolsSummary(&s); got != c.want {
				t.Errorf("consoleToolsSummary = %q, want %q", got, c.want)
			}
		})
	}
}

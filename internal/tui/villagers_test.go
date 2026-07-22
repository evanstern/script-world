package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/script-world/internal/ipc"
	"github.com/evanstern/script-world/internal/sim"
)

// --- T011: grammar/focus — j/k/g/G/⏎/esc scoped to the villagers tab ---

// villagersModel returns a widescreen model with the villagers tab visible
// (solo, so villagersVisible() is unambiguous regardless of layout choice).
func villagersModel(t *testing.T) Model {
	t.Helper()
	m := widescreenModel(t)
	m.dockTab = paneVillagers
	m.solo = true
	return m
}

func TestVillagersSelectionMoveAndJump(t *testing.T) {
	m := villagersModel(t)
	n := len(m.replica.Agents)
	if n < 3 {
		t.Fatalf("test assumes at least 3 agents, got %d", n)
	}
	var mdl tea.Model = m
	mdl = update(mdl, "j")
	if got := mdl.(Model).villSelected; got != 1 {
		t.Fatalf("j from 0 = %d, want 1", got)
	}
	mdl = update(mdl, "j")
	if got := mdl.(Model).villSelected; got != 2 {
		t.Fatalf("second j = %d, want 2", got)
	}
	mdl = update(mdl, "k")
	if got := mdl.(Model).villSelected; got != 1 {
		t.Fatalf("k = %d, want 1", got)
	}
	mdl = update(mdl, "G")
	if got := mdl.(Model).villSelected; got != n-1 {
		t.Fatalf("G should jump to last: got %d, want %d", got, n-1)
	}
	mdl = update(mdl, "j") // clamps at the end, does not wrap
	if got := mdl.(Model).villSelected; got != n-1 {
		t.Fatalf("j past the end should clamp: got %d, want %d", got, n-1)
	}
	mdl = update(mdl, "g")
	if got := mdl.(Model).villSelected; got != 0 {
		t.Fatalf("g should jump to first: got %d", got)
	}
	mdl = update(mdl, "k") // clamps at the start, does not go negative
	if got := mdl.(Model).villSelected; got != 0 {
		t.Fatalf("k before the start should clamp: got %d", got)
	}
}

func TestVillagersEnterOpensDetailEscCloses(t *testing.T) {
	m := villagersModel(t)
	var mdl tea.Model = m
	if mdl.(Model).villDetail {
		t.Fatal("detail must start closed")
	}
	mdl = update(mdl, "enter")
	if !mdl.(Model).villDetail {
		t.Fatal("enter should open the detail view")
	}
	mdl = update(mdl, "esc")
	if mdl.(Model).villDetail {
		t.Fatal("esc should close the detail view")
	}
}

// TestVillagersEscChainDetailThenSolo is focus-contract.md rule 3 applied to
// the villagers tab (contracts/state-and-keys.md): esc closes detail before
// releasing solo — two separate esc presses, two separate effects.
func TestVillagersEscChainDetailThenSolo(t *testing.T) {
	m := villagersModel(t) // already solo
	var mdl tea.Model = m
	mdl = update(mdl, "enter")
	if !mdl.(Model).villDetail {
		t.Fatal("setup: expected detail open")
	}
	mdl = update(mdl, "esc")
	mm := mdl.(Model)
	if mm.villDetail {
		t.Fatal("first esc should close detail, not touch solo")
	}
	if !mm.solo {
		t.Fatal("first esc must not have released solo yet")
	}
	mdl = update(mdl, "esc")
	if mdl.(Model).solo {
		t.Fatal("second esc should release solo (roster had nothing left to close)")
	}
}

// TestVillagersKeysNoOpWithNilOrEmptyReplica: FR-011 / the "confirm with no
// roster loaded" edge case.
func TestVillagersKeysNoOpWithNilOrEmptyReplica(t *testing.T) {
	m := villagersModel(t)
	m.replica = nil
	var mdl tea.Model = m
	for _, k := range []string{"j", "k", "g", "G", "enter"} {
		mdl = update(mdl, k)
	}
	mm := mdl.(Model)
	if mm.villSelected != 0 || mm.villDetail {
		t.Fatalf("keys should no-op with a nil replica: villSelected=%d villDetail=%v", mm.villSelected, mm.villDetail)
	}

	m2 := villagersModel(t)
	m2.replica.Agents = nil
	var mdl2 tea.Model = m2
	for _, k := range []string{"j", "k", "g", "G", "enter"} {
		mdl2 = update(mdl2, k)
	}
	mm2 := mdl2.(Model)
	if mm2.villSelected != 0 || mm2.villDetail {
		t.Fatalf("keys should no-op with an empty roster: villSelected=%d villDetail=%v", mm2.villSelected, mm2.villDetail)
	}
}

// TestVillagersSelectionSurvivesTabSwitch mirrors
// TestInspectStateSurvivesTabSwitch: leaving and returning to the villagers
// tab preserves the roster cursor and detail state (dock.md "Each tab keeps
// its own state ... across switches").
func TestVillagersSelectionSurvivesTabSwitch(t *testing.T) {
	m := widescreenModel(t)
	m.dockTab = paneVillagers
	m.villSelected = 2
	m.villDetail = true
	var mdl tea.Model = m
	mdl = update(mdl, "2") // switch to chronicle
	mdl = update(mdl, "4") // back to villagers
	mm := mdl.(Model)
	if mm.villSelected != 2 || !mm.villDetail {
		t.Errorf("villagers selection/detail not preserved across tab switch: villSelected=%d villDetail=%v", mm.villSelected, mm.villDetail)
	}
}

// TestVillagersSelectionClampsOnReconnect is R5: connectedMsg swaps the
// replica wholesale, so a cursor past the end of a shrunk roster must clamp
// rather than point at nothing.
func TestVillagersSelectionClampsOnReconnect(t *testing.T) {
	m := widescreenModel(t)
	m.villSelected = len(m.replica.Agents) - 1 // last agent
	smaller := sim.NewState(42, m.gameMap)
	smaller.Agents = smaller.Agents[:2] // roster shrank to 2 on reconnect
	mdl, _ := m.Update(connectedMsg{replica: smaller})
	mm := mdl.(Model)
	if mm.villSelected < 0 || mm.villSelected >= len(mm.replica.Agents) {
		t.Fatalf("villSelected = %d out of range for %d agents after reconnect", mm.villSelected, len(mm.replica.Agents))
	}
}

// TestVillagersJKDoNotCollideWithChronicleInspect: only one tab is visible
// at a time, so j/k while the villagers tab is up must move the roster, not
// the chronicle's (unrelated) selection, even while paused.
func TestVillagersJKDoNotCollideWithChronicleInspect(t *testing.T) {
	m := villagersModel(t)
	m.connected = true
	m.status = &ipc.StatusData{Clock: ipc.ClockStatus{Paused: true}}
	m.chronSelected = 3
	var mdl tea.Model = m
	mdl = update(mdl, "j")
	mm := mdl.(Model)
	if mm.villSelected != 1 {
		t.Errorf("villagers j should move villSelected: got %d", mm.villSelected)
	}
	if mm.chronSelected != 3 {
		t.Errorf("villagers j must not touch the unrelated chronicle selection: got %d", mm.chronSelected)
	}
}

// --- T012/T014/T018: render tests ---

func TestVillagerRosterCursorPresent(t *testing.T) {
	m := villagersModel(t)
	m.villSelected = 1
	body := m.villagerRosterBody(m.width-6, m.height-6)
	lines := strings.Split(body, "\n")
	found := false
	for _, l := range lines {
		if strings.Contains(l, "▌") && strings.Contains(l, m.replica.Agents[1].Name) {
			found = true
		}
	}
	if !found {
		t.Errorf("no selection cursor found on the selected row:\n%s", body)
	}
}

// TestVillagerDetailSectionsInPriorityOrder is R4/the rendering contract:
// identity → objective → inventory → beliefs/narrative → memories.
func TestVillagerDetailSectionsInPriorityOrder(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	a := &m.replica.Agents[0]
	a.Intent = &sim.Intent{Goal: "chop", TargetX: 5, TargetY: 6}
	a.Inv.Wood = 3
	a.Beliefs = []sim.Belief{{Statement: "the fire needs tending", Confidence: 70}}
	a.Narrative = "spent the week chopping wood."
	a.Memories = []sim.Memory{{Text: "saw a gru", Salience: 5, Tick: 100}}

	body := m.villagerDetailBody(m.width-6, m.height-6)
	idx := func(s string) int { return strings.Index(body, s) }
	identity := idx(a.Name)
	objective := idx("objective:")
	inventory := idx("inventory:")
	beliefs := idx("beliefs:")
	memories := idx("memories")
	for _, pair := range [][2]int{{identity, objective}, {objective, inventory}, {inventory, beliefs}, {beliefs, memories}} {
		if pair[0] < 0 || pair[1] < 0 || pair[0] >= pair[1] {
			t.Fatalf("sections out of order: identity=%d objective=%d inventory=%d beliefs=%d memories=%d\n%s",
				identity, objective, inventory, beliefs, memories, body)
		}
	}
}

// TestVillagerDetailLiveUpdate is FR-010/SC-005: the detail view renders
// straight from the replica each call — no re-selection needed for a
// mutation to show up on the next render.
func TestVillagerDetailLiveUpdate(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	before := m.villagerDetailBody(m.width-6, m.height-6)
	if strings.Contains(before, "wood 9") {
		t.Fatal("setup: inventory should not already show wood 9")
	}
	m.replica.Agents[0].Inv.Wood = 9
	after := m.villagerDetailBody(m.width-6, m.height-6)
	if !strings.Contains(after, "wood 9") {
		t.Errorf("detail view did not reflect the mutated replica without re-selecting:\n%s", after)
	}
}

// TestVillagersNoOverflowAtNarrowAndShortBudgets is FR-009/SC-004: neither
// the roster nor the detail view may ever emit more lines than `height`, at
// narrow widths too.
func TestVillagersNoOverflowAtNarrowAndShortBudgets(t *testing.T) {
	m := villagersModel(t)
	a := &m.replica.Agents[0]
	a.Beliefs = []sim.Belief{{Statement: "a very long belief statement that could easily overflow a narrow panel if left unclipped", Confidence: 50}}
	a.Narrative = strings.Repeat("a long narrative sentence about the fire and the woods. ", 5)
	for i := 0; i < 50; i++ {
		a.Memories = append(a.Memories, sim.Memory{Text: "chopped wood", Salience: 3, Tick: int64(i)})
	}
	for _, sz := range []struct{ w, h int }{{15, 3}, {15, 20}, {60, 3}, {60, 40}, {8, 8}} {
		roster := m.villagerRosterBody(sz.w, sz.h)
		if got := len(strings.Split(roster, "\n")); got > sz.h {
			t.Errorf("roster at %dx%d = %d lines, want <= %d:\n%s", sz.w, sz.h, got, sz.h, roster)
		}
		detail := m.villagerDetailBody(sz.w, sz.h)
		if got := len(strings.Split(detail, "\n")); got > sz.h {
			t.Errorf("detail at %dx%d = %d lines, want <= %d:\n%s", sz.w, sz.h, got, sz.h, detail)
		}
	}
}

func TestVillagersNoSoulStringAnywhere(t *testing.T) {
	m := villagersModel(t)
	a := &m.replica.Agents[0]
	a.Intent = &sim.Intent{Goal: "chop"}
	a.Beliefs = []sim.Belief{{Statement: "x", Confidence: 1}}
	a.Narrative = "y"
	a.Memories = []sim.Memory{{Text: "z", Salience: 1, Tick: 1}}
	roster := m.villagerRosterBody(m.width-6, m.height-6)
	m.villDetail = true
	detail := m.villagerDetailBody(m.width-6, m.height-6)
	for _, body := range []string{roster, detail, m.footerView(), m.dockTabsRow()} {
		if strings.Contains(strings.ToLower(body), "soul") {
			t.Errorf("user-visible \"soul\" string found: %q", body)
		}
	}
}

// --- T014: the three objective display states ---

func TestVillagerObjectiveActiveState(t *testing.T) {
	a := sim.Agent{Name: "Ash", Intent: &sim.Intent{Goal: "chop", TargetX: 4, TargetY: 5}}
	got := villagerObjectiveSection(a)
	if !strings.Contains(got, "chop") || !strings.Contains(got, "current") {
		t.Errorf("active objective line = %q, want goal + current marker", got)
	}
}

func TestVillagerObjectivePastState(t *testing.T) {
	a := sim.Agent{Name: "Ash", LastGoal: "forage", LastGoalTick: 3600}
	got := villagerObjectiveSection(a)
	if !strings.Contains(got, "forage") || !strings.Contains(got, "last") {
		t.Errorf("past objective line = %q, want goal + last marker", got)
	}
	if strings.Contains(got, "current") {
		t.Errorf("past objective must not be marked current: %q", got)
	}
}

func TestVillagerObjectiveNeverSetState(t *testing.T) {
	a := sim.Agent{Name: "Ash"} // zero-value: never had an intent
	got := villagerObjectiveSection(a)
	if !strings.Contains(got, "no objective yet") {
		t.Errorf("never-set objective line = %q, want \"no objective yet\"", got)
	}
}

// --- T018: memories most-recent-first, truncation order, empty state ---

func TestVillagerMemoriesMostRecentFirst(t *testing.T) {
	a := sim.Agent{Memories: []sim.Memory{
		{Text: "oldest", Tick: 1, Salience: 1},
		{Text: "middle", Tick: 2, Salience: 1},
		{Text: "newest", Tick: 3, Salience: 1},
	}}
	lines := villagerMemoriesLines(a, 10)
	body := strings.Join(lines, "\n")
	if strings.Index(body, "newest") > strings.Index(body, "middle") ||
		strings.Index(body, "middle") > strings.Index(body, "oldest") {
		t.Errorf("memories not most-recent-first:\n%s", body)
	}
}

func TestVillagerMemoriesEmptyState(t *testing.T) {
	a := sim.Agent{}
	lines := villagerMemoriesLines(a, 10)
	body := strings.Join(lines, "\n")
	if !strings.Contains(body, "no memories yet") {
		t.Errorf("empty memories should say so plainly: %q", body)
	}
}

func TestVillagerMemoriesBudgetOmittedWhenNoRoom(t *testing.T) {
	a := sim.Agent{Memories: []sim.Memory{{Text: "x", Tick: 1, Salience: 1}}}
	if got := villagerMemoriesLines(a, 0); got != nil {
		t.Errorf("budget < 1 should omit the memories section entirely, got %v", got)
	}
}

// TestVillagerDetailShedsMemoriesFirst is the spec's "Very short pane
// height" edge case: identity/objective/inventory survive a tight height
// budget; memories are what gets dropped.
func TestVillagerDetailShedsMemoriesFirst(t *testing.T) {
	m := villagersModel(t)
	m.villDetail = true
	a := &m.replica.Agents[0]
	a.Intent = &sim.Intent{Goal: "chop"}
	for i := 0; i < 30; i++ {
		a.Memories = append(a.Memories, sim.Memory{Text: "chopped wood", Salience: 3, Tick: int64(i)})
	}
	body := m.villagerDetailBody(60, 8) // short pane
	if !strings.Contains(body, a.Name) {
		t.Error("identity must survive a tight height budget")
	}
	if !strings.Contains(body, "objective:") {
		t.Error("objective must survive a tight height budget")
	}
	if got := len(strings.Split(body, "\n")); got > 8 {
		t.Errorf("detail at height 8 = %d lines, want <= 8", got)
	}
}

func TestVillagerBeliefsNarrativeShownWhenPresentOmittedWhenAbsent(t *testing.T) {
	present := sim.Agent{Beliefs: []sim.Belief{{Statement: "s", Confidence: 90}}, Narrative: "n"}
	if got := villagerBeliefsSection(present); !strings.Contains(got, "s") || !strings.Contains(got, "n") {
		t.Errorf("beliefs/narrative section missing content: %q", got)
	}
	absent := sim.Agent{}
	if got := villagerBeliefsSection(absent); got != "" {
		t.Errorf("beliefs/narrative section should be silently omitted when absent, got %q", got)
	}
}

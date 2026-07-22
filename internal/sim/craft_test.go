package sim

import (
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

// TestCraftInsufficientInputsNoOp is spec 012 US3 T026/FR-014: a hand-craft
// whose inputs are missing at completion resolves via agent.intent_done only
// — no agent.crafted, no inventory change (the contested-resource pattern
// applied uniformly, even though hand-crafts have no travel window).
func TestCraftInsufficientInputsNoOp(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.Wood = 0 // craft_planks needs 1 wood
	a.Intent = &Intent{Goal: "craft_planks", TargetX: a.X, TargetY: a.Y, WorkStart: 1 - craftPlanksTicks}

	log := driveTicks(t, s, m, 5, nil)
	done := false
	for _, e := range log {
		if e.Type == "agent.crafted" {
			t.Fatal("craft_planks with no wood must not emit agent.crafted")
		}
		if e.Type == "agent.intent_done" {
			var p AgentPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 0 {
				done = true
			}
		}
	}
	if !done {
		t.Error("insufficient-input craft should resolve via agent.intent_done")
	}
	if a.Inv.Planks != 0 {
		t.Errorf("Inv.Planks = %d, want 0 (nothing produced)", a.Inv.Planks)
	}
}

// TestCraftPlanksStoneSpear is spec 012 US3 AC#1-3: each hand-craft converts
// its inputs to outputs at completion, and craft_spear appends a fresh spear
// (spearDurability uses) to Spears rather than a plain int field.
func TestCraftPlanksStoneSpear(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.Wood = 2
	a.Intent = &Intent{Goal: "craft_planks", TargetX: a.X, TargetY: a.Y, WorkStart: 1 - craftPlanksTicks}
	driveTicks(t, s, m, 5, nil)
	if a.Inv.Wood != 1 || a.Inv.Planks != plankYield {
		t.Fatalf("after craft_planks: wood=%d planks=%d, want 1/%d", a.Inv.Wood, a.Inv.Planks, plankYield)
	}

	a.Inv.Stone = 1
	a.Intent = &Intent{Goal: "craft_stone", TargetX: a.X, TargetY: a.Y, WorkStart: s.Tick + 1 - craftStoneTicks}
	driveTicks(t, s, m, s.Tick+5, nil)
	if a.Inv.Stone != 0 || a.Inv.RefinedStone != 1 {
		t.Fatalf("after craft_stone: stone=%d refined_stone=%d, want 0/1", a.Inv.Stone, a.Inv.RefinedStone)
	}

	a.Inv.Wood = 1
	a.Inv.RefinedStone = 1
	a.Intent = &Intent{Goal: "craft_spear", TargetX: a.X, TargetY: a.Y, WorkStart: s.Tick + 1 - craftSpearTicks}
	driveTicks(t, s, m, s.Tick+5, nil)
	if a.Inv.Wood != 0 || a.Inv.RefinedStone != 0 {
		t.Fatalf("after craft_spear: wood=%d refined_stone=%d, want 0/0", a.Inv.Wood, a.Inv.RefinedStone)
	}
	if len(a.Inv.Spears) != 1 || a.Inv.Spears[0] != spearDurability {
		t.Fatalf("Spears = %v, want [%d]", a.Inv.Spears, spearDurability)
	}
}

// TestHuntBareVsSpear is spec 012 US3 AC#4-5: a bare-handed hunt succeeds at
// the modest yield/duration; a spear-carrying hunt is strictly better (more
// yield, less time) and spends the most-worn spear's use.
func TestHuntBareVsSpear(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)
	if len(m.Dens) < 3 {
		t.Fatal("test map needs at least 3 dens")
	}
	// Distinct dens per agent — each hunt's den cooldown must not interfere
	// with the others sharing this one State.
	den0, den1, den2 := m.Dens[0], m.Dens[1], m.Dens[2]

	// Bare-handed: yield huntYieldBare, duration huntTicks.
	a := &s.Agents[0]
	a.Dead = false
	a.X, a.Y = den0.X, den0.Y
	a.Intent = &Intent{Goal: "hunt", TargetX: den0.X, TargetY: den0.Y, WorkStart: 1 - huntTicks}
	log := driveTicks(t, s, m, 5, nil)
	var hunted bool
	for _, e := range log {
		if e.Type == "agent.hunted" {
			hunted = true
		}
		if e.Type == "agent.spear_broke" {
			t.Error("a bare-handed hunt must never emit agent.spear_broke")
		}
	}
	if !hunted {
		t.Fatal("bare hunt at the pinned duration produced no agent.hunted")
	}
	if a.Inv.FoodRaw != huntYieldBare {
		t.Errorf("bare hunt FoodRaw = %d, want %d", a.Inv.FoodRaw, huntYieldBare)
	}
	// One tick short of the bare duration must NOT complete yet — proves the
	// duration actually gates completion (not just "eventually happens").
	b := &s.Agents[1]
	b.Dead = false
	b.X, b.Y = den1.X, den1.Y
	b.Intent = &Intent{Goal: "hunt", TargetX: den1.X, TargetY: den1.Y, WorkStart: s.Tick}
	stillTicks := driveTicks(t, s, m, s.Tick+huntTicks-1, nil)
	for _, e := range stillTicks {
		if e.Type == "agent.hunted" {
			var p HarvestPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 1 {
				t.Fatal("bare hunt completed before its full duration elapsed")
			}
		}
	}

	// Spear-carrying: yield huntYieldSpear, duration huntTicksSpear (shorter).
	c := &s.Agents[2]
	c.Dead = false
	c.X, c.Y = den2.X, den2.Y
	c.Inv.Spears = []int{spearDurability}
	c.Intent = &Intent{Goal: "hunt", TargetX: den2.X, TargetY: den2.Y, WorkStart: s.Tick + 1 - huntTicksSpear}
	log2 := driveTicks(t, s, m, s.Tick+5, nil)
	var speared bool
	for _, e := range log2 {
		if e.Type == "agent.hunted" {
			var p HarvestPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 2 {
				speared = true
			}
		}
	}
	if !speared {
		t.Fatal("spear hunt at the shorter pinned duration produced no agent.hunted for agent 2")
	}
	if c.Inv.FoodRaw != huntYieldSpear {
		t.Errorf("spear hunt FoodRaw = %d, want %d", c.Inv.FoodRaw, huntYieldSpear)
	}
	if len(c.Inv.Spears) != 1 || c.Inv.Spears[0] != spearDurability-1 {
		t.Errorf("Spears after one spear hunt = %v, want [%d]", c.Inv.Spears, spearDurability-1)
	}
}

// TestHuntSpendsMostWornSpearFirst is spec 012 US3 (research R2): with two
// spears of different wear, a hunt spends Spears[0] — the most-worn one —
// keeping the slice sorted ascending.
func TestHuntSpendsMostWornSpearFirst(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)
	if len(m.Dens) == 0 {
		t.Fatal("test map has no dens")
	}
	den := m.Dens[0]

	a := &s.Agents[0]
	a.Dead = false
	a.X, a.Y = den.X, den.Y
	a.Inv.Spears = []int{2, 3} // sorted ascending: most-worn first
	a.Intent = &Intent{Goal: "hunt", TargetX: den.X, TargetY: den.Y, WorkStart: 1 - huntTicksSpear}

	driveTicks(t, s, m, 5, nil)
	if len(a.Inv.Spears) != 2 || a.Inv.Spears[0] != 1 || a.Inv.Spears[1] != 3 {
		t.Errorf("Spears after hunt = %v, want [1 3] (most-worn spent, order preserved)", a.Inv.Spears)
	}
}

// TestSpearBreaksAtZeroWithMemory is spec 012 US3 AC#4 + FR-015: the hunt
// that spends a spear's last use breaks it — removed from Spears — and
// leaves the villager a high-salience memory, both in the same event batch
// as the hunt completion.
func TestSpearBreaksAtZeroWithMemory(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)
	if len(m.Dens) == 0 {
		t.Fatal("test map has no dens")
	}
	den := m.Dens[0]

	a := &s.Agents[0]
	a.Dead = false
	a.X, a.Y = den.X, den.Y
	a.Inv.Spears = []int{1} // last use
	a.Intent = &Intent{Goal: "hunt", TargetX: den.X, TargetY: den.Y, WorkStart: 1 - huntTicksSpear}

	log := driveTicks(t, s, m, 5, nil)
	var sawHunted, sawBroke, sawMemory bool
	var huntedIdx, brokeIdx = -1, -1
	for idx, e := range log {
		switch e.Type {
		case "agent.hunted":
			sawHunted = true
			huntedIdx = idx
		case "agent.spear_broke":
			var p SpearBrokePayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent != 0 {
				t.Errorf("spear_broke agent = %d, want 0", p.Agent)
			}
			sawBroke = true
			brokeIdx = idx
		case "agent.memory_added":
			var p MemoryAddedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 0 && p.Salience == salSpearBroke {
				sawMemory = true
			}
		}
	}
	if !sawHunted {
		t.Fatal("no agent.hunted event")
	}
	if !sawBroke {
		t.Fatal("no agent.spear_broke event — the last-use hunt should break the spear")
	}
	if !sawMemory {
		t.Error("no high-salience memory for the broken spear")
	}
	if huntedIdx >= 0 && brokeIdx >= 0 && brokeIdx < huntedIdx {
		t.Error("agent.spear_broke must apply after agent.hunted in the same batch (hunt decrements to zero first)")
	}
	if len(a.Inv.Spears) != 0 {
		t.Errorf("Spears after breaking = %v, want empty", a.Inv.Spears)
	}
	if a.Inv.FoodRaw != huntYieldSpear {
		t.Errorf("FoodRaw after the breaking hunt = %d, want %d (still spear yield)", a.Inv.FoodRaw, huntYieldSpear)
	}
}

// TestReplayByteIdentityCraftAndHunt is SC-004 over the US3 surface: a run
// that hand-crafts planks, refined stone, and a spear, then hunts with the
// spear, replays to byte-identical state. Every step is a genuine injected
// agent.intent_set (planner-sourced), never a direct Intent/position
// mutation, so the recorded log alone is what reconstructs the replay — the
// actual live/replay contract, not just "the same script run twice".
func TestReplayByteIdentityCraftAndHunt(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	if len(m.Dens) == 0 {
		t.Fatal("test map has no dens")
	}
	den := m.Dens[0]

	genesis := func() *State {
		s := NewState(seed, m)
		isolateAgents(s)
		s.Agents[0].Dead = false
		s.Agents[0].Inv.Wood = 2
		s.Agents[0].Inv.Stone = 1
		return s
	}
	setIntent := func(tick int64, goal string, tx, ty int) map[int64][]store.Event {
		return map[int64][]store.Event{
			tick: {{Tick: tick, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{
				Agent: 0, Goal: goal, TargetX: tx, TargetY: ty, Source: "planner",
			})}},
		}
	}

	live := genesis()
	x0, y0 := live.Agents[0].X, live.Agents[0].Y

	// Hand-crafts are anywhere (target = the agent's own tile) — each
	// segment gives ample slack past its duration before the next command
	// lands, comfortably inside reflexGraceTicks(120) so the reflex (which
	// never crafts) can't preempt.
	var log []store.Event
	log = append(log, driveTicks(t, live, m, live.Tick+craftPlanksTicks+10, setIntent(live.Tick, "craft_planks", x0, y0))...)
	log = append(log, driveTicks(t, live, m, live.Tick+craftStoneTicks+10, setIntent(live.Tick, "craft_stone", x0, y0))...)
	log = append(log, driveTicks(t, live, m, live.Tick+craftSpearTicks+10, setIntent(live.Tick, "craft_spear", x0, y0))...)
	log = append(log, driveTicks(t, live, m, live.Tick+5000, setIntent(live.Tick, "hunt", den.X, den.Y))...)

	var sawCrafted, sawHunted bool
	kinds := map[string]bool{}
	for _, e := range log {
		switch e.Type {
		case "agent.crafted":
			sawCrafted = true
			var p CraftedPayload
			mustUnmarshal(t, e.Payload, &p)
			kinds[p.Kind] = true
		case "agent.hunted":
			sawHunted = true
		}
	}
	if !sawCrafted || !sawHunted {
		t.Fatalf("run did not exercise craft+hunt: crafted=%v hunted=%v", sawCrafted, sawHunted)
	}
	if !kinds["planks"] || !kinds["refined_stone"] || !kinds["spear"] {
		t.Fatalf("did not craft all three kinds: %v", kinds)
	}
	if live.Agents[0].Inv.FoodRaw != huntYieldSpear {
		t.Fatalf("FoodRaw = %d, want %d (spear hunt)", live.Agents[0].Inv.FoodRaw, huntYieldSpear)
	}
	if len(live.Agents[0].Inv.Spears) != 1 || live.Agents[0].Inv.Spears[0] != spearDurability-1 {
		t.Fatalf("Spears = %v, want [%d] (one use spent, no break)", live.Agents[0].Inv.Spears, spearDurability-1)
	}

	replayed := genesis()
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, live.Tick, nil) // re-live the quiet tail, as recovery does

	if live.Hash() != replayed.Hash() {
		t.Fatalf("replayed state diverged:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replayed.Marshal()))
	}
}

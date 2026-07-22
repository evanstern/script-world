package sim

import (
	"testing"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
)

// buildV1Fixture assembles a representative v1 legacyState: eight souls with
// needs, memories, beliefs, carried wood+legacy-food, a mid-flight intent and
// an asleep flag (both must reset), one near-death latch (must carry), plus the
// social/governance fabric (relations, an open debt, a rumor, a chronicle
// entry, Metatron charges, a norm) that the migration carries verbatim.
func buildV1Fixture(seed uint64, tick int64) *legacyState {
	agents := make([]legacyAgent, AgentCount)
	for i := range agents {
		agents[i] = legacyAgent{
			Name:  AgentNames[i],
			X:     i, // arbitrary v1 positions — the migration re-places them
			Y:     i,
			Needs: Needs{Health: 700 + i, Food: 400 + i, Rest: 500, Warmth: 600, Morale: 550},
			Inv:   legacyInventory{Wood: 5 + i, Food: 10 + i},
			Memories: []Memory{
				{Text: "the first fire", Salience: 6, Tick: 1200, Subject: -1},
			},
			Beliefs:  []Belief{{ID: i + 1, Statement: "wood is warmth", Confidence: 80, Provenance: ProvenanceInferred, Source: -1, Subject: -1, Tick: 900}},
			LastTalk: 500,
		}
	}
	// legacyState deliberately does not decode map-/session-bound agent fields
	// (Intent/Asleep/Plan/Hail) — they are reset, so the transform always emits
	// nil/false. The wipe-through-real-JSON-decode proof lives in the world
	// package's migrate test, which drives raw v1 JSON carrying an in-flight
	// intent. Here the carried people-state latches:
	agents[2].NearDeath = true // a lived-through collapse — people-state, carried
	agents[3].Dead = true      // a villager who died in the old world stays dead

	return &legacyState{
		Tick:  tick,
		Speed: clock.Speed4x,
		Seed:  seed,
		Night: true,
		// Map-bound state present in v1 — must be reset by the migration.
		// (Structures live only under the map; legacyState never decodes them.)
		Agents:      agents,
		Relations:   []Relation{{From: 0, To: 1, Trust: 300, Affection: 200}},
		Debts:       []Debt{{ID: 1, Debtor: 0, Creditor: 1, Kind: "food", Due: tick + 3600, Status: "open"}},
		Rumors:      []Rumor{{ID: 1, Subject: 2, Tone: -20, OriginAgent: 3, OriginTick: 800}},
		NextDebtID:  2,
		NextRumorID: 2,
		Chronicle: []ChronicleEntry{
			{Tick: 1000, Day: 1, FromTick: 0, ToTick: 1000, Text: "The village woke to frost."},
		},
		MetatronCharges: 2,
		Norms:           []Norm{{ID: 1, Kind: "curfew", Target: -1, Text: "home by dark", Proposer: 4, DayPassed: 2, Tally: "6-2", Active: true}},
		NextNormID:      2,
	}
}

// TestMigrateStateCarriesPeopleResetsLand is spec 012 US6 / FR-023: the
// transform keeps every villager and the social/governance fabric verbatim
// (tick continuity intact) while resetting map-bound state and re-placing souls
// on passable v2 tiles.
func TestMigrateStateCarriesPeopleResetsLand(t *testing.T) {
	const seed = 42
	const tick = 257400
	m := testMap(seed)
	v1 := buildV1Fixture(seed, tick)

	s := MigrateState(v1, m)

	if s.Tick != tick {
		t.Errorf("Tick = %d, want %d (continuity)", s.Tick, tick)
	}
	if s.Seed != seed {
		t.Errorf("Seed = %d, want %d", s.Seed, seed)
	}
	if !s.Night {
		t.Error("Night should carry (true)")
	}
	if s.Speed != clock.Speed4x {
		t.Errorf("Speed = %v, want 4x", s.Speed)
	}
	if s.Degraded {
		t.Error("Degraded should reset to false on a fresh migrated start")
	}
	if len(s.Agents) != AgentCount {
		t.Fatalf("agent count = %d, want %d", len(s.Agents), AgentCount)
	}

	for i := range s.Agents {
		a := &s.Agents[i]
		v := &v1.Agents[i]
		if a.Name != v.Name {
			t.Errorf("agent %d name = %q, want %q", i, a.Name, v.Name)
		}
		if a.Needs != v.Needs {
			t.Errorf("agent %d needs = %+v, want %+v (verbatim)", i, a.Needs, v.Needs)
		}
		if len(a.Memories) != 1 || a.Memories[0].Text != "the first fire" {
			t.Errorf("agent %d memories not carried: %+v", i, a.Memories)
		}
		if len(a.Beliefs) != 1 {
			t.Errorf("agent %d beliefs not carried: %+v", i, a.Beliefs)
		}
		// Inventory: wood 1:1, legacy food × 3 → meals, everything else zero.
		if a.Inv.Wood != v.Inv.Wood {
			t.Errorf("agent %d wood = %d, want %d (1:1)", i, a.Inv.Wood, v.Inv.Wood)
		}
		if a.Inv.Meals != v.Inv.Food*legacyFoodToMeals {
			t.Errorf("agent %d meals = %d, want %d (food×%d)", i, a.Inv.Meals, v.Inv.Food*legacyFoodToMeals, legacyFoodToMeals)
		}
		if a.Inv.FoodRaw != 0 || a.Inv.Stone != 0 || a.Inv.Water != 0 || len(a.Inv.Spears) != 0 {
			t.Errorf("agent %d has non-zero new-kind inventory: %+v", i, a.Inv)
		}
		// Map-/session-bound state reset.
		if a.Intent != nil {
			t.Errorf("agent %d intent should be cleared, got %+v", i, a.Intent)
		}
		if a.Asleep {
			t.Errorf("agent %d should wake standing (Asleep=false)", i)
		}
		if a.IdleSince != tick {
			t.Errorf("agent %d IdleSince = %d, want migration tick %d", i, a.IdleSince, tick)
		}
		// Re-placed on a passable v2 tile.
		if !m.Passable(a.X, a.Y) {
			t.Errorf("agent %d placed on impassable tile (%d,%d)", i, a.X, a.Y)
		}
	}

	// People-state latches carried; Dead preserved.
	if !s.Agents[2].NearDeath {
		t.Error("agent 2's NearDeath latch should carry (people-state)")
	}
	if !s.Agents[3].Dead {
		t.Error("agent 3 died in the old world and should stay dead")
	}

	// Re-placement is distinct (no two souls stacked).
	seen := map[Point]bool{}
	for i := range s.Agents {
		p := Point{X: s.Agents[i].X, Y: s.Agents[i].Y}
		if seen[p] {
			t.Errorf("two agents share tile %+v", p)
		}
		seen[p] = true
	}

	// Social/governance fabric carried verbatim.
	if len(s.Relations) != 1 || s.Relations[0].Trust != 300 {
		t.Errorf("relations not carried: %+v", s.Relations)
	}
	if len(s.Debts) != 1 || s.Debts[0].Status != "open" {
		t.Errorf("open debt not carried: %+v", s.Debts)
	}
	if len(s.Rumors) != 1 || s.NextRumorID != 2 {
		t.Errorf("rumors/counters not carried: rumors=%+v next=%d", s.Rumors, s.NextRumorID)
	}
	if len(s.Chronicle) != 1 {
		t.Errorf("chronicle ring not carried: %+v", s.Chronicle)
	}
	if s.MetatronCharges != 2 {
		t.Errorf("Metatron charges = %d, want 2", s.MetatronCharges)
	}
	if len(s.Norms) != 1 || !s.Norms[0].Active {
		t.Errorf("norms not carried: %+v", s.Norms)
	}

	// Map-bound state and the gru reset.
	if len(s.Structures) != 0 || len(s.Quarried) != 0 || len(s.Cleared) != 0 || len(s.Harvested) != 0 {
		t.Errorf("map-bound overlays should reset: struct=%v quarried=%v cleared=%v harvested=%v",
			s.Structures, s.Quarried, s.Cleared, s.Harvested)
	}
	if s.Gru != nil || s.MeetingConvention != nil || s.MeetingPlace != nil {
		t.Errorf("gru/meeting state should reset: gru=%v conv=%v place=%v", s.Gru, s.MeetingConvention, s.MeetingPlace)
	}
}

// TestMigratedReducerWholesaleReplace is spec 012 FR-026: replaying
// world.created → world.migrated from genesis (no snapshots) reproduces the
// transformed state byte-identically — the reducer replaces state wholesale and
// the JSON round-trip is stable.
func TestMigratedReducerWholesaleReplace(t *testing.T) {
	const seed = 42
	const tick = 257400
	m := testMap(seed)
	want := MigrateState(buildV1Fixture(seed, tick), m)

	// A fresh genesis state + the two-event migrated log, exactly as the daemon
	// recovers a snapshot-less migrated world.
	got := NewState(seed, m)
	events := []store.Event{
		{Tick: tick, Type: "world.created", Payload: mustPayload(WorldCreatedPayload{Name: "w", Seed: seed})},
		{Tick: tick, Type: "world.migrated", Payload: mustPayload(WorldMigratedPayload{
			FromFormat: 1, SourceEvents: 42, SourceTick: tick, State: *want,
		})},
	}
	for _, e := range events {
		if err := got.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
		if e.Tick > got.Tick {
			got.Tick = e.Tick
		}
	}

	if got.Hash() != want.Hash() {
		t.Fatalf("replayed state diverged from transform:\nwant: %s\ngot:  %s", string(want.Marshal()), string(got.Marshal()))
	}
}

// buildV2Fixture assembles a representative v2-shaped sim.State (spec 013
// v2→v3): structurally a subset of v3 (no piles; structures without Owner/Store;
// intents without Kind/Qty), so it is exactly what a real v2 daemon would have
// snapshotted. It includes an over-cap living agent, a dead agent carrying
// goods, a mid-flight intent, a fire with FuelUntil, the three overlay kinds,
// and the social fabric — everything the v2→v3 transform must carry verbatim
// while spilling only over-cap/dead carry.
func buildV2Fixture(seed uint64, tick int64) *State {
	agents := make([]Agent, AgentCount)
	for i := range agents {
		agents[i] = Agent{
			Name:     AgentNames[i],
			X:        20 + i, // distinct tiles, clear of the special ones below
			Y:        20 + i,
			Needs:    Needs{Health: 800, Food: 500, Rest: 600, Warmth: 700, Morale: 550},
			Inv:      Inventory{Wood: 2},
			Memories: []Memory{{Text: "we survived the frost", Salience: 5, Tick: 1000, Subject: -1}},
			LastTalk: 400,
		}
	}
	// Agent 0 — over the cap: bulk 30 (food_raw 5 + food_cooked 5 + meals 20).
	// Excess 6 spills least-nutritious-first: all 5 food_raw + 1 food_cooked,
	// keeping the meals (best food).
	agents[0].X, agents[0].Y = 5, 5
	agents[0].Inv = Inventory{FoodRaw: 5, FoodCooked: 5, Meals: 20}
	// Agent 1 — a mid-flight intent that must carry verbatim (no land reset).
	agents[1].Intent = &Intent{Goal: "chop", TargetX: 2, TargetY: 2, ResX: 2, ResY: 3, WorkStart: tick - 10}
	// Agent 3 — dead carrying goods: under v3 death spills, so the whole Inv
	// spills at its tile (wood 3, meals 2, one worn spear).
	agents[3].X, agents[3].Y = 9, 9
	agents[3].Dead = true
	agents[3].Inv = Inventory{Wood: 3, Meals: 2, Spears: []int{8}}

	return &State{
		Tick:            tick,
		Speed:           clock.Speed4x,
		Degraded:        true, // must reset to false on a fresh migrated start
		EffectiveRate:   3.0,
		Seed:            seed,
		Night:           true,
		Agents:          agents,
		Structures:      []Structure{{Kind: "fire", X: 10, Y: 10, FuelUntil: 5000}},
		Quarried:        []Point{{X: 1, Y: 1}},
		Cleared:         []Point{{X: 2, Y: 2}},
		Harvested:       []Harvest{{X: 3, Y: 3, Regrow: tick + 1000}},
		Relations:       []Relation{{From: 0, To: 1, Trust: 250, Affection: 150}},
		Rumors:          []Rumor{{ID: 1, Subject: 2, Tone: -20, OriginAgent: 3, OriginTick: 800}},
		NextRumorID:     2,
		MetatronCharges: 3,
	}
}

// TestTransformV2StateCarriesVerbatimSpillsOverCap is spec 013 T035/R3: the
// v2→v3 transform carries people AND land verbatim (positions unchanged — NO
// re-placement, NO land reset) and applies only the bulk-cap invariant —
// over-cap living carry and the whole inventory of the dead spill to ground
// piles at the agents' own tiles, spilled food stamped with a fresh deadline.
func TestTransformV2StateCarriesVerbatimSpillsOverCap(t *testing.T) {
	const seed = 42
	const tick = 300000
	v2 := buildV2Fixture(seed, tick)
	s := TransformV2State(v2)

	// Clock continuity; derived fields freshened.
	if s.Tick != tick || s.Seed != seed || !s.Night || s.Speed != clock.Speed4x {
		t.Errorf("clock/identity not carried: tick=%d seed=%d night=%v speed=%v", s.Tick, s.Seed, s.Night, s.Speed)
	}
	if s.Degraded {
		t.Error("Degraded should reset to false on a fresh migrated start")
	}
	if s.EffectiveRate != clock.Speed4x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v (freshened)", s.EffectiveRate, clock.Speed4x.TicksPerSecond())
	}

	// Land carried verbatim — NO reset (the whole point vs. v1→v2).
	if len(s.Structures) != 1 || s.Structures[0].Kind != "fire" || s.Structures[0].FuelUntil != 5000 {
		t.Errorf("fire structure not carried verbatim: %+v", s.Structures)
	}
	if len(s.Quarried) != 1 || len(s.Cleared) != 1 || len(s.Harvested) != 1 {
		t.Errorf("overlays should carry verbatim: quarried=%v cleared=%v harvested=%v", s.Quarried, s.Cleared, s.Harvested)
	}
	if len(s.Relations) != 1 || len(s.Rumors) != 1 || s.NextRumorID != 2 || s.MetatronCharges != 3 {
		t.Errorf("social fabric not carried: rel=%v rumors=%v next=%d charges=%d", s.Relations, s.Rumors, s.NextRumorID, s.MetatronCharges)
	}

	// Positions carried verbatim (exact coordinates — no re-placement).
	if s.Agents[0].X != 5 || s.Agents[0].Y != 5 {
		t.Errorf("agent 0 moved: (%d,%d), want (5,5)", s.Agents[0].X, s.Agents[0].Y)
	}
	if s.Agents[3].X != 9 || s.Agents[3].Y != 9 {
		t.Errorf("agent 3 moved: (%d,%d), want (9,9)", s.Agents[3].X, s.Agents[3].Y)
	}
	for i := 4; i < AgentCount; i++ {
		if s.Agents[i].X != 20+i || s.Agents[i].Y != 20+i {
			t.Errorf("agent %d moved: (%d,%d), want (%d,%d)", i, s.Agents[i].X, s.Agents[i].Y, 20+i, 20+i)
		}
	}
	// Mid-flight intent carried verbatim.
	if s.Agents[1].Intent == nil || s.Agents[1].Intent.Goal != "chop" || s.Agents[1].Intent.WorkStart != tick-10 {
		t.Errorf("agent 1 mid-flight intent not carried verbatim: %+v", s.Agents[1].Intent)
	}

	// Over-cap living spill (agent 0): keeps meals, lands exactly at cap.
	a0 := s.Agents[0].Inv
	if bulk(a0) != bulkCap {
		t.Errorf("agent 0 bulk = %d, want %d (spilled to cap)", bulk(a0), bulkCap)
	}
	if a0.FoodRaw != 0 || a0.FoodCooked != 4 || a0.Meals != 20 {
		t.Errorf("agent 0 inv after spill = %+v, want food_raw 0 / food_cooked 4 / meals 20", a0)
	}
	p0 := s.pileAt(5, 5)
	if p0 == nil {
		t.Fatal("no spill pile at agent 0's tile (5,5)")
	}
	wantSpoil := int64(tick + rotWindowTicks)
	if len(p0.Food) != 2 ||
		p0.Food[0] != (FoodBatch{Kind: "food_raw", N: 5, SpoilAt: wantSpoil}) ||
		p0.Food[1] != (FoodBatch{Kind: "food_cooked", N: 1, SpoilAt: wantSpoil}) {
		t.Errorf("agent 0 spill pile food = %+v, want food_raw 5 + food_cooked 1 @ spoil %d", p0.Food, wantSpoil)
	}

	// Dead agent's entire inventory spilled (agent 3).
	if bulk(s.Agents[3].Inv) != 0 {
		t.Errorf("dead agent 3 should be emptied, inv = %+v", s.Agents[3].Inv)
	}
	p3 := s.pileAt(9, 9)
	if p3 == nil {
		t.Fatal("no death-spill pile at agent 3's tile (9,9)")
	}
	if p3.Wood != 3 || len(p3.Spears) != 1 || p3.Spears[0] != 8 {
		t.Errorf("agent 3 spill non-food = wood %d spears %v, want wood 3 / [8]", p3.Wood, p3.Spears)
	}
	if len(p3.Food) != 1 || p3.Food[0] != (FoodBatch{Kind: "meals", N: 2, SpoilAt: wantSpoil}) {
		t.Errorf("agent 3 spill food = %+v, want meals 2 @ spoil %d", p3.Food, wantSpoil)
	}

	// Only the two spill piles exist; no under-cap agent leaked one.
	if len(s.Piles) != 2 {
		t.Errorf("piles = %d, want exactly 2 (over-cap + dead)", len(s.Piles))
	}

	// Purity: the input state was not mutated by the transform.
	if v2.Degraded != true || v2.Agents[0].Inv.FoodRaw != 5 || v2.Agents[3].Inv.Wood != 3 || len(v2.Piles) != 0 {
		t.Error("TransformV2State mutated its input (not pure)")
	}
}

// TestTransformV1ChainTo3 is spec 013 T035: a v1 world chains 1→2→3 in one run.
// The 012 conversion math still holds at the v2 waypoint, and the v3 invariant
// (no living villager over the bulk cap) holds at the end.
func TestTransformV1ChainTo3(t *testing.T) {
	const seed = 42
	const tick = 257400
	m := testMap(seed)

	// 1→2 (the existing transform): the 012 meals conversion holds here.
	v2 := MigrateState(buildV1Fixture(seed, tick), m)
	if v2.Agents[0].Inv.Meals != (10+0)*legacyFoodToMeals {
		t.Errorf("v2 waypoint meals = %d, want %d (012 conversion)", v2.Agents[0].Inv.Meals, 10*legacyFoodToMeals)
	}
	// buildV1Fixture carries enough food that every agent is over-cap in v2 — a
	// meaningful chain test (the 2→3 step must actually spill).
	overInV2 := false
	for i := range v2.Agents {
		if !v2.Agents[i].Dead && bulk(v2.Agents[i].Inv) > bulkCap {
			overInV2 = true
		}
	}
	if !overInV2 {
		t.Fatal("fixture should leave at least one over-cap living agent at the v2 waypoint")
	}

	// 2→3: the v3 invariant — no living villager over the cap.
	v3 := TransformV2State(v2)
	for i := range v3.Agents {
		a := &v3.Agents[i]
		if !a.Dead && bulk(a.Inv) > bulkCap {
			t.Errorf("agent %d over cap after chain: bulk %d > %d", i, bulk(a.Inv), bulkCap)
		}
	}
	// Tick continuity survives the whole chain.
	if v3.Tick != tick {
		t.Errorf("chain tick = %d, want %d (continuity)", v3.Tick, tick)
	}
	// The dead agent (index 3 in buildV1Fixture) is emptied by the death-spill
	// invariant carried forward.
	if v3.Agents[3].Dead && bulk(v3.Agents[3].Inv) != 0 {
		t.Errorf("dead agent 3 should be emptied by the chain, inv = %+v", v3.Agents[3].Inv)
	}
}

// TestTransformV2ChainReducerReplay proves the v2→v3 output replays
// byte-identically through the reducer (world.created → world.migrated from
// genesis, zero snapshots) — SC-005 determinism at the transform seam.
func TestTransformV2ChainReducerReplay(t *testing.T) {
	const seed = 42
	const tick = 300000
	m := testMap(seed)
	want := TransformV2State(buildV2Fixture(seed, tick))

	got := NewState(seed, m)
	events := []store.Event{
		{Tick: tick, Type: "world.created", Payload: mustPayload(WorldCreatedPayload{Name: "w", Seed: seed})},
		{Tick: tick, Type: "world.migrated", Payload: mustPayload(WorldMigratedPayload{
			FromFormat: 2, SourceEvents: 99, SourceTick: tick, State: *want,
		})},
	}
	for _, e := range events {
		if err := got.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
		if e.Tick > got.Tick {
			got.Tick = e.Tick
		}
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("replayed v2→v3 state diverged:\nwant %s\ngot  %s", string(want.Marshal()), string(got.Marshal()))
	}
}

// TestMigratedReducerRejectsForeignSeed is the reducer-total guard: a
// world.migrated event whose payload seed disagrees with the world being
// replayed is a no-op (never applied, never errored).
func TestMigratedReducerRejectsForeignSeed(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	foreign := MigrateState(buildV1Fixture(999, 5000), testMap(999)) // different seed

	s := NewState(seed, m)
	before := s.Hash()
	e := store.Event{Tick: 5000, Type: "world.migrated", Payload: mustPayload(WorldMigratedPayload{
		FromFormat: 1, SourceEvents: 10, SourceTick: 5000, State: *foreign,
	})}
	if err := s.Apply(e); err != nil {
		t.Fatalf("apply should be a total no-op, got error: %v", err)
	}
	if s.Hash() != before {
		t.Error("a foreign-seed migration event should not mutate state")
	}
}

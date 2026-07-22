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

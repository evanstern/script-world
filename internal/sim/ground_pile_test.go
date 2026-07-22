package sim

import (
	"testing"

	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// Ground piles (spec 013 US2, T020): drop creates/merges one pile per tile,
// pickup truncates to free bulk, same-tick contested pickup arbitrates by agent
// order (the second taker finds the remainder), death spills a lootable pile
// (spear durabilities riding along), building on a pile is refused, and a
// scripted drop/pickup/death run replays byte-identically.

func applyEvent(t *testing.T, s *State, tick int64, typ string, pl any) {
	t.Helper()
	e := store.Event{Tick: tick, Type: typ, Payload: mustPayload(pl)}
	if err := s.Apply(e); err != nil {
		t.Fatalf("apply %s at tick %d: %v", typ, tick, err)
	}
}

// findBuildTile scans for a tile buildSite accepts (fresh state, no overlays).
func findBuildTile(m *worldmap.Map, s *State) (x, y int, ok bool) {
	for yy := 0; yy < m.H; yy++ {
		for xx := 0; xx < m.W; xx++ {
			if buildSite(m, s, xx, yy) {
				return xx, yy, true
			}
		}
	}
	return 0, 0, false
}

// TestDropCreatesAndMergesPile is US2-AS1/AS2: a drop puts goods on the tile's
// pile (created if absent), a second drop on the same tile accumulates into the
// SAME pile (one pile per tile), and dropped food becomes a batch stamped with a
// fresh rot deadline. The executor half asserts the clamped ACTUAL count and the
// empty-Kind / nothing-carried intent_done rule.
func TestDropCreatesAndMergesPile(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	// Reducer-level: create → merge → food batch stamp, always one pile.
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{Wood: 5, FoodRaw: 4}

	applyEvent(t, s, 10, "agent.dropped", DroppedPayload{Agent: 0, X: 5, Y: 5, Kind: "wood", N: 3})
	if len(s.Piles) != 1 || s.Piles[0].Wood != 3 {
		t.Fatalf("after first drop, Piles = %+v, want one pile with Wood 3", s.Piles)
	}
	if a.Inv.Wood != 2 {
		t.Errorf("agent Wood = %d, want 2 after dropping 3 of 5", a.Inv.Wood)
	}

	applyEvent(t, s, 10, "agent.dropped", DroppedPayload{Agent: 0, X: 5, Y: 5, Kind: "wood", N: 2})
	if len(s.Piles) != 1 || s.Piles[0].Wood != 5 {
		t.Fatalf("after second drop, Piles = %+v, want ONE pile accumulating Wood 5 (one pile per tile)", s.Piles)
	}
	if a.Inv.Wood != 0 {
		t.Errorf("agent Wood = %d, want 0", a.Inv.Wood)
	}

	applyEvent(t, s, 20, "agent.dropped", DroppedPayload{Agent: 0, X: 5, Y: 5, Kind: "food_raw", N: 4})
	if len(s.Piles) != 1 || len(s.Piles[0].Food) != 1 {
		t.Fatalf("after food drop, pile = %+v, want one food batch", s.Piles)
	}
	b := s.Piles[0].Food[0]
	if b.Kind != "food_raw" || b.N != 4 || b.SpoilAt != 20+rotWindowTicks {
		t.Errorf("food batch = %+v, want food_raw x4 spoil_at %d", b, 20+rotWindowTicks)
	}
	if a.Inv.FoodRaw != 0 {
		t.Errorf("agent FoodRaw = %d, want 0", a.Inv.FoodRaw)
	}

	// Executor-level: the emitted count is clamped to what is carried, and an
	// empty Kind or nothing carried resolves via intent_done only.
	dropRun := func(t *testing.T, inv Inventory, kind string, qty int) []store.Event {
		t.Helper()
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = inv
		a.Intent = &Intent{Goal: "drop", TargetX: a.X, TargetY: a.Y, Kind: kind, Qty: qty}
		return driveTicks(t, s, m, 2, nil)
	}

	t.Run("clamped_to_carried", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = Inventory{Wood: 2}
		a.Intent = &Intent{Goal: "drop", TargetX: a.X, TargetY: a.Y, Kind: "wood", Qty: 5}
		log := driveTicks(t, s, m, 2, nil)
		var got int
		for _, e := range log {
			if e.Type == "agent.dropped" {
				var p DroppedPayload
				mustUnmarshal(t, e.Payload, &p)
				if p.Agent == 0 {
					got = p.N
				}
			}
		}
		if got != 2 {
			t.Errorf("dropped N = %d, want 2 (Qty 5 clamped to carried 2)", got)
		}
		if a.Inv.Wood != 0 || s.pileAt(a.X, a.Y) == nil || s.pileAt(a.X, a.Y).Wood != 2 {
			t.Errorf("post-drop Inv/pile wrong: Wood %d pile %+v", a.Inv.Wood, s.pileAt(a.X, a.Y))
		}
	})

	t.Run("empty_kind_is_noop", func(t *testing.T) {
		log := dropRun(t, Inventory{Wood: 3}, "", 0)
		for _, e := range log {
			if e.Type == "agent.dropped" {
				t.Fatal("an empty-Kind drop emitted agent.dropped — must be intent_done only")
			}
		}
	})

	t.Run("nothing_carried_is_noop", func(t *testing.T) {
		log := dropRun(t, Inventory{Wood: 0}, "wood", 0)
		for _, e := range log {
			if e.Type == "agent.dropped" {
				t.Fatal("a drop of an unheld kind emitted agent.dropped — must be intent_done only")
			}
		}
	})
}

// TestPickupTruncatesToFreeBulk is US2-AS3: a pickup moves only as much as fits
// under the bulk cap; the remainder stays in the pile. Driven through the
// executor so both the emit truncation and the reducer clamp are exercised.
func TestPickupTruncatesToFreeBulk(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{Wood: bulkCap - 3} // free bulk 3
	s.Piles = []Pile{{X: a.X, Y: a.Y, Wood: 10}}
	a.Intent = &Intent{Goal: "pick_up", TargetX: a.X, TargetY: a.Y, Kind: "wood"}

	log := driveTicks(t, s, m, 2, nil)

	var got int
	for _, e := range log {
		if e.Type == "agent.picked_up" {
			var p PickedUpPayload
			mustUnmarshal(t, e.Payload, &p)
			got = p.N
		}
	}
	if got != 3 {
		t.Errorf("picked_up N = %d, want 3 (truncated to free bulk)", got)
	}
	if bulk(a.Inv) != bulkCap {
		t.Errorf("agent bulk = %d, want %d (filled to the cap)", bulk(a.Inv), bulkCap)
	}
	p := s.pileAt(a.X, a.Y)
	if p == nil || p.Wood != 7 {
		t.Errorf("pile Wood = %+v, want 7 remaining", p)
	}
}

// TestContestedPickupSameTick is the spec edge case "two villagers, one pile,
// same tick": both takers are resolved against the pre-tick pile, so both emit;
// the reducer applies them in agent order, clamping — the first taker gets what
// its free bulk allows, the second finds only the remainder. Deterministic.
func TestContestedPickupSameTick(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	for i := 2; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}
	a0, a1 := &s.Agents[0], &s.Agents[1]
	a1.X, a1.Y = a0.X, a0.Y // co-located on the pile tile
	a0.Inv = Inventory{Wood: bulkCap - 3} // free 3 — the first taker is nearly full
	a1.Inv = Inventory{}                  // free 24 — room for the remainder
	s.Piles = []Pile{{X: a0.X, Y: a0.Y, Wood: 5}}
	a0.Intent = &Intent{Goal: "pick_up", TargetX: a0.X, TargetY: a0.Y, Kind: "wood"}
	a1.Intent = &Intent{Goal: "pick_up", TargetX: a1.X, TargetY: a1.Y, Kind: "wood"}

	driveTicks(t, s, m, 2, nil)

	if a0.Inv.Wood != bulkCap {
		t.Errorf("first taker Wood = %d, want %d (took its free bulk of 3)", a0.Inv.Wood, bulkCap)
	}
	if a1.Inv.Wood != 2 {
		t.Errorf("second taker Wood = %d, want 2 (only the remainder after the first)", a1.Inv.Wood)
	}
	if s.pileAt(a0.X, a0.Y) != nil {
		t.Errorf("pile survived, want removed once drained: %+v", s.pileAt(a0.X, a0.Y))
	}
}

// TestDeathSpillWithSpears is US2-AS4 / FR-006: a death moves the agent's entire
// inventory into the death-tile pile — food stamped with a fresh deadline,
// spears riding along with their exact durabilities merged sorted-ascending.
func TestDeathSpillWithSpears(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{Wood: 3, FoodRaw: 2, Spears: []int{1, 3}}
	// A pre-existing pile on the death tile to prove create-or-merge + sorting.
	s.Piles = []Pile{{X: a.X, Y: a.Y, Spears: []int{2}}}

	applyEvent(t, s, 100, "agent.died", DiedPayload{Agent: 0, Cause: "starvation"})

	if !a.Dead {
		t.Fatal("agent not marked dead")
	}
	if bulk(a.Inv) != 0 {
		t.Errorf("dead agent still carries %d bulk, want 0 (fully spilled)", bulk(a.Inv))
	}
	p := s.pileAt(a.X, a.Y)
	if p == nil {
		t.Fatal("no pile at the death tile")
	}
	if p.Wood != 3 {
		t.Errorf("pile Wood = %d, want 3", p.Wood)
	}
	if len(p.Spears) != 3 || p.Spears[0] != 1 || p.Spears[1] != 2 || p.Spears[2] != 3 {
		t.Errorf("pile Spears = %v, want [1 2 3] (durabilities preserved, sorted, merged)", p.Spears)
	}
	if len(p.Food) != 1 || p.Food[0].Kind != "food_raw" || p.Food[0].N != 2 || p.Food[0].SpoilAt != 100+rotWindowTicks {
		t.Errorf("pile Food = %+v, want one food_raw x2 batch stamped %d", p.Food, 100+rotWindowTicks)
	}
}

// TestBuildOnPileRefused is the spec edge case / FR-007: a tile holding a pile is
// not buildable — both the buildSite predicate (the search) and the executor's
// completion re-validation reject it, so goods are never buried.
func TestBuildOnPileRefused(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	bx, by, ok := findBuildTile(m, s)
	if !ok {
		t.Skip("no buildable tile on this map")
	}
	if !buildSite(m, s, bx, by) {
		t.Fatal("setup: tile should be buildable")
	}
	s.Piles = []Pile{{X: bx, Y: by, Wood: 1}}
	if buildSite(m, s, bx, by) {
		t.Error("buildSite accepted a tile holding a pile — FR-007 requires refusal")
	}

	// Executor re-validation: a build completing onto a pile tile yields no
	// structure, only intent_done (the contested-resource pattern).
	s2 := NewState(seed, m)
	a := &s2.Agents[0]
	a.X, a.Y = bx, by
	a.Inv = Inventory{Wood: fireWoodCost}
	a.Intent = &Intent{Goal: "build_fire", TargetX: bx, TargetY: by, WorkStart: 1 - buildFireTicks}
	s2.Piles = []Pile{{X: bx, Y: by, Wood: 1}}
	log := driveTicks(t, s2, m, 3, nil)
	for _, e := range log {
		if e.Type == "agent.built" {
			t.Fatal("build completed on a pile tile — the executor must re-validate and refuse")
		}
	}
	if s2.hasStructure("fire") {
		t.Error("a fire structure was placed on a pile tile")
	}
}

// TestReplayByteIdentityGroundPiles is SC-005 over US2: a scripted drop, pickup,
// and death run replays from genesis (log only) to a byte-identical state hash.
func TestReplayByteIdentityGroundPiles(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	genesis := func() *State {
		s := NewState(seed, m)
		for i := 3; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true // quiet the run to the scripted trio
		}
		s.Agents[1].X, s.Agents[1].Y = s.Agents[0].X, s.Agents[0].Y // co-locate the picker
		s.Agents[0].Inv = Inventory{Wood: 10}
		s.Agents[2].Inv = Inventory{FoodRaw: 4, Spears: []int{2}}
		return s
	}

	pl := func(v any) []byte { return mustPayload(v) }
	commands := map[int64][]store.Event{
		// Agent 0 drops 6 wood onto its (and agent 1's) tile.
		30: {{Tick: 30, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 0, Goal: "drop", TargetX: genesis().Agents[0].X, TargetY: genesis().Agents[0].Y,
			Kind: "wood", Qty: 6, Source: "planner"})}},
		// Agent 2 dies with food + a spear → a spill pile at its tile.
		40: {{Tick: 40, Type: "agent.died", Payload: pl(DiedPayload{Agent: 2, Cause: "starvation"})}},
		// Agent 1 picks up everything from the shared tile.
		60: {{Tick: 60, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 1, Goal: "pick_up", TargetX: genesis().Agents[1].X, TargetY: genesis().Agents[1].Y,
			Source: "planner"})}},
	}

	const ticks = 100
	live := genesis()
	log := driveTicks(t, live, m, ticks, commands)

	// The scripted storage happenings actually occurred.
	var sawDrop, sawPick bool
	for _, e := range log {
		switch e.Type {
		case "agent.dropped":
			sawDrop = true
		case "agent.picked_up":
			sawPick = true
		}
	}
	if !sawDrop || !sawPick {
		t.Fatalf("scripted run missing storage events (drop %v, pick %v)", sawDrop, sawPick)
	}
	if !live.Agents[2].Dead || live.pileAt(live.Agents[2].X, live.Agents[2].Y) == nil {
		t.Error("agent 2's death did not leave a spill pile")
	}
	if live.Agents[1].Inv.Wood != 6 {
		t.Errorf("picker Wood = %d, want 6 (picked up the whole drop)", live.Agents[1].Inv.Wood)
	}

	// Replay the log over genesis, re-live the quiet tail, compare hashes.
	replay := genesis()
	for _, e := range log {
		if err := replay.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replay.Tick = e.Tick
	}
	driveTicks(t, replay, m, ticks, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("replay diverged:\nlive:     %s\nreplayed: %s", string(live.Marshal()), string(replay.Marshal()))
	}
}

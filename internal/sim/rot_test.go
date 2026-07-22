package sim

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

// Rot: the ground is not a larder (spec 013 US5, T033). Ground-pile food spoils
// on the per-game-minute heartbeat within rotWindowTicks + 1 game minute
// (SC-004); non-food is immortal anywhere; death-spill batches inherit fresh
// death-tick deadlines; the rot sweep and a same-tick pickup resolve by batch
// order under the contested re-validation idiom (both orders deterministic); and
// a rot run replays byte-identically with spoil_at deadlines surviving a
// snapshot round-trip. Chest-food immunity is covered by TestChestFoodNeverSpoils
// (US3), so it is not re-proven here.

// rotFixture builds a quiet state (all agents dead — no reflex/social churn) with
// a single food_raw pile on agent 0's tile stamped to spoil at spoilAt. Returns
// the state and the pile tile.
func rotFixture(seed uint64, n int, spoilAt int64) (*State, int, int) {
	s := NewState(seed, testMap(seed))
	for i := range s.Agents {
		s.Agents[i].Dead = true
	}
	px, py := s.Agents[0].X, s.Agents[0].Y
	s.Piles = []Pile{{X: px, Y: py, Food: []FoodBatch{{Kind: "food_raw", N: n, SpoilAt: spoilAt}}}}
	return s, px, py
}

// TestRotWithinWindowPlusOneMinute is SC-004: a food batch stamped at drop tick +
// rotWindowTicks survives every minute boundary before its deadline and is gone
// at the first heartbeat at/after it — always within one game minute (60 ticks)
// of the deadline, exactly the sweep cadence. Driven cheaply by pre-advancing the
// clock to just before the deadline rather than simulating two whole game days.
func TestRotWithinWindowPlusOneMinute(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	const dropTick = int64(10)
	spoilAt := dropTick + rotWindowTicks // the real drop-time stamp (US2/agents.go)

	s, px, py := rotFixture(seed, 3, spoilAt)
	// Advance to shortly before the deadline; drive up to (but not through) the
	// deadline tick — the last boundary crossed here is < spoilAt, so no rot.
	s.Tick = spoilAt - 120
	driveTicks(t, s, m, spoilAt, nil)
	if p := s.pileAt(px, py); p == nil || p.avail("food_raw") != 3 {
		t.Fatalf("food rotted before its deadline: pile = %+v (tick %d, spoilAt %d)", p, s.Tick, spoilAt)
	}

	// Continue past the next heartbeat: the first minute boundary at/after the
	// deadline sweeps the batch away, within +1 game minute of spoilAt.
	log := driveTicks(t, s, m, spoilAt+60, nil)
	if p := s.pileAt(px, py); p != nil {
		t.Errorf("food not gone within rotWindowTicks + 1 game minute: pile = %+v", p)
	}
	if s.Tick-spoilAt > 60 {
		t.Errorf("rot resolved %d ticks after the deadline, > one game minute", s.Tick-spoilAt)
	}
	var got FoodRottedPayload
	var sawRot bool
	for _, e := range log {
		if e.Type == "sim.food_rotted" {
			mustUnmarshal(t, e.Payload, &got)
			sawRot = true
		}
	}
	if !sawRot {
		t.Fatal("no sim.food_rotted event emitted by the sweep")
	}
	if got.X != px || got.Y != py || got.Kind != "food_raw" || got.N != 3 {
		t.Errorf("food_rotted payload = %+v, want {%d %d food_raw 3}", got, px, py)
	}
}

// TestNonFoodNeverDecays is US5-AS2 / FR-013: wood, stone, water, planks,
// refined stone, and spears in a ground pile carry no deadline and survive
// arbitrary time — the sweep only ever touches food batches. Driven well past a
// full rot window with no food_rotted ever emitted and every count intact.
func TestNonFoodNeverDecays(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	for i := range s.Agents {
		s.Agents[i].Dead = true
	}
	px, py := s.Agents[0].X, s.Agents[0].Y
	want := Pile{X: px, Y: py, Wood: 3, Stone: 2, Water: 1, Planks: 4, RefinedStone: 5, Spears: []int{1, 2, 3}}
	s.Piles = []Pile{want}

	log := driveTicks(t, s, m, rotWindowTicks+120, nil)

	for _, e := range log {
		if e.Type == "sim.food_rotted" {
			t.Fatalf("sim.food_rotted emitted for a food-free pile: %s", e.Payload)
		}
	}
	p := s.pileAt(px, py)
	if p == nil {
		t.Fatal("non-food pile vanished — it must be immortal")
	}
	if p.Wood != 3 || p.Stone != 2 || p.Water != 1 || p.Planks != 4 || p.RefinedStone != 5 {
		t.Errorf("non-food counts changed after %d ticks: %+v", rotWindowTicks+120, p)
	}
	if len(p.Spears) != 3 || p.Spears[0] != 1 || p.Spears[1] != 2 || p.Spears[2] != 3 {
		t.Errorf("spears changed: %v, want [1 2 3]", p.Spears)
	}
}

// TestDeathSpillInheritsFreshDeadline is the death-spill half of US5: a spill's
// food batch is stamped death tick + rotWindowTicks, so it survives every
// heartbeat right after the death and only spoils on its own fresh clock — a
// villager's effects are recoverable, not instantly rotting.
func TestDeathSpillInheritsFreshDeadline(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	for i := 1; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}
	a := &s.Agents[0]
	a.Inv = Inventory{FoodRaw: 4}
	const deathTick = int64(600)

	applyEvent(t, s, deathTick, "agent.died", DiedPayload{Agent: 0, Cause: "starvation"})

	p := s.pileAt(a.X, a.Y)
	if p == nil || len(p.Food) != 1 {
		t.Fatalf("death spill left no single food batch: pile = %+v", p)
	}
	if p.Food[0].SpoilAt != deathTick+rotWindowTicks {
		t.Errorf("spill batch SpoilAt = %d, want death tick + window %d", p.Food[0].SpoilAt, deathTick+rotWindowTicks)
	}

	// Drive several heartbeats past the death — the fresh deadline is far in the
	// future, so nothing rots yet.
	s.Tick = deathTick
	log := driveTicks(t, s, m, deathTick+600, nil)
	for _, e := range log {
		if e.Type == "sim.food_rotted" {
			t.Fatalf("a fresh death-spill batch rotted immediately: %s", e.Payload)
		}
	}
	if p := s.pileAt(a.X, a.Y); p == nil || p.avail("food_raw") != 4 {
		t.Errorf("death-spill food gone too soon: pile = %+v", p)
	}
}

// TestRotVsPickupSameTick is the spec edge case "Rot mid-pickup": when the rot
// sweep and a pickup both resolve against the same pre-tick pile, they land in
// one batch and the reducer's clamp makes whichever applies SECOND find only the
// remainder — the established contested re-validation idiom. Both batch orders
// are exercised, and each replays byte-identically (deterministic outcomes).
func TestRotVsPickupSameTick(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	const tick = int64(600) // a minute boundary; the batch is spoiled by now
	pl := func(v any) json.RawMessage { return mustPayload(v) }

	// A fresh state: agent 0 with room, a pile of 5 spoiled food_raw on its tile.
	// The sweep would emit food_rotted{N:5} (all spoiled); a pickup that fits 3
	// emits picked_up{N:3} — both against the pre-tick pile of 5.
	fresh := func() *State {
		s := NewState(seed, m)
		for i := 1; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true
		}
		s.Tick = tick
		a := &s.Agents[0]
		a.Inv = Inventory{}
		s.Piles = []Pile{{X: a.X, Y: a.Y, Food: []FoodBatch{{Kind: "food_raw", N: 5, SpoilAt: tick}}}}
		return s
	}
	px, py := fresh().Agents[0].X, fresh().Agents[0].Y
	rot := store.Event{Tick: tick, Type: "sim.food_rotted", Payload: pl(FoodRottedPayload{X: px, Y: py, Kind: "food_raw", N: 5})}
	pick := store.Event{Tick: tick, Type: "agent.picked_up", Payload: pl(PickedUpPayload{Agent: 0, X: px, Y: py, Kind: "food_raw", N: 3})}

	apply := func(order []store.Event) *State {
		s := fresh()
		for _, e := range order {
			if err := s.Apply(e); err != nil {
				t.Fatalf("apply %s: %v", e.Type, err)
			}
		}
		return s
	}

	// Order A — rot lands first: it removes all 5 spoiled units, the pile is gone,
	// and the pickup (emitted against the pre-tick pile) finds nothing.
	a1, a2 := apply([]store.Event{rot, pick}), apply([]store.Event{rot, pick})
	if a1.Hash() != a2.Hash() {
		t.Fatal("rot-then-pickup did not replay deterministically")
	}
	if a1.Agents[0].Inv.FoodRaw != 0 {
		t.Errorf("rot-first: taker got %d food, want 0 (rot swept the pile before the pickup)", a1.Agents[0].Inv.FoodRaw)
	}
	if a1.pileAt(px, py) != nil {
		t.Errorf("rot-first: pile survived, want emptied and removed: %+v", a1.pileAt(px, py))
	}

	// Order B — pickup lands first: it takes 3 (oldest-batch-first), leaving 2;
	// the rot event (N:5) then clamps to the 2 spoiled units that remain.
	b1, b2 := apply([]store.Event{pick, rot}), apply([]store.Event{pick, rot})
	if b1.Hash() != b2.Hash() {
		t.Fatal("pickup-then-rot did not replay deterministically")
	}
	if b1.Agents[0].Inv.FoodRaw != 3 {
		t.Errorf("pickup-first: taker got %d food, want 3 (pickup won, rot found only the remainder)", b1.Agents[0].Inv.FoodRaw)
	}
	if b1.pileAt(px, py) != nil {
		t.Errorf("pickup-first: pile survived, want the remaining 2 units rotted away and removed: %+v", b1.pileAt(px, py))
	}
}

// TestRotReplayByteIdentity is SC-005 over US5: a run in which a pile's food
// batch reaches its deadline and the sweep removes it replays from genesis (log
// only) to a byte-identical state hash, and every surviving spoil_at deadline
// round-trips through a snapshot (Marshal → Unmarshal) unchanged.
func TestRotReplayByteIdentity(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	// One pile spoils inside the run (SpoilAt 60), a second is stamped a full
	// window out so its deadline must survive the snapshot round-trip.
	genesis := func() *State {
		s := NewState(seed, m)
		for i := range s.Agents {
			s.Agents[i].Dead = true // a quiet run: only the rot sweep and ambient beats
		}
		rx, ry := s.Agents[0].X, s.Agents[0].Y
		kx, ky := s.Agents[1].X, s.Agents[1].Y
		s.Piles = []Pile{
			{X: rx, Y: ry, Food: []FoodBatch{{Kind: "food_raw", N: 2, SpoilAt: 60}}},
			{X: kx, Y: ky, Food: []FoodBatch{{Kind: "meals", N: 1, SpoilAt: rotWindowTicks}}},
		}
		return s
	}

	const ticks = 200
	live := genesis()
	log := driveTicks(t, live, m, ticks, nil)

	var sawRot bool
	for _, e := range log {
		if e.Type == "sim.food_rotted" {
			sawRot = true
		}
	}
	if !sawRot {
		t.Fatal("scripted run never rotted its food batch")
	}

	// Snapshot round-trip: the surviving batch's spoil_at deadline is preserved
	// byte-for-byte through Marshal → Unmarshal (SC-005 rot-deadline clause).
	var snap State
	if err := json.Unmarshal(live.Marshal(), &snap); err != nil {
		t.Fatalf("snapshot unmarshal: %v", err)
	}
	if snap.Hash() != live.Hash() {
		t.Fatal("state hash changed across a snapshot round-trip")
	}
	survivor := snap.pileAt(genesis().Agents[1].X, genesis().Agents[1].Y)
	if survivor == nil || len(survivor.Food) != 1 || survivor.Food[0].SpoilAt != rotWindowTicks {
		t.Errorf("surviving spoil_at deadline did not round-trip: %+v", survivor)
	}

	// Replay the log over a fresh genesis, re-live the quiet tail, compare hashes.
	replay := genesis()
	for _, e := range log {
		if err := replay.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replay.Tick = e.Tick
	}
	driveTicks(t, replay, m, ticks, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("replay diverged:\nlive:     %s\nreplayed: %s", live.Marshal(), replay.Marshal())
	}
}

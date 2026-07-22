package sim

import (
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

// TestEatOrderingSatietyAbsolute is spec 012 US2 AC#1 + FR-007: eating consumes
// units most-nutritious-first (Meals → FoodCooked → FoodRaw) until the Food
// need reaches satiety, records the outcome as an absolute post-eat value, and
// never touches a unit once sated (the eating-overshoot edge case). The reducer
// applies the counts and the absolute need — no arithmetic that could drift.
func TestEatOrderingSatietyAbsolute(t *testing.T) {
	// Satiety stop + most-nutritious-first: 700 → 800 → 900 (2 meals), the
	// third meal untouched because 900 ≥ satietyAt.
	a := &Agent{Needs: Needs{Food: 700}, Inv: Inventory{Meals: 3}}
	p, ok := eatOutcome(a)
	if !ok {
		t.Fatal("a hungry agent with food should eat")
	}
	if p.Meals != 2 || p.Cooked != 0 || p.Raw != 0 {
		t.Errorf("eat counts = %+v, want 2 meals / 0 / 0", p)
	}
	if p.FoodAfter != 900 {
		t.Errorf("food_after = %d, want 900 (stops at satiety)", p.FoodAfter)
	}

	// Ordering across all three forms from 500: +100 meal, +80 cooked, then
	// raws (+40) until ≥ 900: 680 → 720 → 760 → 800 → 840 → 880 → 920.
	a2 := &Agent{Needs: Needs{Food: 500}, Inv: Inventory{Meals: 1, FoodCooked: 1, FoodRaw: 10}}
	p2, ok := eatOutcome(a2)
	if !ok {
		t.Fatal("should eat")
	}
	if p2.Meals != 1 || p2.Cooked != 1 || p2.Raw != 6 {
		t.Errorf("eat counts = %+v, want 1 meal / 1 cooked / 6 raw", p2)
	}
	if p2.FoodAfter != 920 {
		t.Errorf("food_after = %d, want 920", p2.FoodAfter)
	}

	// Already sated: no unit is ever consumed (overshoot edge).
	if _, ok := eatOutcome(&Agent{Needs: Needs{Food: 950}, Inv: Inventory{FoodRaw: 5}}); ok {
		t.Error("a sated agent (food ≥ satiety) must not eat")
	}
	// Empty-handed: nothing to eat.
	if _, ok := eatOutcome(&Agent{Needs: Needs{Food: 100}}); ok {
		t.Error("an empty-handed agent must not eat")
	}

	// Reducer applies the outcome absolutely: counts decremented, need set.
	s := &State{Agents: []Agent{{Needs: Needs{Food: 500}, Inv: Inventory{Meals: 1, FoodCooked: 1, FoodRaw: 10}}}}
	if err := s.Apply(store.Event{Tick: 1, Type: "agent.ate", Payload: mustPayload(p2)}); err != nil {
		t.Fatalf("apply agent.ate: %v", err)
	}
	got := s.Agents[0]
	if got.Inv.Meals != 0 || got.Inv.FoodCooked != 0 || got.Inv.FoodRaw != 4 {
		t.Errorf("post-eat inventory = %+v, want 0 meals / 0 cooked / 4 raw", got.Inv)
	}
	if got.Needs.Food != 920 {
		t.Errorf("post-eat food need = %d, want 920 (absolute)", got.Needs.Food)
	}
}

// isolateAgents kills every agent (so the reflex/needs loops are silent),
// leaving the caller to revive the ones a scenario needs.
func isolateAgents(s *State) {
	for i := range s.Agents {
		s.Agents[i].Dead = true
	}
}

// TestFireBurnoutEmitsOnce is spec 012 US2 AC#3 + FR-009/010: a fire whose fuel
// window elapses goes cold on the exact burnout tick, the sweep emits
// sim.fire_burned_out exactly once, and a cold fire grants no warmth.
func TestFireBurnoutEmitsOnce(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s) // pure fire sweep, no agent interference

	fx, fy := s.Agents[0].X, s.Agents[0].Y
	burnout := int64(500)
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy, FuelUntil: burnout})

	log := driveTicks(t, s, m, burnout+300, nil)

	count := 0
	var at int64
	for _, e := range log {
		if e.Type == "sim.fire_burned_out" {
			var p FireBurnedOutPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.X == fx && p.Y == fy {
				count++
				at = e.Tick
			}
		}
	}
	if count != 1 {
		t.Fatalf("sim.fire_burned_out fired %d times, want exactly 1", count)
	}
	if at != burnout {
		t.Errorf("burnout at tick %d, want %d (tick == FuelUntil)", at, burnout)
	}
	if warmAt(s, fx, fy, s.Tick) {
		t.Error("a burned-out fire must grant no warmth")
	}
}

// TestRefuelRelightsAndReArms is spec 012 US2 AC#4 + FR-009: refueling a cold
// fire spends one wood, relights it, extends FuelUntil by the per-wood burn
// window (absolute deadline in the payload), and re-arms the burnout sweep so
// the fire burns out again exactly once at the new deadline.
func TestRefuelRelightsAndReArms(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.Wood = 3
	fx, fy := a.X, a.Y
	// A cold fire (FuelUntil 0 — already out).
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy, FuelUntil: 0})
	// refuel_fire completes on arrival; the agent already stands on the tile.
	a.Intent = &Intent{Goal: "refuel_fire", TargetX: fx, TargetY: fy}

	log := driveTicks(t, s, m, 5, nil)

	var refuelTick, newDeadline int64
	refueled := false
	for _, e := range log {
		if e.Type == "agent.refueled" {
			var p RefueledPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent != 0 || p.X != fx || p.Y != fy {
				t.Errorf("agent.refueled payload = %+v, want agent 0 at (%d,%d)", p, fx, fy)
			}
			refueled = true
			refuelTick = e.Tick
			newDeadline = p.FuelUntil
		}
	}
	if !refueled {
		t.Fatal("no agent.refueled event emitted")
	}
	// Cold relight: the deadline is measured from the refuel tick, not the
	// stale FuelUntil (which was in the past).
	if newDeadline != refuelTick+fireBurnPerWood {
		t.Errorf("new FuelUntil = %d, want %d (refuel tick + fireBurnPerWood)", newDeadline, refuelTick+fireBurnPerWood)
	}
	if a.Inv.Wood != 2 {
		t.Errorf("wood after refuel = %d, want 2", a.Inv.Wood)
	}
	if s.Structures[0].FuelUntil != newDeadline {
		t.Errorf("structure FuelUntil = %d, want %d", s.Structures[0].FuelUntil, newDeadline)
	}
	if !warmAt(s, fx, fy, s.Tick) {
		t.Error("a relit fire should grant warmth again")
	}

	// Re-arm: freeze the agent (dead) and drive to just past the new deadline;
	// the sweep must burn the fire out exactly once, at the new deadline.
	a.Dead = true
	a.Intent = nil
	log2 := driveTicks(t, s, m, newDeadline+120, nil)
	count := 0
	var at int64
	for _, e := range log2 {
		if e.Type == "sim.fire_burned_out" {
			var p FireBurnedOutPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.X == fx && p.Y == fy {
				count++
				at = e.Tick
			}
		}
	}
	if count != 1 {
		t.Fatalf("re-armed burnout fired %d times, want exactly 1", count)
	}
	if at != newDeadline {
		t.Errorf("re-armed burnout at tick %d, want %d", at, newDeadline)
	}
}

// TestRefuelAtCapIsNoOp is the fuel-cap edge case: refueling a fire already at
// its fuel ceiling extends nothing and consumes nothing.
func TestRefuelAtCapIsNoOp(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.Wood = 2
	fx, fy := a.X, a.Y
	// Fire already at the cap relative to the completion tick (tick 1).
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy, FuelUntil: 1 + fireFuelCap})
	a.Intent = &Intent{Goal: "refuel_fire", TargetX: fx, TargetY: fy}

	log := driveTicks(t, s, m, 5, nil)
	for _, e := range log {
		if e.Type == "agent.refueled" {
			t.Error("refueling a fire at its fuel cap should emit no agent.refueled")
		}
	}
	if a.Inv.Wood != 2 {
		t.Errorf("wood = %d, want 2 (nothing consumed at cap)", a.Inv.Wood)
	}
}

// TestCookAtLitFire is spec 012 US2 AC#2 + FR-011: cooking at a lit fire turns
// up to a batch of raw food into fire-cooked food (worth double when eaten).
func TestCookAtLitFire(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.FoodRaw = 10 // over the batch cap of 8
	fx, fy := a.X, a.Y
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy, FuelUntil: s.Tick + 24*3600})
	// WorkStart preset so the first tick already satisfies the cook duration.
	a.Intent = &Intent{Goal: "cook", TargetX: fx, TargetY: fy, WorkStart: 1 - cookFireTicks}

	log := driveTicks(t, s, m, 5, nil)
	cooked := false
	for _, e := range log {
		if e.Type == "agent.cooked" {
			var p CookedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Station != "fire" || p.Kind != "food_cooked" {
				t.Errorf("cooked payload station/kind = %q/%q, want fire/food_cooked", p.Station, p.Kind)
			}
			if p.Consumed != ovenBatchSize || p.Produced != ovenBatchSize {
				t.Errorf("cooked consumed/produced = %d/%d, want %d/%d (batch cap)", p.Consumed, p.Produced, ovenBatchSize, ovenBatchSize)
			}
			cooked = true
		}
	}
	if !cooked {
		t.Fatal("no agent.cooked event at a lit fire")
	}
	if a.Inv.FoodRaw != 2 || a.Inv.FoodCooked != 8 {
		t.Errorf("post-cook inventory = %d raw / %d cooked, want 2 / 8", a.Inv.FoodRaw, a.Inv.FoodCooked)
	}
}

// TestColdFireRefusesCook is spec 012 US2 edge case (fire burns out mid-cook)
// + FR-010: a cold fire cannot cook — the completion re-validates lit-ness,
// finds it cold, and resolves without effect (the contested-resource pattern).
func TestColdFireRefusesCook(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.FoodRaw = 5
	fx, fy := a.X, a.Y
	// Cold fire (FuelUntil 0): litFireAt is false at any tick ≥ 0.
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy, FuelUntil: 0})
	a.Intent = &Intent{Goal: "cook", TargetX: fx, TargetY: fy, WorkStart: 1 - cookFireTicks}

	log := driveTicks(t, s, m, 5, nil)
	done := false
	for _, e := range log {
		if e.Type == "agent.cooked" {
			t.Error("a cold fire must not produce agent.cooked")
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
		t.Error("cook at a cold fire should resolve via agent.intent_done")
	}
	if a.Inv.FoodRaw != 5 || a.Inv.FoodCooked != 0 {
		t.Errorf("cold-fire cook changed inventory: %d raw / %d cooked, want 5 / 0", a.Inv.FoodRaw, a.Inv.FoodCooked)
	}
}

// TestReflexRefuelsDyingFire is spec 012 US2 FR-012: the reflex's one new rule —
// when carrying wood, refuel a dying or cold fire; and only then (a healthy
// fire or an empty-handed agent must not trigger it).
func TestReflexRefuelsDyingFire(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	a := &s.Agents[0]
	a.Needs = Needs{Health: 1000, Food: 600, Rest: 600, Warmth: 600, Morale: 600}
	a.Inv.Wood = 2
	fx, fy := a.X, a.Y

	// Dying fire (under refuelDyingBelow left) + wood in hand ⇒ refuel.
	s.Structures = []Structure{{Kind: "fire", X: fx, Y: fy, FuelUntil: 100}}
	if d := decideIntent(s, m, 0, 0); d.intent == nil || d.intent.Goal != "refuel_fire" {
		t.Fatalf("reflex should refuel a dying fire; got %+v", d)
	}

	// Healthy fire ⇒ the reflex must NOT refuel.
	s.Structures = []Structure{{Kind: "fire", X: fx, Y: fy, FuelUntil: 100000}}
	if d := decideIntent(s, m, 0, 0); d.intent != nil && d.intent.Goal == "refuel_fire" {
		t.Error("reflex refueled a healthy fire — should not")
	}

	// Dying fire but no wood ⇒ no refuel (the reflex never chops just to refuel).
	s.Structures = []Structure{{Kind: "fire", X: fx, Y: fy, FuelUntil: 100}}
	a.Inv.Wood = 0
	if d := decideIntent(s, m, 0, 0); d.intent != nil && d.intent.Goal == "refuel_fire" {
		t.Error("empty-handed reflex refueled a fire — should not")
	}
}

// TestReplayByteIdentityFoodFire is SC-004 over the US2 surface: a degraded-mode
// run that eats, builds/refuels/burns fires replays to byte-identical state,
// following the codebase's replay-test idiom (sim_test.go / quarry_test.go).
func TestReplayByteIdentityFoodFire(t *testing.T) {
	const seed = 42
	const ticks = 2 * 24 * 3600 // two game days: fires built, food eaten
	m := testMap(seed)

	live := NewState(seed, m)
	log := driveTicks(t, live, m, ticks, nil)

	var sawAte, sawFire, sawRefuel, sawBurnout bool
	for _, e := range log {
		switch e.Type {
		case "agent.ate":
			sawAte = true
		case "agent.built":
			var p BuiltPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Kind == "fire" {
				sawFire = true
			}
		case "agent.refueled":
			sawRefuel = true
		case "sim.fire_burned_out":
			sawBurnout = true
		}
	}
	if !sawAte {
		t.Fatal("run produced no agent.ate — the food path was not exercised")
	}
	if !sawFire {
		t.Fatal("run produced no fire — the fire path was not exercised")
	}
	t.Logf("food+fire run exercised: ate=%v fire=%v refuel=%v burnout=%v", sawAte, sawFire, sawRefuel, sawBurnout)

	replayed := NewState(seed, m)
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil) // re-live the quiet tail, as recovery does

	if live.Hash() != replayed.Hash() {
		t.Fatalf("replayed state diverged:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replayed.Marshal()))
	}
}

// TestDegradedModeVillageSurvivesThreeDays is spec 012 US2 AC#5 + SC-002 — the
// doctrine gate for this feature: a planner-less (degraded-mode) village of 8
// survives at least three full game days on the raw survival loop alone
// (forage/hunt/eat/chop/build+refuel fire), producing ZERO crafting, cooking,
// or bathing events. If survival fails at the pinned numbers, the numbers are
// the planning tier's to change — this test reports, it does not retune.
func TestDegradedModeVillageSurvivesThreeDays(t *testing.T) {
	if testing.Short() {
		t.Skip("three full game days")
	}
	for _, seed := range []uint64{42, 7, 101} {
		m := testMap(seed)
		s := NewState(seed, m)
		log := driveTicks(t, s, m, 3*24*3600, nil)

		// The subsistence contract: no civilization events ever originate from
		// the reflex.
		for _, e := range log {
			switch e.Type {
			case "agent.crafted", "agent.cooked", "agent.bathed":
				t.Errorf("seed %d: degraded mode emitted %s — the reflex must never craft/cook/bathe", seed, e.Type)
			}
		}

		alive := 0
		var causes []string
		for _, a := range s.Agents {
			if !a.Dead {
				alive++
			}
		}
		if alive != agentCount {
			for _, e := range log {
				if e.Type == "agent.died" {
					var p DiedPayload
					mustUnmarshal(t, e.Payload, &p)
					causes = append(causes, p.Cause)
				}
			}
			t.Errorf("seed %d: only %d/%d survived three days (deaths: %v)", seed, alive, agentCount, causes)
		}
	}
}

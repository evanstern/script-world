package sim

import (
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

// TestOvenBuildCost is spec 012 US4 AC#1 + T030: building an oven consumes
// 4 RefinedStone + 2 Planks (recipes.go, the single source), adds the
// structure at the site, and leaves the builder a high-salience "first
// oven" memory.
func TestOvenBuildCost(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.RefinedStone = 4
	a.Inv.Planks = 2
	site, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) })
	if !ok {
		t.Fatal("no build site reachable from agent 0's genesis position")
	}
	a.X, a.Y = site.X, site.Y
	fx, fy := a.X, a.Y
	a.Intent = &Intent{Goal: "build_oven", TargetX: fx, TargetY: fy, WorkStart: 1 - buildOvenTicks}

	log := driveTicks(t, s, m, 5, nil)
	var built, memory bool
	for _, e := range log {
		if e.Type == "agent.built" {
			var p BuiltPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Kind == "oven" && p.X == fx && p.Y == fy {
				built = true
			}
		}
		if e.Type == "agent.memory_added" {
			var p MemoryAddedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 0 && p.Salience == salOvenBuilt {
				memory = true
			}
		}
	}
	if !built {
		t.Fatal("no agent.built{oven} event emitted")
	}
	if !memory {
		t.Error("no high-salience oven-built memory for the builder")
	}
	if a.Inv.RefinedStone != 0 || a.Inv.Planks != 0 {
		t.Errorf("post-build inventory = %d refined stone / %d planks, want 0/0", a.Inv.RefinedStone, a.Inv.Planks)
	}
	if !s.structureAt("oven", fx, fy) {
		t.Error("no oven structure at the build site")
	}
}

// TestOvenCookBatch is spec 012 US4 AC#2 + T031: cooking at an oven consumes
// 1 wood fuel plus up to a batch of raw food, producing the same count of
// meals (the best per-unit food).
func TestOvenCookBatch(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.FoodRaw = 10 // over the batch cap of 8
	a.Inv.Wood = 3
	ox, oy := a.X, a.Y
	s.Structures = append(s.Structures, Structure{Kind: "oven", X: ox, Y: oy})
	a.Intent = &Intent{Goal: "cook", TargetX: ox, TargetY: oy, WorkStart: 1 - cookOvenTicks}

	log := driveTicks(t, s, m, 5, nil)
	var cooked bool
	for _, e := range log {
		if e.Type == "agent.cooked" {
			var p CookedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Station != "oven" || p.Kind != "meals" {
				t.Errorf("cooked payload station/kind = %q/%q, want oven/meals", p.Station, p.Kind)
			}
			if p.Consumed != ovenBatchSize || p.Produced != ovenBatchSize {
				t.Errorf("cooked consumed/produced = %d/%d, want %d/%d", p.Consumed, p.Produced, ovenBatchSize, ovenBatchSize)
			}
			cooked = true
		}
	}
	if !cooked {
		t.Fatal("no agent.cooked event at the oven")
	}
	if a.Inv.FoodRaw != 2 || a.Inv.Meals != ovenBatchSize || a.Inv.Wood != 2 {
		t.Errorf("post-cook inventory = %d raw / %d meals / %d wood, want 2/%d/2", a.Inv.FoodRaw, a.Inv.Meals, a.Inv.Wood, ovenBatchSize)
	}
}

// TestOvenCookNoFuelNoOp is spec 012 US4 AC#4 + FR-017: cooking at an oven
// with no carried wood resolves without effect (fuel is required from day
// one) — the contested-resource pattern extended to a missing input.
func TestOvenCookNoFuelNoOp(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.FoodRaw = 5
	a.Inv.Wood = 0
	ox, oy := a.X, a.Y
	s.Structures = append(s.Structures, Structure{Kind: "oven", X: ox, Y: oy})
	a.Intent = &Intent{Goal: "cook", TargetX: ox, TargetY: oy, WorkStart: 1 - cookOvenTicks}

	log := driveTicks(t, s, m, 5, nil)
	done := false
	for _, e := range log {
		if e.Type == "agent.cooked" {
			t.Fatal("no wood at an oven must not produce agent.cooked")
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
		t.Error("fuel-less oven cook should resolve via agent.intent_done")
	}
	if a.Inv.FoodRaw != 5 || a.Inv.Meals != 0 {
		t.Errorf("inventory changed on a no-fuel cook: %d raw / %d meals, want 5/0", a.Inv.FoodRaw, a.Inv.Meals)
	}
}

// TestBatheEffectsAbsoluteCapped is spec 012 US4 AC#3 + edge case ("bath
// while already warm/happy"): bathing consumes 1 water + 1 wood and grants
// the pinned morale/warmth bumps, capped at 1000 and recorded as absolute
// post-values — even started from near the cap, effects never overshoot.
func TestBatheEffectsAbsoluteCapped(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)

	a := &s.Agents[0]
	a.Dead = false
	a.Inv.Water = 2
	a.Inv.Wood = 2
	a.Needs.Morale = 900 // 900+150 would overshoot 1000 without the cap
	a.Needs.Warmth = 800 // 800+300 would overshoot 1000 without the cap
	ox, oy := a.X, a.Y
	s.Structures = append(s.Structures, Structure{Kind: "oven", X: ox, Y: oy})
	a.Intent = &Intent{Goal: "bathe", TargetX: ox, TargetY: oy, WorkStart: 1 - batheTicks}

	log := driveTicks(t, s, m, 5, nil)
	var bathed bool
	for _, e := range log {
		if e.Type == "agent.bathed" {
			var p BathedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.MoraleAfter != 1000 {
				t.Errorf("morale_after = %d, want 1000 (capped)", p.MoraleAfter)
			}
			if p.WarmthAfter != 1000 {
				t.Errorf("warmth_after = %d, want 1000 (capped)", p.WarmthAfter)
			}
			bathed = true
		}
	}
	if !bathed {
		t.Fatal("no agent.bathed event")
	}
	if a.Needs.Morale != 1000 || a.Needs.Warmth != 1000 {
		t.Errorf("post-bathe needs = morale %d / warmth %d, want 1000/1000", a.Needs.Morale, a.Needs.Warmth)
	}
	if a.Inv.Water != 1 || a.Inv.Wood != 1 {
		t.Errorf("post-bathe inventory = %d water / %d wood, want 1/1", a.Inv.Water, a.Inv.Wood)
	}
	var sawToned bool
	for _, mem := range a.Memories {
		if mem.Salience == salBath && mem.Tone == toneBath {
			sawToned = true
		}
	}
	if !sawToned {
		t.Error("no positive-tone bath memory recorded")
	}
}

// TestBatheNoFuelNoOp is spec 012 US4 edge case: bathing without water or
// without wood resolves via agent.intent_done only — no partial effect.
func TestBatheNoFuelNoOp(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	cases := []struct {
		name        string
		water, wood int
	}{
		{"no water", 0, 2},
		{"no wood", 2, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewState(seed, m)
			isolateAgents(s)
			a := &s.Agents[0]
			a.Dead = false
			a.Inv.Water = c.water
			a.Inv.Wood = c.wood
			a.Needs.Morale, a.Needs.Warmth = 500, 500
			ox, oy := a.X, a.Y
			s.Structures = append(s.Structures, Structure{Kind: "oven", X: ox, Y: oy})
			a.Intent = &Intent{Goal: "bathe", TargetX: ox, TargetY: oy, WorkStart: 1 - batheTicks}

			log := driveTicks(t, s, m, 5, nil)
			for _, e := range log {
				if e.Type == "agent.bathed" {
					t.Fatalf("%s: should not bathe", c.name)
				}
			}
			if a.Needs.Morale != 500 || a.Needs.Warmth != 500 {
				t.Errorf("%s: needs changed on a no-op bathe: morale %d / warmth %d", c.name, a.Needs.Morale, a.Needs.Warmth)
			}
		})
	}
}

// TestReplayByteIdentityOven is SC-004 over the US4 surface: build an oven,
// cook a meal batch, and bathe — every step a genuine injected
// agent.intent_set (planner-sourced), replaying to byte-identical state.
func TestReplayByteIdentityOven(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	genesis := func() *State {
		s := NewState(seed, m)
		isolateAgents(s)
		a := &s.Agents[0]
		a.Dead = false
		a.Inv.RefinedStone = 4
		a.Inv.Planks = 2
		a.Inv.FoodRaw = 8
		a.Inv.Water = 1
		a.Inv.Wood = 3
		// A valid build site (plain Grass, no structure) — genesis placement
		// only guarantees passable, which also admits Forage tiles.
		if site, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
			a.X, a.Y = site.X, site.Y
		}
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

	var log []store.Event
	log = append(log, driveTicks(t, live, m, live.Tick+buildOvenTicks+10, setIntent(live.Tick, "build_oven", x0, y0))...)
	log = append(log, driveTicks(t, live, m, live.Tick+cookOvenTicks+10, setIntent(live.Tick, "cook", x0, y0))...)
	log = append(log, driveTicks(t, live, m, live.Tick+batheTicks+10, setIntent(live.Tick, "bathe", x0, y0))...)

	var sawBuilt, sawCooked, sawBathed bool
	for _, e := range log {
		switch e.Type {
		case "agent.built":
			var p BuiltPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Kind == "oven" {
				sawBuilt = true
			}
		case "agent.cooked":
			sawCooked = true
		case "agent.bathed":
			sawBathed = true
		}
	}
	if !sawBuilt || !sawCooked || !sawBathed {
		t.Fatalf("run did not exercise the oven surface: built=%v cooked=%v bathed=%v", sawBuilt, sawCooked, sawBathed)
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

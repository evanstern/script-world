package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// TestReplayByteIdentityWholeFeature is SC-004 over the ENTIRE spec 012
// surface (T044, Phase 9): one scripted run for a single agent (the other
// seven isolated, per the established isolateAgents idiom) that exercises
// every new event type introduced by resources/food/crafting v2 — quarrying,
// water, the full craft chain, both cook stations, bathing, refueling (both
// a cold relight and a genuine mid-life extension), a spear breaking, a fire
// actually burning out, an absolute-payload eat, and all three new/re-costed
// agent.built kinds — then replays from genesis (log only) to a
// byte-identical state hash. world.migrated is excluded (Phase 8 covers it).
//
// Chaining idiom: every scripted goal is a genuine injected agent.intent_set
// (planner-sourced), exactly like craft_test.go/oven_test.go/quarry_test.go.
// Because a completion event clears Intent in the very same Apply that
// produces it (state.go), and the reflex only ever acts when Intent is nil
// (executor.go), injecting the next command the instant the previous one's
// completion event is observed leaves a zero-tick idle gap — comfortably
// under reflexGraceTicks(120), so the reflex never gets a window to
// preempt the script with an unplanned action. stepUntil below is that
// discovery loop: it is the same tick-by-tick stepEvents/Apply loop
// driveTicks uses, just stopping as soon as the awaited event lands instead
// of running to a pre-known tick (chaining many goals for one agent means
// each goal's travel+work ticks aren't known in advance).
func TestReplayByteIdentityWholeFeature(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	if len(m.Dens) < 3 {
		t.Fatal("test map needs at least 3 dens (three spear hunts)")
	}

	genesis := func() *State {
		s := NewState(seed, m)
		isolateAgents(s)
		a := &s.Agents[0]
		a.Dead = false
		// Below satietyAt so the very first scripted step (a direct agent.ate
		// injection, mirroring the eatOutcome-then-Apply idiom already used by
		// food_fire_test.go's TestEatOrderingSatietyAbsolute) has something to
		// do. Under the spec-013 bulk cap (24) the agent can no longer hoard
		// everything at once, so this seed is deliberately lean: 8 FoodRaw
		// (entirely eaten by the opening step) and 8 Wood — sized to the exact
		// sum this consume-as-you-go script spends (craft_spear 1 + craft_planks
		// x3 + build_fire 2 + cook_oven 1 + bathe 1 = 8), so wood lands at
		// exactly 0 right after the bath and the reflex can never (re)refuel
		// during the burnout wait. The raw food each cook batch needs is earned
		// from the spear hunts, not seeded — the cap makes gathering the source.
		a.Needs.Food = 300
		a.Inv = Inventory{Wood: 8, FoodRaw: 8}
		return s
	}

	live := genesis()

	// stepUntil advances live tick-by-tick (driveTicks' own stepEvents/Apply
	// loop) until an event satisfying match appears in that tick's batch.
	stepUntil := func(maxTick int64, match func(store.Event) bool) []store.Event {
		t.Helper()
		var out []store.Event
		for live.Tick < maxTick {
			next := live.Tick + 1
			evs := stepEvents(live, m, next)
			live.Tick = next
			for _, e := range evs {
				if err := live.Apply(e); err != nil {
					t.Fatalf("apply %s at tick %d: %v", e.Type, live.Tick, err)
				}
			}
			out = append(out, evs...)
			for _, e := range evs {
				if match(e) {
					return out
				}
			}
		}
		t.Fatalf("expected event not observed by tick %d (last tick %d)", maxTick, live.Tick)
		return out
	}

	// setIntent injects one planner-sourced agent.intent_set for agent 0 at
	// live's current tick — the same event shape the loop.go live layer emits.
	setIntent := func(goal string, tx, ty, rx, ry int) store.Event {
		t.Helper()
		e := store.Event{Tick: live.Tick, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{
			Agent: 0, Goal: goal, TargetX: tx, TargetY: ty, ResX: rx, ResY: ry, Source: "planner",
		})}
		if err := live.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
		return e
	}

	const stepBudget = 4000 // generous travel+work headroom per leg (largest duration is 1200)
	isType := func(typ string) func(store.Event) bool {
		return func(e store.Event) bool { return e.Type == typ }
	}

	var log []store.Event
	a := &live.Agents[0]

	// --- agent.ate (new AtePayload) --------------------------------------
	// Computed the same way the reducer-level unit test does (eatOutcome is a
	// pure read of Needs/Inv, no mutation), then injected directly — eat has
	// no Intent/travel of its own (contracts/events.md: "reflex/planner eat
	// (instant)"), so there is nothing to chain through the executor here.
	if p, ok := eatOutcome(a); ok {
		p.Agent = 0
		e := store.Event{Tick: live.Tick, Type: "agent.ate", Payload: mustPayload(p)}
		if err := live.Apply(e); err != nil {
			t.Fatalf("apply agent.ate: %v", err)
		}
		log = append(log, e)
	} else {
		t.Fatal("genesis agent should have something to eat")
	}

	// NOTE (spec 013 US1): the bulk cap (24) forbids hoarding, so this script
	// consumes-as-it-goes — builds spend their inputs before more are made, cook
	// batches are earned from the spear hunts rather than a big seeded larder,
	// and no single moment exceeds the cap (peak is exactly 24, at the hunts).
	// Every spec-012 event type is still exercised; only the ordering and the
	// batch sizes changed to fit under the cap.

	// --- agent.quarried x3 (US1) — 6 stone for the refining chain -----------
	for i := 0; i < 3; i++ {
		stand, res, ok := nearestAdjacentTo(m, live, a.X, a.Y, func(x, y int) bool {
			return m.InBounds(x, y) && effectiveKind(m, live, x, y) == worldmap.Rock
		})
		if !ok {
			t.Fatalf("quarry %d: no reachable rock outcrop", i)
		}
		log = append(log, setIntent("quarry", stand.X, stand.Y, res.X, res.Y))
		log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.quarried"))...)
	}
	if a.Inv.Stone != 3*quarryYield {
		t.Fatalf("Stone after 3 quarries = %d, want %d", a.Inv.Stone, 3*quarryYield)
	}

	// --- agent.crafted{refined_stone} x5 (US3) — 4 for the oven, 1 for a spear -
	for i := 0; i < 5; i++ {
		log = append(log, setIntent("craft_stone", a.X, a.Y, 0, 0))
		log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.crafted"))...)
	}
	if a.Inv.RefinedStone != 5 {
		t.Fatalf("RefinedStone after 5 craft_stone = %d, want 5", a.Inv.RefinedStone)
	}

	// --- agent.collected_water (US1) — 1 for the bath ----------------------
	{
		stand, res, ok := nearestAdjacentTo(m, live, a.X, a.Y, func(x, y int) bool {
			return m.InBounds(x, y) && effectiveKind(m, live, x, y) == worldmap.Water
		})
		if !ok {
			t.Fatal("no reachable water tile")
		}
		log = append(log, setIntent("collect_water", stand.X, stand.Y, res.X, res.Y))
		log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.collected_water"))...)
	}
	if a.Inv.Water != 1 {
		t.Fatalf("Water after collect = %d, want 1", a.Inv.Water)
	}

	// --- agent.crafted{spear} (US3) — before the planks pile up, so the
	// refined-stone bulk is spent down early (keeps the peak under the cap) ---
	log = append(log, setIntent("craft_spear", a.X, a.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.crafted"))...)
	if len(a.Inv.Spears) != 1 || a.Inv.Spears[0] != spearDurability {
		t.Fatalf("Spears after craft_spear = %v, want [%d]", a.Inv.Spears, spearDurability)
	}

	// --- agent.crafted{planks} x2 (US3) — 8 planks: 2 for the oven now, the
	// rest toward the shelter (a third batch is crafted after the oven) -------
	for i := 0; i < 2; i++ {
		log = append(log, setIntent("craft_planks", a.X, a.Y, 0, 0))
		log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.crafted"))...)
	}
	if a.Inv.Planks != 2*plankYield {
		t.Fatalf("Planks after 2 craft_planks = %d, want %d", a.Inv.Planks, 2*plankYield)
	}

	// --- agent.built{fire} (US2) ---------------------------------------------
	fireSite, ok := nearest(m, live, a.X, a.Y, func(x, y int) bool { return buildSite(m, live, x, y) })
	if !ok {
		t.Fatal("no build site reachable for the fire")
	}
	log = append(log, setIntent("build_fire", fireSite.X, fireSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.built"))...)
	fireBuiltTick := live.Tick
	if !live.structureAt("fire", fireSite.X, fireSite.Y) {
		t.Fatal("no fire structure at the build site")
	}

	// --- agent.hunted #1 (spear) — the raw food for the fire-cook, gathered
	// under the cap rather than seeded (US1: the cap makes gathering the source) -
	log = append(log, setIntent("hunt", m.Dens[0].X, m.Dens[0].Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.hunted"))...)

	// --- agent.cooked{fire} (US2) — the fire is still freshly lit ------------
	log = append(log, setIntent("cook", fireSite.X, fireSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, func(e store.Event) bool {
		if e.Type != "agent.cooked" {
			return false
		}
		var p CookedPayload
		mustUnmarshal(t, e.Payload, &p)
		return p.Station == "fire"
	})...)

	// --- agent.built{oven} (US4) — spends the 4 refined stone + 2 planks -----
	ovenSite, ok := nearest(m, live, a.X, a.Y, func(x, y int) bool { return buildSite(m, live, x, y) })
	if !ok {
		t.Fatal("no build site reachable for the oven")
	}
	log = append(log, setIntent("build_oven", ovenSite.X, ovenSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.built"))...)
	if !live.structureAt("oven", ovenSite.X, ovenSite.Y) {
		t.Fatal("no oven structure at the build site")
	}

	// --- agent.crafted{planks} x1 (US3) — top up to 10 planks for the shelter -
	log = append(log, setIntent("craft_planks", a.X, a.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.crafted"))...)

	// --- agent.built{shelter} (US5, plank-costed) --------------------------
	shelterSite, ok := nearest(m, live, a.X, a.Y, func(x, y int) bool { return buildSite(m, live, x, y) })
	if !ok {
		t.Fatal("no build site reachable for the shelter")
	}
	log = append(log, setIntent("build_shelter", shelterSite.X, shelterSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.built"))...)
	if !live.structureAt("shelter", shelterSite.X, shelterSite.Y) {
		t.Fatal("no shelter structure at the build site")
	}
	if a.Inv.RefinedStone != 0 || a.Inv.Planks != 2 {
		t.Fatalf("after oven+shelter: refined_stone=%d planks=%d, want 0/2", a.Inv.RefinedStone, a.Inv.Planks)
	}

	// --- agent.hunted #2 (spear) — the raw food for the oven-cook ------------
	log = append(log, setIntent("hunt", m.Dens[1].X, m.Dens[1].Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.hunted"))...)

	// --- agent.cooked{oven} (US4) --------------------------------------------
	log = append(log, setIntent("cook", ovenSite.X, ovenSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, func(e store.Event) bool {
		if e.Type != "agent.cooked" {
			return false
		}
		var p CookedPayload
		mustUnmarshal(t, e.Payload, &p)
		return p.Station == "oven"
	})...)

	// --- agent.bathed (US4) --------------------------------------------------
	log = append(log, setIntent("bathe", ovenSite.X, ovenSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.bathed"))...)
	if a.Inv.Water != 0 || a.Inv.Wood != 0 {
		t.Fatalf("after bathe: water=%d wood=%d, want 0/0 (exact budget)", a.Inv.Water, a.Inv.Wood)
	}

	// --- agent.hunted #3 (spear) + agent.spear_broke (US3) — the third hunt
	// spends the spear's last use (durability 3), breaking it ----------------
	log = append(log, setIntent("hunt", m.Dens[2].X, m.Dens[2].Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.hunted"))...)
	if len(a.Inv.Spears) != 0 {
		t.Fatalf("Spears after 3 hunts = %v, want empty (broke on the third)", a.Inv.Spears)
	}

	// --- sim.fire_burned_out (US2) ---------------------------------------
	// Wood is exactly 0 (spent by build_fire/craft_spear/craft_planks/cook_
	// oven/bathe above), so the reflex's one refuel rule can never fire — the
	// fire runs its full natural fuel window uncontested. The deadline is
	// deterministic (fireBuiltTick + 2*fireBurnPerWood); wait to a bit past it
	// rather than a blind large budget, to keep the unattended window short.
	burnoutDeadline := fireBuiltTick + 2*fireBurnPerWood
	log = append(log, stepUntil(burnoutDeadline+2000, func(e store.Event) bool {
		if e.Type != "sim.fire_burned_out" {
			return false
		}
		var p FireBurnedOutPayload
		mustUnmarshal(t, e.Payload, &p)
		return p.X == fireSite.X && p.Y == fireSite.Y
	})...)
	if a.Dead {
		t.Fatal("the scripted agent died during the unattended burnout wait — script needs re-tuning")
	}

	// --- agent.refueled: relighting the now-cold fire (US2) -------------------
	// Wood is 0 again after the wait (the reflex never chops — build_fire
	// already exists, so that ladder rung never triggers) — a plain "chop"
	// (pre-existing goal, not new) restocks 2 wood so the refuel is genuine
	// (a cold relight, not a no-op).
	standTree, resTree, ok := nearestAdjacentTo(m, live, a.X, a.Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, live, x, y) == worldmap.Tree
	})
	if !ok {
		t.Fatal("no reachable tree to chop for refuel wood")
	}
	log = append(log, setIntent("chop", standTree.X, standTree.Y, resTree.X, resTree.Y))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.chopped"))...)

	log = append(log, setIntent("refuel_fire", fireSite.X, fireSite.Y, 0, 0))
	log = append(log, stepUntil(live.Tick+stepBudget, isType("agent.refueled"))...)

	// --- Assert every required event type actually occurred ------------------
	seen := map[string]bool{}
	craftedKinds := map[string]bool{}
	cookedStations := map[string]bool{}
	builtKinds := map[string]bool{}
	for _, e := range log {
		seen[e.Type] = true
		switch e.Type {
		case "agent.crafted":
			var p CraftedPayload
			mustUnmarshal(t, e.Payload, &p)
			craftedKinds[p.Kind] = true
		case "agent.cooked":
			var p CookedPayload
			mustUnmarshal(t, e.Payload, &p)
			cookedStations[p.Station] = true
		case "agent.built":
			var p BuiltPayload
			mustUnmarshal(t, e.Payload, &p)
			builtKinds[p.Kind] = true
		}
	}
	required := []string{
		"agent.quarried", "agent.collected_water", "agent.crafted", "agent.built",
		"agent.ate", "agent.cooked", "agent.bathed", "agent.refueled",
		"agent.spear_broke", "sim.fire_burned_out",
	}
	for _, typ := range required {
		if !seen[typ] {
			t.Errorf("required event type %q never occurred in the scripted run", typ)
		}
	}
	for _, kind := range []string{"planks", "refined_stone", "spear"} {
		if !craftedKinds[kind] {
			t.Errorf("agent.crafted never occurred with kind %q", kind)
		}
	}
	for _, station := range []string{"fire", "oven"} {
		if !cookedStations[station] {
			t.Errorf("agent.cooked never occurred with station %q", station)
		}
	}
	for _, kind := range []string{"fire", "oven", "shelter"} {
		if !builtKinds[kind] {
			t.Errorf("agent.built never occurred with kind %q", kind)
		}
	}

	// --- Replay from genesis (log only) to a byte-identical state hash -----
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

// TestReplayByteIdentityWholeFeatureStorage is SC-005 over the ENTIRE spec 013
// surface (T038, Phase 9): ONE scripted run that exercises every new event type
// introduced by inventory/storage v1 — agent.dropped, agent.picked_up,
// agent.deposited, agent.withdrew (an owner fetch AND a non-owner theft with its
// full companion batch: social.chest_taken, a reason-"theft" social.relation_
// changed, and owner + witness agent.memory_added), sim.food_rotted, and
// agent.built{kind: chest} — plus a death spill — then replays from genesis (log
// only) to a byte-identical state hash. It complements (does not replace) the
// spec-012 whole-feature test above, which still exercises its own event set.
//
// Idiom: the loop's driveTicks + a command timeline (ground_pile_test.go /
// chest_test.go / theft_test.go), so every scripted goal is a genuine
// planner-sourced agent.intent_set driven to completion through the executor,
// exactly as the live layer does. Scripted intents are spaced so no idle gap for
// a scripted agent exceeds reflexGraceTicks(120) before its next command, so the
// reflex never preempts the script; any trailing reflex churn is post-script,
// deterministic, and reproduced identically on replay.
func TestReplayByteIdentityWholeFeatureStorage(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	// Fixed tiles from the genesis spawns: the thief/chest tile (t1), the
	// drop/pickup tile (t2), the death-spill tile (t3), and the rot-pile tile
	// (t6). The live chest is built on a scanned build tile kept clear of all of
	// them so no pile ever blocks the build (FR-007).
	base := NewState(seed, m)
	t1x, t1y := base.Agents[1].X, base.Agents[1].Y
	t2x, t2y := base.Agents[2].X, base.Agents[2].Y
	t3x, t3y := base.Agents[3].X, base.Agents[3].Y
	t6x, t6y := base.Agents[6].X, base.Agents[6].Y
	avoid := map[[2]int]bool{{t1x, t1y}: true, {t2x, t2y}: true, {t3x, t3y}: true, {t6x, t6y}: true}
	bx, by, ok := -1, -1, false
	for yy := 0; yy < m.H && !ok; yy++ {
		for xx := 0; xx < m.W; xx++ {
			if buildSite(m, base, xx, yy) && !avoid[[2]int{xx, yy}] {
				bx, by, ok = xx, yy, true
				break
			}
		}
	}
	if !ok {
		t.Skip("no buildable tile clear of the scripted pile tiles on this map")
	}

	genesis := func() *State {
		s := NewState(seed, m)
		// Quiet the crowd to the scripted cast (0,1,2,3,4); agent 6's tile hosts
		// the rot pile, so it is dead too.
		s.Agents[5].Dead = true
		s.Agents[6].Dead = true
		s.Agents[7].Dead = true
		// Agent 0: owner/builder — stands on the build tile, holds planks for the
		// chest (6) plus wood to deposit and fetch back.
		a0 := &s.Agents[0]
		a0.X, a0.Y = bx, by
		a0.Inv = Inventory{Planks: 8, Wood: 6}
		// Agent 1: the thief — co-located with the genesis chest C1.
		s.Agents[1].Inv = Inventory{}
		// Agent 4: a living, awake witness co-located with C1 (distance 0 ≤
		// witnessRadius), so the theft batch includes a witness memory.
		s.Agents[4].Dead = false
		s.Agents[4].X, s.Agents[4].Y = t1x, t1y
		// Agent 2: dropper/picker on its own tile.
		s.Agents[2].Inv = Inventory{Wood: 8}
		// Agent 3: dies carrying goods → a death-spill pile (spears riding along).
		s.Agents[3].Inv = Inventory{Wood: 5, FoodRaw: 2, Spears: []int{2}}
		// C1: a genesis chest owned by agent 0, on the thief's tile, stocked so a
		// non-owner withdrawal is a real theft.
		s.Structures = append(s.Structures, Structure{
			Kind: "chest", X: t1x, Y: t1y, Owner: 0, Store: &Inventory{Wood: 10},
		})
		// A ground food batch stamped to spoil early (tick 60), on the dead
		// agent 6's tile, so the per-game-minute rot sweep fires within the run.
		s.Piles = []Pile{{X: t6x, Y: t6y, Food: []FoodBatch{{Kind: "food_raw", N: 4, SpoilAt: 60}}}}
		return s
	}

	pl := func(v any) []byte { return mustPayload(v) }
	commands := map[int64][]store.Event{
		30: {
			// Agent 0 builds the live chest C2 (fire-comparable duration).
			{Tick: 30, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
				Agent: 0, Goal: "build_chest", TargetX: bx, TargetY: by, Source: "planner"})},
			// Agent 2 drops 4 wood onto its tile → a ground pile.
			{Tick: 30, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
				Agent: 2, Goal: "drop", TargetX: t2x, TargetY: t2y, Kind: "wood", Qty: 4, Source: "planner"})},
			// Agent 1 (non-owner) withdraws from C1 → theft companion batch.
			{Tick: 30, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
				Agent: 1, Goal: "withdraw", TargetX: t1x, TargetY: t1y, Kind: "wood", Qty: 3, Source: "planner"})},
		},
		// Agent 2 picks the wood back up (< 120 ticks after the drop).
		90: {{Tick: 90, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 2, Goal: "pick_up", TargetX: t2x, TargetY: t2y, Kind: "wood", Source: "planner"})}},
		// Agent 3 dies with goods → death spill.
		100: {{Tick: 100, Type: "agent.died", Payload: pl(DiedPayload{Agent: 3, Cause: "starvation"})}},
		// Owner deposits into C2, then fetches from it (own chest → no social).
		700: {{Tick: 700, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 0, Goal: "deposit", TargetX: bx, TargetY: by, Kind: "wood", Qty: 3, Source: "planner"})}},
		760: {{Tick: 760, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 0, Goal: "withdraw", TargetX: bx, TargetY: by, Kind: "wood", Qty: 2, Source: "planner"})}},
	}

	const ticks = 900
	live := genesis()
	log := driveTicks(t, live, m, ticks, commands)

	// Every new 013 event type actually occurred, and the two withdrawal shapes
	// (owner fetch vs. non-owner theft) are both present.
	seen := map[string]bool{}
	builtKinds := map[string]bool{}
	var withdrewOwnerFetch, withdrewTheft, sawTheftReason bool
	for _, e := range log {
		seen[e.Type] = true
		switch e.Type {
		case "agent.built":
			var p BuiltPayload
			mustUnmarshal(t, e.Payload, &p)
			builtKinds[p.Kind] = true
		case "agent.withdrew":
			var p WithdrewPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == p.Owner {
				withdrewOwnerFetch = true
			} else {
				withdrewTheft = true
			}
		case "social.relation_changed":
			var p RelationChangedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Reason == "theft" {
				sawTheftReason = true
			}
		}
	}
	required := []string{
		"agent.dropped", "agent.picked_up", "agent.deposited", "agent.withdrew",
		"social.chest_taken", "sim.food_rotted", "agent.built",
		"agent.memory_added", "social.relation_changed",
	}
	for _, typ := range required {
		if !seen[typ] {
			t.Errorf("required event type %q never occurred in the scripted storage run", typ)
		}
	}
	if !builtKinds["chest"] {
		t.Error("agent.built never occurred with kind \"chest\"")
	}
	if !withdrewOwnerFetch {
		t.Error("no owner self-fetch agent.withdrew (agent == owner) occurred")
	}
	if !withdrewTheft {
		t.Error("no non-owner theft agent.withdrew (agent != owner) occurred")
	}
	if !sawTheftReason {
		t.Error("no reason-\"theft\" social.relation_changed occurred in the companion batch")
	}
	if !live.Agents[3].Dead || live.pileAt(t3x, t3y) == nil {
		t.Error("agent 3's death did not leave a spill pile at its tile")
	}

	// Replay the log over a fresh genesis, re-live the quiet tail, compare hashes
	// (SC-005): byte-identical including piles, chests, owners, and rot deadlines.
	replay := genesis()
	for _, e := range log {
		if err := replay.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replay.Tick = e.Tick
	}
	driveTicks(t, replay, m, ticks, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("storage replay diverged:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replay.Marshal()))
	}
}

// TestStorageEventsNoOpUnderUnknownConvention is the SC-005 forward-compat clause
// (T038): every new 013 event type no-ops under pre-feature replay code. A
// pre-013 reducer has no case for these types, so — by the reducer's unknown-type
// convention (Apply's switch has no default; an unmatched type returns nil,
// mutating nothing, exactly like daemon.* and foreign migration records) — each
// is recorded history with zero state effect. We can't instantiate the retired
// reducer, so we mirror the convention faithfully: feed each new-013 payload
// under a type string the current switch does NOT recognize and assert the
// reducer neither errors nor mutates, on a substrate where the real 013 case
// WOULD have (a stocked chest, a pile, carried goods). That is precisely the
// behavior old code exhibits for these types.
func TestStorageEventsNoOpUnderUnknownConvention(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	substrate := func() *State {
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = Inventory{Wood: 6, FoodRaw: 4}
		s.Structures = append(s.Structures, Structure{
			Kind: "chest", X: a.X, Y: a.Y, Owner: 1, Store: &Inventory{Wood: 8},
		})
		s.Piles = []Pile{{X: a.X, Y: a.Y, Wood: 5, Food: []FoodBatch{{Kind: "food_raw", N: 3, SpoilAt: 60}}}}
		return s
	}

	x, y := substrate().Agents[0].X, substrate().Agents[0].Y
	cases := []struct {
		typ     string
		payload any
	}{
		{"agent.dropped", DroppedPayload{Agent: 0, X: x, Y: y, Kind: "wood", N: 3}},
		{"agent.picked_up", PickedUpPayload{Agent: 0, X: x, Y: y, Kind: "wood", N: 3}},
		{"agent.deposited", DepositedPayload{Agent: 0, X: x, Y: y, Kind: "wood", N: 3}},
		{"agent.withdrew", WithdrewPayload{Agent: 0, X: x, Y: y, Kind: "wood", N: 3, Owner: 1}},
		{"social.chest_taken", ChestTakenPayload{Owner: 1, Taker: 0, X: x, Y: y}},
		{"sim.food_rotted", FoodRottedPayload{X: x, Y: y, Kind: "food_raw", N: 3}},
	}
	for _, c := range cases {
		s := substrate()
		before := s.Hash()
		// "unknown:" prefix ⇒ the switch does not match, exactly as a pre-013
		// reducer sees these type names: fall through to the total no-op.
		e := store.Event{Tick: 61, Type: "unknown:" + c.typ, Payload: mustPayload(c.payload)}
		if err := s.Apply(e); err != nil {
			t.Errorf("%s under the unknown-type convention errored, want a total no-op: %v", c.typ, err)
		}
		if s.Hash() != before {
			t.Errorf("%s under the unknown-type convention mutated state, want a total no-op", c.typ)
		}
	}
}

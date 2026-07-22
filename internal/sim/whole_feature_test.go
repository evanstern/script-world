package sim

import (
	"testing"

	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
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

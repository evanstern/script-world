package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Bulk-cap audit (spec 013 US1, research R2). Every inventory-increasing edge
// touched by this feature has a row here: gather truncation at partial space
// (reducer clamp), zero-space no-event/no-depletion (executor guard), a craft
// whose net bulk won't fit (intent cleared, no event), a give skipped at a full
// receiver, eating freeing bulk, and the standing assertion that every
// cook/bathe/build recipe has a non-positive net bulk delta (so none of them
// ever needs a cap check).

// findForageTile scans for a Forage tile — a fresh state has no Harvested
// overlay, so map kind == effectiveKind there.
func findForageTile(m *worldmap.Map) (x, y int, ok bool) {
	for yy := 0; yy < m.H; yy++ {
		for xx := 0; xx < m.W; xx++ {
			if m.At(xx, yy) == worldmap.Forage {
				return xx, yy, true
			}
		}
	}
	return 0, 0, false
}

// TestBulkYieldTruncatesAtPartialSpace is US1-AS2: a gather completion whose
// yield would overflow the cap is clamped by the reducer to the remaining free
// bulk, and the overlay/depletion still applies (the remainder is forfeit). One
// row per gather event, each with exactly one free bulk before the event.
func TestBulkYieldTruncatesAtPartialSpace(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	apply := func(t *testing.T, s *State, typ string, pl any) {
		t.Helper()
		e := store.Event{Tick: 1, Type: typ, Payload: mustPayload(pl)}
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", typ, err)
		}
	}

	t.Run("forage", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = Inventory{Wood: bulkCap - 1} // free 1; forage yields forageYieldV2 (2)
		apply(t, s, "agent.foraged", HarvestPayload{Agent: 0, X: 3, Y: 4})
		if a.Inv.FoodRaw != 1 {
			t.Errorf("FoodRaw = %d, want 1 (truncated from %d)", a.Inv.FoodRaw, forageYieldV2)
		}
		if bulk(a.Inv) != bulkCap {
			t.Errorf("bulk = %d, want %d", bulk(a.Inv), bulkCap)
		}
		if len(s.Harvested) != 1 || s.Harvested[0].X != 3 || s.Harvested[0].Y != 4 {
			t.Errorf("Harvested = %+v, want the forage tile marked despite truncation", s.Harvested)
		}
	})

	t.Run("chop", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		// Spec 032 US2: with an axe a chop yields chopYieldAxe (3); one free bulk
		// truncates it to 1. The axe occupies a bulk itself, so wood starts at
		// cap-2 (22 wood + 1 axe = 23, one free).
		a.Inv = Inventory{Wood: bulkCap - 2, Axes: []int{axeDurability}}
		apply(t, s, "agent.chopped", HarvestPayload{Agent: 0, X: 5, Y: 6})
		if a.Inv.Wood != bulkCap-1 { // 22 + min(3,1) = 23
			t.Errorf("Wood = %d, want %d (axe yield 3 truncated to one free bulk)", a.Inv.Wood, bulkCap-1)
		}
		if len(s.Cleared) != 1 {
			t.Errorf("Cleared = %+v, want the tree cleared despite truncation", s.Cleared)
		}
	})

	t.Run("quarry", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		// Spec 032 US2: axe quarry yields quarryYieldAxe (3), truncated to one free.
		a.Inv = Inventory{Wood: bulkCap - 2, Axes: []int{axeDurability}}
		apply(t, s, "agent.quarried", HarvestPayload{Agent: 0, X: 7, Y: 8})
		if a.Inv.Stone != 1 { // 22 wood + 1 stone + 1 axe = 24 = cap
			t.Errorf("Stone = %d, want 1 (axe yield 3 truncated to one free bulk)", a.Inv.Stone)
		}
		if len(s.Quarried) != 1 {
			t.Errorf("Quarried = %+v, want the outcrop depleted despite truncation", s.Quarried)
		}
	})

	t.Run("collect_water", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = Inventory{Wood: bulkCap - 1}
		apply(t, s, "agent.collected_water", HarvestPayload{Agent: 0, X: 9, Y: 9})
		if a.Inv.Water != 1 { // collectWaterYield is already 1; still exercises the clamp path
			t.Errorf("Water = %d, want 1", a.Inv.Water)
		}
	})

	t.Run("hunt_bare_truncates", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = Inventory{Wood: bulkCap - 1} // free 1; bare hunt yields huntYieldBare (8)
		apply(t, s, "agent.hunted", HarvestPayload{Agent: 0, X: 2, Y: 2})
		if a.Inv.FoodRaw != 1 {
			t.Errorf("FoodRaw = %d, want 1 (truncated from %d)", a.Inv.FoodRaw, huntYieldBare)
		}
		if len(s.DenUses) != 1 {
			t.Errorf("DenUses = %+v, want the den on cooldown despite truncation", s.DenUses)
		}
	})

	t.Run("hunt_spear_truncates_and_still_spends_spear", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		// Wood 22 + one spear (1 bulk) = 23 → free 1; spear hunt yields 12.
		a.Inv = Inventory{Wood: bulkCap - 2, Spears: []int{3}}
		apply(t, s, "agent.hunted", HarvestPayload{Agent: 0, X: 2, Y: 2})
		if a.Inv.FoodRaw != 1 {
			t.Errorf("FoodRaw = %d, want 1 (truncated from %d)", a.Inv.FoodRaw, huntYieldSpear)
		}
		if len(a.Inv.Spears) != 1 || a.Inv.Spears[0] != 2 {
			t.Errorf("Spears = %v, want [2] (a use spent even on a truncated hunt)", a.Inv.Spears)
		}
		if bulk(a.Inv) != bulkCap {
			t.Errorf("bulk = %d, want %d", bulk(a.Inv), bulkCap)
		}
	})
}

// TestBulkZeroSpaceGatherNoEventNoDepletion is US1-AS1: a gather completed with
// zero free bulk emits agent.intent_done only — no harvest event, and no
// depletion (the forage tile is left for later). Driven through the executor so
// the guard, not just the reducer clamp, is exercised.
func TestBulkZeroSpaceGatherNoEventNoDepletion(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	fx, fy, ok := findForageTile(m)
	if !ok {
		t.Skip("no forage tile on this map")
	}
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.X, a.Y = fx, fy
	a.Inv = Inventory{Wood: bulkCap} // full: zero free bulk
	// WorkStart pre-set so the work is already complete on tick 1 (quarry-test idiom).
	a.Intent = &Intent{Goal: "forage", TargetX: fx, TargetY: fy, WorkStart: 1 - forageTicks}

	log := driveTicks(t, s, m, 3, nil)

	var sawForaged, sawDone bool
	for _, e := range log {
		switch e.Type {
		case "agent.foraged":
			var p HarvestPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 0 {
				sawForaged = true
			}
		case "agent.intent_done":
			var p AgentPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 0 {
				sawDone = true
			}
		}
	}
	if sawForaged {
		t.Error("a full-pouch forage emitted agent.foraged — it must not happen at zero free bulk")
	}
	if !sawDone {
		t.Error("a full-pouch forage did not resolve via agent.intent_done")
	}
	if len(s.Harvested) != 0 {
		t.Errorf("Harvested = %+v, want empty — no depletion at zero free bulk (US1-AS1)", s.Harvested)
	}
	if s.Agents[0].Intent != nil {
		t.Error("intent should be cleared after the no-space gather resolves")
	}
	if a.Inv.FoodRaw != 0 {
		t.Errorf("FoodRaw = %d, want 0 — nothing gathered into a full pouch", a.Inv.FoodRaw)
	}
}

// TestBulkCraftNoFitClearsIntent is the R2 craft row: a craft doesn't truncate —
// its whole net bulk delta must fit or it doesn't happen (intent cleared, no
// agent.crafted). craft_planks is the only positive-net hand-craft (+3).
func TestBulkCraftNoFitClearsIntent(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	drive := func(t *testing.T, inv Inventory) []store.Event {
		t.Helper()
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = inv
		a.Intent = &Intent{Goal: "craft_planks", TargetX: a.X, TargetY: a.Y, WorkStart: 1 - craftPlanksTicks}
		return driveTicks(t, s, m, 3, nil)
	}

	// Sanity: the constant this test hinges on.
	if r, _ := recipeFor("craft_planks"); craftNetBulk(r) != plankYield-1 {
		t.Fatalf("craft_planks net bulk = %d, want %d", craftNetBulk(r), plankYield-1)
	}

	t.Run("no_fit", func(t *testing.T) {
		// Wood 1 (the input) + Stone 21 = bulk 22 → free 2 < net 3: no fit.
		log := drive(t, Inventory{Wood: 1, Stone: bulkCap - 3})
		for _, e := range log {
			if e.Type == "agent.crafted" {
				t.Fatal("craft_planks emitted agent.crafted with no room for the net bulk delta")
			}
		}
		var sawDone bool
		for _, e := range log {
			if e.Type == "agent.intent_done" {
				sawDone = true
			}
		}
		if !sawDone {
			t.Error("a no-fit craft did not resolve via agent.intent_done")
		}
	})

	t.Run("fits_exactly", func(t *testing.T) {
		// Wood 1 + Stone 20 = bulk 21 → free 3 == net 3: it just fits.
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = Inventory{Wood: 1, Stone: bulkCap - 4}
		a.Intent = &Intent{Goal: "craft_planks", TargetX: a.X, TargetY: a.Y, WorkStart: 1 - craftPlanksTicks}
		log := driveTicks(t, s, m, 3, nil)
		var sawCrafted bool
		for _, e := range log {
			if e.Type == "agent.crafted" {
				sawCrafted = true
			}
		}
		if !sawCrafted {
			t.Fatal("craft_planks with exactly enough room did not craft")
		}
		if a.Inv.Planks != plankYield || a.Inv.Wood != 0 {
			t.Errorf("post-craft Inv = %+v, want %d planks and 0 wood", a.Inv, plankYield)
		}
		if bulk(a.Inv) != bulkCap {
			t.Errorf("post-craft bulk = %d, want %d (filled exactly to the cap)", bulk(a.Inv), bulkCap)
		}
	})
}

// TestBulkGiveSkippedAtFullReceiver is the R2 give row: the executor's give rule
// skips a receiver with zero free bulk (the gift would overflow the cap). Only
// agents 0 and 1 are left alive so the single social beat is unambiguous.
func TestBulkGiveSkippedAtFullReceiver(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	setup := func(recvInv Inventory) *State {
		s := NewState(seed, m)
		for i := 2; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true // isolate the (0,1) pair
		}
		a, b := &s.Agents[0], &s.Agents[1]
		a.X, a.Y = 1, 1
		b.X, b.Y = 1, 2 // Manhattan 1: adjacent
		a.Needs.Food = giveNeedBelow - 50 // starving receiver
		a.Inv = recvInv
		b.Needs.Food = 800 // the giver is fine
		b.Inv = Inventory{FoodRaw: 5}
		b.LastGive = 0
		return s
	}

	gaveCount := func(evs []store.Event) int {
		n := 0
		for _, e := range evs {
			if e.Type == "social.gave" {
				n++
			}
		}
		return n
	}

	// Full receiver: no give.
	full := setup(Inventory{Wood: bulkCap})
	if n := gaveCount(socialEvents(full, 90)); n != 0 {
		t.Errorf("a give landed on a full receiver (%d social.gave) — it must be skipped under the cap", n)
	}

	// Control: the same starving receiver with room gets fed.
	room := setup(Inventory{})
	if n := gaveCount(socialEvents(room, 90)); n != 1 {
		t.Errorf("a starving receiver with free bulk was not fed (%d social.gave, want 1)", n)
	}
}

// TestBulkGaveReducerClampsDefensively is the defensive half of the give row: a
// forged social.gave into a full pouch never pushes carried bulk over the cap
// (FR-001), even though the executor would never emit it.
func TestBulkGaveReducerClampsDefensively(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	giver, recv := &s.Agents[0], &s.Agents[1]
	giver.Inv = Inventory{FoodRaw: 5}
	recv.Inv = Inventory{Wood: bulkCap} // already at the cap

	e := store.Event{Tick: 1, Type: "social.gave", Payload: mustPayload(GavePayload{From: 0, To: 1, Kind: "food"})}
	if err := s.Apply(e); err != nil {
		t.Fatalf("apply social.gave: %v", err)
	}
	if bulk(recv.Inv) > bulkCap {
		t.Errorf("receiver bulk = %d, want <= %d — the reducer must clamp defensively", bulk(recv.Inv), bulkCap)
	}
	if recv.Inv.FoodRaw != 0 {
		t.Errorf("receiver FoodRaw = %d, want 0 — the over-cap unit is not added", recv.Inv.FoodRaw)
	}
}

// TestBulkEatingFreesBulk is US1-AS3: eating consumed units frees carried bulk
// immediately, so a full pouch is never a permanent deadlock.
func TestBulkEatingFreesBulk(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{FoodRaw: bulkCap} // full, all food
	a.Needs.Food = 100                  // hungry enough to eat toward satiety
	before := bulk(a.Inv)
	if before != bulkCap {
		t.Fatalf("setup bulk = %d, want %d", before, bulkCap)
	}
	p, ok := eatOutcome(a)
	if !ok {
		t.Fatal("a hungry, food-carrying agent produced no eat outcome")
	}
	p.Agent = 0
	e := store.Event{Tick: 1, Type: "agent.ate", Payload: mustPayload(p)}
	if err := s.Apply(e); err != nil {
		t.Fatalf("apply agent.ate: %v", err)
	}
	after := bulk(a.Inv)
	if after >= before {
		t.Errorf("bulk after eating = %d, want < %d (eating frees bulk)", after, before)
	}
	if after != before-(p.Meals+p.Cooked+p.Raw) {
		t.Errorf("bulk freed = %d, want %d (one per consumed unit)", before-after, p.Meals+p.Cooked+p.Raw)
	}
}

// TestNonGatherRecipesNetBulkNonPositive is the R2 "no check needed" row: every
// cook/bathe/build/refuel recipe has a net bulk delta <= 0, so none of them can
// ever overflow the cap (the tests assert what the executor relies on). Of the
// hand-crafts, only craft_planks is positive — the single row that gets a cap
// guard (T012).
func TestNonGatherRecipesNetBulkNonPositive(t *testing.T) {
	for _, r := range recipes {
		net := craftNetBulk(r)
		switch r.Goal {
		case "craft_planks":
			if net <= 0 {
				t.Errorf("%s net bulk = %d, want > 0 (the one positive-net hand-craft)", r.Goal, net)
			}
		default:
			if net > 0 {
				t.Errorf("%s net bulk = %d, want <= 0 — a positive net needs a cap guard", r.Goal, net)
			}
		}
	}
	// Spot-check the two other hand-crafts named in research R2.
	if r, _ := recipeFor("craft_stone"); craftNetBulk(r) != 0 {
		t.Errorf("craft_stone net bulk = %d, want 0", craftNetBulk(r))
	}
	if r, _ := recipeFor("craft_spear"); craftNetBulk(r) >= 0 {
		t.Errorf("craft_spear net bulk = %d, want < 0", craftNetBulk(r))
	}
}

// TestDegradedModeVillageSurvivesUnderBulkCap is SC-001 / FR-003 — the doctrine
// gate for this feature: a planner-less village of 8 survives at least three
// full game days under the bulk cap with ZERO storage events in the log (the
// five new storage goals are planner-only, so the reflex never reaches them),
// and the cap never deadlocks the raw survival loop. Modeled on spec 012's
// degraded-mode gate (food_fire_test.go). If survival fails at the pinned
// numbers, the numbers are the planning tier's to change — this test reports.
func TestDegradedModeVillageSurvivesUnderBulkCap(t *testing.T) {
	if testing.Short() {
		t.Skip("three full game days")
	}
	storage := map[string]bool{
		"agent.dropped":      true,
		"agent.picked_up":    true,
		"agent.deposited":    true,
		"agent.withdrew":     true,
		"social.chest_taken": true,
		"sim.food_rotted":    true,
	}
	for _, seed := range []uint64{42, 7, 101} {
		m := testMap(seed)
		s := NewState(seed, m)
		log := driveTicks(t, s, m, 3*24*3600, nil)

		// SC-001: the subsistence loop never touches storage.
		for _, e := range log {
			if storage[e.Type] {
				t.Errorf("seed %d: degraded mode emitted storage event %s — SC-001 requires zero", seed, e.Type)
			}
		}

		alive := 0
		for _, a := range s.Agents {
			if !a.Dead {
				alive++
			}
		}
		if alive != agentCount {
			var causes []string
			for _, e := range log {
				if e.Type == "agent.died" {
					var p DiedPayload
					mustUnmarshal(t, e.Payload, &p)
					causes = append(causes, p.Cause)
				}
			}
			t.Errorf("seed %d: only %d/%d survived three days under the bulk cap (deaths: %v) — the cap deadlocked the raw loop",
				seed, alive, agentCount, causes)
		}
		// The cap must never leave a villager pinned over it.
		for i, a := range s.Agents {
			if bulk(a.Inv) > bulkCap {
				t.Errorf("seed %d: agent %d carries %d bulk, over the cap of %d", seed, i, bulk(a.Inv), bulkCap)
			}
		}
	}
}

package sim

import (
	"fmt"

	"github.com/evanstern/script-world/internal/worldmap"
)

// The reflex policy: a deterministic pure function deciding what an idle,
// awake agent does next. It is the stand-in for the LLM planner until TASK-7
// — and stays forever as the degraded-mode fallback (the executor must keep
// bodies alive when no planner thoughts arrive).
//
// Priority order: eat → get food → survive the night (fire/warmth) → rest →
// prepare (wood, fire, shelter, stockpile) → wander.
//
// It returns either a direct event (eating happens instantly) or an intent.

type decision struct {
	directEvent string // "agent.ate" or ""
	intent      *Intent
}

func decideIntent(s *State, m *worldmap.Map, idx int, tick int64) decision {
	a := &s.Agents[idx]

	// Eat from inventory the moment hunger bites (most-nutritious-first, T018).
	if a.Needs.Food < hungryAt && hasAnyFood(a) {
		return decision{directEvent: "agent.ate"}
	}

	// Hungry with nothing carried: get food (forage first, hunt as backup).
	if a.Needs.Food < hungryAt {
		if d, ok := foodIntent(s, m, a, tick); ok {
			return decision{intent: d}
		}
	}

	if s.Night {
		if !warmAt(s, a.X, a.Y, tick) {
			// Reach warmth, or make it, or get the wood to make it.
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y, tick) && passable(m, s, x, y) }); ok {
				return decision{intent: &Intent{Goal: "goto_warmth", TargetX: p.X, TargetY: p.Y}}
			}
			// The one reflex addition (T020, FR-012): a cold/dying fire nearby
			// and wood in hand — relight it (cheaper than a fresh build).
			if in, ok := reflexRefuelIntent(s, m, a, tick); ok {
				return decision{intent: in}
			}
			if a.Inv.Wood >= fireWoodCost {
				if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
					return decision{intent: &Intent{Goal: "build_fire", TargetX: p.X, TargetY: p.Y}}
				}
			}
			if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
				return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Tree
			}); ok {
				return decision{intent: &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}}
			}
		}
		// Warm (or nothing to be done about it): sleep where you stand.
		return decision{intent: &Intent{Goal: "sleep", TargetX: a.X, TargetY: a.Y}}
	}

	// Daytime. Exhausted agents nap somewhere warm if possible.
	if a.Needs.Rest < tiredAt {
		tx, ty := a.X, a.Y
		if !warmAt(s, tx, ty, tick) {
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y, tick) && passable(m, s, x, y) }); ok {
				tx, ty = p.X, p.Y
			}
		}
		return decision{intent: &Intent{Goal: "sleep", TargetX: tx, TargetY: ty}}
	}

	// Village prep: a fire before the first night, then keep it burning, then a
	// full larder. Shelter-building is planner-only now (T020, FR-012): it costs
	// planks, and the reflex never enters the crafting economy.
	if !s.hasStructure("fire") {
		if a.Inv.Wood >= fireWoodCost {
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return decision{intent: &Intent{Goal: "build_fire", TargetX: p.X, TargetY: p.Y}}
			}
		}
		if d, ok := chopIntent(s, m, a); ok {
			return decision{intent: d}
		}
	}
	// Keep the fire alive (T020): top up a dying/cold fire while carrying wood.
	if in, ok := reflexRefuelIntent(s, m, a, tick); ok {
		return decision{intent: in}
	}
	if a.Inv.FoodRaw < stockFoodRawTo {
		if d, ok := foodIntent(s, m, a, tick); ok {
			return decision{intent: d}
		}
	}

	// Nothing urgent: wander a little (seeded, tick-pure).
	r := rngAt(s.Seed, "wander", tick, idx)
	for try := 0; try < 8; try++ {
		dx := int(r.Uint64N(9)) - 4
		dy := int(r.Uint64N(9)) - 4
		if dx == 0 && dy == 0 {
			continue
		}
		if passable(m, s, a.X+dx, a.Y+dy) {
			return decision{intent: &Intent{Goal: "wander", TargetX: a.X + dx, TargetY: a.Y + dy}}
		}
	}
	return decision{} // stay idle this round
}

// hasAnyFood reports whether the agent carries any edible unit (T018): the
// eat/wake checks key on the full triplet, not raw food alone.
func hasAnyFood(a *Agent) bool {
	return a.Inv.Meals+a.Inv.FoodCooked+a.Inv.FoodRaw > 0
}

// reflexRefuelIntent is the reflex's one new rule (T020, FR-012): when carrying
// wood, refuel the nearest fire that is cold (tick ≥ FuelUntil) or dying
// (under refuelDyingBelow left). Returns no intent when the agent has no wood
// or no such fire is reachable — the reflex never chops just to refuel.
func reflexRefuelIntent(s *State, m *worldmap.Map, a *Agent, tick int64) (*Intent, bool) {
	if a.Inv.Wood < 1 {
		return nil, false
	}
	if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
		st, ok := fireStructAt(s, x, y)
		return ok && st.FuelUntil-tick < refuelDyingBelow
	}); ok {
		return &Intent{Goal: "refuel_fire", TargetX: p.X, TargetY: p.Y}, true
	}
	return nil, false
}

func foodIntent(s *State, m *worldmap.Map, a *Agent, tick int64) (*Intent, bool) {
	if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
		return effectiveKind(m, s, x, y) == worldmap.Forage
	}); ok {
		return &Intent{Goal: "forage", TargetX: p.X, TargetY: p.Y}, true
	}
	if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
		for _, d := range m.Dens {
			if d.X == x && d.Y == y && denReadyAt(s, x, y, tick) {
				return true
			}
		}
		return false
	}); ok {
		return &Intent{Goal: "hunt", TargetX: p.X, TargetY: p.Y}, true
	}
	return nil, false
}

// resolveGoal turns a planner-chosen goal into a concrete, deterministic
// intent at the tick boundary (research R5). The model steers; the sim
// drives. Errors mean the goal is impossible right now — nothing is emitted.
func resolveGoal(s *State, m *worldmap.Map, idx int, goal string, targetAgent int, tick int64) (*Intent, string, error) {
	a := &s.Agents[idx]
	switch goal {
	case "eat":
		// T018: eat over the full food triplet, most-nutritious-first to
		// satiety. Refuse if empty-handed or already sated (no unit is ever
		// consumed at satiety — the eating-overshoot edge case).
		if !hasAnyFood(a) {
			return nil, "", fmt.Errorf("%s has nothing to eat", a.Name)
		}
		if a.Needs.Food >= satietyAt {
			return nil, "", fmt.Errorf("%s is already sated", a.Name)
		}
		return nil, "agent.ate", nil
	case "forage":
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
			return effectiveKind(m, s, x, y) == worldmap.Forage
		}); ok {
			return &Intent{Goal: "forage", TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no forage reachable")
	case "hunt":
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
			for _, d := range m.Dens {
				if d.X == x && d.Y == y && denReadyAt(s, x, y, tick) {
					return true
				}
			}
			return false
		}); ok {
			return &Intent{Goal: "hunt", TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no ready den reachable")
	case "chop":
		if in, ok := chopIntent(s, m, a); ok {
			return in, "", nil
		}
		return nil, "", fmt.Errorf("no tree reachable")
	case "quarry":
		// Planner-only (research R5, FR-020): never added to decideIntent's
		// reflex ladder.
		if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
			return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Rock
		}); ok {
			return &Intent{Goal: "quarry", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no rock outcrop reachable")
	case "collect_water":
		// Planner-only, same as quarry.
		if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
			return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Water
		}); ok {
			return &Intent{Goal: "collect_water", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no water reachable")
	case "build_fire":
		if a.Inv.Wood < fireWoodCost {
			return nil, "", fmt.Errorf("%s lacks wood (%d < %d)", a.Name, a.Inv.Wood, fireWoodCost)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
			return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no build site reachable")
	case "build_shelter":
		// T036: re-costed to planks (was wood) — shelter joins the plank
		// economy; planner-only (FR-012), never reflex-chosen.
		if a.Inv.Planks < shelterPlankCost {
			return nil, "", fmt.Errorf("%s lacks planks (%d < %d)", a.Name, a.Inv.Planks, shelterPlankCost)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
			return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no build site reachable")
	case "build_oven":
		// T030: the flagship stone-cost station, on-site like fire/shelter.
		r, _ := recipeFor("build_oven")
		if !hasItems(a.Inv, r.Inputs) {
			return nil, "", fmt.Errorf("%s lacks inputs for an oven (%d refined stone + %d planks)", a.Name, r.Inputs[0].N, r.Inputs[1].N)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
			return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no build site reachable")
	case "craft_planks", "craft_stone", "craft_spear":
		// T026: hand-crafts anywhere — target is the agent's own tile, no
		// travel. Planner-only (FR-020); never enters the reflex ladder.
		r, ok := recipeFor(goal)
		if !ok {
			return nil, "", fmt.Errorf("unknown recipe %q", goal)
		}
		if !hasItems(a.Inv, r.Inputs) {
			return nil, "", fmt.Errorf("%s lacks inputs for %s", a.Name, goal)
		}
		return &Intent{Goal: goal, TargetX: a.X, TargetY: a.Y}, "", nil
	case "refuel_fire":
		// T020: planner OR reflex (the one shared goal, FR-020). Target the
		// nearest fire, lit or cold — the completion relights a cold one.
		if a.Inv.Wood < 1 {
			return nil, "", fmt.Errorf("%s lacks wood to refuel a fire", a.Name)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return s.structureAt("fire", x, y) }); ok {
			return &Intent{Goal: "refuel_fire", TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no fire reachable to refuel")
	case "cook":
		// T031: cook raw food at the nearest valid station — a lit fire or an
		// oven, whichever is nearer (the shared `nearest` BFS helper's fixed
		// neighbor order makes the tie-break deterministic). Station-specific
		// duration/output (fire → food_cooked, oven → meals + 1 wood fuel) is
		// resolved from the target at the executor (workDuration/completion).
		if a.Inv.FoodRaw <= 0 {
			return nil, "", fmt.Errorf("%s has no raw food to cook", a.Name)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
			return litFireAt(s, x, y, tick) || s.structureAt("oven", x, y)
		}); ok {
			return &Intent{Goal: "cook", TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no lit fire or oven reachable to cook at")
	case "bathe":
		// T032: water's only v1 consumer — bathe at an oven.
		r, _ := recipeFor("bathe")
		if !hasItems(a.Inv, r.Inputs) {
			return nil, "", fmt.Errorf("%s lacks water/wood to bathe", a.Name)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return s.structureAt("oven", x, y) }); ok {
			return &Intent{Goal: "bathe", TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no oven reachable to bathe at")
	case "sleep":
		return &Intent{Goal: "sleep", TargetX: a.X, TargetY: a.Y}, "", nil
	case "goto_warmth":
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y, tick) && passable(m, s, x, y) }); ok {
			return &Intent{Goal: "goto_warmth", TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no warmth anywhere")
	case "wander":
		r := rngAt(s.Seed, "wander", tick, idx)
		for try := 0; try < 8; try++ {
			dx, dy := int(r.Uint64N(9))-4, int(r.Uint64N(9))-4
			if (dx != 0 || dy != 0) && passable(m, s, a.X+dx, a.Y+dy) {
				return &Intent{Goal: "wander", TargetX: a.X + dx, TargetY: a.Y + dy}, "", nil
			}
		}
		return nil, "", fmt.Errorf("nowhere to wander")
	case "talk_to", "seek":
		if targetAgent < 0 || targetAgent >= len(s.Agents) || targetAgent == idx {
			return nil, "", fmt.Errorf("no such agent to seek")
		}
		t := &s.Agents[targetAgent]
		if t.Dead {
			return nil, "", fmt.Errorf("%s is dead", t.Name)
		}
		return &Intent{Goal: "seek", TargetX: t.X, TargetY: t.Y}, "", nil
	}
	return nil, "", fmt.Errorf("unknown goal %q", goal)
}

func chopIntent(s *State, m *worldmap.Map, a *Agent) (*Intent, bool) {
	stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Tree
	})
	if !ok {
		return nil, false
	}
	return &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, true
}

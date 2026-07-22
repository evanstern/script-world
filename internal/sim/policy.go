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

	// Eat from inventory the moment hunger bites.
	// TODO(T018): eat over the full food triplet (Meals/FoodCooked/FoodRaw).
	if a.Needs.Food < hungryAt && a.Inv.FoodRaw > 0 {
		return decision{directEvent: "agent.ate"}
	}

	// Hungry with nothing carried: get food (forage first, hunt as backup).
	if a.Needs.Food < hungryAt {
		if d, ok := foodIntent(s, m, a, tick); ok {
			return decision{intent: d}
		}
	}

	if s.Night {
		if !warmAt(s, a.X, a.Y) {
			// Reach warmth, or make it, or get the wood to make it.
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y) && passable(m, s, x, y) }); ok {
				return decision{intent: &Intent{Goal: "goto_warmth", TargetX: p.X, TargetY: p.Y}}
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
		if !warmAt(s, tx, ty) {
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y) && passable(m, s, x, y) }); ok {
				tx, ty = p.X, p.Y
			}
		}
		return decision{intent: &Intent{Goal: "sleep", TargetX: tx, TargetY: ty}}
	}

	// Village prep: a fire before the first night, then a shelter, then a
	// full larder.
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
	if !s.hasStructure("shelter") {
		if a.Inv.Wood >= shelterWoodCost {
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return decision{intent: &Intent{Goal: "build_shelter", TargetX: p.X, TargetY: p.Y}}
			}
		}
		if d, ok := chopIntent(s, m, a); ok {
			return decision{intent: d}
		}
	}
	// TODO(T018/T020): larder stocking in raw units for now.
	if a.Inv.FoodRaw < stockFoodTo {
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
		// TODO(T018): eat over the full food triplet.
		if a.Inv.FoodRaw <= 0 {
			return nil, "", fmt.Errorf("%s has nothing to eat", a.Name)
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
	case "build_fire", "build_shelter":
		cost := fireWoodCost
		if goal == "build_shelter" {
			cost = shelterWoodCost
		}
		if a.Inv.Wood < cost {
			return nil, "", fmt.Errorf("%s lacks wood (%d < %d)", a.Name, a.Inv.Wood, cost)
		}
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
			return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no build site reachable")
	case "sleep":
		return &Intent{Goal: "sleep", TargetX: a.X, TargetY: a.Y}, "", nil
	case "goto_warmth":
		if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y) && passable(m, s, x, y) }); ok {
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

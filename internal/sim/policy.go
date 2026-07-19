package sim

import (
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
	if a.Needs.Food < hungryAt && a.Inv.Food > 0 {
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
	if a.Inv.Food < stockFoodTo {
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

func chopIntent(s *State, m *worldmap.Map, a *Agent) (*Intent, bool) {
	stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Tree
	})
	if !ok {
		return nil, false
	}
	return &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, true
}

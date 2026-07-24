package sim

import (
	"fmt"

	"github.com/evanstern/promptworld/internal/worldmap"
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

// goalResolver produces a concrete, deterministic intent (or a direct-event
// tag, or an error) for one goal against live state. The signature carries
// everything the per-verb bodies need; unused params in a given resolver are
// fine (the bodies are the old switch arms, moved verbatim).
type goalResolver func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error)

// goalResolvers is the name-keyed resolution table (spec 014, R2): the former
// resolveGoal switch, one arm per entry. It replaces the switch so startup can
// verify — against the tool registry — that every world tool has a resolver
// (internal/sim/toolcheck.go). The per-verb semantics are byte-identical to the
// old arms; only the dispatch shape changed. The reflex ladder (decideIntent)
// is keyed by intent, not by this table, and stays hand-written (R6).
var goalResolvers = buildGoalResolvers()

func buildGoalResolvers() map[string]goalResolver {
	// craft handles the three hand-crafts (planks/stone/spear), which shared
	// one switch arm — keyed on `goal`.
	craft := func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
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
	}
	// wallBuild resolves both wall builds (spec 032 US1, research R2). Unlike
	// fire/shelter/oven/chest (which build on the tile the agent stands on),
	// walls build ADJACENT: the builder stands on a passable tile (Target) and
	// the wall lands on the neighboring buildable tile (Res). Building where you
	// stand would entomb the builder the instant the wall lands (FR-007), so
	// nearestAdjacentTo over buildSite gives the stand/build pair — the chop/
	// quarry adjacency pattern.
	wallBuild := func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
		r, ok := recipeFor(goal)
		if !ok {
			return nil, "", fmt.Errorf("unknown recipe %q", goal)
		}
		if !hasItems(a.Inv, r.Inputs) {
			return nil, "", fmt.Errorf("%s lacks inputs for %s", a.Name, goal)
		}
		if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
			return &Intent{Goal: goal, TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
		}
		return nil, "", fmt.Errorf("no build site reachable")
	}
	// talk resolves both talk_to (the planner verb) and its internal "seek"
	// alias — they shared one switch arm.
	talk := func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
		if targetAgent < 0 || targetAgent >= len(s.Agents) || targetAgent == idx {
			return nil, "", fmt.Errorf("no such agent to seek")
		}
		t := &s.Agents[targetAgent]
		if t.Dead {
			return nil, "", fmt.Errorf("%s is dead", t.Name)
		}
		return &Intent{Goal: "seek", TargetX: t.X, TargetY: t.Y}, "", nil
	}

	return map[string]goalResolver{
		"eat": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
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
		},
		"forage": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
				return effectiveKind(m, s, x, y) == worldmap.Forage
			}); ok {
				return &Intent{Goal: "forage", TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no forage reachable")
		},
		"hunt": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
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
		},
		"chop": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			if in, ok := chopIntent(s, m, a); ok {
				return in, "", nil
			}
			return nil, "", fmt.Errorf("no tree reachable")
		},
		"quarry": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Planner-only (research R5, FR-020): never added to decideIntent's
			// reflex ladder.
			if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
				return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Rock
			}); ok {
				return &Intent{Goal: "quarry", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no rock outcrop reachable")
		},
		"collect_water": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Planner-only, same as quarry.
			if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
				return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Water
			}); ok {
				return &Intent{Goal: "collect_water", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no water reachable")
		},
		"build_fire": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			if a.Inv.Wood < fireWoodCost {
				return nil, "", fmt.Errorf("%s lacks wood (%d < %d)", a.Name, a.Inv.Wood, fireWoodCost)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no build site reachable")
		},
		"build_shelter": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T036: re-costed to planks (was wood) — shelter joins the plank
			// economy; planner-only (FR-012), never reflex-chosen.
			if a.Inv.Planks < shelterPlankCost {
				return nil, "", fmt.Errorf("%s lacks planks (%d < %d)", a.Name, a.Inv.Planks, shelterPlankCost)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no build site reachable")
		},
		"build_oven": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T030: the flagship stone-cost station, on-site like fire/shelter.
			r, _ := recipeFor("build_oven")
			if !hasItems(a.Inv, r.Inputs) {
				return nil, "", fmt.Errorf("%s lacks inputs for an oven (%d refined stone + %d planks)", a.Name, r.Inputs[0].N, r.Inputs[1].N)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no build site reachable")
		},
		"build_chest": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T023 (spec 013 US3): the first owned container — 6 planks on the
			// nearest buildable tile (build_oven pattern; the pile-tile exclusion
			// already lives in buildSite, T019). Planner/plan-only (FR-014). Timed
			// work; the completion re-validates inputs + site (contested pattern).
			r, _ := recipeFor("build_chest")
			if !hasItems(a.Inv, r.Inputs) {
				return nil, "", fmt.Errorf("%s lacks planks for a chest (%d < %d)", a.Name, a.Inv.Planks, r.Inputs[0].N)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no build site reachable")
		},
		"craft_planks": craft,
		"craft_stone":  craft,
		"craft_spear":  craft,
		"craft_axe":    craft, // spec 032 US2: same shared hand-craft closure
		"build_path": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Spec 032 US3 (research R3): a path is built ON the tile the agent
			// stands on (stand-on-target, the build_fire pattern) — paths are
			// walkable, so there is no entombment risk, unlike walls. The generic
			// build completion + reducer arm handle the rest.
			r, _ := recipeFor("build_path")
			if !hasItems(a.Inv, r.Inputs) {
				return nil, "", fmt.Errorf("%s lacks stone for a path (%d < %d)", a.Name, a.Inv.Stone, pathStoneCost)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return buildSite(m, s, x, y) }); ok {
				return &Intent{Goal: goal, TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no build site reachable")
		},
		"build_wall_plank": wallBuild,
		"build_wall_stone": wallBuild,
		"demolish": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Spec 032 US1 (research R5): tear down the nearest wall. Adjacent-
			// stand (wall tiles are impassable), so nearestAdjacentTo over isWall
			// gives the stand tile (Target) beside the wall tile (Res). No material
			// needed to demolish.
			if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool { return wallAt(s, x, y) != nil }); ok {
				return &Intent{Goal: "demolish", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no wall reachable to demolish")
		},
		"repair": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Spec 032 US1 (research R5): mend the nearest DAMAGED wall the agent
			// can afford — HP below the derived max AND at least 1 unit of that
			// wall's build material carried (planks for a plank wall, refined
			// stone for a stone wall). A wall already at full health never
			// resolves (nothing to repair). Adjacent-stand, like demolish.
			if stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
				w := wallAt(s, x, y)
				return w != nil && w.HP < wallMaxHP(w.Kind) && invField(a.Inv, wallRepairMaterial(w.Kind)) >= 1
			}); ok {
				return &Intent{Goal: "repair", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y}, "", nil
			}
			return nil, "", fmt.Errorf("%s has no damaged wall reachable to repair (with the right material)", a.Name)
		},
		"refuel_fire": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T020: planner OR reflex (the one shared goal, FR-020). Target the
			// nearest fire, lit or cold — the completion relights a cold one.
			if a.Inv.Wood < 1 {
				return nil, "", fmt.Errorf("%s lacks wood to refuel a fire", a.Name)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return s.structureAt("fire", x, y) }); ok {
				return &Intent{Goal: "refuel_fire", TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no fire reachable to refuel")
		},
		"cook": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
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
		},
		"bathe": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T032: water's only v1 consumer — bathe at an oven.
			r, _ := recipeFor("bathe")
			if !hasItems(a.Inv, r.Inputs) {
				return nil, "", fmt.Errorf("%s lacks water/wood to bathe", a.Name)
			}
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return s.structureAt("oven", x, y) }); ok {
				return &Intent{Goal: "bathe", TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no oven reachable to bathe at")
		},
		"drop": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Planner/plan-only (FR-014). Instant on the agent's current tile; the
			// completion emits agent.dropped with the actual post-clamp counts. An
			// empty Kind or nothing carried resolves via intent_done at completion
			// (executor) — resolveGoal creates the intent regardless (the goal is
			// possible; the re-validation is where it may become a no-op).
			return &Intent{Goal: "drop", TargetX: a.X, TargetY: a.Y, Kind: kind, Qty: qty}, "", nil
		},
		"pick_up": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// Planner/plan-only. Target the nearest pile tile — piles sit on
			// passable ground, so the agent walks onto it; the completion
			// re-validates a pile on/adjacent and moves goods truncated to free
			// bulk (Kind "" sweeps every kind in canonical order).
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return s.pileAt(x, y) != nil }); ok {
				return &Intent{Goal: "pick_up", TargetX: p.X, TargetY: p.Y, Kind: kind, Qty: qty}, "", nil
			}
			return nil, "", fmt.Errorf("no pile reachable")
		},
		"deposit": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T024 (spec 013 US3): planner/plan-only. Target the nearest chest (any
			// owner — the commons/ownership split is social, not mechanical; anyone
			// may deposit). The completion re-validates the chest and truncates to its
			// free space (chestCap − bulk(*Store)); Kind "" or nothing that fits
			// resolves via intent_done at completion.
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return s.chestAt(x, y) != nil }); ok {
				return &Intent{Goal: "deposit", TargetX: p.X, TargetY: p.Y, Kind: kind, Qty: qty}, "", nil
			}
			return nil, "", fmt.Errorf("no chest reachable")
		},
		"withdraw": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			// T024: planner/plan-only. Target the nearest chest whose Store holds Kind
			// (Kind "" ⇒ the nearest chest with anything in it). The completion
			// truncates to the taker's free bulk and to what the chest holds; a
			// non-owner take co-emits the theft companion batch (US4, T029) — this
			// goal only resolves the intent here.
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool {
				ch := s.chestAt(x, y)
				if ch == nil || ch.Store == nil {
					return false
				}
				if kind == "" {
					return bulk(*ch.Store) > 0
				}
				return carriedCount(*ch.Store, kind) > 0
			}); ok {
				return &Intent{Goal: "withdraw", TargetX: p.X, TargetY: p.Y, Kind: kind, Qty: qty}, "", nil
			}
			return nil, "", fmt.Errorf("no chest with those goods reachable")
		},
		"sleep": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			return &Intent{Goal: "sleep", TargetX: a.X, TargetY: a.Y}, "", nil
		},
		"goto_warmth": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			if p, ok := nearest(m, s, a.X, a.Y, func(x, y int) bool { return warmAt(s, x, y, tick) && passable(m, s, x, y) }); ok {
				return &Intent{Goal: "goto_warmth", TargetX: p.X, TargetY: p.Y}, "", nil
			}
			return nil, "", fmt.Errorf("no warmth anywhere")
		},
		"wander": func(s *State, m *worldmap.Map, a *Agent, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
			r := rngAt(s.Seed, "wander", tick, idx)
			for try := 0; try < 8; try++ {
				dx, dy := int(r.Uint64N(9))-4, int(r.Uint64N(9))-4
				if (dx != 0 || dy != 0) && passable(m, s, a.X+dx, a.Y+dy) {
					return &Intent{Goal: "wander", TargetX: a.X + dx, TargetY: a.Y + dy}, "", nil
				}
			}
			return nil, "", fmt.Errorf("nowhere to wander")
		},
		"talk_to": talk,
		"seek":    talk,
	}
}

// resolveGoal turns a planner-chosen goal into a concrete, deterministic
// intent at the tick boundary (research R5). The model steers; the sim
// drives. Errors mean the goal is impossible right now — nothing is emitted.
// Dispatch is now table-driven (spec 014, R2); the per-verb semantics are
// unchanged, and an unknown goal errors exactly as the old switch default did.
func resolveGoal(s *State, m *worldmap.Map, idx int, goal string, targetAgent int, kind string, qty int, tick int64) (*Intent, string, error) {
	a := &s.Agents[idx]
	if r, ok := goalResolvers[goal]; ok {
		return r(s, m, a, idx, goal, targetAgent, kind, qty, tick)
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

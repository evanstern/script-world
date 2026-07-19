package sim

import (
	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// The executor: the deterministic layer that runs agent bodies unattended
// between planner calls (TASK-5). stepEvents is a pure function of
// (state, map, next tick) — it must not mutate s; the loop applies its
// returned events through the reducer.

const (
	nightStartSecond = 22 * 3600 // 22:00
	dayStartSecond   = 6 * 3600  // 06:00
)

func stepEvents(s *State, m *worldmap.Map, nextTick int64) []store.Event {
	var events []store.Event
	emit := func(typ string, payload any) {
		events = append(events, store.Event{Tick: nextTick, Type: typ, Payload: mustPayload(payload)})
	}

	// Day/night boundaries.
	day, _, _, _ := clock.GameTime(nextTick)
	night := s.Night
	switch clock.SecondOfDay(nextTick) {
	case nightStartSecond:
		emit("sim.night_started", DayPayload{Day: day})
		night = true
	case dayStartSecond:
		emit("sim.day_started", DayPayload{Day: day})
		night = false
		for i := range s.Agents {
			a := &s.Agents[i]
			if !a.Dead && a.Needs.Warmth < coldNightBelow {
				events = append(events, memoryEvent(nextTick, i, salColdNight,
					"Survived a freezing night in the open."))
			}
		}
	}

	// Forage regrowth.
	for _, h := range s.Harvested {
		if h.Regrow == nextTick {
			emit("sim.forage_regrown", RegrownPayload{X: h.X, Y: h.Y})
		}
	}

	// Per-game-minute needs heartbeat: decay, warmth, death.
	if nextTick%60 == 0 {
		for i := range s.Agents {
			a := &s.Agents[i]
			if a.Dead {
				continue
			}
			n := decayNeeds(a.Needs, a.Asleep, night, warmAt(s, a.X, a.Y))
			emit("agent.needs_changed", NeedsPayload{
				Agent: i, Health: n.Health, Food: n.Food, Rest: n.Rest, Warmth: n.Warmth, Morale: n.Morale,
			})
			// Own near-death is a formative memory, once per collapse (latch).
			if n.Health < nearDeathBelow && !a.NearDeath && n.Health > 0 {
				cause := "cold and hunger"
				switch {
				case n.Food == 0 && n.Warmth > 0:
					cause = "hunger"
				case n.Warmth == 0 && n.Food > 0:
					cause = "the cold"
				}
				events = append(events, memoryEvent(nextTick, i, salNearDeath, "Nearly died — %s almost took me.", cause))
			}
			if n.Health == 0 {
				cause := "collapse"
				switch {
				case n.Food == 0:
					cause = "starvation"
				case n.Warmth == 0:
					cause = "exposure"
				}
				emit("agent.died", DiedPayload{Agent: i, Cause: cause})
				// Death marks every witness close enough to see it.
				for w := range s.Agents {
					if w == i || s.Agents[w].Dead {
						continue
					}
					if abs(s.Agents[w].X-a.X)+abs(s.Agents[w].Y-a.Y) <= witnessRadius {
						events = append(events, memoryEvent(nextTick, w, salWitnessDeath,
							"Watched %s die of %s.", a.Name, cause))
					}
				}
			}
		}
	}

	// Per-agent execution. Uses current state s (pre-tick); all effects
	// land as events.
	for i := range s.Agents {
		a := &s.Agents[i]
		if a.Dead {
			continue
		}

		if a.Asleep {
			if wakeReason(a, night) {
				emit("agent.woke", AgentPayload{Agent: i})
			}
			continue
		}

		if a.Intent == nil {
			// The reflex is the fallback mind (TASK-7): it acts only on
			// agents idle past the grace window, leaving room for planner
			// injections; with no planner it remains the permanent
			// degraded mode. Staggered so agents don't all think at once.
			if nextTick-a.IdleSince >= reflexGraceTicks && (nextTick+int64(i)*7)%20 == 0 {
				d := decideIntent(s, m, i, nextTick)
				switch {
				case d.directEvent == "agent.ate":
					emit("agent.ate", AgentPayload{Agent: i})
				case d.intent != nil:
					emit("agent.intent_set", IntentSetPayload{
						Agent: i, Goal: d.intent.Goal,
						TargetX: d.intent.TargetX, TargetY: d.intent.TargetY,
						ResX: d.intent.ResX, ResY: d.intent.ResY,
						Source: "reflex",
					})
				}
			}
			continue
		}

		in := a.Intent
		if a.X == in.TargetX && a.Y == in.TargetY {
			events = append(events, executeAtTarget(s, m, i, nextTick)...)
			continue
		}

		// En route: one tile per moveEveryTicks, staggered like decisions.
		if (nextTick+int64(i)*3)%moveEveryTicks == 0 {
			nx, ny := nextStep(m, s, a.X, a.Y, in.TargetX, in.TargetY)
			if nx == a.X && ny == a.Y {
				emit("agent.intent_done", AgentPayload{Agent: i}) // unreachable
				continue
			}
			emit("agent.moved", AgentMovedPayload{Agent: i, X: nx, Y: ny})
		}
	}

	// Adjacent idle agents pass the time together (talk-intent primitive;
	// the social fabric proper is TASK-8).
	if nextTick%60 == 30 {
		events = append(events, talkEvents(s, nextTick)...)
	}

	return events
}

// executeAtTarget runs the arrival/work/completion state machine for the
// agent standing on its intent target.
func executeAtTarget(s *State, m *worldmap.Map, i int, nextTick int64) []store.Event {
	var events []store.Event
	emit := func(typ string, payload any) {
		events = append(events, store.Event{Tick: nextTick, Type: typ, Payload: mustPayload(payload)})
	}
	a := &s.Agents[i]
	in := a.Intent

	// Instant goals complete on arrival.
	switch in.Goal {
	case "sleep":
		emit("agent.slept", AgentPayload{Agent: i})
		return events
	case "wander", "goto_warmth", "seek":
		emit("agent.intent_done", AgentPayload{Agent: i})
		return events
	}

	// Validity: the resource may have vanished while walking (someone else
	// got there first).
	valid := true
	switch in.Goal {
	case "forage":
		valid = effectiveKind(m, s, in.TargetX, in.TargetY) == worldmap.Forage
	case "chop":
		valid = effectiveKind(m, s, in.ResX, in.ResY) == worldmap.Tree
	case "hunt":
		valid = denReadyAt(s, in.TargetX, in.TargetY, nextTick)
	case "build_fire", "build_shelter":
		valid = buildSite(m, s, in.TargetX, in.TargetY)
	}
	if !valid {
		emit("agent.intent_done", AgentPayload{Agent: i})
		return events
	}

	if in.WorkStart == 0 {
		emit("agent.work_started", WorkStartedPayload{Agent: i, Tick: nextTick})
		return events
	}
	if nextTick-in.WorkStart < intentDuration(in.Goal) {
		return events // still working
	}

	switch in.Goal {
	case "forage":
		emit("agent.foraged", HarvestPayload{Agent: i, X: in.TargetX, Y: in.TargetY})
		if a.Needs.Food < 150 {
			events = append(events, memoryEvent(nextTick, i, salStarvingForage,
				"Found food when I was starving."))
		}
	case "chop":
		emit("agent.chopped", HarvestPayload{Agent: i, X: in.ResX, Y: in.ResY})
	case "hunt":
		emit("agent.hunted", HarvestPayload{Agent: i, X: in.TargetX, Y: in.TargetY})
		events = append(events, memoryEvent(nextTick, i, salHunt, "Hunted at the den and came back with meat."))
	case "build_fire":
		emit("agent.built", BuiltPayload{Agent: i, Kind: "fire", X: in.TargetX, Y: in.TargetY})
		events = append(events, memoryEvent(nextTick, i, salFire, "Built a fire."))
	case "build_shelter":
		emit("agent.built", BuiltPayload{Agent: i, Kind: "shelter", X: in.TargetX, Y: in.TargetY})
		events = append(events, memoryEvent(nextTick, i, salShelter, "Raised a shelter with my own hands."))
	}
	return events
}

func decayNeeds(n Needs, asleep, night, warm bool) Needs {
	n.Food = maxInt(0, n.Food-foodDecay)
	if asleep {
		n.Rest = minInt(1000, n.Rest+restRegenSleep)
	} else {
		n.Rest = maxInt(0, n.Rest-restDecayAwake)
	}
	switch {
	case warm:
		n.Warmth = minInt(1000, n.Warmth+warmthGainFire)
	case night:
		n.Warmth = maxInt(0, n.Warmth-warmthLossCold)
	default:
		n.Warmth = minInt(1000, n.Warmth+warmthGainDay)
	}
	if n.Food == 0 || n.Warmth == 0 {
		n.Health = maxInt(0, n.Health-healthLoss)
	} else if n.Food > 300 && n.Rest > 200 {
		n.Health = minInt(1000, n.Health+healthRegen)
	}
	if n.Food < 200 || n.Rest < 200 || n.Warmth < 200 {
		n.Morale = maxInt(0, n.Morale-1)
	} else if n.Morale < 700 {
		n.Morale++
	}
	return n
}

// wakeReason: day breaks with decent rest, or a hunger emergency the agent
// can actually act on (food in hand). Fully-rested agents sleep through the
// night regardless — waking bored at 4am with nothing to do but sleep again
// churned sleep/wake events endlessly.
func wakeReason(a *Agent, night bool) bool {
	if !night && a.Needs.Rest >= 600 {
		return true
	}
	return a.Needs.Food < 150 && a.Inv.Food > 0
}

func talkEvents(s *State, nextTick int64) []store.Event {
	var events []store.Event
	for i := range s.Agents {
		a := &s.Agents[i]
		if a.Dead || a.Asleep || a.Intent != nil || nextTick-a.LastTalk < talkCooldownSec {
			continue
		}
		for j := i + 1; j < len(s.Agents); j++ {
			b := &s.Agents[j]
			if b.Dead || b.Asleep || b.Intent != nil || nextTick-b.LastTalk < talkCooldownSec {
				continue
			}
			if abs(a.X-b.X)+abs(a.Y-b.Y) == 1 {
				events = append(events, store.Event{
					Tick: nextTick, Type: "agent.talked",
					Payload: mustPayload(TalkedPayload{A: i, B: j}),
				})
				events = append(events,
					memoryEvent(nextTick, i, salTalk, "Talked with %s.", b.Name),
					memoryEvent(nextTick, j, salTalk, "Talked with %s.", a.Name))
				return events // one conversation per heartbeat keeps it simple
			}
		}
	}
	return events
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

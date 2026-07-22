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

	// Metatron charge regeneration (TASK-12): absolute 6-game-hour
	// boundaries, pure function of (state, tick).
	if nextTick%chargeRegenTicks == 0 && s.MetatronCharges < MetatronChargeCap {
		emit("metatron.charge_regenerated", ChargeRegeneratedPayload{})
	}

	// The gru: nightly emergence, stalking, wounds, dawn withdrawal (gru.go).
	events = append(events, gruStep(s, m, night, nextTick)...)

	// Governance (TASK-13): the meeting lifecycle (only once a convention is
	// established — TASK-36) and the per-minute violation detectors
	// (governance.go).
	events = append(events, governanceEvents(s, m, nextTick)...)

	// Forage regrowth.
	for _, h := range s.Harvested {
		if h.Regrow == nextTick {
			emit("sim.forage_regrown", RegrownPayload{X: h.X, Y: h.Y})
		}
	}

	// Fire fuel burnout (T019): a fire whose deadline falls in this tick's
	// window goes cold — emit sim.fire_burned_out exactly once on the
	// transition (tick-1 < FuelUntil <= tick). Pure function of (state, tick);
	// lit-ness stays derived from FuelUntil, so the event carries no state
	// effect. Refuel pushes FuelUntil forward, re-arming this detection.
	for _, st := range s.Structures {
		if st.Kind == "fire" && st.FuelUntil > nextTick-1 && st.FuelUntil <= nextTick {
			emit("sim.fire_burned_out", FireBurnedOutPayload{X: st.X, Y: st.Y})
			// Deferred Phase-4 item (contracts/events.md): a fire going cold
			// nearby is background texture, not formative — low salience,
			// purely personal (no gossip subject), same witness-radius idiom
			// as the oven-built/death witnessing above. Fixed agent
			// iteration order keeps this deterministic.
			for w := range s.Agents {
				if s.Agents[w].Dead {
					continue
				}
				if abs(s.Agents[w].X-st.X)+abs(s.Agents[w].Y-st.Y) <= witnessRadius {
					events = append(events, memoryEvent(nextTick, w, salFireOut,
						"Watched the fire burn out."))
				}
			}
		}
	}

	// Per-game-minute needs heartbeat: decay, warmth, death.
	if nextTick%60 == 0 {
		for i := range s.Agents {
			a := &s.Agents[i]
			if a.Dead {
				continue
			}
			n := decayNeeds(a.Needs, a.Asleep, night, warmAt(s, a.X, a.Y, nextTick), s.structureAt("shelter", a.X, a.Y))
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
				if s.Gru != nil && s.Gru.LastVictim == i && nextTick-s.Gru.LastAttack <= 3600 {
					cause = "the gru"
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
						events = append(events, memoryAboutEvent(nextTick, w, i, -80, salWitnessDeath,
							"Watched %s die of %s.", a.Name, cause))
					}
				}
			}
		}
	}

	// Hail sweep (TASK-47): resolve outstanding pauses — met (hailer arrived)
	// or expired — before anyone moves this tick, so met-vs-expired is a
	// deterministic race with met winning ties (research D4).
	events = append(events, hailStep(s, nextTick)...)

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

		// Meeting pinning (TASK-13): while the village convenes, attendees
		// drop what they're doing and head for the meeting place. The goal
		// is executor-set only — never planner-choosable — and stale pins
		// clear once the meeting ends.
		if meetingActive(s) && s.MeetingPlace != nil && attendCandidate(s, i) &&
			(a.Intent == nil || a.Intent.Goal != "attend_meeting") {
			emit("agent.intent_set", IntentSetPayload{
				Agent: i, Goal: "attend_meeting",
				TargetX: s.MeetingPlace.X, TargetY: s.MeetingPlace.Y,
				Source: "meeting",
			})
			continue
		}
		if !meetingActive(s) && a.Intent != nil && a.Intent.Goal == "attend_meeting" {
			emit("agent.intent_done", AgentPayload{Agent: i})
			continue
		}

		// Hail pause (TASK-47): a flagged-down agent stands still — no reflex,
		// no plan-step evaluation, no stepping en route — until the window
		// lifts. Its needs still decay (heartbeat above), it still takes part
		// in social beats, and stationary work at a tile it already stands on
		// continues; intent and plan are left exactly as they were (FR-004).
		paused := hailPaused(a, nextTick)

		if a.Intent == nil {
			if paused {
				continue
			}
			// Guarded plan steps (TASK-32 US4) own an idle agent while the
			// head step's window is open: holding emits nothing, firing
			// sets the intent, expiry clears the plan — all deterministic,
			// no model at firing time (FR-017).
			if len(a.Plan) > 0 {
				events = append(events, planStepEvents(s, m, i, nextTick)...)
				continue
			}
			// The reflex is the fallback mind (TASK-7): it acts only on
			// agents idle past the grace window, leaving room for planner
			// injections; with no planner it remains the permanent
			// degraded mode. Staggered so agents don't all think at once.
			if nextTick-a.IdleSince >= reflexGraceTicks && (nextTick+int64(i)*7)%20 == 0 {
				d := decideIntent(s, m, i, nextTick)
				switch {
				case d.directEvent == "agent.ate":
					if p, ok := eatOutcome(a); ok {
						p.Agent = i
						emit("agent.ate", p)
					}
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
		if paused {
			continue // frozen in place: no stepping toward the target
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

	// Adjacent idle agents: give/repay first (debts bind), then talk with a
	// verbatim rumor fallback (the social fabric's model-free floor).
	if nextTick%60 == 30 {
		events = append(events, socialEvents(s, nextTick)...)
	}

	// Hourly ledger due-check: overdue open debts break, permanently.
	if nextTick%3600 == 0 {
		repayNorm := activeNormOfKind(s, NormRepayDebts)
		for _, d := range s.Debts {
			if d.Status == "open" && nextTick > d.Due {
				events = append(events,
					store.Event{Tick: nextTick, Type: "social.promise_broken",
						Payload: mustPayload(PromiseBrokenPayload{ID: d.ID})},
					store.Event{Tick: nextTick, Type: "social.relation_changed",
						Payload: mustPayload(RelationChangedPayload{
							A: d.Creditor, B: d.Debtor,
							TrustDelta: brokenTrustPenalty, AffectionDelta: brokenAffectPenalty,
							Reason: "promise broken"})},
					memoryAboutEvent(nextTick, d.Creditor, d.Debtor, toneNeverPaid, salNeverPaid,
						"%s never repaid the food I gave them.", s.Agents[d.Debtor].Name))
				// A repay-debts norm in force makes the broken promise a
				// witnessed crime too (TASK-13).
				if repayNorm != nil {
					events = append(events, violationEvents(s, repayNorm, d.Debtor, nextTick)...)
				}
			}
		}
	}

	return events
}

// socialEvents runs the adjacency slot: repayment, gifts to the starving,
// or a talk (with the deterministic verbatim rumor fallback). One social
// beat per heartbeat keeps the fabric legible.
func socialEvents(s *State, nextTick int64) []store.Event {
	var events []store.Event
	give := func(from, to int) {
		f, t := &s.Agents[from], &s.Agents[to]
		events = append(events,
			store.Event{Tick: nextTick, Type: "social.gave",
				Payload: mustPayload(GavePayload{From: from, To: to, Kind: "food"})},
			store.Event{Tick: nextTick, Type: "social.relation_changed",
				Payload: mustPayload(RelationChangedPayload{
					A: to, B: from, TrustDelta: giveTrustToGiver, AffectionDelta: giveAffectionToGiver,
					Reason: "shared food"})},
			store.Event{Tick: nextTick, Type: "social.relation_changed",
				Payload: mustPayload(RelationChangedPayload{
					A: from, B: to, TrustDelta: 0, AffectionDelta: giveAffectionToRecv,
					Reason: "shared food"})},
			memoryAboutEvent(nextTick, to, from, toneSaved, salWasSaved,
				"%s gave me food when I needed it.", f.Name),
			memoryEvent(nextTick, from, salGaveHelp, "Gave food to %s.", t.Name))
	}

	for i := range s.Agents {
		a := &s.Agents[i]
		if a.Dead || a.Asleep {
			continue
		}
		for j := i + 1; j < len(s.Agents); j++ {
			b := &s.Agents[j]
			if b.Dead || b.Asleep || abs(a.X-b.X)+abs(a.Y-b.Y) != 1 {
				continue
			}
			// 1) Repay an open debt when able.
			if deb, cred, ok := repayable(s, i, j, nextTick); ok {
				give(deb, cred)
				return events
			}
			// 2) Give to a starving neighbor.
			if giver, recv, ok := giveable(s, i, j, nextTick); ok {
				give(giver, recv)
				return events
			}
			// 3) Talk (+ verbatim rumor fallback). Villagers chat while
			// working — requiring mutual idleness starved the fabric once
			// planners kept everyone permanently tasked (cooldowns still
			// bound the chatter).
			if canTalk(a, nextTick) && canTalk(b, nextTick) {
				return append(events, talkEvents(s, i, j, nextTick)...)
			}
		}
	}
	return events
}

// talkEvents founds a talk between adjacent agents i and j: the morale/
// affection/memory shape plus the deterministic verbatim rumor floor (the
// better-stocked teller passes one rumor; the mind's conversations paraphrase
// instead when a model is available). Shared by the ambient social beat and
// the hail sweep — the sweep founds deliberately, bypassing the ambient
// cooldown (the caller here gates on canTalk; hailStep does not).
func talkEvents(s *State, i, j int, nextTick int64) []store.Event {
	a, b := &s.Agents[i], &s.Agents[j]
	events := []store.Event{
		{Tick: nextTick, Type: "agent.talked",
			Payload: mustPayload(TalkedPayload{A: i, B: j})},
		{Tick: nextTick, Type: "social.relation_changed",
			Payload: mustPayload(RelationChangedPayload{
				A: i, B: j, AffectionDelta: talkAffection, Reason: "talked"})},
		{Tick: nextTick, Type: "social.relation_changed",
			Payload: mustPayload(RelationChangedPayload{
				A: j, B: i, AffectionDelta: talkAffection, Reason: "talked"})},
		memoryEvent(nextTick, i, salTalk, "Talked with %s.", b.Name),
		memoryEvent(nextTick, j, salTalk, "Talked with %s.", a.Name),
	}
	if tell, ok := TellableFor(s, i, j); ok {
		events = append(events, rumorTellEvent(nextTick, i, j, tell))
	} else if tell, ok := TellableFor(s, j, i); ok {
		events = append(events, rumorTellEvent(nextTick, j, i, tell))
	}
	return events
}

func rumorTellEvent(tick int64, from, to int, tell Tellable) store.Event {
	return store.Event{Tick: tick, Type: "social.rumor_told",
		Payload: mustPayload(RumorToldPayload{
			From: from, To: to, RumorID: tell.RumorID, Subject: tell.Subject,
			Tone: tell.Tone, Text: tell.Text, Confidence: tell.Confidence,
		})}
}

// repayable: one of the pair owes the other and can spare a meal — and the
// creditor has bulk to receive it (T012: a gift into a full pouch is skipped
// under the cap, research R2; the debt simply stays open until there's room).
func repayable(s *State, i, j int, tick int64) (debtor, creditor int, ok bool) {
	for _, d := range s.Debts {
		if d.Status != "open" || d.Kind != "food" {
			continue
		}
		if d.Debtor == i && d.Creditor == j && canGive(&s.Agents[i], tick) && freeBulk(s.Agents[j].Inv) > 0 {
			return i, j, true
		}
		if d.Debtor == j && d.Creditor == i && canGive(&s.Agents[j], tick) && freeBulk(s.Agents[i].Inv) > 0 {
			return j, i, true
		}
	}
	return 0, 0, false
}

// giveable: one is starving, the other has spare food — and the starving
// receiver has free bulk (T012: never over the cap; a starving villager at the
// cap is carrying food already and would eat rather than receive).
func giveable(s *State, i, j int, tick int64) (giver, recv int, ok bool) {
	a, b := &s.Agents[i], &s.Agents[j]
	if a.Needs.Food < giveNeedBelow && canGive(b, tick) && freeBulk(a.Inv) > 0 {
		return j, i, true
	}
	if b.Needs.Food < giveNeedBelow && canGive(a, tick) && freeBulk(b.Inv) > 0 {
		return i, j, true
	}
	return 0, 0, false
}

func canGive(a *Agent, tick int64) bool {
	// Give-to-starving stays on raw food (T018 decision: simplest re-expression
	// of the pre-feature gift; the food triplet's least-nutritious form is what
	// a subsistence village shares — see social.go apply).
	return a.Inv.FoodRaw >= giveKeepsAtLeast &&
		(a.LastGive == 0 || tick-a.LastGive >= giveCooldownSec)
}

func canTalk(a *Agent, tick int64) bool {
	return a.LastTalk == 0 || tick-a.LastTalk >= talkCooldownSec
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
	case "refuel_fire":
		// T020: instant on arrival (like eat/sleep). Re-validate at completion
		// (contested pattern): fire still present and wood still carried, else
		// resolve with no effect. The new deadline is absolute and capped; a
		// cold fire relights. At-cap refuels are a no-op (edge case: consumes
		// and extends nothing) — detected as no gain over the current deadline.
		st, ok := fireStructAt(s, in.TargetX, in.TargetY)
		if !ok || a.Inv.Wood < 1 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		base := st.FuelUntil
		if base < nextTick {
			base = nextTick // cold or expired: relight from now
		}
		deadline := base + fireBurnPerWood
		if capAt := nextTick + fireFuelCap; deadline > capAt {
			deadline = capAt
		}
		if deadline <= st.FuelUntil {
			emit("agent.intent_done", AgentPayload{Agent: i}) // already at the fuel cap
			return events
		}
		emit("agent.refueled", RefueledPayload{Agent: i, X: in.TargetX, Y: in.TargetY, FuelUntil: deadline})
		return events
	case "attend_meeting":
		// Assembled: stand at the meeting place until it closes (the
		// executor clears the pin once the meeting ends).
		return events
	case "drop":
		// T016 (spec 013 US2): instant on the agent's current tile. Emit
		// agent.dropped with the ACTUAL post-clamp count — min(Qty-or-all,
		// carried). Kind is required; an empty Kind or nothing carried resolves
		// via intent_done only (no pile is touched, contested-resource pattern).
		n := carriedCount(a.Inv, in.Kind)
		if in.Qty > 0 && in.Qty < n {
			n = in.Qty
		}
		if in.Kind == "" || n <= 0 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		emit("agent.dropped", DroppedPayload{Agent: i, X: a.X, Y: a.Y, Kind: in.Kind, N: n})
		return events
	case "pick_up":
		// T017 (spec 013 US2): instant on arrival. Re-validate a pile on/
		// adjacent (it may have been drained while walking over) and emit ONE
		// agent.picked_up per kind actually moved, truncated cumulatively to
		// free bulk. Kind "" sweeps every kind in canonical field order (the
		// reducer drains food oldest-batch-first). Nothing moved ⇒ intent_done.
		pile := s.pileOnOrAdjacent(a.X, a.Y)
		if pile == nil {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		kinds := []string{in.Kind}
		if in.Kind == "" {
			kinds = canonicalKinds
		}
		free := freeBulk(a.Inv)
		moved := false
		for _, kind := range kinds {
			if free <= 0 {
				break
			}
			take := pile.avail(kind)
			if in.Kind != "" && in.Qty > 0 && in.Qty < take {
				take = in.Qty
			}
			if take > free {
				take = free
			}
			if take <= 0 {
				continue
			}
			emit("agent.picked_up", PickedUpPayload{Agent: i, X: pile.X, Y: pile.Y, Kind: kind, N: take})
			free -= take
			moved = true
		}
		if !moved {
			emit("agent.intent_done", AgentPayload{Agent: i})
		}
		return events
	case "deposit":
		// T024 (spec 013 US3): instant on arrival at the chest. Re-validate the
		// chest still stands (contested pattern) and truncate the move to its free
		// space (chestCap − bulk(*Store)). Kind is required (an empty Kind, an
		// unheld kind, or a full chest ⇒ intent_done only, no effect event). The
		// payload carries the ACTUAL post-clamp count.
		ch := s.chestAt(in.TargetX, in.TargetY)
		if ch == nil || ch.Store == nil || in.Kind == "" {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		n := carriedCount(a.Inv, in.Kind)
		if in.Qty > 0 && in.Qty < n {
			n = in.Qty
		}
		if free := chestCap - bulk(*ch.Store); n > free {
			n = free
		}
		if n <= 0 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		emit("agent.deposited", DepositedPayload{Agent: i, X: in.TargetX, Y: in.TargetY, Kind: in.Kind, N: n})
		return events
	case "withdraw":
		// T024: instant on arrival. Re-validate the chest, then emit ONE
		// agent.withdrew per kind actually moved, truncated cumulatively to the
		// taker's free bulk and to what the chest holds. A named Kind honors Qty;
		// Kind "" sweeps every kind in canonical field order. Owner rides the
		// payload (the theft companion batch is US4, T029 — not emitted here).
		// Nothing moved ⇒ intent_done only.
		ch := s.chestAt(in.TargetX, in.TargetY)
		if ch == nil || ch.Store == nil {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		kinds := []string{in.Kind}
		if in.Kind == "" {
			kinds = canonicalKinds
		}
		free := freeBulk(a.Inv)
		moved := false
		for _, kind := range kinds {
			if free <= 0 {
				break
			}
			take := carriedCount(*ch.Store, kind)
			if in.Kind != "" && in.Qty > 0 && in.Qty < take {
				take = in.Qty
			}
			if take > free {
				take = free
			}
			if take <= 0 {
				continue
			}
			emit("agent.withdrew", WithdrewPayload{Agent: i, X: in.TargetX, Y: in.TargetY, Kind: kind, N: take, Owner: ch.Owner})
			free -= take
			moved = true
		}
		if !moved {
			emit("agent.intent_done", AgentPayload{Agent: i})
		}
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
	case "build_fire", "build_shelter", "build_oven", "build_chest":
		valid = buildSite(m, s, in.TargetX, in.TargetY)
	case "quarry":
		// Contested-resource pattern (FR-002, spec 012 AC#5): someone else may
		// have quarried this outcrop while this agent walked over.
		valid = effectiveKind(m, s, in.ResX, in.ResY) == worldmap.Rock
		// collect_water: no depletion check — water sources are inexhaustible.
	case "cook":
		// T031: the station must still be a lit fire OR an oven (ovens carry
		// no fuel window of their own) — a fire that went cold while walking
		// over (or during the work) yields no cooked food (edge case: fire
		// burns out mid-cook). Re-validated every tick.
		valid = litFireAt(s, in.TargetX, in.TargetY, nextTick) || s.structureAt("oven", in.TargetX, in.TargetY)
	case "bathe":
		// T032: the oven itself must still be there (it never goes cold —
		// only carried water/wood, checked at completion).
		valid = s.structureAt("oven", in.TargetX, in.TargetY)
	}
	if !valid {
		emit("agent.intent_done", AgentPayload{Agent: i})
		return events
	}

	if in.WorkStart == 0 {
		emit("agent.work_started", WorkStartedPayload{Agent: i, Tick: nextTick})
		return events
	}
	if nextTick-in.WorkStart < workDuration(s, a, in) {
		return events // still working
	}

	// US1-AS1 zero-space guard (T011): a gather whose taker has no free bulk
	// does not happen — no harvest event and, crucially, no depletion (the
	// tree/den/outcrop/forage tile is left untouched for later). The intent
	// simply resolves. Same contested-resource re-validation as the vanished-
	// resource case above, keyed on the pouch instead of the world (research R2).
	switch in.Goal {
	case "forage", "chop", "hunt", "quarry", "collect_water":
		if freeBulk(a.Inv) == 0 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
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
		// T027: carrying a spear (checked against pre-mutation state, exactly
		// what the reducer will independently re-derive when it applies this
		// event) raises the yield and spends the most-worn spear's last use.
		// Spent-to-zero breaks it — a companion agent.spear_broke rides the
		// same batch, immediately after, so apply order matches: the hunt
		// reducer decrements Spears[0] to 0, then spear_broke removes it.
		emit("agent.hunted", HarvestPayload{Agent: i, X: in.TargetX, Y: in.TargetY})
		events = append(events, memoryEvent(nextTick, i, salHunt, "Hunted at the den and came back with meat."))
		if len(a.Inv.Spears) > 0 && a.Inv.Spears[0] == 1 {
			emit("agent.spear_broke", SpearBrokePayload{Agent: i})
			events = append(events, memoryEvent(nextTick, i, salSpearBroke,
				"My spear broke on the hunt — I'll need to craft another."))
		}
	case "build_fire":
		emit("agent.built", BuiltPayload{Agent: i, Kind: "fire", X: in.TargetX, Y: in.TargetY})
		events = append(events, memoryEvent(nextTick, i, salFire, "Built a fire."))
	case "build_shelter":
		emit("agent.built", BuiltPayload{Agent: i, Kind: "shelter", X: in.TargetX, Y: in.TargetY})
		events = append(events, memoryEvent(nextTick, i, salShelter, "Raised a shelter with my own hands."))
	case "build_oven":
		// T030: the flagship station. "First oven" wording (research R8) is
		// accurate here — s.hasStructure checks the pre-mutation state, before
		// this very build lands. Village-visible: nearby living agents get a
		// witness memory too, same pattern as a witnessed death.
		first := !s.hasStructure("oven")
		emit("agent.built", BuiltPayload{Agent: i, Kind: "oven", X: in.TargetX, Y: in.TargetY})
		text := "Raised an oven for the village."
		if first {
			text = "Raised the village's first oven — meals and baths, at last."
		}
		events = append(events, memoryEvent(nextTick, i, salOvenBuilt, "%s", text))
		for w := range s.Agents {
			if w == i || s.Agents[w].Dead {
				continue
			}
			if abs(s.Agents[w].X-in.TargetX)+abs(s.Agents[w].Y-in.TargetY) <= witnessRadius {
				events = append(events, memoryAboutEvent(nextTick, w, i, toneOvenBuilt, salOvenBuilt,
					"Watched %s raise an oven for the village.", a.Name))
			}
		}
	case "build_chest":
		// T023 (spec 013 US3): the first owned container. Site re-validated above
		// (buildSite, including the pile-tile exclusion); the reducer consumes the
		// planks and stamps Owner + an empty Store. Village-visible salience/memory
		// is deferred to US4 (T030), matching this slice's scope.
		emit("agent.built", BuiltPayload{Agent: i, Kind: "chest", X: in.TargetX, Y: in.TargetY})
	case "quarry":
		emit("agent.quarried", HarvestPayload{Agent: i, X: in.ResX, Y: in.ResY})
	case "collect_water":
		emit("agent.collected_water", HarvestPayload{Agent: i, X: in.ResX, Y: in.ResY})
	case "craft_planks", "craft_stone", "craft_spear":
		// T026: inputs re-validated at completion (contested-resource
		// pattern) — insufficient inputs resolve via intent_done only, no
		// agent.crafted. Hand-crafts have no travel window (target = the
		// agent's own tile), so this is normally a formality, but the rule
		// applies uniformly with every other completion.
		r, _ := recipeFor(in.Goal)
		if !hasItems(a.Inv, r.Inputs) {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		// US1 (T012): a craft doesn't truncate — it either fits or it doesn't
		// happen. The completion re-validation extends to the net bulk delta
		// (outputs − inputs, the inputs freeing their own space first); if the
		// net won't fit, no agent.crafted, intent cleared. Only craft_planks
		// has a positive net (research R2).
		if craftNetBulk(r) > freeBulk(a.Inv) {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		emit("agent.crafted", CraftedPayload{Agent: i, Kind: craftKindFor(in.Goal)})
	case "cook":
		// T021/T031: convert up to a batch of raw food — fire produces
		// food_cooked (fuel-free, the fire's own fire burns); an oven
		// produces meals and additionally burns 1 carried wood fuel. No
		// carried wood at an oven ⇒ intent_done only (fuel required from day
		// one, FR-017); nothing to cook (no raw carried) is the same no-op.
		atOven := s.structureAt("oven", in.TargetX, in.TargetY)
		if atOven && a.Inv.Wood < 1 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		consumed := a.Inv.FoodRaw
		if consumed > ovenBatchSize {
			consumed = ovenBatchSize
		}
		if consumed <= 0 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		if atOven {
			emit("agent.cooked", CookedPayload{
				Agent: i, Station: "oven", Consumed: consumed, Produced: consumed, Kind: "meals",
			})
		} else {
			emit("agent.cooked", CookedPayload{
				Agent: i, Station: "fire", Consumed: consumed, Produced: consumed, Kind: "food_cooked",
			})
		}
	case "bathe":
		// T032: re-validate carried water + wood at completion — missing
		// either resolves via intent_done only (water's only v1 consumer).
		if a.Inv.Water < 1 || a.Inv.Wood < 1 {
			emit("agent.intent_done", AgentPayload{Agent: i})
			return events
		}
		morale := minInt(1000, a.Needs.Morale+bathMorale)
		warmth := minInt(1000, a.Needs.Warmth+bathWarmth)
		emit("agent.bathed", BathedPayload{Agent: i, MoraleAfter: morale, WarmthAfter: warmth})
		events = append(events, memoryEventToned(nextTick, i, salBath, toneBath,
			"Took a hot bath at the oven — warm, clean, and content."))
	}
	return events
}

// workDuration is the completion-timing rule for the two goals whose
// duration depends on context rather than the goal string alone (spec 012):
// a spear-carrying hunt is faster, and cooking at an oven takes longer than
// at a fire. Both are derived from current state — Spears/the target
// structure — never persisted on the Intent, matching the codebase's
// "duration is encoded in WorkStart + completion timing, not the payload"
// convention (contracts/events.md).
func workDuration(s *State, a *Agent, in *Intent) int64 {
	switch in.Goal {
	case "hunt":
		if len(a.Inv.Spears) > 0 {
			return huntTicksSpear
		}
	case "cook":
		if s.structureAt("oven", in.TargetX, in.TargetY) {
			return cookOvenTicks
		}
	}
	return intentDuration(in.Goal)
}

func decayNeeds(n Needs, asleep, night, warm, onShelter bool) Needs {
	n.Food = maxInt(0, n.Food-foodDecay)
	if asleep {
		// T037: sleeping on a shelter tile recovers rest at the boosted rate
		// (the plank economy's payoff for the structure).
		regen := restRegenSleep
		if onShelter {
			regen = restRegenShelter
		}
		n.Rest = minInt(1000, n.Rest+regen)
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
	// Wake to a hunger emergency the agent can act on: any food in hand (T018).
	return a.Needs.Food < 150 && hasAnyFood(a)
}

// eatOutcome computes the most-nutritious-first eat (T018, FR-007): Meals →
// FoodCooked → FoodRaw, one unit at a time, until the Food need reaches
// satietyAt or the inventory runs out. It returns the outcome payload
// (consumed counts per form + the absolute post-eat need) and whether anything
// is eaten — false when already sated or carrying no food, so no unit is ever
// consumed at satiety (the eating-overshoot edge case). The caller sets Agent.
func eatOutcome(a *Agent) (AtePayload, bool) {
	food := a.Needs.Food
	if food >= satietyAt {
		return AtePayload{}, false
	}
	availM, availC, availR := a.Inv.Meals, a.Inv.FoodCooked, a.Inv.FoodRaw
	var meals, cooked, raw int
	for food < satietyAt && (availM > 0 || availC > 0 || availR > 0) {
		switch {
		case availM > 0:
			availM--
			meals++
			food = minInt(1000, food+mealRestore)
		case availC > 0:
			availC--
			cooked++
			food = minInt(1000, food+foodCookedRestore)
		default: // availR > 0
			availR--
			raw++
			food = minInt(1000, food+foodRawRestore)
		}
	}
	if meals == 0 && cooked == 0 && raw == 0 {
		return AtePayload{}, false
	}
	return AtePayload{Meals: meals, Cooked: cooked, Raw: raw, FoodAfter: food}, true
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

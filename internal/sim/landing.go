package sim

// The landing ladder (TASK-32, FR-010..FR-013; extracted TASK-70): the ordered
// decision a cognition result passes through when it lands on the world —
// unavailable pre-checks → superseded → stale → guard walk → plan-or-goal
// resolution → outcome emission. Enforcement is against the world as it is NOW;
// every metered rejection is recorded atomically with the verdict (the rejection
// events land even though the error is returned — silent failure is gone,
// FR-015). Each doctrine rung is a named unit; the walk produces one explicit
// landingDecision, replacing the former adapted/failed/hailTarget flag interplay.

import (
	"fmt"

	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/tool"
)

// landingDecision is the explicit outcome of the guard walk, consumed once each
// by the plan/goal paths and the final cog.outcome. It replaces the three
// cross-loop flags: outcome carries adapt (OutcomeLanded vs OutcomeAdapted) or a
// rejection; reason is the verbatim rejection text (empty on accept); hailTarget
// is the agent the goal path hails on success (-1 = none), last-write-wins.
type landingDecision struct {
	outcome    string
	reason     string
	hailTarget int
}

// landIntent runs the landing ladder for one inject_intent command. It returns
// the command error (a rejection both emits its records via emit AND returns the
// error — the only command path that pairs the two) and emits every landing
// event in the frozen order. Class=="" is the pre-TASK-32 contract: an unmetered
// caller skips the generation, staleness, and guard rungs and emits no telemetry.
func (l *Loop) landIntent(in *InjectArgs, emit func(string, any)) error {
	if in.Agent < 0 || in.Agent >= len(l.state.Agents) {
		// Bounds failure emits nothing (no such agent to attribute a rejection to).
		return fmt.Errorf("no such agent %d", in.Agent)
	}
	a := &l.state.Agents[in.Agent]
	staleness := l.state.Tick - in.SnapshotTick
	if staleness < 0 {
		staleness = 0
	}
	// reject records the metered rejection (agent.intent_rejected + cog.outcome)
	// AND returns the error; Class=="" rejections emit nothing but still error.
	reject := func(outcome, reason string) error {
		if in.Class != "" {
			kind := RejectKindWorldChange
			if in.PredictedWallMs > 0 && in.ActualWallMs > PredictionMissFactor*in.PredictedWallMs {
				kind = RejectKindPredictionMiss
			}
			emit("agent.intent_rejected", IntentRejectedPayload{
				Agent: in.Agent, Goal: in.Goal, Reason: reason, StalenessTicks: staleness,
			})
			emit("cog.outcome", CogOutcomePayload{
				Job: in.JobID, Class: in.Class, Agent: in.Agent,
				Outcome: outcome, SnapshotTick: in.SnapshotTick,
				LandingTick: l.state.Tick, StalenessTicks: staleness,
				PredictedWallMs: in.PredictedWallMs, ActualWallMs: in.ActualWallMs,
				Kind: kind, Reason: reason,
			})
		}
		return fmt.Errorf("%s: %s", outcome, reason)
	}

	if reason := rungUnavailable(a); reason != "" {
		return reject(OutcomeUnavailable, reason)
	}

	decision := landingDecision{outcome: OutcomeLanded, hailTarget: -1}
	if in.Class != "" {
		if reason := rungSuperseded(a, in.Generation); reason != "" {
			return reject(OutcomeSuperseded, reason)
		}
		if reason := rungStale(in.Class, staleness); reason != "" {
			return reject(OutcomeRejectedStale, reason)
		}
		decision = walkGuards(l.state, in)
		if decision.reason != "" {
			return reject(OutcomeRejectedGuard, decision.reason)
		}
	}

	if len(in.Plan) > 0 {
		// A guarded conditional plan (US4): validate at the door, then record the
		// steps — the executor evaluates guards per tick. The plan path never
		// hails (decision.hailTarget is consumed only on the goal path below).
		if len(in.Plan) > PlanStepCap {
			return reject(OutcomeRejectedGuard, fmt.Sprintf("plan has %d steps (cap %d)", len(in.Plan), PlanStepCap))
		}
		// The plan-step accept set is DERIVED from the tool registry (spec 014,
		// FR-006): names carrying PlanStep == true.
		planStepGoals := tool.PlanStepGoals()
		for si := range in.Plan {
			if !planStepGoals[in.Plan[si].Goal] {
				return reject(OutcomeRejectedGuard, fmt.Sprintf("plan step %d: unknown goal %q", si, in.Plan[si].Goal))
			}
			if in.Plan[si].Until == 0 {
				in.Plan[si].Until = l.state.Tick + PlanDefaultWindowTicks
			}
		}
		if in.Reason != "" {
			emit("agent.thought", ThoughtPayload{Agent: in.Agent, Text: in.Reason, Source: "planner"})
		}
		emit("agent.plan_set", PlanSetPayload{Agent: in.Agent, Job: in.JobID, Steps: in.Plan})
	} else {
		// Roster door check (spec 014 US3, FR-008/FR-009): capability is roster
		// membership. The goal must be a World tool on the villager roster; an
		// out-of-roster tool (a metatron converse/nudge) or an unknown name is
		// rejected here — recorded, non-fatal, same reason/kind as an unknown goal.
		if td, ok := tool.Lookup(in.Goal); !ok || td.Effect != tool.World || !tool.OnRoster(tool.RosterVillager, in.Goal) {
			return reject(OutcomeRejectedGuard, fmt.Sprintf("unknown goal %q", in.Goal))
		}
		intent, direct, rerr := resolveGoal(l.state, l.m, in.Agent, in.Goal, in.TargetAgent, in.Kind, in.Qty, l.state.Tick)
		if rerr != nil {
			// resolveGoal is the repair path; failing here means no deterministic
			// adaptation exists — a world change.
			return reject(OutcomeRejectedGuard, rerr.Error())
		}
		if in.Reason != "" {
			emit("agent.thought", ThoughtPayload{Agent: in.Agent, Text: in.Reason, Source: "planner"})
		}
		if direct == "agent.ate" {
			if p, ok := eatOutcome(&l.state.Agents[in.Agent]); ok {
				p.Agent = in.Agent
				emit("agent.ate", p)
			}
		} else if intent != nil {
			emit("agent.intent_set", IntentSetPayload{
				Agent: in.Agent, Goal: intent.Goal,
				TargetX: intent.TargetX, TargetY: intent.TargetY,
				ResX: intent.ResX, ResY: intent.ResY,
				Kind: intent.Kind, Qty: intent.Qty,
				Source: "planner", Job: in.JobID,
				// Spec 019 (R2): carry the planner's reason onto the intent so it
				// survives to completion, where the executor bakes it into the
				// memory's Why. Recorded input (already narrated as the
				// agent.thought above) — replay repopulates it from this event, so
				// live and replay stay identical with no new event.
				Reason: in.Reason,
			})
		}
		// The hail (TASK-47): a talk_to landing pauses a hailable target so the
		// hailer can close distance and the pair actually meets — every hailable
		// landing, in- or out-of-radius (FR-001, research D2).
		if decision.hailTarget >= 0 {
			emit("social.hailed", HailedPayload{
				From: in.Agent, To: decision.hailTarget, Until: l.state.Tick + hailWindowTicks})
		}
	}

	if in.Class != "" {
		emit("cog.outcome", CogOutcomePayload{
			Job: in.JobID, Class: in.Class, Agent: in.Agent,
			Outcome: decision.outcome, SnapshotTick: in.SnapshotTick,
			LandingTick: l.state.Tick, StalenessTicks: staleness,
			PredictedWallMs: in.PredictedWallMs, ActualWallMs: in.ActualWallMs,
		})
	}
	return nil
}

// rungUnavailable — the unavailable pre-check: a dead or asleep actor lands
// nothing. Dead is checked before asleep (ordering frozen), both before
// generation/staleness. Returns the doctrine reason, or "" when the actor can act.
func rungUnavailable(a *Agent) string {
	if a.Dead {
		return a.Name + " is dead"
	}
	if a.Asleep {
		return a.Name + " is asleep"
	}
	return ""
}

// rungSuperseded — the thought was formed under a prior generation; an emergency
// memory bumped the actor's generation since (FR-014). Returns the reason, or ""
// when the generation still matches.
func rungSuperseded(a *Agent, generation int64) string {
	if generation != a.Generation {
		return fmt.Sprintf("generation %d, thought was %d", a.Generation, generation)
	}
	return ""
}

// rungStale — the landing exceeded its class's staleness budget. Returns the
// reason, or "" when fresh (or the class carries no budget).
func rungStale(class string, staleness int64) string {
	if dc, ok := cognition.ClassFor(class); ok && staleness > dc.BudgetTicks {
		return fmt.Sprintf("staleness %d > budget %d", staleness, dc.BudgetTicks)
	}
	return ""
}

// walkGuards runs the guard rungs in order and produces one landingDecision. The
// first failing guard short-circuits (after the hail-relaxation attempt) into a
// guard-failed rejection; holding guards contribute adapt detection and in-radius
// hail marking. adapted, once set, stays; hailTarget is last-write-wins across
// the walk (order preserved). Called only when in.Class != "".
func walkGuards(s *State, in *InjectArgs) landingDecision {
	a := &s.Agents[in.Agent]
	d := landingDecision{outcome: OutcomeLanded, hailTarget: -1}
	for _, g := range in.Guards {
		ok, why := g.Eval(s, in.Agent)
		if !ok {
			if relaxed, hail := rungHailRelaxed(s, in, a, g); relaxed {
				d.outcome = OutcomeAdapted
				if hail >= 0 {
					d.hailTarget = hail
				}
				continue
			}
			return rungGuardFailed(why)
		}
		if rungAdapted(s, g) {
			d.outcome = OutcomeAdapted
		}
		if t, mark := rungInRadiusHail(s, in, g); mark {
			d.hailTarget = t
		}
	}
	return d
}

// rungHailRelaxed — the hail rung (TASK-47): a talk_to landing whose
// target_present guard just FAILED is not dead if the target can be flagged down
// — the world pauses the target so the hailer can close the distance. The guard
// vocabulary stays closed; the relaxation lives here, for alive targets only
// (dead/out-of-range fall through to the plain rejection). Two sub-cases, in
// order: the mutual-presence rung (D6 — the target is the actor's own hailer;
// the pair is already converging, so land adapted with NO new hail, never freeze
// a hailer), then the hailable target (land adapted AND hail). Returns
// (relaxed, hailTarget): relaxed asks the walk to land adapted; hailTarget >= 0
// asks the goal path to hail (-1 = adapt only).
func rungHailRelaxed(s *State, in *InjectArgs, a *Agent, g Guard) (bool, int) {
	if g.Type != GuardTargetPresent || in.Goal != "talk_to" ||
		g.Target < 0 || g.Target >= len(s.Agents) || s.Agents[g.Target].Dead {
		return false, -1
	}
	if a.Hail != nil && a.Hail.By == g.Target {
		return true, -1
	}
	if hailable(s, in.Agent, g.Target) {
		return true, g.Target
	}
	return false, -1
}

// rungGuardFailed — the plain guard rejection: the first failing guard that no
// relaxation covers short-circuits the walk with the guard's own reason.
func rungGuardFailed(why string) landingDecision {
	return landingDecision{outcome: OutcomeRejectedGuard, reason: why, hailTarget: -1}
}

// rungAdapted — a target_present guard that HOLDS but whose target moved from its
// snapshot position (g.X/g.Y) means resolveGoal repaired the intent: land
// adapted, not fresh. The zero snapshot (g.X==0 && g.Y==0) records no position
// and is never treated as a move.
func rungAdapted(s *State, g Guard) bool {
	if g.Type == GuardTargetPresent && (g.X != 0 || g.Y != 0) {
		t := &s.Agents[g.Target]
		return t.X != g.X || t.Y != g.Y
	}
	return false
}

// rungInRadiusHail — a talk_to whose target_present guard HOLDS and whose target
// is hailable is still hailed: it can wander during the walk-over and the
// courtesy pause is cheap (FR-001, research D2). Returns (target, true) to mark.
func rungInRadiusHail(s *State, in *InjectArgs, g Guard) (int, bool) {
	if g.Type == GuardTargetPresent && in.Goal == "talk_to" &&
		hailable(s, in.Agent, g.Target) {
		return g.Target, true
	}
	return -1, false
}

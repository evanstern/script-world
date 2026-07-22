package sim

// Guarded conditional plans (TASK-32 US4, FR-017): a planner reply may be a
// short sequence of steps, each gated by a deterministic guard and bounded
// by a validity window. Steps execute with no model involvement at firing
// time — timed guards are the sole act-at-time-T mechanism. The whole plan
// is recorded state (agent.plan_set), so replay is untouched.

import (
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// PlanStepCap bounds a plan: prompt-expressible in 256 tokens, and per-tick
// evaluation stays trivial.
const PlanStepCap = 3

// PlanDefaultWindowTicks is the validity window when the model gives none:
// 2 game hours.
const PlanDefaultWindowTicks = 2 * 3600

// PlanStep is one guarded step in deterministic state.
type PlanStep struct {
	Job    string `json:"job"`
	Goal   string `json:"goal"`
	Target int    `json:"target,omitempty"` // agent index for talk_to; -1/absent otherwise
	When   *Guard `json:"when,omitempty"`   // gate to start; nil = immediately
	Until  int64  `json:"until"`            // validity deadline (tick)
	// Kind/Qty (spec 013 R4) argue a storage plan step (drop/pick_up/deposit/
	// withdraw): Kind is an inventory item key ("" = all kinds), Qty the amount
	// (0 = all of kind / as much as fits). Both omitempty keep pre-013 and every
	// non-storage plan step byte-identical.
	Kind string `json:"kind,omitempty"`
	Qty  int    `json:"qty,omitempty"`
}

// planGoals mirrors the planner goal vocabulary — the loop validates plan
// steps against it at the door so garbage never enters state.
var planGoals = map[string]bool{
	"forage": true, "chop": true, "hunt": true,
	"build_fire": true, "build_shelter": true,
	"eat": true, "sleep": true, "wander": true,
	"goto_warmth": true, "talk_to": true,
	// Storage goals (spec 013 US2, FR-014): planner/plan-only — never in the
	// reflex ladder.
	"drop": true, "pick_up": true,
}

// PlanSetPayload — agent.plan_set (loop-emitted on a plan landing).
type PlanSetPayload struct {
	Agent int        `json:"agent"`
	Job   string     `json:"job"`
	Steps []PlanStep `json:"steps"`
}

// PlanStepPayload — agent.plan_step_started / agent.plan_expired.
type PlanStepPayload struct {
	Agent  int    `json:"agent"`
	Job    string `json:"job"`
	Step   string `json:"step"`
	Reason string `json:"reason,omitempty"`
}

// planStepEvents evaluates the head step for an idle agent (executor-called,
// pure). Holding (guard not yet true, window open) emits nothing; expiry
// clears the whole plan (v1 semantics: a broken sequence is not resumed);
// firing starts the step and sets its intent — Source "plan".
func planStepEvents(s *State, m *worldmap.Map, idx int, tick int64) []store.Event {
	a := &s.Agents[idx]
	st := a.Plan[0]
	ev := func(typ string, payload any) store.Event {
		return store.Event{Tick: tick, Type: typ, Payload: mustPayload(payload)}
	}
	if st.Until > 0 && tick >= st.Until {
		return []store.Event{ev("agent.plan_expired", PlanStepPayload{
			Agent: idx, Job: st.Job, Step: st.Goal, Reason: "window closed"})}
	}
	if st.When != nil {
		if ok, _ := st.When.EvalAt(s, idx, tick); !ok {
			return nil // holding — deterministically re-checked next tick
		}
	}
	intent, direct, err := resolveGoal(s, m, idx, st.Goal, st.Target, st.Kind, st.Qty, tick)
	if err != nil {
		return []store.Event{ev("agent.plan_expired", PlanStepPayload{
			Agent: idx, Job: st.Job, Step: st.Goal, Reason: err.Error()})}
	}
	evs := []store.Event{ev("agent.plan_step_started", PlanStepPayload{
		Agent: idx, Job: st.Job, Step: st.Goal})}
	if direct == "agent.ate" {
		if p, ok := eatOutcome(a); ok {
			p.Agent = idx
			evs = append(evs, ev("agent.ate", p))
		}
	} else if intent != nil {
		evs = append(evs, ev("agent.intent_set", IntentSetPayload{
			Agent: idx, Goal: intent.Goal,
			TargetX: intent.TargetX, TargetY: intent.TargetY,
			ResX: intent.ResX, ResY: intent.ResY,
			Kind: intent.Kind, Qty: intent.Qty,
			Source: "plan",
		}))
	}
	// The hail (TASK-47): a talk_to step firing pauses a hailable target so
	// the pair meets, exactly as a planner talk_to landing does (FR-001).
	if st.Goal == "talk_to" && hailable(s, idx, st.Target) {
		evs = append(evs, ev("social.hailed", HailedPayload{
			From: idx, To: st.Target, Until: tick + hailWindowTicks}))
	}
	return evs
}

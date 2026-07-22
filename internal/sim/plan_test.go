package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/tool"
)

// spec012Verbs are the nine goals the resource economy (spec 012) added to the
// planner vocabulary but which the old hand-maintained planGoals map never
// picked up — so they were silently rejected as multi-step plan steps
// (violating spec 012 FR-020). Deriving the plan-step door from the tool
// registry cures that drift: they now carry PlanStep == true and are accepted.
var spec012Verbs = []string{
	"quarry", "collect_water", "cook", "refuel_fire",
	"craft_planks", "craft_stone", "craft_spear", "build_oven", "bathe",
}

// TestPlanStepVocabulary (T018, FR-012 / TASK-55 AC#1): each of the nine
// spec-012 verbs is accepted as a guarded plan step at the intent door — the
// SOLE permitted behavioral delta of the tool-registry layer. Before the cure,
// planGoals dropped these nine and the door rejected them as "unknown goal".
func TestPlanStepVocabulary(t *testing.T) {
	// The derived plan-step accept set (what the door consults) admits all nine.
	planSet := tool.PlanStepGoals()
	for _, v := range spec012Verbs {
		if !planSet[v] {
			t.Errorf("spec-012 verb %q missing from PlanStepGoals — drift cure regressed", v)
		}
	}

	// Drive the real door: each verb, as a single-step plan, lands (no
	// rejection) with a metered planner outcome of "landed". A plan is capped at
	// PlanStepCap steps, so the nine are injected one per plan.
	h := newLadderHarness(t, nil)
	for _, v := range spec012Verbs {
		args := meteredArgs(0, "")
		args.JobID = "planner-drift-" + v
		args.Plan = []PlanStep{{Job: args.JobID, Goal: v}}
		if err := h.loop.InjectIntent(args); err != nil {
			t.Errorf("plan step %q rejected at the door: %v (FR-012 drift cure regressed)", v, err)
			continue
		}
		p, ok := h.lastOutcome(t)
		if !ok || p.Outcome != OutcomeLanded {
			t.Errorf("plan step %q: outcome %+v, want landed", v, p)
		}
	}

	// Control: a genuinely unknown goal is still rejected at the door — the
	// registry closed the drift without opening the door to garbage.
	args := meteredArgs(0, "")
	args.JobID = "planner-drift-control"
	args.Plan = []PlanStep{{Job: args.JobID, Goal: "not_a_real_goal"}}
	if err := h.loop.InjectIntent(args); err == nil {
		t.Error("unknown plan-step goal was accepted at the door")
	}
	if p, ok := h.lastOutcome(t); !ok || p.Outcome != OutcomeRejectedGuard {
		t.Errorf("unknown goal control: outcome %+v, want rejected-guard", p)
	}
}

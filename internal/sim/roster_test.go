package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/tool"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// intentSetLanded reports how many agent.intent_set events the harness recorded
// (countType lives in hail_test.go — this reads the store once).
func intentSetLanded(t *testing.T, h *ladderHarness) int {
	t.Helper()
	evs, err := h.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return countType(evs, "agent.intent_set")
}

// TestVillagerDoorRejectsOutOfRoster (US3 scenarios 1–2, FR-009/SC-005): a
// villager action naming a metatron-only tool (converse, nudge_dream), an
// expressive tool that is not an intent-door goal (say), or an unknown name is
// rejected at the intent door — recorded, non-fatal, and NO agent.intent_set
// lands. Rejection is identical in shape to today's unknown-goal handling.
func TestVillagerDoorRejectsOutOfRoster(t *testing.T) {
	for _, goal := range []string{"nudge_dream", "nudge_omen", "converse", "say", "not_a_real_tool"} {
		h := newLadderHarness(t, nil)
		args := meteredArgs(0, goal)
		args.JobID = "planner-roster-" + goal
		if err := h.loop.InjectIntent(args); err == nil {
			t.Errorf("goal %q was accepted at the villager door", goal)
		}
		if p, ok := h.lastOutcome(t); !ok || p.Outcome != OutcomeRejectedGuard {
			t.Errorf("goal %q: outcome %+v, want rejected-guard", goal, p)
		}
		if n := intentSetLanded(t, h); n != 0 {
			t.Errorf("goal %q: %d intent_set events landed despite rejection", goal, n)
		}
	}

	// The rejection is data-driven: the metatron tools are simply absent from
	// the villager roster.
	for _, n := range []string{"nudge_dream", "nudge_omen", "converse"} {
		if tool.OnRoster(tool.RosterVillager, n) {
			t.Errorf("metatron tool %q is unexpectedly on the villager roster", n)
		}
	}

	// Control: a real world verb still lands (accept set unchanged for real
	// traffic).
	h := newLadderHarness(t, nil)
	args := meteredArgs(0, "wander")
	args.JobID = "planner-roster-control"
	if err := h.loop.InjectIntent(args); err != nil {
		t.Errorf("real world verb wander was rejected: %v", err)
	}
	if p, ok := h.lastOutcome(t); !ok || p.Outcome != OutcomeLanded {
		t.Errorf("wander: outcome %+v, want landed", p)
	}
}

// TestMetatronReducerRejectsOutOfRoster (US3 scenario 2): a nudge whose form is
// not a metatron-roster nudge tool is refused by the reducer dry-run, and no
// charge is spent — the same non-fatal handling as an unknown form.
func TestMetatronReducerRejectsOutOfRoster(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	for _, form := range []string{"converse", "whisper"} {
		s := NewState(7, m)
		before := s.MetatronCharges
		err := s.Apply(nudgeEvent(t, 50, MetatronNudgedPayload{Form: form, Targets: []int{0}, Text: "x"}))
		if err == nil {
			t.Errorf("form %q: reducer accepted an out-of-roster nudge form", form)
		}
		if s.MetatronCharges != before {
			t.Errorf("form %q: rejected nudge changed charges %d -> %d", form, before, s.MetatronCharges)
		}
	}

	// The metatron roster names only converse and the two nudge forms.
	if tool.OnRoster(tool.RosterMetatron, "nudge_whisper") {
		t.Error("nudge_whisper is unexpectedly on the metatron roster")
	}
}

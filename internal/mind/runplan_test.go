package mind

// T012 runPlan integration tests: runPlan drives a scripted loop (the runLoop
// seam) through the REAL handlers and door, then maps the termination onto
// telemetry + rearm. These pin the outcome/no-double-emission and rearm
// semantics of the migration (the real toolloop.Run is covered by
// internal/toolloop; the real transport by the e2e).

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/toolloop"
)

// countByType tallies store events by type after a synchronous runPlan.
func (lm *loopMind) countByType(t *testing.T) map[string]int {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := map[string]int{}
	for _, e := range evs {
		c[e.Type]++
	}
	return c
}

// plannerIntents counts agent.intent_set events sourced by the planner (the
// running loop's reflex lands its own "reflex"-sourced intents, which these
// tests must not count as planner landings).
func (lm *loopMind) plannerIntents(t *testing.T) int {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range evs {
		if e.Type != "agent.intent_set" {
			continue
		}
		var p sim.IntentSetPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Source == "planner" {
			n++
		}
	}
	return n
}

// outcomesFor returns the cog.outcome payloads for a job, in order.
func (lm *loopMind) outcomesFor(t *testing.T, job string) []sim.CogOutcomePayload {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []sim.CogOutcomePayload
	for _, e := range evs {
		if e.Type != "cog.outcome" {
			continue
		}
		var p sim.CogOutcomePayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Job == job {
			out = append(out, p)
		}
	}
	return out
}

// TestRunPlanWorldVerbLandsIntent: the model calls one world verb; it lands as
// an intent, the door emits the sole cog.outcome(landed), and runPlan adds no
// second outcome (no double-emission) and does not rearm.
func TestRunPlanWorldVerbLandsIntent(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		out := j.Handlers["wander"](ctx, call("wander", "{}"))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "wander", Verdict: out.Verdict})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}

	lm.md.runPlan(job)

	counts := lm.countByType(t)
	if counts["cog.thought"] != 1 {
		t.Errorf("cog.thought = %d, want 1", counts["cog.thought"])
	}
	if lm.plannerIntents(t) == 0 {
		t.Fatal("planner intent never landed")
	}
	outs := lm.outcomesFor(t, job.meta.job)
	if len(outs) != 1 {
		t.Fatalf("job has %d cog.outcome events, want exactly 1 (no double-emission)", len(outs))
	}
	if outs[0].Outcome != sim.OutcomeLanded {
		t.Errorf("outcome = %q, want landed", outs[0].Outcome)
	}
	if len(lm.md.rearm) != 0 {
		t.Error("a landed cognition must not rearm")
	}
}

// TestRunPlanSetPlanLands: a set_plan call lands a plan_set with one outcome.
func TestRunPlanSetPlanLands(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		out := j.Handlers["set_plan"](ctx, call("set_plan", `{"steps":[{"goal":"wander"},{"goal":"forage"}]}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "set_plan", Verdict: out.Verdict})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}

	lm.md.runPlan(job)

	if lm.countByType(t)["agent.plan_set"] == 0 {
		t.Fatal("no plan landed")
	}
	if len(lm.outcomesFor(t, job.meta.job)) != 1 {
		t.Error("set_plan landing must emit exactly one cog.outcome")
	}
}

// TestRunPlanMuseLandsThought: a muse call lands an agent.thought + its
// cog.outcome(landed); runPlan adds no second outcome and does not rearm.
func TestRunPlanMuseLandsThought(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		out := j.Handlers["muse"](ctx, call("muse", `{"text":"The river runs low."}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "muse", Verdict: out.Verdict})
		if out.Verdict == toolloop.VerdictLanded {
			return toolloop.Result{Term: toolloop.TermLanded}, nil
		}
		return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
	}

	lm.md.runPlan(job)

	evs, _ := lm.st.EventsSince(0, 0)
	var thought bool
	for _, e := range evs {
		if e.Type == "agent.thought" {
			var p sim.ThoughtPayload
			json.Unmarshal(e.Payload, &p)
			if p.Source == "musing" && p.Text == "The river runs low." {
				thought = true
			}
		}
	}
	if !thought {
		t.Fatal("muse tool did not land its thought")
	}
	if len(lm.outcomesFor(t, job.meta.job)) != 1 {
		t.Error("muse landing must emit exactly one cog.outcome")
	}
	if len(lm.md.rearm) != 0 {
		t.Error("a landed muse must not rearm")
	}
}

// TestRunPlanGateRejectionThenRetryLands: a first acting call is gate-rejected
// (door records the rejection, fed back), the model retries and the second
// call lands. The cognition ends landed — no rearm — and the door recorded both
// the rejection and the landing.
func TestRunPlanGateRejectionThenRetryLands(t *testing.T) {
	lm := newLoopMind(t)
	// Kill target 1 so the first talk_to is gate-rejected by the alive guard.
	target := 1
	tname := lm.md.replica.Agents[target].Name
	lm.md.replica.Agents[target].Dead = true
	job := lm.newJob(0)

	var rejReason string
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		out1 := j.Handlers["talk_to"](ctx, call("talk_to", `{"target":"`+tname+`"}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "talk_to", Verdict: out1.Verdict, Reason: out1.ResultForModel})
		if out1.Verdict != toolloop.VerdictRejectedGate {
			t.Fatalf("first call verdict = %q, want rejected_gate", out1.Verdict)
		}
		rejReason = out1.ResultForModel // fed back to the model
		out2 := j.Handlers["wander"](ctx, call("wander", "{}"))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 2, Tool: "wander", Verdict: out2.Verdict})
		if out2.Verdict == toolloop.VerdictLanded {
			return toolloop.Result{Term: toolloop.TermLanded}, nil
		}
		return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
	}

	lm.md.runPlan(job)

	if rejReason == "" {
		t.Error("rejection reason not threaded back for retry")
	}
	if lm.plannerIntents(t) == 0 {
		t.Fatal("retry never landed a planner intent")
	}
	outs := lm.outcomesFor(t, job.meta.job)
	// The door records a verdict per acting InjectIntent: one rejection, then
	// one landing — its non-silent-rejection contract, preserved by the loop.
	var landed, rejected int
	for _, o := range outs {
		switch o.Outcome {
		case sim.OutcomeLanded, sim.OutcomeAdapted:
			landed++
		case sim.OutcomeRejectedGuard, sim.OutcomeRejectedStale, sim.OutcomeUnavailable, sim.OutcomeSuperseded:
			rejected++
		}
	}
	if landed != 1 || rejected != 1 {
		t.Errorf("job outcomes: %d landed, %d rejected; want 1 and 1 (%+v)", landed, rejected, outs)
	}
	if len(lm.md.rearm) != 0 {
		t.Error("a cognition that ultimately landed must not rearm")
	}
}

// TestRunPlanModelDoneFailsNoRearm: the model produced no tool call (plain
// text). No door was touched, so runPlan records the terminal unusable outcome
// (FR-015) and does NOT rearm — the reflex floor covers, matching today's
// unusable-reply path.
func TestRunPlanModelDoneFailsNoRearm(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		return toolloop.Result{Term: toolloop.TermModelDone}, nil
	}

	lm.md.runPlan(job)

	outs := lm.outcomesFor(t, job.meta.job)
	if len(outs) != 1 || outs[0].Outcome != sim.OutcomeUnusable {
		t.Fatalf("model_done outcomes = %+v, want exactly one unusable (FR-015)", outs)
	}
	if outs[0].Reason == "" {
		t.Error("unusable outcome carries no reason")
	}
	if lm.plannerIntents(t) != 0 {
		t.Error("model_done must not land any planner intent")
	}
	if len(lm.md.rearm) != 0 {
		t.Error("a no-action failure must not rearm (reflex covers)")
	}
}

// TestRunPlanCapAfterRejectionRearms: every acting call is gate-rejected until
// the cap; the door recorded the rejection(s), so the mind adds no outcome but
// DOES rearm a re-plan — mirroring today's rejection-rearm.
func TestRunPlanCapAfterRejectionRearms(t *testing.T) {
	lm := newLoopMind(t)
	target := 1
	tname := lm.md.replica.Agents[target].Name
	lm.md.replica.Agents[target].Dead = true
	job := lm.newJob(0)

	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		out := j.Handlers["talk_to"](ctx, call("talk_to", `{"target":"`+tname+`"}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "talk_to", Verdict: out.Verdict, Reason: out.ResultForModel})
		return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
	}

	lm.md.runPlan(job)

	// The door recorded the rejection; the mind must not add an unusable on top.
	outs := lm.outcomesFor(t, job.meta.job)
	if len(outs) != 1 || outs[0].Outcome == sim.OutcomeUnusable {
		t.Fatalf("cap-after-rejection outcomes = %+v, want the door's single rejection (no added unusable)", outs)
	}
	if len(lm.md.rearm) != 1 {
		t.Errorf("a gate-rejected cognition that landed nothing must rearm; rearm depth = %d", len(lm.md.rearm))
	}
}

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
	"time"

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

// toolCallsFor returns the cog.tool_call payloads for a job, in log order.
func (lm *loopMind) toolCallsFor(t *testing.T, job string) []sim.CogToolCallPayload {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []sim.CogToolCallPayload
	for _, e := range evs {
		if e.Type != "cog.tool_call" {
			continue
		}
		var p sim.CogToolCallPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Job == job {
			out = append(out, p)
		}
	}
	return out
}

// plannerIntentJobs returns the Job of every planner-sourced agent.intent_set —
// the grounding events a cog.tool_call chain resolves against.
func (lm *loopMind) plannerIntentJobs(t *testing.T) []string {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var jobs []string
	for _, e := range evs {
		if e.Type != "agent.intent_set" {
			continue
		}
		var p sim.IntentSetPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Source == "planner" {
			jobs = append(jobs, p.Job)
		}
	}
	return jobs
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

// --- T018: CallRecords land as cog.tool_call events ---

// TestRunPlanEmitsToolCallChain: a read lookup (ordinal 1) precedes a landed
// world verb (ordinal 2). Both records land as cog.tool_call events in ordinal
// order carrying the cognition's job, correct verdicts, tier and snapshot tick;
// and the landed agent.intent_set carries the SAME job — the AC#5 chain from
// grounding event to its causing call resolves by identifier alone.
func TestRunPlanEmitsToolCallChain(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		// Ordinal 1: a read lookup grounds nothing (recorded, present).
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "recall",
			Args: json.RawMessage(`{"q":"river"}`), Verdict: toolloop.VerdictReadOK, Tier: "local"})
		// Ordinal 2: an acting call lands through the real door.
		out := j.Handlers["wander"](ctx, call("wander", "{}"))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 2, Tool: "wander",
			Verdict: out.Verdict, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}

	lm.md.runPlan(job)

	tcs := lm.toolCallsFor(t, job.meta.job)
	if len(tcs) != 2 {
		t.Fatalf("cog.tool_call count = %d, want 2 (%+v)", len(tcs), tcs)
	}
	if tcs[0].Ordinal != 1 || tcs[1].Ordinal != 2 {
		t.Errorf("ordinals out of order: %d, %d", tcs[0].Ordinal, tcs[1].Ordinal)
	}
	if tcs[0].Verdict != "read_ok" || tcs[1].Verdict != "landed" {
		t.Errorf("verdicts = %q, %q; want read_ok, landed", tcs[0].Verdict, tcs[1].Verdict)
	}
	if tcs[0].Tier != "local" || tcs[0].SnapshotTick != job.meta.snapshotTick {
		t.Errorf("tier/snapshot_tick = %q/%d, want local/%d", tcs[0].Tier, tcs[0].SnapshotTick, job.meta.snapshotTick)
	}
	if string(tcs[0].Args) != `{"q":"river"}` {
		t.Errorf("args = %s, want the recorded lookup args", tcs[0].Args)
	}
	// The chain: the landed intent_set carries the same job as the landed call.
	var chained bool
	for _, j := range lm.plannerIntentJobs(t) {
		if j == job.meta.job && j == tcs[1].Job {
			chained = true
		}
	}
	if !chained {
		t.Errorf("landed intent_set does not carry job %q — the chain is broken", job.meta.job)
	}
}

// TestRunPlanEmitsToolCallsOnRejectionNoGrounding: every acting call is
// gate-rejected to the cap. The rejected call is still recorded as a
// cog.tool_call with its reason (present and queryable), and NO grounding
// event (agent.intent_set) carries the job — a never-grounded call, recorded.
func TestRunPlanEmitsToolCallsOnRejectionNoGrounding(t *testing.T) {
	lm := newLoopMind(t)
	target := 1
	tname := lm.md.replica.Agents[target].Name
	lm.md.replica.Agents[target].Dead = true
	job := lm.newJob(0)

	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		out := j.Handlers["talk_to"](ctx, call("talk_to", `{"target":"`+tname+`"}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "talk_to",
			Args:    json.RawMessage(`{"target":"` + tname + `"}`),
			Verdict: out.Verdict, Reason: out.ResultForModel, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
	}

	lm.md.runPlan(job)

	tcs := lm.toolCallsFor(t, job.meta.job)
	if len(tcs) != 1 {
		t.Fatalf("cog.tool_call count = %d, want 1 (%+v)", len(tcs), tcs)
	}
	if tcs[0].Verdict != "rejected_gate" {
		t.Errorf("verdict = %q, want rejected_gate", tcs[0].Verdict)
	}
	if tcs[0].Reason == "" {
		t.Error("a rejected cog.tool_call must carry a non-empty reason (AC#5)")
	}
	// No grounding event shares the job — it grounded nothing.
	for _, j := range lm.plannerIntentJobs(t) {
		if j == job.meta.job {
			t.Errorf("a rejected-only cognition must not have an intent_set carrying its job %q", job.meta.job)
		}
	}
}

// TestRunPlanNoToolCallsWhenBufferEmpty: a cognition that records nothing (the
// model produced no tool call) emits no cog.tool_call events — no empty batch.
func TestRunPlanNoToolCallsWhenBufferEmpty(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		return toolloop.Result{Term: toolloop.TermModelDone}, nil
	}

	lm.md.runPlan(job)

	if n := lm.countByType(t)["cog.tool_call"]; n != 0 {
		t.Errorf("cog.tool_call count = %d, want 0 (empty buffer emits no batch)", n)
	}
}

// --- T019: SC-003 correlation gate ---

// groundingJobs returns every job carried by a grounding event (agent.intent_set
// / agent.plan_set) — the events a tool-call chain resolves back to.
func (lm *loopMind) groundingJobs(t *testing.T) map[string]bool {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	jobs := map[string]bool{}
	for _, e := range evs {
		switch e.Type {
		case "agent.intent_set":
			var p sim.IntentSetPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Job != "" {
				jobs[p.Job] = true
			}
		case "agent.plan_set":
			var p sim.PlanSetPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Job != "" {
				jobs[p.Job] = true
			}
		}
	}
	return jobs
}

// awaitReflexIntentSet returns the raw payload of the running loop's first
// reflex-authored agent.intent_set — the negative control for the chain. The
// loop runs at max speed over idle agents, so a reflex fires within the window.
func (lm *loopMind) awaitReflexIntentSet(t *testing.T) json.RawMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs, err := lm.st.EventsSince(0, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if e.Type != "agent.intent_set" {
				continue
			}
			var p sim.IntentSetPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Source == "reflex" {
				return e.Payload
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no reflex agent.intent_set appeared within the deadline")
	return nil
}

// TestToolCallCorrelationChainSC003 is the spec 017 SC-003 gate: for every tool
// call the call artifact + verdict resolve from the event log by identifier
// (job+ordinal) alone, and every tool-caused grounding event reaches its causing
// call the same way — with ZERO adjacency inference. Fixture: one landed
// cognition with a preceding rejection, one rejected-only cognition, and the
// running loop's reflex intent_set (no job, the negative control).
func TestToolCallCorrelationChainSC003(t *testing.T) {
	lm := newLoopMind(t)
	// Kill agent 1 so a talk_to to it is gate-rejected by the alive guard.
	target := 1
	tname := lm.md.replica.Agents[target].Name
	lm.md.replica.Agents[target].Dead = true

	// Cognition A (agent 0): a gate-rejected call (ordinal 1) precedes a landed
	// world verb (ordinal 2) — a landing whose job has a preceding rejection.
	jobA := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		o1 := j.Handlers["talk_to"](ctx, call("talk_to", `{"target":"`+tname+`"}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "talk_to",
			Verdict: o1.Verdict, Reason: o1.ResultForModel, Tier: "local"})
		o2 := j.Handlers["wander"](ctx, call("wander", "{}"))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 2, Tool: "wander",
			Verdict: o2.Verdict, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}
	lm.md.runPlan(jobA)

	// Cognition B (agent 2): a single gate-rejected call, nothing lands.
	jobB := lm.newJob(2)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		o := j.Handlers["talk_to"](ctx, call("talk_to", `{"target":"`+tname+`"}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "talk_to",
			Verdict: o.Verdict, Reason: o.ResultForModel, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
	}
	lm.md.runPlan(jobB)

	// Index every cog.tool_call by {job, ordinal} — the ONLY correlation key.
	// Resolution below never inspects log position or neighbors.
	byJobOrdinal := map[string]map[int]sim.CogToolCallPayload{}
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type != "cog.tool_call" {
			continue
		}
		var p sim.CogToolCallPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if byJobOrdinal[p.Job] == nil {
			byJobOrdinal[p.Job] = map[int]sim.CogToolCallPayload{}
		}
		byJobOrdinal[p.Job][p.Ordinal] = p
	}

	// (a) From the landed grounding event, extract .job and resolve the causing
	// landed call + its sibling records purely by job+ordinal.
	var landedJob string
	for _, e := range evs {
		if e.Type != "agent.intent_set" {
			continue
		}
		var p sim.IntentSetPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Source == "planner" && p.Job != "" {
			landedJob = p.Job
		}
	}
	if landedJob != jobA.meta.job {
		t.Fatalf("landed grounding job = %q, want cognition A's job %q", landedJob, jobA.meta.job)
	}
	siblings := byJobOrdinal[landedJob]
	if len(siblings) != 2 {
		t.Fatalf("job %q resolves %d tool calls, want 2 (the rejection + the landing)", landedJob, len(siblings))
	}
	if siblings[2].Verdict != "landed" {
		t.Errorf("ordinal 2 verdict = %q, want landed (the causing call)", siblings[2].Verdict)
	}
	if siblings[1].Verdict != "rejected_gate" || siblings[1].Reason == "" {
		t.Errorf("ordinal 1 sibling = %q/%q, want a rejected_gate with a reason", siblings[1].Verdict, siblings[1].Reason)
	}

	// (b) A rejected call resolves to its cog.tool_call, and NO grounding event
	// shares its job — present, queryable, grounding nothing.
	rej := byJobOrdinal[jobB.meta.job]
	if len(rej) != 1 || rej[1].Verdict != "rejected_gate" {
		t.Fatalf("rejected-only job %q resolves %+v, want one rejected_gate", jobB.meta.job, rej)
	}
	if lm.groundingJobs(t)[jobB.meta.job] {
		t.Errorf("a grounding event carries the rejected-only job %q — it should ground nothing", jobB.meta.job)
	}
	// Sanity: the landed job IS grounded (the positive half of the same check).
	if !lm.groundingJobs(t)[landedJob] {
		t.Errorf("landed job %q has no grounding event", landedJob)
	}

	// (c) Negative control: the reflex intent_set has no job key, so it is
	// outside the chain — unreachable from any cog.tool_call.
	reflex := lm.awaitReflexIntentSet(t)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(reflex, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["job"]; ok {
		t.Errorf("reflex agent.intent_set carries a job key, joining the chain: %s", reflex)
	}
}

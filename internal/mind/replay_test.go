package mind

// T015 (spec 017 US2 / SC-002): live-vs-replay determinism for a LOOP-ERA run.
//
// The e2e's TestCognitionReplayByteIdentical proves the daemon's own
// snapshot+tail path against a genesis replay on a live mock-LLM run. This test
// goes beyond it in three ways the spec's SC-002 names explicitly:
//
//  1. Full loop artifact set. The event log here carries every durable trace
//     the tool-use loop produces — cog.tool_call records including a REJECTED
//     verdict, an agent.intent_set carrying its causing job (IntentSetPayload.Job),
//     an agent.plan_set carrying its job, and a muse-tool agent.thought landing.
//  2. Byte-identical BOTH ways against the LIVE state. Genesis replay and
//     snapshot+tail replay are each compared to the authoritative live state
//     captured coherently from the loop (DoState), not merely to each other.
//  3. Structural zero-invocation during replay. The replay legs construct no
//     Mind, no Orchestrator, and no handlers — only sim.State + store.ReplayEvents
//     + State.Apply — and we assert affirmatively that replay appended zero
//     events and invoked the model/handler seam zero times.
//
// The harness runs the sim.Loop PAUSED, so the ONLY events in the log are the
// scripted cognitions' — no free-running executor ticks, no reflex intents —
// giving a fully deterministic, race-free log whose live state is a pure
// function of the events (no un-logged state mutation anywhere).

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// newPausedLoopMind builds the same bare Mind + real sim.Loop as newLoopMind,
// but starts the loop PAUSED so no tick/reflex events ever enter the log — only
// the cognitions we script. It returns the loop too, for coherent DoState reads.
func newPausedLoopMind(t *testing.T) (*loopMind, *sim.Loop, *worldmap.Map) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	state.Paused = true // the isolation: a paused loop steps no ticks, fires no reflex
	loop := sim.NewLoop(state, m, st, nil)
	md := &Mind{
		loop:    loop,
		social:  loop,
		replica: state,
		m:       m,
		rearm:   make(chan int, sim.AgentCount),
	}
	md.tick.Store(state.Tick)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("loop did not stop")
		}
		st.Close()
	})
	return &loopMind{md: md, st: st}, loop, m
}

// replayState rebuilds a sim.State purely from the store: a fresh genesis state
// (snap == nil) or a snapshot overlaid onto one, then every event after fromSeq
// applied through the reducer. This is the ENTIRE replay surface — no Mind, no
// Orchestrator, no handlers, no model are in scope here (SC-002's "zero
// tool-handler or model invocations during replay" holds by construction).
func replayState(t *testing.T, seed uint64, m *worldmap.Map, st *store.Store, fromSeq int64, snap []byte) *sim.State {
	t.Helper()
	s := sim.NewState(seed, m)
	if snap != nil {
		if err := json.Unmarshal(snap, s); err != nil {
			t.Fatalf("unmarshal snapshot: %v", err)
		}
	} else {
		// Genesis reconstruction. This fixture world starts PAUSED so its log is
		// deterministic (a paused loop steps no ticks, fires no reflex). "Paused"
		// is an INITIAL CONDITION of the world — like seed and the map, it is not
		// a logged event — so genesis-from-constructor must reconstruct it before
		// replaying the log onto it. (A snapshot leg gets it for free: the flag is
		// serialized in the snapshot bytes.)
		s.Paused = true
	}
	if err := st.ReplayEvents(fromSeq, func(e store.Event) error {
		if e.Tick > s.Tick {
			s.Tick = e.Tick
		}
		return s.Apply(e)
	}); err != nil {
		t.Fatalf("replay from seq %d: %v", fromSeq, err)
	}
	return s
}

func TestLoopRunReplayByteIdenticalSC002(t *testing.T) {
	lm, loop, m := newPausedLoopMind(t)

	// loopRuns counts every scripted-driver (model seam) invocation. Replay must
	// never touch it — it is the affirmative half of the zero-invocation proof.
	loopRuns := 0

	// --- Cognition A (agent 0): a set_plan landing → agent.plan_set{job}. ---
	jobA := lm.newJob(0)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		loopRuns++
		out := j.Handlers["set_plan"](ctx, call("set_plan", `{"steps":[{"goal":"wander"},{"goal":"forage"}]}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "set_plan", Verdict: out.Verdict, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}
	lm.md.runPlan(jobA)

	// Snapshot the world coherently AFTER cognition A (the loop is paused, so
	// this state reflects exactly the events up to snapSeq).
	snapBytes, snapStatus, err := loop.DoState()
	if err != nil {
		t.Fatalf("snapshot DoState: %v", err)
	}
	snapSeq := snapStatus.LastSeq

	// --- Cognition B (agent 1): a driver-side REJECTED call (an off-roster tool
	// the model hallucinated — recorded rejected_unknown, no door touched, no
	// state mutated) precedes a landed world verb → cog.tool_call{rejected} +
	// agent.intent_set{job}. ---
	jobB := lm.newJob(1)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		loopRuns++
		// Ordinal 1: off-roster tool, rejected by the driver before any handler
		// runs — exactly what toolloop.Run records for a hallucinated tool. It
		// grounds nothing and mutates nothing, so the log fully determines state.
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "teleport",
			Verdict: toolloop.VerdictRejectedUnknown,
			Reason:  `tool "teleport" is not on this cognition's roster`, Tier: "local"})
		// Ordinal 2: a valid world verb lands.
		out := j.Handlers["wander"](ctx, call("wander", "{}"))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 2, Tool: "wander", Verdict: out.Verdict, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}
	lm.md.runPlan(jobB)

	// --- Cognition C (agent 2): a muse landing → agent.thought{source musing}. ---
	jobC := lm.newJob(2)
	lm.md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		loopRuns++
		out := j.Handlers["muse"](ctx, call("muse", `{"text":"The river runs low tonight."}`))
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "muse", Verdict: out.Verdict, Tier: "local"})
		return toolloop.Result{Term: toolloop.TermLanded}, nil
	}
	lm.md.runPlan(jobC)

	// Capture the authoritative live state + log position after all cognitions.
	liveBytes, liveStatus, err := loop.DoState()
	if err != nil {
		t.Fatalf("live DoState: %v", err)
	}
	liveSeq := liveStatus.LastSeq
	if liveSeq <= snapSeq {
		t.Fatalf("no events after the snapshot (snapSeq=%d liveSeq=%d) — the snapshot+tail leg would be vacuous", snapSeq, liveSeq)
	}

	// --- Guard: the log genuinely carries the full loop artifact set, or the
	// byte-identity proof below would be vacuous. ---
	assertFullArtifactSet(t, lm)

	// --- Derivation A: genesis → full replay. Byte-identical to live. ---
	genesis := replayState(t, 42, m, lm.st, 0, nil)
	if string(genesis.Marshal()) != string(liveBytes) {
		t.Errorf("SC-002: genesis replay diverged from live state\nlive:    %s\nreplay:  %s", liveBytes, genesis.Marshal())
	}

	// --- Derivation B: snapshot + tail (the daemon's own recovery path).
	// Byte-identical to live. ---
	fromSnap := replayState(t, 42, m, lm.st, snapSeq, snapBytes)
	if string(fromSnap.Marshal()) != string(liveBytes) {
		t.Errorf("SC-002: snapshot+tail replay diverged from live state\nlive:    %s\nreplay:  %s", liveBytes, fromSnap.Marshal())
	}

	// --- Structural zero-invocation proof. ---
	// (a) Replay appended NOTHING to the log: a handler landing (or any door
	// call) would have emitted events and grown LastSeq. Zero growth ⇒ no
	// handler/door ran during either replay leg.
	if got := lm.st.LastSeq(); got != liveSeq {
		t.Errorf("replay appended %d event(s) (LastSeq %d→%d) — replay must be event-free", got-liveSeq, liveSeq, got)
	}
	// (b) The model seam ran exactly the three times the live run drove it, and
	// not once more during replay.
	if loopRuns != 3 {
		t.Errorf("model seam invoked %d times, want 3 (once per live cognition, zero during replay)", loopRuns)
	}
}

// assertFullArtifactSet fails unless the log carries every trace SC-002 wants a
// loop-era run to exercise: a rejected-verdict cog.tool_call, an intent_set and
// a plan_set each carrying a job, and a muse agent.thought.
func assertFullArtifactSet(t *testing.T, lm *loopMind) {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sawRejectedCall, sawIntentJob, sawPlanJob, sawMuse bool
	for _, e := range evs {
		switch e.Type {
		case "cog.tool_call":
			var p sim.CogToolCallPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Verdict == "rejected_unknown" {
				sawRejectedCall = true
			}
		case "agent.intent_set":
			var p sim.IntentSetPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Job != "" {
				sawIntentJob = true
			}
		case "agent.plan_set":
			var p sim.PlanSetPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Job != "" {
				sawPlanJob = true
			}
		case "agent.thought":
			var p sim.ThoughtPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Source == "musing" {
				sawMuse = true
			}
		}
	}
	if !sawRejectedCall {
		t.Error("log carries no rejected-verdict cog.tool_call — artifact set incomplete")
	}
	if !sawIntentJob {
		t.Error("log carries no agent.intent_set with a job — artifact set incomplete")
	}
	if !sawPlanJob {
		t.Error("log carries no agent.plan_set with a job — artifact set incomplete")
	}
	if !sawMuse {
		t.Error("log carries no muse agent.thought — artifact set incomplete")
	}
}

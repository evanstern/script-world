package sim

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/store"
)

// TestCognitionTelemetryWhitelisted: the cog.* lifecycle types ride the
// inject_social door; agent.intent_rejected is loop-emitted only and must
// NOT be injectable from the mind.
func TestCognitionTelemetryWhitelisted(t *testing.T) {
	for _, typ := range []string{"cog.thought", "cog.outcome", "cog.recalibration_recommended"} {
		if !injectSocialWhitelist[typ] {
			t.Errorf("%s not whitelisted", typ)
		}
	}
	if injectSocialWhitelist["agent.intent_rejected"] {
		t.Error("agent.intent_rejected must be loop-emitted only, not injectable")
	}
}

// TestCognitionTelemetryIsNoOp: applying any telemetry event leaves state
// byte-identical — recorded observability, zero state effect.
func TestCognitionTelemetryIsNoOp(t *testing.T) {
	s := NewState(42, testMap(42))
	before := s.Marshal()
	payloads := map[string]any{
		"cog.thought": CogThoughtPayload{
			Job: "planner-3-100", Class: "planner", Agent: 3,
			SnapshotTick: 100, TriggerSeq: 42, Points: 3,
			PredictedWallMs: 51000, PredictedLandTick: 1732,
		},
		"cog.outcome": CogOutcomePayload{
			Job: "planner-3-100", Class: "planner", Agent: 3,
			Outcome: OutcomeSuppressed, Reason: "3pt x 17.0s/pt x 32x = 1632 ticks > budget 1200",
		},
		"cog.recalibration_recommended": RecalibrationPayload{
			Tier: "local", EstimateSPerPt: 17.2, SpikeRate: 0.35, Window: 20,
		},
		"agent.intent_rejected": IntentRejectedPayload{
			Agent: 3, Goal: "talk_to", Reason: "stale", StalenessTicks: 1646,
		},
	}
	for typ, p := range payloads {
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if err := s.Apply(store.Event{Type: typ, Tick: 1, Payload: b}); err != nil {
			t.Errorf("Apply(%s): %v", typ, err)
		}
	}
	if string(s.Marshal()) != string(before) {
		t.Error("telemetry event mutated state")
	}
}

// --- US3: the landing ladder ---

// ladderHarness: a paused loop at a preset tick — staleness is fully
// controlled, no ticks flow, InjectIntent works while paused (FR-018).
type ladderHarness struct {
	st   *store.Store
	loop *Loop
}

func newLadderHarness(t *testing.T, mutate func(*State)) *ladderHarness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := testMap(42)
	s := NewState(42, m)
	s.Paused = true
	s.Tick = 10000
	if mutate != nil {
		mutate(s)
	}
	loop := NewLoop(s, m, st, nil)
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
	return &ladderHarness{st: st, loop: loop}
}

func (h *ladderHarness) lastOutcome(t *testing.T) (CogOutcomePayload, bool) {
	t.Helper()
	evs, err := h.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == "cog.outcome" {
			var p CogOutcomePayload
			if err := json.Unmarshal(evs[i].Payload, &p); err != nil {
				t.Fatal(err)
			}
			return p, true
		}
	}
	return CogOutcomePayload{}, false
}

func meteredArgs(agent int, goal string) InjectArgs {
	return InjectArgs{
		Agent: agent, Goal: goal, TargetAgent: -1,
		Class: "planner", JobID: "planner-test", SnapshotTick: 10000,
		PredictedWallMs: 51000, ActualWallMs: 51000,
	}
}

func TestLadderRejectsStale(t *testing.T) {
	h := newLadderHarness(t, nil)
	args := meteredArgs(0, "wander")
	args.SnapshotTick = 8000 // staleness 2000 > planner budget 1200
	if err := h.loop.InjectIntent(args); err == nil {
		t.Fatal("stale intent executed")
	}
	p, ok := h.lastOutcome(t)
	if !ok || p.Outcome != OutcomeRejectedStale {
		t.Fatalf("outcome = %+v", p)
	}
	if p.StalenessTicks != 2000 || p.Kind != RejectKindWorldChange {
		t.Errorf("staleness %d kind %q", p.StalenessTicks, p.Kind)
	}
}

func TestLadderClassifiesPredictionMiss(t *testing.T) {
	h := newLadderHarness(t, nil)
	args := meteredArgs(0, "wander")
	args.SnapshotTick = 8000
	args.ActualWallMs = 4 * args.PredictedWallMs // spiked call
	h.loop.InjectIntent(args)
	p, _ := h.lastOutcome(t)
	if p.Kind != RejectKindPredictionMiss {
		t.Errorf("kind = %q, want prediction-miss", p.Kind)
	}
}

func TestLadderRejectsSuperseded(t *testing.T) {
	h := newLadderHarness(t, func(s *State) { s.Agents[0].Generation = 3 })
	args := meteredArgs(0, "wander")
	args.Generation = 2 // thought predates the emergency
	if err := h.loop.InjectIntent(args); err == nil {
		t.Fatal("superseded intent executed")
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeSuperseded {
		t.Errorf("outcome = %+v", p)
	}
}

func TestLadderRejectsGuardAndRecordsUnavailable(t *testing.T) {
	h := newLadderHarness(t, func(s *State) {
		s.Agents[1].Dead = true
		s.Agents[0].X, s.Agents[0].Y = 10, 10
	})
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{{Type: GuardTargetAlive, Target: 1}}
	if err := h.loop.InjectIntent(args); err == nil {
		t.Fatal("guard-failed intent executed")
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeRejectedGuard {
		t.Errorf("outcome = %+v", p)
	}

	// Dead ACTOR: recorded rejected-unavailable, not silence.
	args2 := meteredArgs(1, "wander")
	if err := h.loop.InjectIntent(args2); err == nil {
		t.Fatal("dead agent acted")
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeUnavailable {
		t.Errorf("outcome = %+v", p)
	}
}

func TestLadderLandsAndAdapts(t *testing.T) {
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10
	})
	// Fresh, in-budget, guards hold, target moved since snapshot → adapted.
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{
		{Type: GuardTargetAlive, Target: 1},
		{Type: GuardTargetPresent, Target: 1, X: 14, Y: 10}, // snapshot position
	}
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("healthy landing rejected: %v", err)
	}
	p, _ := h.lastOutcome(t)
	if p.Outcome != OutcomeAdapted {
		t.Errorf("outcome = %q, want adapted (target moved, repair via resolveGoal)", p.Outcome)
	}

	// Same landing with the target exactly where the snapshot said: landed.
	args.JobID = "planner-test-2"
	args.Guards[1].X, args.Guards[1].Y = 12, 10
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("landing rejected: %v", err)
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeLanded {
		t.Errorf("outcome = %q, want landed", p.Outcome)
	}
}

func TestLadderUnmeteredCallersKeepOldContract(t *testing.T) {
	h := newLadderHarness(t, func(s *State) { s.Agents[0].Dead = true })
	if err := h.loop.InjectIntent(InjectArgs{Agent: 0, Goal: "wander", TargetAgent: -1}); err == nil {
		t.Fatal("dead agent acted")
	}
	if _, found := h.lastOutcome(t); found {
		t.Error("unmetered caller produced telemetry")
	}
}

func TestGenerationBumpsOnHighSalience(t *testing.T) {
	s := NewState(42, testMap(42))
	add := func(sal int) {
		b, _ := json.Marshal(MemoryAddedPayload{Agent: 0, Text: "x", Salience: sal, Subject: -1})
		if err := s.Apply(store.Event{Type: "agent.memory_added", Tick: 1, Payload: b}); err != nil {
			t.Fatal(err)
		}
	}
	add(8) // dream-level: no interrupt
	if s.Agents[0].Generation != 0 {
		t.Errorf("salience 8 bumped generation")
	}
	add(9)  // near-death
	add(10) // witnessed death
	if s.Agents[0].Generation != 2 {
		t.Errorf("generation = %d, want 2", s.Agents[0].Generation)
	}
}

func TestGuardEvalTable(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Tick = 5000
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 11, 10
	s.Agents[2].Dead = true
	s.Agents[3].X, s.Agents[3].Y = 60, 60
	s.Agents[0].Generation = 2
	cases := []struct {
		g    Guard
		hold bool
	}{
		{Guard{Type: GuardTargetAlive, Target: 1}, true},
		{Guard{Type: GuardTargetAlive, Target: 2}, false},
		{Guard{Type: GuardTargetPresent, Target: 1}, true},
		{Guard{Type: GuardTargetPresent, Target: 3}, false}, // beyond presentRadius
		{Guard{Type: GuardNotSuperseded, Generation: 2}, true},
		{Guard{Type: GuardNotSuperseded, Generation: 1}, false},
		{Guard{Type: GuardAfterTick, Tick: 4000}, true},
		{Guard{Type: GuardAfterTick, Tick: 6000}, false},
		{Guard{Type: GuardBeforeTick, Tick: 6000}, true},
		{Guard{Type: GuardBeforeTick, Tick: 4000}, false},
		{Guard{Type: "bogus"}, false},
	}
	for _, c := range cases {
		if hold, why := c.g.Eval(s, 0); hold != c.hold {
			t.Errorf("Eval(%+v) = %v (%s), want %v", c.g, hold, why, c.hold)
		}
	}
}

// --- US4: guarded conditional plans ---

func TestPlanActsAtTickT(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	s.Tick = 1000
	s.Agents[0].Plan = []PlanStep{{
		Job: "planner-0-1000", Goal: "wander",
		When:  &Guard{Type: GuardAfterTick, Tick: 1500},
		Until: 3000,
	}}
	// Holding: before T the head step emits nothing, tick after tick.
	for tick := int64(1001); tick < 1500; tick += 100 {
		if evs := planStepEvents(s, m, 0, tick); len(evs) != 0 {
			t.Fatalf("plan fired early at tick %d: %v", tick, evs[0].Type)
		}
	}
	// At T: started + intent, deterministically, no model anywhere.
	evs := planStepEvents(s, m, 0, 1500)
	if len(evs) != 2 || evs[0].Type != "agent.plan_step_started" || evs[1].Type != "agent.intent_set" {
		t.Fatalf("at T: %v", evs)
	}
	var ip IntentSetPayload
	json.Unmarshal(evs[1].Payload, &ip)
	if ip.Source != "plan" {
		t.Errorf("intent source = %q, want plan", ip.Source)
	}
	// Reducer pops the head.
	for _, e := range evs {
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Agents[0].Plan) != 0 {
		t.Errorf("plan not consumed: %+v", s.Agents[0].Plan)
	}
}

func TestPlanExpiryClearsWholePlan(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	s.Tick = 5000
	s.Agents[0].Plan = []PlanStep{
		{Job: "j", Goal: "wander", Until: 4000}, // window already closed
		{Job: "j", Goal: "forage", Until: 9000},
	}
	evs := planStepEvents(s, m, 0, 5000)
	if len(evs) != 1 || evs[0].Type != "agent.plan_expired" {
		t.Fatalf("expiry events: %v", evs)
	}
	if err := s.Apply(evs[0]); err != nil {
		t.Fatal(err)
	}
	if s.Agents[0].Plan != nil {
		t.Errorf("expiry must clear the whole plan, got %+v", s.Agents[0].Plan)
	}
}

func TestLadderValidatesPlans(t *testing.T) {
	h := newLadderHarness(t, nil)
	args := meteredArgs(0, "")
	args.Plan = []PlanStep{
		{Goal: "wander"}, {Goal: "forage"}, {Goal: "sleep"}, {Goal: "eat"},
	}
	if err := h.loop.InjectIntent(args); err == nil {
		t.Fatal("over-cap plan accepted")
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeRejectedGuard {
		t.Errorf("outcome = %+v", p)
	}

	args.JobID = "planner-test-2"
	args.Plan = []PlanStep{{Goal: "fly"}}
	if err := h.loop.InjectIntent(args); err == nil {
		t.Fatal("unknown plan goal accepted")
	}

	args.JobID = "planner-test-3"
	args.Plan = []PlanStep{{Goal: "wander"}, {Goal: "forage", When: &Guard{Type: GuardAfterTick, Tick: 10500}}}
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeLanded {
		t.Errorf("outcome = %+v", p)
	}
	// The default window was stamped at the door.
	evs, _ := h.st.EventsSince(0, 0)
	for _, e := range evs {
		if e.Type == "agent.plan_set" {
			var p PlanSetPayload
			json.Unmarshal(e.Payload, &p)
			if len(p.Steps) != 2 || p.Steps[0].Until != 10000+PlanDefaultWindowTicks {
				t.Errorf("plan_set steps: %+v", p.Steps)
			}
		}
	}
}

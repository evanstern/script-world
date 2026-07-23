package sim

import (
	"strings"
	"testing"
)

// Landing-ladder rung tests (TASK-70): each doctrine rung is exercised directly
// as a pure function, and the whole ladder through landIntent with a capturing
// emit — no store, no goroutine, no command round-trip. The determinism suite
// (TestDeterminism*/TestReplay*) is the bit-identity gate; these tests pin the
// rung decomposition in isolation.

// --- capturing emit ---

type emittedEvent struct {
	typ     string
	payload any
}

// captureEmit returns an emit closure and a pointer to the events it records,
// standing in for handleCommand's real emit without the store/apply path.
func captureEmit() (func(string, any), *[]emittedEvent) {
	var evs []emittedEvent
	emit := func(typ string, payload any) {
		evs = append(evs, emittedEvent{typ: typ, payload: payload})
	}
	return emit, &evs
}

func countEmitted(evs []emittedEvent, typ string) int {
	n := 0
	for _, e := range evs {
		if e.typ == typ {
			n++
		}
	}
	return n
}

// lastCogOutcome returns the payload of the last cog.outcome emitted.
func lastCogOutcome(t *testing.T, evs []emittedEvent) CogOutcomePayload {
	t.Helper()
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].typ == "cog.outcome" {
			p, ok := evs[i].payload.(CogOutcomePayload)
			if !ok {
				t.Fatalf("cog.outcome payload is %T, want CogOutcomePayload", evs[i].payload)
			}
			return p
		}
	}
	t.Fatal("no cog.outcome emitted")
	return CogOutcomePayload{}
}

// landingLoop builds a minimal Loop for direct landIntent calls: landIntent
// reads only l.state and l.m (no store, notify, or channels).
func landingLoop(mutate func(*State)) *Loop {
	m := testMap(42)
	s := NewState(42, m)
	s.Tick = 10000
	if mutate != nil {
		mutate(s)
	}
	return &Loop{state: s, m: m}
}

// --- T010: per-rung isolation ---

func TestLandingRungUnavailable(t *testing.T) {
	s := NewState(42, testMap(42))
	if got := rungUnavailable(&s.Agents[0]); got != "" {
		t.Errorf("healthy actor unavailable: %q", got)
	}
	s.Agents[0].Dead = true
	if got := rungUnavailable(&s.Agents[0]); !strings.Contains(got, "is dead") {
		t.Errorf("dead reason = %q", got)
	}
	s.Agents[1].Asleep = true
	if got := rungUnavailable(&s.Agents[1]); !strings.Contains(got, "is asleep") {
		t.Errorf("asleep reason = %q", got)
	}
	// Dead is checked before asleep: an actor that is both reports dead.
	s.Agents[2].Dead, s.Agents[2].Asleep = true, true
	if got := rungUnavailable(&s.Agents[2]); !strings.Contains(got, "is dead") {
		t.Errorf("dead-before-asleep ordering broken: %q", got)
	}
}

func TestLandingRungSuperseded(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Agents[0].Generation = 3
	if got := rungSuperseded(&s.Agents[0], 3); got != "" {
		t.Errorf("matching generation superseded: %q", got)
	}
	got := rungSuperseded(&s.Agents[0], 2)
	if got != "generation 3, thought was 2" {
		t.Errorf("superseded reason = %q", got)
	}
}

func TestLandingRungStale(t *testing.T) {
	// planner has a real budget: staleness above it is stale, at/below is fresh.
	if got := rungStale("planner", 100000); got == "" {
		t.Error("large staleness not flagged stale")
	}
	if got := rungStale("planner", 0); got != "" {
		t.Errorf("fresh landing flagged stale: %q", got)
	}
	// An unmetered/unknown class carries no budget: never stale.
	if got := rungStale("no-such-class", 100000); got != "" {
		t.Errorf("unknown class flagged stale: %q", got)
	}
}

func TestLandingRungGuardFailedShortCircuits(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Tick = 5000
	in := &InjectArgs{
		Agent: 0, Goal: "wander", TargetAgent: -1,
		Guards: []Guard{
			{Type: GuardBeforeTick, Tick: 4000}, // fails first: "past tick 4000"
			{Type: GuardAfterTick, Tick: 6000},  // would fail "before tick 6000"
		},
	}
	d := walkGuards(s, in)
	if d.reason != "past tick 4000" {
		t.Errorf("reason = %q, want the first guard's — later guards must not evaluate", d.reason)
	}
	if d.outcome != OutcomeRejectedGuard {
		t.Errorf("outcome = %q, want rejected-guard", d.outcome)
	}
}

func TestLandingRungGuardFailedHelper(t *testing.T) {
	d := rungGuardFailed("Bex is gone (distance 40)")
	if d.outcome != OutcomeRejectedGuard || d.reason != "Bex is gone (distance 40)" || d.hailTarget != -1 {
		t.Errorf("rungGuardFailed = %+v", d)
	}
}

func TestLandingFreshFallThrough(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Tick = 5000
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 11, 10
	// A holding, non-relaxable guard set: land fresh, no hail.
	in := &InjectArgs{
		Agent: 0, Goal: "seek", TargetAgent: 1,
		Guards: []Guard{{Type: GuardTargetAlive, Target: 1}, {Type: GuardAfterTick, Tick: 1000}},
	}
	d := walkGuards(s, in)
	if d.outcome != OutcomeLanded || d.reason != "" || d.hailTarget != -1 {
		t.Errorf("fresh walk = %+v, want landed/no-reason/no-hail", d)
	}
}

// --- T011: hail special-cases ---

func TestLandingMutualHailerAdaptsNoHail(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 10, 45 // beyond presentRadius: guard fails
	// Actor(0) is answering target(1)'s hail — the pair is already converging.
	s.Agents[0].Hail = &AgentHail{By: 1, Until: 20000}
	in := &InjectArgs{
		Agent: 0, Goal: "talk_to", TargetAgent: 1,
		Guards: []Guard{{Type: GuardTargetPresent, Target: 1, X: 10, Y: 45}},
	}
	d := walkGuards(s, in)
	if d.outcome != OutcomeAdapted {
		t.Errorf("outcome = %q, want adapted", d.outcome)
	}
	if d.hailTarget != -1 {
		t.Errorf("mutual-hailer must NOT hail (deadlock prevention): hailTarget = %d", d.hailTarget)
	}
	// The rung agrees directly.
	if relaxed, hail := rungHailRelaxed(s, in, &s.Agents[0], in.Guards[0]); !relaxed || hail != -1 {
		t.Errorf("rungHailRelaxed mutual = (%v, %d), want (true, -1)", relaxed, hail)
	}
}

func TestLandingInRadiusHailableIsFreshAndHails(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 12, 10 // within presentRadius: guard holds
	in := &InjectArgs{
		Agent: 0, Goal: "talk_to", TargetAgent: 1,
		Guards: []Guard{{Type: GuardTargetPresent, Target: 1, X: 12, Y: 10}}, // snapshot == current: not adapted
	}
	d := walkGuards(s, in)
	if d.outcome != OutcomeLanded {
		t.Errorf("outcome = %q, want landed (in-radius, unmoved is fresh)", d.outcome)
	}
	if d.hailTarget != 1 {
		t.Errorf("hailTarget = %d, want 1 (in-radius hailable is still hailed)", d.hailTarget)
	}
}

func TestLandingMovedTargetAdapts(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 12, 10 // present, but moved from the snapshot
	// Non-talk_to goal isolates the adapt rung from the in-radius hail marking.
	in := &InjectArgs{
		Agent: 0, Goal: "seek", TargetAgent: 1,
		Guards: []Guard{{Type: GuardTargetPresent, Target: 1, X: 14, Y: 10}}, // snapshot elsewhere
	}
	d := walkGuards(s, in)
	if d.outcome != OutcomeAdapted {
		t.Errorf("outcome = %q, want adapted (target moved since snapshot)", d.outcome)
	}
	if d.hailTarget != -1 {
		t.Errorf("non-talk_to adapt must not hail: hailTarget = %d", d.hailTarget)
	}
	// rungAdapted, and its zero-snapshot exemption, directly.
	if !rungAdapted(s, in.Guards[0]) {
		t.Error("rungAdapted false for a moved target")
	}
	if rungAdapted(s, Guard{Type: GuardTargetPresent, Target: 1}) {
		t.Error("zero snapshot (X==0 && Y==0) must never count as a move")
	}
}

func TestLandingDeadOrOutOfRangeFallsThroughToGuardFailed(t *testing.T) {
	// Dead target: relaxation is for alive targets only → plain guard rejection.
	dead := NewState(42, testMap(42))
	dead.Agents[0].X, dead.Agents[0].Y = 10, 10
	dead.Agents[1].Dead = true
	inDead := &InjectArgs{
		Agent: 0, Goal: "talk_to", TargetAgent: 1,
		Guards: []Guard{{Type: GuardTargetPresent, Target: 1, X: 10, Y: 45}},
	}
	if d := walkGuards(dead, inDead); d.outcome != OutcomeRejectedGuard || d.hailTarget != -1 {
		t.Errorf("dead target walk = %+v, want guard-failed no-hail", d)
	}

	// Out-of-range AND unhailable (asleep): also falls through.
	asleep := NewState(42, testMap(42))
	asleep.Agents[0].X, asleep.Agents[0].Y = 10, 10
	asleep.Agents[1].X, asleep.Agents[1].Y = 10, 45 // beyond presentRadius
	asleep.Agents[1].Asleep = true
	inAsleep := &InjectArgs{
		Agent: 0, Goal: "talk_to", TargetAgent: 1,
		Guards: []Guard{{Type: GuardTargetPresent, Target: 1, X: 10, Y: 45}},
	}
	d := walkGuards(asleep, inAsleep)
	if d.outcome != OutcomeRejectedGuard {
		t.Errorf("asleep out-of-range outcome = %q, want rejected-guard", d.outcome)
	}
	if !strings.Contains(d.reason, "is gone") {
		t.Errorf("reason = %q, want the distance reason", d.reason)
	}
}

// --- T012: decision consumption through landIntent ---

func TestLandingRejectionEmitsAndErrors(t *testing.T) {
	l := landingLoop(func(s *State) { s.Agents[0].Dead = true })
	emit, evs := captureEmit()
	args := meteredArgs(0, "wander")
	err := l.landIntent(&args, emit)
	if err == nil {
		t.Fatal("dead actor did not error")
	}
	// A metered rejection pairs the rejection record with the error.
	if countEmitted(*evs, "agent.intent_rejected") != 1 {
		t.Errorf("agent.intent_rejected count = %d, want 1", countEmitted(*evs, "agent.intent_rejected"))
	}
	if p := lastCogOutcome(t, *evs); p.Outcome != OutcomeUnavailable {
		t.Errorf("cog.outcome = %q, want %q", p.Outcome, OutcomeUnavailable)
	}
}

func TestLandingUncognizedRejectionIsSilent(t *testing.T) {
	l := landingLoop(func(s *State) { s.Agents[0].Dead = true })
	emit, evs := captureEmit()
	// Class == "" (unmetered): still errors, but emits nothing.
	args := InjectArgs{Agent: 0, Goal: "wander", TargetAgent: -1}
	if err := l.landIntent(&args, emit); err == nil {
		t.Fatal("dead actor did not error")
	}
	if len(*evs) != 0 {
		t.Errorf("uncognized rejection emitted %d events, want 0", len(*evs))
	}
}

func TestLandingPlanPathNeverHails(t *testing.T) {
	l := landingLoop(func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10 // in radius + hailable
	})
	emit, evs := captureEmit()
	// A plan landing whose guard walk (Goal talk_to) WOULD mark a hail target:
	// the plan path must ignore it — social.hailed lands only on the goal path.
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{{Type: GuardTargetPresent, Target: 1, X: 12, Y: 10}}
	args.Plan = []PlanStep{{Goal: "wander"}}
	if err := l.landIntent(&args, emit); err != nil {
		t.Fatalf("plan landing rejected: %v", err)
	}
	if countEmitted(*evs, "agent.plan_set") != 1 {
		t.Errorf("plan_set count = %d, want 1", countEmitted(*evs, "agent.plan_set"))
	}
	if got := countEmitted(*evs, "social.hailed"); got != 0 {
		t.Errorf("plan path emitted %d social.hailed, want 0", got)
	}
}

func TestLandingGoalPathHailsOnce(t *testing.T) {
	l := landingLoop(func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10
	})
	emit, evs := captureEmit()
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{{Type: GuardTargetPresent, Target: 1, X: 12, Y: 10}}
	if err := l.landIntent(&args, emit); err != nil {
		t.Fatalf("talk_to landing rejected: %v", err)
	}
	if got := countEmitted(*evs, "social.hailed"); got != 1 {
		t.Errorf("goal path emitted %d social.hailed, want 1", got)
	}
	if p := lastCogOutcome(t, *evs); p.Outcome != OutcomeLanded {
		t.Errorf("in-radius unmoved outcome = %q, want landed", p.Outcome)
	}
}

func TestLandingFinalOutcomeAdaptedVsLanded(t *testing.T) {
	// Target moved since snapshot → the final cog.outcome reads adapted.
	adapt := landingLoop(func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10
	})
	emit, evs := captureEmit()
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{
		{Type: GuardTargetAlive, Target: 1},
		{Type: GuardTargetPresent, Target: 1, X: 14, Y: 10}, // snapshot elsewhere
	}
	if err := adapt.landIntent(&args, emit); err != nil {
		t.Fatalf("adapted landing rejected: %v", err)
	}
	if p := lastCogOutcome(t, *evs); p.Outcome != OutcomeAdapted {
		t.Errorf("moved-target outcome = %q, want adapted", p.Outcome)
	}

	// Same guard at the target's actual position → landed.
	landed := landingLoop(func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10
	})
	emit2, evs2 := captureEmit()
	args2 := meteredArgs(0, "talk_to")
	args2.TargetAgent = 1
	args2.Guards = []Guard{
		{Type: GuardTargetAlive, Target: 1},
		{Type: GuardTargetPresent, Target: 1, X: 12, Y: 10}, // exactly where it stands
	}
	if err := landed.landIntent(&args2, emit2); err != nil {
		t.Fatalf("fresh landing rejected: %v", err)
	}
	if p := lastCogOutcome(t, *evs2); p.Outcome != OutcomeLanded {
		t.Errorf("unmoved outcome = %q, want landed", p.Outcome)
	}
}

package sim

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// --- helpers ---

func hailEvent(from, to int, until int64, tick int64) store.Event {
	return store.Event{Tick: tick, Type: "social.hailed",
		Payload: mustPayload(HailedPayload{From: from, To: to, Until: until})}
}

func countType(evs []store.Event, typ string) int {
	n := 0
	for _, e := range evs {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func countAgentType(evs []store.Event, typ string, agentField string, idx int) int {
	n := 0
	for _, e := range evs {
		if e.Type != typ {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(e.Payload, &m); err != nil {
			continue
		}
		raw, ok := m[agentField]
		if !ok {
			continue
		}
		var v int
		if json.Unmarshal(raw, &v) == nil && v == idx {
			n++
		}
	}
	return n
}

// --- T005: foundational reducer lifecycle + snapshot round-trip ---

func TestHailReducerLifecycle(t *testing.T) {
	apply := func(s *State, e store.Event) {
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
	}
	// social.hailed sets the pause.
	s := NewState(42, testMap(42))
	apply(s, hailEvent(0, 1, 500, 100))
	if s.Agents[1].Hail == nil || s.Agents[1].Hail.By != 0 || s.Agents[1].Hail.Until != 500 {
		t.Fatalf("hailed did not set Hail: %+v", s.Agents[1].Hail)
	}

	// Each terminator clears it.
	for _, term := range []struct {
		name string
		e    store.Event
	}{
		{"hail_met", store.Event{Tick: 120, Type: "social.hail_met", Payload: mustPayload(HailMetPayload{From: 0, To: 1})}},
		{"hail_expired", store.Event{Tick: 120, Type: "social.hail_expired", Payload: mustPayload(HailExpiredPayload{From: 0, To: 1})}},
		{"died", store.Event{Tick: 120, Type: "agent.died", Payload: mustPayload(DiedPayload{Agent: 1, Cause: "collapse"})}},
		{"slept", store.Event{Tick: 120, Type: "agent.slept", Payload: mustPayload(AgentPayload{Agent: 1})}},
	} {
		s := NewState(42, testMap(42))
		apply(s, hailEvent(0, 1, 500, 100))
		apply(s, term.e)
		if s.Agents[1].Hail != nil {
			t.Errorf("%s did not clear Hail: %+v", term.name, s.Agents[1].Hail)
		}
	}
}

func TestHailSnapshotRoundTrip(t *testing.T) {
	// Un-hailed agents produce byte-identical canonical state (no "hail" key).
	s := NewState(42, testMap(42))
	before := s.Marshal()
	if bytes.Contains(before, []byte(`"hail"`)) {
		t.Fatal("un-hailed state should not carry a hail key")
	}

	// A hailed agent round-trips through marshal/unmarshal mid-pause.
	s.Agents[3].Hail = &AgentHail{By: 5, Until: 9999}
	blob := s.Marshal()
	if !bytes.Contains(blob, []byte(`"hail":{"by":5,"until":9999}`)) {
		t.Fatalf("hail did not serialize as expected: %s", blob)
	}
	var back State
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatal(err)
	}
	if back.Agents[3].Hail == nil || *back.Agents[3].Hail != (AgentHail{By: 5, Until: 9999}) {
		t.Fatalf("hail lost in round-trip: %+v", back.Agents[3].Hail)
	}
	for i, a := range back.Agents {
		if i != 3 && a.Hail != nil {
			t.Errorf("agent %d gained a spurious hail", i)
		}
	}
	if back.Hash() != s.Hash() {
		t.Error("hailed state hash not stable across round-trip")
	}
}

// --- T003: the hailable predicate matrix (US3 exemptions, D6/D7) ---

func TestHailablePredicate(t *testing.T) {
	base := func() *State {
		s := NewState(42, testMap(42))
		for i := range s.Agents {
			s.Agents[i].X, s.Agents[i].Y = 10, 10 // co-located: in range for all
		}
		return s
	}
	cases := []struct {
		name   string
		mutate func(*State)
		want   bool
	}{
		{"healthy target in range", nil, true},
		{"dead target", func(s *State) { s.Agents[1].Dead = true }, false},
		{"asleep target", func(s *State) { s.Agents[1].Asleep = true }, false},
		{"already hailed", func(s *State) { s.Agents[1].Hail = &AgentHail{By: 4, Until: 9999} }, false},
		{"active hailer (target hailed someone)", func(s *State) { s.Agents[2].Hail = &AgentHail{By: 1, Until: 9999} }, false},
		{"meeting attendee", func(s *State) { s.Meeting.Phase = "open" }, false},
		{"meeting-pinned intent", func(s *State) { s.Agents[1].Intent = &Intent{Goal: "attend_meeting"} }, false},
		{"out of hail range", func(s *State) { s.Agents[1].X, s.Agents[1].Y = 10, 10+hailRadius+1 }, false},
		{"at exact hail range", func(s *State) { s.Agents[1].X, s.Agents[1].Y = 10, 10+hailRadius }, true},
		{"self", nil, false}, // checked separately below
	}
	for _, c := range cases {
		if c.name == "self" {
			s := base()
			if hailable(s, 1, 1) {
				t.Error("an agent must not be able to hail itself")
			}
			continue
		}
		s := base()
		if c.mutate != nil {
			c.mutate(s)
		}
		if got := hailable(s, 0, 1); got != c.want {
			t.Errorf("hailable(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// --- T011: US1 relaxed landing (out of radius) + in-radius hail ---

func TestLandingRelaxedByHail(t *testing.T) {
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 10, 45 // distance 35: beyond presentRadius(16), inside hailRadius(64)
	})
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{
		{Type: GuardTargetAlive, Target: 1},
		{Type: GuardTargetPresent, Target: 1, X: 10, Y: 45},
	}
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("out-of-radius talk_to rejected instead of hailed: %v", err)
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeAdapted {
		t.Errorf("outcome = %q, want adapted", p.Outcome)
	}
	evs, _ := h.st.EventsSince(0, 0)
	if countType(evs, "social.hailed") != 1 {
		t.Errorf("expected one social.hailed, got %d", countType(evs, "social.hailed"))
	}
	blob, _, _ := h.loop.DoState()
	var st State
	json.Unmarshal(blob, &st)
	if st.Agents[1].Hail == nil || st.Agents[1].Hail.By != 0 {
		t.Fatalf("target not paused: %+v", st.Agents[1].Hail)
	}
	if st.Agents[1].Hail.Until != 10000+hailWindowTicks {
		t.Errorf("until = %d, want %d", st.Agents[1].Hail.Until, 10000+hailWindowTicks)
	}
	// The intent landed as a seek toward the target's current tile.
	if st.Agents[0].Intent == nil || st.Agents[0].Intent.Goal != "seek" {
		t.Errorf("hailer intent = %+v, want seek", st.Agents[0].Intent)
	}
}

func TestInRadiusLandingAlsoHails(t *testing.T) {
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10 // distance 2: within presentRadius
	})
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{
		{Type: GuardTargetAlive, Target: 1},
		{Type: GuardTargetPresent, Target: 1, X: 12, Y: 10}, // exactly where it stands: landed, not adapted
	}
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("in-radius talk_to rejected: %v", err)
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeLanded {
		t.Errorf("outcome = %q, want landed", p.Outcome)
	}
	evs, _ := h.st.EventsSince(0, 0)
	if countType(evs, "social.hailed") != 1 {
		t.Errorf("in-radius landing must still hail (FR-001): got %d", countType(evs, "social.hailed"))
	}
}

// --- T011/T013: pause suppresses movement; needs decay; intent/plan intact ---

func TestPauseSuppressesMovement(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	// Agent 0 seeks agent 1; agent 2 (the notional hailer) is dead and far, so
	// the sweep never founds a meeting — an isolated test of the pause alone.
	s.Agents[2].Dead = true
	s.Agents[0].Intent = &Intent{Goal: "seek", TargetX: s.Agents[1].X, TargetY: s.Agents[1].Y}
	intentBefore := mustPayload(s.Agents[0].Intent)

	// Control: with no hail, agent 0 walks toward agent 1.
	ctrl := NewState(42, m)
	ctrl.Agents[2].Dead = true
	ctrl.Agents[0].Intent = &Intent{Goal: "seek", TargetX: ctrl.Agents[1].X, TargetY: ctrl.Agents[1].Y}
	ctrlLog := driveTicks(t, ctrl, m, 600, nil)
	if countAgentType(ctrlLog, "agent.moved", "agent", 0) == 0 {
		t.Fatal("control: agent 0 should have moved toward its seek target")
	}

	// Paused: agent 0 must not move for the whole (long) window.
	s.Agents[0].Hail = &AgentHail{By: 2, Until: 100000}
	log := driveTicks(t, s, m, 600, nil)
	if got := countAgentType(log, "agent.moved", "agent", 0); got != 0 {
		t.Errorf("paused agent moved %d times", got)
	}
	// Needs keep decaying while paused (the world does not freeze around it).
	if countAgentType(log, "agent.needs_changed", "agent", 0) == 0 {
		t.Error("paused agent's needs did not decay")
	}
	// Intent left exactly as the pause found it.
	if !bytes.Equal(mustPayload(s.Agents[0].Intent), intentBefore) {
		t.Errorf("pause mutated intent: %s vs %s", mustPayload(s.Agents[0].Intent), intentBefore)
	}
}

// --- T013: US2 safe expiry — resume with intent byte-identical (SC-003) ---

func TestHailExpiresAndResumes(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	s.Agents[2].Dead = true // hailer never arrives
	s.Agents[0].Intent = &Intent{Goal: "seek", TargetX: s.Agents[1].X, TargetY: s.Agents[1].Y}
	intentBefore := mustPayload(s.Agents[0].Intent)
	planBefore := mustPayload(s.Agents[0].Plan)

	const until = 480
	s.Agents[0].Hail = &AgentHail{By: 2, Until: until}

	// Strictly before the window edge: frozen, no expiry yet.
	pre := driveTicks(t, s, m, until-1, nil)
	if countAgentType(pre, "agent.moved", "agent", 0) != 0 {
		t.Error("agent moved before expiry")
	}
	if countType(pre, "social.hail_expired") != 0 {
		t.Error("expired early")
	}
	if s.Agents[0].Hail == nil {
		t.Fatal("Hail cleared before the window closed")
	}
	// At the window edge (tick == Until): expiry lands and the pause lifts.
	edge := driveTicks(t, s, m, until, nil)
	if countAgentType(edge, "social.hail_expired", "to", 0) != 1 {
		t.Fatalf("expected exactly one hail_expired at the edge, got %d",
			countAgentType(edge, "social.hail_expired", "to", 0))
	}
	if s.Agents[0].Hail != nil {
		t.Error("Hail not cleared after expiry")
	}
	// Intent and plan byte-identical pre-pause vs post-expiry (SC-003).
	if !bytes.Equal(mustPayload(s.Agents[0].Intent), intentBefore) {
		t.Error("intent changed across the pause")
	}
	if !bytes.Equal(mustPayload(s.Agents[0].Plan), planBefore) {
		t.Error("plan changed across the pause")
	}
	// Movement resumes after expiry.
	post := driveTicks(t, s, m, until+600, nil)
	if countAgentType(post, "agent.moved", "agent", 0) == 0 {
		t.Error("agent did not resume moving after expiry")
	}
}

// --- T011: US1 met — arrival founds a talk despite a fresh cooldown ---

func TestHailMetFoundsTalkBypassingCooldown(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	s.Tick = 5000
	// Hailer (0) already adjacent to paused target (1); both talked one tick
	// ago, so the ambient cooldown is nowhere near elapsed.
	s.Agents[0].X, s.Agents[0].Y = 20, 20
	s.Agents[1].X, s.Agents[1].Y = 21, 20
	s.Agents[0].LastTalk, s.Agents[1].LastTalk = 4999, 4999
	s.Agents[1].Hail = &AgentHail{By: 0, Until: 5480}

	evs := hailStep(s, 5001)
	if countType(evs, "social.hail_met") != 1 {
		t.Fatalf("expected hail_met, got events %v", evs)
	}
	if countType(evs, "agent.talked") != 1 {
		t.Error("hail_met did not found a talk despite the fresh cooldown")
	}
	// Apply and confirm the pause lifts.
	for _, e := range evs {
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if s.Agents[1].Hail != nil {
		t.Error("met did not clear the hail")
	}
}

// Met wins the same-tick race with expiry (data-model.md).
func TestHailMetWinsExpiryTie(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	s.Agents[0].X, s.Agents[0].Y = 20, 20
	s.Agents[1].X, s.Agents[1].Y = 21, 20
	s.Agents[1].Hail = &AgentHail{By: 0, Until: 700}
	evs := hailStep(s, 700) // tick == Until AND hailer adjacent
	if countType(evs, "social.hail_met") != 1 || countType(evs, "social.hail_expired") != 0 {
		t.Errorf("met must win the tie at Until: %v", evs)
	}
}

// --- T015: US3 exemptions through the landing path + mutual hail ---

func TestOutOfRadiusLandingRejectsUnhailable(t *testing.T) {
	// A target beyond presentRadius that is NOT hailable rejects exactly as
	// before — here, an asleep target.
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 10, 45 // distance 35
		s.Agents[1].Asleep = true
	})
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{{Type: GuardTargetPresent, Target: 1, X: 10, Y: 45}}
	if err := h.loop.InjectIntent(args); err == nil {
		t.Fatal("asleep out-of-radius target should reject as before")
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeRejectedGuard {
		t.Errorf("outcome = %q, want rejected-guard", p.Outcome)
	}
	evs, _ := h.st.EventsSince(0, 0)
	if countType(evs, "social.hailed") != 0 {
		t.Error("an unhailable target must never be hailed")
	}
	if p, _ := h.lastOutcome(t); !strings.Contains(p.Reason, "is gone") {
		t.Errorf("reason = %q, want the existing distance reason", p.Reason)
	}
}

func TestSecondHailDoesNotExtendPause(t *testing.T) {
	// A target already paused by agent 4, standing within presentRadius of a
	// second hailer (agent 0): the second landing succeeds (present) but the
	// pause is neither re-targeted nor extended (US3-3).
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 12, 10 // in presentRadius of agent 0
		s.Agents[1].Hail = &AgentHail{By: 4, Until: 12345}
	})
	args := meteredArgs(0, "talk_to")
	args.TargetAgent = 1
	args.Guards = []Guard{{Type: GuardTargetPresent, Target: 1, X: 12, Y: 10}}
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("second (in-radius) landing rejected: %v", err)
	}
	evs, _ := h.st.EventsSince(0, 0)
	if countType(evs, "social.hailed") != 0 {
		t.Error("already-paused target must not be re-hailed")
	}
	blob, _, _ := h.loop.DoState()
	var st State
	json.Unmarshal(blob, &st)
	if st.Agents[1].Hail == nil || st.Agents[1].Hail.By != 4 || st.Agents[1].Hail.Until != 12345 {
		t.Errorf("pause re-targeted or extended: %+v", st.Agents[1].Hail)
	}
}

func TestMutualHailNoDeadlock(t *testing.T) {
	// A(0) has hailed B(1): B is paused. Now B's own talk_to(A) lands with A
	// beyond presentRadius. B's landing must succeed (mutual-presence rung)
	// WITHOUT hailing A — two agents must never be mutually frozen.
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].X, s.Agents[0].Y = 10, 10
		s.Agents[1].X, s.Agents[1].Y = 10, 45 // A beyond presentRadius from B
		s.Agents[1].Hail = &AgentHail{By: 0, Until: 10480}
	})
	args := meteredArgs(1, "talk_to") // actor = B
	args.TargetAgent = 0              // target = A, B's own hailer
	args.Guards = []Guard{{Type: GuardTargetPresent, Target: 0, X: 10, Y: 10}}
	if err := h.loop.InjectIntent(args); err != nil {
		t.Fatalf("mutual landing rejected: %v", err)
	}
	if p, _ := h.lastOutcome(t); p.Outcome != OutcomeAdapted {
		t.Errorf("outcome = %q, want adapted", p.Outcome)
	}
	evs, _ := h.st.EventsSince(0, 0)
	if countType(evs, "social.hailed") != 0 {
		t.Error("the mutual-presence rung must not emit a new hail")
	}
	blob, _, _ := h.loop.DoState()
	var st State
	json.Unmarshal(blob, &st)
	if st.Agents[0].Hail != nil {
		t.Error("hailer A was frozen by B — a mutual freeze")
	}
	if st.Agents[1].Intent == nil || st.Agents[1].Intent.Goal != "seek" {
		t.Errorf("B's landing did not set a seek intent: %+v", st.Agents[1].Intent)
	}
}

// --- T008: plan-step talk_to firing hails a hailable target ---

func TestPlanStepTalkToHails(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)
	s.Tick = 1000
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 10, 40 // in hail range
	s.Agents[0].Plan = []PlanStep{{Job: "p", Goal: "talk_to", Target: 1, Until: 5000}}
	evs := planStepEvents(s, m, 0, 1001)
	if countType(evs, "social.hailed") != 1 {
		t.Fatalf("plan-step talk_to did not hail: %v", evs)
	}
	// A bare seek step (movement, not conversation) must NOT hail.
	s.Agents[0].Plan = []PlanStep{{Job: "p", Goal: "wander", Until: 5000}}
	if evs := planStepEvents(s, m, 0, 1002); countType(evs, "social.hailed") != 0 {
		t.Error("a non-talk_to plan step must not hail")
	}
}

// --- T017: replay determinism over hailed → met and hailed → expired ---

func TestReplayDeterminismWithHails(t *testing.T) {
	const seed, ticks = 314, 2000
	m := testMap(seed)
	mkScenario := func() *State {
		s := NewState(seed, m)
		// Pair A: hailer(0) adjacent to target(1) → met next sweep.
		s.Agents[0].X, s.Agents[0].Y = 20, 20
		s.Agents[1].X, s.Agents[1].Y = 21, 20
		// Pair B: hailer(2) dead (never arrives) → the pause runs to expiry.
		s.Agents[2].Dead = true
		s.Agents[3].X, s.Agents[3].Y = 60, 60
		return s
	}
	cmds := map[int64][]store.Event{
		100: {hailEvent(0, 1, 100+hailWindowTicks, 100), hailEvent(2, 3, 100+hailWindowTicks, 100)},
	}

	live := mkScenario()
	log := driveTicks(t, live, m, ticks, cmds)
	if countType(log, "social.hail_met") == 0 || countType(log, "social.hail_expired") == 0 {
		t.Fatalf("scenario did not exercise both outcomes: met=%d expired=%d",
			countType(log, "social.hail_met"), countType(log, "social.hail_expired"))
	}

	replayed := mkScenario()
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil) // re-live the quiet tail as recovery does
	if live.Hash() != replayed.Hash() {
		t.Fatalf("replay diverged:\nlive:     %s\nreplayed: %s", live.Marshal(), replayed.Marshal())
	}
}

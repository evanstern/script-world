package mind

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
)

// TestConvoRawTruncation (TASK-42 T004): oversized raw replies are cut on a rune
// boundary with a marker, stay ≤ cap, and remain valid UTF-8; small replies
// pass through untouched.
func TestConvoRawTruncation(t *testing.T) {
	if got := truncateRaw("short reply"); got != "short reply" {
		t.Errorf("small reply mutated: %q", got)
	}
	// A reply of multi-byte runes (é = 2 bytes) longer than the cap must cut
	// mid-string without splitting a rune.
	big := strings.Repeat("é", rawReplyCap) // 2*cap bytes
	got := truncateRaw(big)
	if len(got) > rawReplyCap {
		t.Errorf("truncated length %d exceeds cap %d", len(got), rawReplyCap)
	}
	if !strings.HasSuffix(got, rawTruncMarker) {
		t.Errorf("missing truncation marker: %q", got[len(got)-20:])
	}
	if !utf8.ValidString(got) {
		t.Error("truncation split a rune (invalid UTF-8)")
	}
	// Exactly at the cap: no truncation.
	exact := strings.Repeat("a", rawReplyCap)
	if got := truncateRaw(exact); got != exact {
		t.Error("reply at the cap should not be truncated")
	}
}

// TestPlannerTelemetryLanded (US1): a successful planner thought leaves a
// cog.thought and exactly one landed cog.outcome sharing its job id, with
// the prediction stamped at snapshot time.
func TestPlannerTelemetryLanded(t *testing.T) {
	h := newHarness(t, `{"goal":"forage","reason":"hungry"}`)
	thoughts := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		if e.Type != "cog.thought" {
			return false
		}
		var p sim.CogThoughtPayload
		return json.Unmarshal(e.Payload, &p) == nil && p.Class == "planner"
	})
	if len(thoughts) == 0 {
		t.Fatal("no planner cog.thought recorded")
	}
	var tp sim.CogThoughtPayload
	if err := json.Unmarshal(thoughts[0].Payload, &tp); err != nil {
		t.Fatal(err)
	}
	if tp.Job == "" || tp.Points != 3 || tp.PredictedWallMs <= 0 {
		t.Errorf("thought payload incomplete: %+v", tp)
	}
	outcomes := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		if e.Type != "cog.outcome" {
			return false
		}
		var p sim.CogOutcomePayload
		return json.Unmarshal(e.Payload, &p) == nil && p.Job == tp.Job
	})
	if len(outcomes) != 1 {
		t.Fatalf("job %s has %d outcomes, want exactly 1", tp.Job, len(outcomes))
	}
	var op sim.CogOutcomePayload
	if err := json.Unmarshal(outcomes[0].Payload, &op); err != nil {
		t.Fatal(err)
	}
	if op.Outcome != sim.OutcomeLanded {
		t.Errorf("outcome = %q, want landed (reason %q)", op.Outcome, op.Reason)
	}
	if op.SnapshotTick != tp.SnapshotTick {
		t.Errorf("outcome snapshot %d != thought snapshot %d", op.SnapshotTick, tp.SnapshotTick)
	}
}

// TestPlannerTelemetryUnusable (US1): garbage output still terminates in a
// recorded outcome — silent failure is gone (FR-015).
func TestPlannerTelemetryUnusable(t *testing.T) {
	h := newHarness(t, "I simply cannot decide!!")
	outcomes := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		if e.Type != "cog.outcome" {
			return false
		}
		var p sim.CogOutcomePayload
		return json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "planner" && p.Outcome == sim.OutcomeUnusable
	})
	if len(outcomes) == 0 {
		t.Fatal("garbage planner reply left no recorded outcome")
	}
	var p sim.CogOutcomePayload
	json.Unmarshal(outcomes[0].Payload, &p)
	if p.Reason == "" {
		t.Error("unusable outcome carries no reason")
	}
}

// TestPlannerSuppressedAtHighSpeed (US2): at 32x under bootstrap calibration
// (20 s/pt), a planner thought's predicted drift (1920 ticks) exceeds its
// budget (1200) — no model call is made, the reflex floor covers, and the
// suppression is recorded with its arithmetic.
func TestPlannerSuppressedAtHighSpeed(t *testing.T) {
	h := newHarnessAt(t, `{"goal":"forage","reason":"hungry"}`, "32x")

	suppressed := h.waitEvents(t, 30*time.Second, func(e store.Event) bool {
		if e.Type != "cog.outcome" {
			return false
		}
		var p sim.CogOutcomePayload
		return json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "planner" && p.Outcome == sim.OutcomeSuppressed
	})
	if len(suppressed) == 0 {
		t.Fatal("no planner suppression recorded at 32x")
	}
	var p sim.CogOutcomePayload
	json.Unmarshal(suppressed[0].Payload, &p)
	if !strings.Contains(p.Reason, "> budget") {
		t.Errorf("suppression reason lacks arithmetic: %q", p.Reason)
	}
	h.model.mu.Lock()
	for _, k := range h.model.kinds {
		if k == llm.KindPlanner {
			t.Error("a planner model call was made despite suppression")
		}
	}
	h.model.mu.Unlock()
}

// TestFutureDatedLine (US4): the helper states now and the landing estimate;
// no line when there is no meaningful prediction.
func TestFutureDatedLine(t *testing.T) {
	line := futureDated(0, 1800)
	if !strings.Contains(line, "day 1 06:00") || !strings.Contains(line, "day 1 06:30") {
		t.Errorf("future-dated line: %q", line)
	}
	if futureDated(1800, 1800) != "" || futureDated(1800, 0) != "" {
		t.Error("no-prediction cases must be empty")
	}
}

// TestPlanFormLandsAndExecutes (US4 integration): a plan reply parses, lands
// through the door as agent.plan_set, and the executor fires the steps with
// Source "plan" — no model at firing time.
func TestPlanFormLandsAndExecutes(t *testing.T) {
	h := newHarness(t, `{"plan":[{"goal":"wander"},{"goal":"forage","for_min":120}],"reason":"stretch then gather"}`)
	planSets := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		return e.Type == "agent.plan_set"
	})
	if len(planSets) == 0 {
		t.Fatal("no plan landed")
	}
	started := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		return e.Type == "agent.plan_step_started"
	})
	if len(started) == 0 {
		t.Fatal("plan never started executing")
	}
	intents := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		if e.Type != "agent.intent_set" {
			return false
		}
		var p sim.IntentSetPayload
		return json.Unmarshal(e.Payload, &p) == nil && p.Source == "plan"
	})
	if len(intents) == 0 {
		t.Fatal("no plan-sourced intents executed")
	}
}

// --- US5: pause semantics — world freezes, minds catch up (FR-018) ---

// TestPauseInFlightThoughtLandsAtFrozenTick: a planner call in flight when
// the world pauses completes on the wall clock and lands at the frozen tick;
// the wall time spent paused adds zero game-tick staleness.
func TestPauseInFlightThoughtLandsAtFrozenTick(t *testing.T) {
	h := newHarnessAt(t, `{"goal":"wander","reason":"stretching"}`, "16x")
	gate := make(chan struct{})
	h.model.mu.Lock()
	h.model.planGate = gate
	h.model.mu.Unlock()

	// Wait for a planner call to be in flight (blocked on the gate).
	deadline := time.Now().Add(30 * time.Second)
	for h.model.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.model.calls.Load() == 0 {
		t.Fatal("no planner call started")
	}
	st, err := h.loop.Do("pause", "")
	if err != nil {
		t.Fatal(err)
	}
	frozen := st.Tick
	time.Sleep(1500 * time.Millisecond) // wall time passes; ticks must not
	close(gate)                         // the mind finishes thinking mid-pause

	outcomes := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		if e.Type != "cog.outcome" {
			return false
		}
		var p sim.CogOutcomePayload
		return json.Unmarshal(e.Payload, &p) == nil && p.Class == "planner" &&
			(p.Outcome == sim.OutcomeLanded || p.Outcome == sim.OutcomeAdapted)
	})
	if len(outcomes) == 0 {
		t.Fatal("in-flight thought never landed during pause")
	}
	if outcomes[0].Tick != frozen {
		t.Errorf("landed at tick %d, world frozen at %d", outcomes[0].Tick, frozen)
	}
	var p sim.CogOutcomePayload
	json.Unmarshal(outcomes[0].Payload, &p)
	if p.LandingTick != frozen {
		t.Errorf("landing_tick %d != frozen %d", p.LandingTick, frozen)
	}
	if p.StalenessTicks > frozen-p.SnapshotTick {
		t.Errorf("pause accrued staleness: %d > %d", p.StalenessTicks, frozen-p.SnapshotTick)
	}
}

// TestPauseStartsNoNewThoughts: scheduling is tick-driven — once a paused
// world quiesces, no new planner jobs start no matter how much wall time
// passes. (A landing batch arriving mid-pause may first settle one
// debounce-bounded catch-up round at zero staleness — FR-018 as refined by
// the live validation run; this test drains before measuring.)
func TestPauseStartsNoNewThoughts(t *testing.T) {
	h := newHarnessAt(t, `{"goal":"wander","reason":"stretching"}`, "16x")
	if _, err := h.loop.Do("pause", ""); err != nil {
		t.Fatal(err)
	}
	// Drain: give any pre-pause in-flight work a moment to finish.
	time.Sleep(1 * time.Second)
	before := h.model.calls.Load()
	time.Sleep(2 * time.Second)
	if after := h.model.calls.Load(); after != before {
		t.Errorf("model called %d times while paused", after-before)
	}
}

// TestPauseConversationLandsAtFrozenTick: a scene founded before the pause
// completes on the wall clock and lands atomically at the frozen tick.
func TestPauseConversationLandsAtFrozenTick(t *testing.T) {
	model := &scriptedModel{replies: convoScript(
		`{"gist": "talked shelter", "tone_a": 1, "tone_b": 1}`)}
	h, md := setupConvo(t, model)
	st, err := h.loop.Do("pause", "")
	if err != nil {
		t.Fatal(err)
	}
	frozen := st.Tick
	md.maybeStartConversation(store.Event{
		Tick: frozen, Type: "agent.talked",
		Payload: mustJSON(t, sim.TalkedPayload{A: 0, B: 1}),
	})
	convs := h.waitEvents(t, 15*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("scene never landed during pause")
	}
	if convs[0].Tick != frozen {
		t.Errorf("scene landed at tick %d, world frozen at %d", convs[0].Tick, frozen)
	}
}

// TestResumeNoBurst: pause accrues no cognition debt — after resume, thought
// volume is cadence-normal, not a compensating flood.
func TestResumeNoBurst(t *testing.T) {
	// A real (finite) speed: at 16x, 2 wall-seconds after resume is only 32
	// game-ticks — far under one 1800-tick planner cadence — so anything
	// beyond stragglers IS a burst. (At max speed the same window spans
	// dozens of cadences and high volume is legitimate.)
	h := newHarnessAt(t, `{"goal":"wander","reason":"stretching"}`, "16x")
	// Let the world think a little, then pause for real wall time.
	h.waitEvents(t, 30*time.Second, func(e store.Event) bool { return e.Type == "cog.thought" })
	if _, err := h.loop.Do("pause", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	before := h.model.calls.Load()
	if _, err := h.loop.Do("resume", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	burst := h.model.calls.Load() - before
	if burst > int64(2*sim.AgentCount) {
		t.Errorf("resume burst: %d calls in 2s", burst)
	}
}

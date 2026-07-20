package mind

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

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

// TestMusingTelemetryLandsAtomically (US1): a landed musing's agent.thought
// and its cog.outcome arrive in the same batch (same tick, adjacent seqs).
func TestMusingTelemetryLandsAtomically(t *testing.T) {
	h := newHarness(t, `{"goal":"rest","reason":"tired"}`)
	h.model.mu.Lock()
	h.model.musingReply = "The fire needs tending before the frost."
	h.model.mu.Unlock()
	outcomes := h.waitEvents(t, 20*time.Second, func(e store.Event) bool {
		if e.Type != "cog.outcome" {
			return false
		}
		var p sim.CogOutcomePayload
		return json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "musing" && p.Outcome == sim.OutcomeLanded
	})
	if len(outcomes) == 0 {
		t.Fatal("no landed musing outcome recorded")
	}
	// The batch partner: an agent.thought at the same tick, adjacent seq.
	evs, err := h.st.EventsSince(outcomes[0].Seq-2, 3)
	if err != nil {
		t.Fatal(err)
	}
	foundThought := false
	for _, e := range evs {
		if e.Type == "agent.thought" && e.Tick == outcomes[0].Tick {
			foundThought = true
		}
	}
	if !foundThought {
		t.Error("landed musing outcome not batched with its agent.thought")
	}
}

// TestPlannerSuppressedAtHighSpeed (US2): at 32x under bootstrap calibration
// (20 s/pt), a planner thought's predicted drift (1920 ticks) exceeds its
// budget (1200) — no model call is made, the reflex floor covers, and the
// suppression is recorded with its arithmetic. Musings (1 point, 640 ticks
// vs 3600) still think.
func TestPlannerSuppressedAtHighSpeed(t *testing.T) {
	h := newHarnessAt(t, `{"goal":"forage","reason":"hungry"}`, "32x")
	h.model.mu.Lock()
	h.model.musingReply = "The wind is turning."
	h.model.mu.Unlock()

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

	musings := h.waitEvents(t, 30*time.Second, func(e store.Event) bool {
		return e.Type == "agent.thought" && strings.Contains(string(e.Payload), "musing")
	})
	if len(musings) == 0 {
		t.Error("musings should survive 32x (1 point rides under its budget)")
	}
}

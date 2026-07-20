package sim

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/store"
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

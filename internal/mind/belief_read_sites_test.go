package mind

import (
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/sim"
)

// Spec 030 (US2, T008): the nightly consolidation held-beliefs block is the
// documented exception to the general below-floor exclusion rule
// (data-model.md "Read sites") — unlike other belief-surfacing prompts, it
// renders EFFECTIVE confidence (never stored) and marks faded beliefs
// "(faded)" but keeps every held belief listed by ID so it stays revisable.
// consolidateUserPrompt is a pure function of consolJob, so these drive it
// directly with no harness/model.

func TestConsolidateUserPromptHeldBeliefsShowEffectiveConfidence(t *testing.T) {
	const day = int64(86400)
	const R = int64(1000)
	tick := R + 8*day // one half-life: 80 -> 40 (TestEffectiveConfidenceCurve)

	job := consolJob{
		name:      "Ash",
		sleepTick: tick,
		held: []sim.Belief{
			{ID: 7, Statement: "The well runs deep.", Confidence: 80,
				Provenance: sim.ProvenanceWitnessed, Reinforced: R},
		},
	}
	if got := sim.EffectiveConfidence(job.held[0], tick); got != 40 {
		t.Fatalf("test setup: expected effective 40 at one half-life, got %d", got)
	}
	prompt := consolidateUserPrompt(job)

	if !strings.Contains(prompt, "- [id 7] (confidence 40, witnessed) The well runs deep.") {
		t.Errorf("prompt missing the effective-confidence line; got:\n%s", prompt)
	}
	if strings.Contains(prompt, "confidence 80") {
		t.Errorf("prompt leaked the STORED confidence (80) instead of the effective value; got:\n%s", prompt)
	}
	if strings.Contains(prompt, "(faded)") {
		t.Errorf("a live belief (effective 40 >= floor) must not be marked faded; got:\n%s", prompt)
	}
}

func TestConsolidateUserPromptHeldBeliefsFadedMarkerStaysListed(t *testing.T) {
	const day = int64(86400)
	const R = int64(1000)
	tick := R + 17*day // per TestEffectiveConfidenceCurve: 80 -> 18, below the floor (20)

	job := consolJob{
		name:      "Ash",
		sleepTick: tick,
		held: []sim.Belief{
			{ID: 3, Statement: "Tendrils lurk past the ridge.", Confidence: 80,
				Provenance: sim.ProvenanceTold, Reinforced: R},
		},
	}
	if got := sim.EffectiveConfidence(job.held[0], tick); got >= sim.BeliefConfidenceFloor {
		t.Fatalf("test setup: expected the held belief below the floor (%d), got effective %d", sim.BeliefConfidenceFloor, got)
	}
	prompt := consolidateUserPrompt(job)

	// Still listed by ID (revisable) — NOT excluded, unlike other prompts
	// (contracts/events-and-decay.md: the held-beliefs block is the exception).
	if !strings.Contains(prompt, "[id 3]") {
		t.Errorf("faded held belief was dropped from the prompt; want it still listed by ID; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Tendrils lurk past the ridge.") {
		t.Errorf("faded held belief's statement missing from the prompt; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "(faded)") {
		t.Errorf("below-floor held belief missing the (faded) marker; got:\n%s", prompt)
	}
}

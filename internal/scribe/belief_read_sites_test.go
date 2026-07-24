package scribe

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Spec 030 (US2, T008): the soul's Beliefs section renders EFFECTIVE
// confidence (computed on read against the replica's current tick, never
// the stored value) and, below the floor, hedges instead of showing a
// number — the belief has stopped being a conviction and become myth
// (contracts/events-and-decay.md "Read sites").

func TestSoulRendersEffectiveBeliefConfidence(t *testing.T) {
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	scr, err := New(dir, 42, m, state.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	defer scr.Close()

	const day = int64(86400)
	const R = int64(1000)
	renderTick := R + 8*day // one half-life: 80 -> 40 (TestEffectiveConfidenceCurve)

	wantEff := sim.EffectiveConfidence(sim.Belief{Confidence: 80, Reinforced: R}, renderTick)
	if wantEff != 40 {
		t.Fatalf("test setup: expected effective 40 at one half-life, got %d", wantEff)
	}

	scr.Observe([]store.Event{{
		Tick: R, Type: "agent.belief_revised",
		Payload: mustPayloadJSON(t, sim.BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "The well runs deep.",
			Confidence: 80, Provenance: sim.ProvenanceWitnessed, Source: -1, Subject: -1,
		}),
	}})
	// Advance the replica's clock without touching the belief, so the render
	// reads decayed effective confidence, not the untouched stored value.
	scr.Observe([]store.Event{{
		Tick: renderTick, Type: "agent.memory_added",
		Payload: mustPayloadJSON(t, map[string]any{"agent": 0, "text": "A quiet day.", "salience": 1}),
	}})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
		s := string(data)
		if strings.Contains(s, "The well runs deep. *(40% sure — witnessed,") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
	t.Fatalf("soul.md never rendered the effective confidence (want 40%%, the stored 80%% decayed); content:\n%s", data)
}

func TestSoulRendersHedgedBelowFloorBelief(t *testing.T) {
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	scr, err := New(dir, 42, m, state.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	defer scr.Close()

	const day = int64(86400)
	const R = int64(1000)
	renderTick := R + 17*day // per TestEffectiveConfidenceCurve: 80 -> 18, below the floor (20)

	wantLiveEff := sim.EffectiveConfidence(sim.Belief{Confidence: 95, Reinforced: R}, renderTick)
	if wantLiveEff < sim.BeliefConfidenceFloor {
		t.Fatalf("test setup: expected the 95%% belief to stay live at day 17, got effective %d", wantLiveEff)
	}
	wantFadedEff := sim.EffectiveConfidence(sim.Belief{Confidence: 80, Reinforced: R}, renderTick)
	if wantFadedEff >= sim.BeliefConfidenceFloor {
		t.Fatalf("test setup: expected the 80%% belief to fall below the floor at day 17, got effective %d", wantFadedEff)
	}

	// A live belief (stays above the floor) and a faded one, so this also
	// pins the grouping: hedged lines render AFTER live ones.
	scr.Observe([]store.Event{{
		Tick: R, Type: "agent.belief_revised",
		Payload: mustPayloadJSON(t, sim.BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "The council meets at dawn.",
			Confidence: 95, Provenance: sim.ProvenanceWitnessed, Source: -1, Subject: -1,
		}),
	}})
	scr.Observe([]store.Event{{
		Tick: R, Type: "agent.belief_revised",
		Payload: mustPayloadJSON(t, sim.BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "Tendrils lurk past the ridge.",
			Confidence: 80, Provenance: sim.ProvenanceTold, Source: -1, Subject: -1,
		}),
	}})
	scr.Observe([]store.Event{{
		Tick: renderTick, Type: "agent.memory_added",
		Payload: mustPayloadJSON(t, map[string]any{"agent": 0, "text": "A quiet day.", "salience": 1}),
	}})

	const hedgedLine = "half-remembered: Tendrils lurk past the ridge."
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
		s := string(data)
		liveIdx := strings.Index(s, "The council meets at dawn.")
		hedgedIdx := strings.Index(s, hedgedLine)
		if liveIdx >= 0 && hedgedIdx >= 0 {
			if strings.Contains(s, "Tendrils lurk past the ridge. *(") {
				t.Fatalf("faded belief rendered a confidence number; want the hedged form with no number; content:\n%s", s)
			}
			if hedgedIdx < liveIdx {
				t.Fatalf("hedged (below-floor) belief rendered BEFORE the live one; want it grouped after; content:\n%s", s)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
	t.Fatalf("soul.md never rendered both beliefs; content:\n%s", data)
}

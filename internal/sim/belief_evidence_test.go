package sim

// Spec 030 (US1, T005) reducer + replay tests: belief formation anchors the
// decay clock (Reinforced = formation tick), belief_revised carries evidence +
// direct as landed input, and a log containing coerced beliefs replays
// byte-identically (SC-003 first half). Model-free and deterministic.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// TestBeliefFormationStampsReinforced (T005): a new belief's Reinforced is the
// formation tick (the curve starts at formation for every belief, direct or
// not); a later revision leaves Reinforced at its formation value (the
// direct-evidence refresh is US2/T006). Evidence + direct ride the payload.
func TestBeliefFormationStampsReinforced(t *testing.T) {
	s := NewState(42, testMap(42))

	// Formation of a hearsay-only belief still stamps Reinforced = formation tick.
	if err := s.Apply(consolidationEvent(t, 1000, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: 0, Statement: "Rowan saw tendrils.", Confidence: 50,
		Provenance: ProvenanceTold, Source: 3, Subject: -1,
		Evidence: []MemoryRef{{Tick: 100, Hash: "deadbeef"}}, Direct: false,
	})); err != nil {
		t.Fatal(err)
	}
	b := s.Agents[0].Beliefs[0]
	if b.Reinforced != 1000 {
		t.Errorf("formation Reinforced = %d, want 1000 (formation tick)", b.Reinforced)
	}

	// Revision at a later tick leaves Reinforced at the formation value (T005
	// scope: no revision-time refresh yet).
	if err := s.Apply(consolidationEvent(t, 90000, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: b.ID, Statement: "Rowan saw tendrils.", Confidence: 40,
		Provenance: ProvenanceTold, Source: 3, Subject: -1,
	})); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents[0].Beliefs[0].Reinforced; got != 1000 {
		t.Errorf("post-revision Reinforced = %d, want 1000 (unchanged in T005)", got)
	}
	if got := s.Agents[0].Beliefs[0].Tick; got != 90000 {
		t.Errorf("post-revision Tick = %d, want 90000 (last revision)", got)
	}
}

// TestBeliefEvidenceReplayDeterminism (SC-003 first half): a log whose
// belief_revised events carry evidence identities + a direct flag (a coerced
// hearsay belief and a direct one) replays byte-identically and round-trips the
// snapshot path.
func TestBeliefEvidenceReplayDeterminism(t *testing.T) {
	seed := uint64(11)
	m := testMap(seed)

	events := []store.Event{
		consolidationEvent(t, 10, "agent.memory_added", MemoryAddedPayload{Agent: 0, Text: "Built a fire.", Salience: 5, Subject: -1, Origin: OriginAction}),
		consolidationEvent(t, 20, "agent.memory_added", MemoryAddedPayload{Agent: 0, Text: "Rowan claims tendrils.", Salience: 4, Subject: 3, Origin: OriginGist}),
		// A direct-evidence belief keeps witnessed.
		consolidationEvent(t, 30, "agent.belief_revised", BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "I built a fire on the ridge.", Confidence: 80,
			Provenance: ProvenanceWitnessed, Source: -1, Subject: -1,
			Evidence: []MemoryRef{{Tick: 10, Hash: MemoryHash("Built a fire.")}}, Direct: true,
		}),
		// A coerced hearsay belief (was "witnessed", landed "told") citing the gist.
		consolidationEvent(t, 31, "agent.belief_revised", BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "Tendrils lurk past the ridge.", Confidence: 68,
			Provenance: ProvenanceTold, Source: 3, Subject: -1,
			Evidence: []MemoryRef{{Tick: 20, Hash: MemoryHash("Rowan claims tendrils.")}}, Direct: false,
		}),
		consolidationEvent(t, 32, "agent.consolidated", ConsolidatedPayload{Agent: 0, Night: 1, UpTo: 20, Outcome: ConsolidationAccepted, Beliefs: 2, Coerced: 1}),
	}

	replay := func() *State {
		s := NewState(seed, m)
		for _, e := range events {
			if err := s.Apply(e); err != nil {
				t.Fatal(err)
			}
		}
		s.Tick = 32
		return s
	}
	a, b := replay(), replay()
	if !bytes.Equal(a.Marshal(), b.Marshal()) {
		t.Error("two replays of a coerced-belief log diverged")
	}

	// Snapshot round-trip.
	var thawed State
	if err := json.Unmarshal(a.Marshal(), &thawed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Marshal(), thawed.Marshal()) {
		t.Error("snapshot round-trip changed state")
	}

	// The coerced belief landed as told, and both beliefs carry a formation-
	// anchored Reinforced clock.
	beliefs := a.Agents[0].Beliefs
	if len(beliefs) != 2 {
		t.Fatalf("beliefs = %d, want 2", len(beliefs))
	}
	for _, bl := range beliefs {
		if bl.Reinforced == 0 {
			t.Errorf("belief %d never anchored its clock (Reinforced 0)", bl.ID)
		}
		if bl.Provenance == ProvenanceWitnessed && bl.Statement == "Tendrils lurk past the ridge." {
			t.Errorf("coerced belief still witnessed (SC-001 breach)")
		}
	}
}

// TestPre030BeliefByteIdentical (FR-011): a belief formed by a pre-030
// belief_revised (no evidence/direct fields) marshals without those keys — the
// belief_revised event shape is additive.
func TestPre030BeliefByteIdentical(t *testing.T) {
	m := testMap(42)
	log := []store.Event{
		{Tick: 500, Type: "agent.belief_revised", Payload: mustPayload(BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "old belief", Confidence: 50,
			Provenance: ProvenanceInferred, Source: -1, Subject: -1})},
	}
	s := NewState(42, m)
	for _, e := range log {
		if err := s.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	blob := string(s.Marshal())
	// The payload carried no evidence/direct; the reduced Belief has a Reinforced
	// stamp (formation) but the event shape stays additive.
	if s.Agents[0].Beliefs[0].Reinforced != 500 {
		t.Errorf("formation Reinforced = %d, want 500", s.Agents[0].Beliefs[0].Reinforced)
	}
	// A belief that predates decay (Reinforced 0) would omit the key; a freshly
	// formed one carries it — assert the omitempty behavior on a zero value.
	var thawed State
	if err := json.Unmarshal([]byte(blob), &thawed); err != nil {
		t.Fatal(err)
	}
	thawed.Agents[0].Beliefs[0].Reinforced = 0
	if strings.Contains(string(thawed.Marshal()), `"reinforced"`) {
		t.Error("a zero Reinforced must omit the key (grandfathered belief byte-stable)")
	}
}

package sim

// Spec 030 (US2, T007) reinforcement-seam tests: agent.belief_reinforced is
// whitelisted through the injection door and reduced as a total arm that
// re-anchors a held belief's decay clock; a vanished target no-ops; a log
// containing the event replays byte-identically (SC-003 second half). The seam
// is the grounded-observation channel — no in-tree producer yet. Model-free.

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// TestBeliefReinforcedReducer: the reducer arm re-anchors the named belief's
// clock to the event tick, and a reinforcement of an ID no longer held is a
// total no-op (never an error), matching the other consolidation arms.
func TestBeliefReinforcedReducer(t *testing.T) {
	s := NewState(42, testMap(42))

	// Form a belief at tick 1000 (Reinforced = 1000).
	if err := s.Apply(consolidationEvent(t, 1000, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: 0, Statement: "Tendrils lurk past the ridge.", Confidence: 80,
		Provenance: ProvenanceTold, Source: 3, Subject: -1,
	})); err != nil {
		t.Fatal(err)
	}
	id := s.Agents[0].Beliefs[0].ID

	// Reinforcement at a much later tick re-anchors the clock.
	const reinfTick = int64(1000) + 40*86400
	if err := s.Apply(consolidationEvent(t, reinfTick, "agent.belief_reinforced", BeliefReinforcedPayload{
		Agent: 0, BeliefID: id,
	})); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents[0].Beliefs[0].Reinforced; got != reinfTick {
		t.Errorf("reinforcement did not re-anchor the clock: Reinforced = %d, want %d", got, reinfTick)
	}
	// Post-reinforcement the belief reads full conviction at the new anchor.
	if got := EffectiveConfidence(s.Agents[0].Beliefs[0], reinfTick); got != 80 {
		t.Errorf("post-reinforcement effective = %d, want 80 (curve reset)", got)
	}

	// Vanished target (unknown belief ID) is a total no-op — no error, no change.
	before := s.Marshal()
	if err := s.Apply(consolidationEvent(t, reinfTick+100, "agent.belief_reinforced", BeliefReinforcedPayload{
		Agent: 0, BeliefID: 9999,
	})); err != nil {
		t.Fatalf("vanished-target reinforcement errored: %v", err)
	}
	if got := string(s.Marshal()); got != string(before) {
		t.Error("vanished-target reinforcement mutated state (expected total no-op)")
	}
}

// TestBeliefReinforcedThroughDoor: the event is admitted through the mind's
// InjectSocial door (whitelist admission), re-stamped to the loop tick, and its
// reducer re-anchors the seeded belief. A vanished target passes the door and
// no-ops. This is the seam a future grounded-observation producer will emit
// through — 030 proves the consumer side end-to-end.
func TestBeliefReinforcedThroughDoor(t *testing.T) {
	h := newLadderHarness(t, func(s *State) {
		s.Agents[0].Beliefs = append(s.Agents[0].Beliefs, Belief{
			ID: 1, Statement: "Tendrils lurk past the ridge.", Confidence: 80,
			Provenance: ProvenanceTold, Source: 3, Subject: -1,
			Tick: 1000, Reinforced: 1000,
		})
		if s.NextBeliefID < 2 {
			s.NextBeliefID = 2
		}
	})

	inject := func(t *testing.T, beliefID int) {
		t.Helper()
		b, err := json.Marshal(BeliefReinforcedPayload{Agent: 0, BeliefID: beliefID})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.loop.InjectSocial([]store.Event{{Type: "agent.belief_reinforced", Payload: b}}); err != nil {
			t.Fatalf("InjectSocial rejected whitelisted agent.belief_reinforced: %v", err)
		}
	}

	// A vanished target passes the door (whitelisted) and no-ops in the reducer.
	before, _, err := h.loop.DoState()
	if err != nil {
		t.Fatal(err)
	}
	inject(t, 9999)
	after, _, err := h.loop.DoState()
	if err != nil {
		t.Fatal(err)
	}
	var beforeS, afterS State
	if err := json.Unmarshal(before, &beforeS); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &afterS); err != nil {
		t.Fatal(err)
	}
	if beforeS.Agents[0].Beliefs[0].Reinforced != 1000 {
		t.Errorf("vanished-target door injection changed Reinforced to %d, want 1000",
			afterS.Agents[0].Beliefs[0].Reinforced)
	}

	// A live target: the door re-stamps to the loop tick (10000) and re-anchors.
	inject(t, 1)
	after, _, err = h.loop.DoState()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &afterS); err != nil {
		t.Fatal(err)
	}
	if got := afterS.Agents[0].Beliefs[0].Reinforced; got != 10000 {
		t.Errorf("door reinforcement Reinforced = %d, want 10000 (loop tick)", got)
	}

	// The event was admitted to the log (recorded input, replayable).
	evs, err := h.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range evs {
		if e.Type == "agent.belief_reinforced" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("agent.belief_reinforced admitted %d times through the door, want 2", found)
	}
}

// TestReinforcementReplayDeterminism (SC-003 second half): a log containing a
// belief formation and a later reinforcement event replays byte-identically and
// round-trips the snapshot path; the reinforcement re-anchors the clock as
// recorded input (never re-derived).
func TestReinforcementReplayDeterminism(t *testing.T) {
	seed := uint64(11)
	m := testMap(seed)
	const reinfTick = int64(30) + 50*86400

	events := []store.Event{
		consolidationEvent(t, 20, "agent.memory_added", MemoryAddedPayload{Agent: 0, Text: "Rowan claims tendrils.", Salience: 4, Subject: 3, Origin: OriginGist}),
		consolidationEvent(t, 30, "agent.belief_revised", BeliefRevisedPayload{
			Agent: 0, BeliefID: 0, Statement: "Tendrils lurk past the ridge.", Confidence: 68,
			Provenance: ProvenanceTold, Source: 3, Subject: -1,
			Evidence: []MemoryRef{{Tick: 20, Hash: MemoryHash("Rowan claims tendrils.")}}, Direct: false,
		}),
		// The grounded-observation seam fires much later (stand-in producer).
		consolidationEvent(t, reinfTick, "agent.belief_reinforced", BeliefReinforcedPayload{Agent: 0, BeliefID: 1}),
	}

	replay := func() *State {
		s := NewState(seed, m)
		for _, e := range events {
			if err := s.Apply(e); err != nil {
				t.Fatal(err)
			}
		}
		s.Tick = reinfTick
		return s
	}
	a, b := replay(), replay()
	if !bytes.Equal(a.Marshal(), b.Marshal()) {
		t.Error("two replays of a reinforcement log diverged")
	}

	var thawed State
	if err := json.Unmarshal(a.Marshal(), &thawed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Marshal(), thawed.Marshal()) {
		t.Error("snapshot round-trip changed state")
	}

	// The reinforcement re-anchored the clock to its tick (recorded input).
	if got := a.Agents[0].Beliefs[0].Reinforced; got != reinfTick {
		t.Errorf("replayed Reinforced = %d, want %d", got, reinfTick)
	}
}

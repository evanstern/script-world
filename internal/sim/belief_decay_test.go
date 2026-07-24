package sim

// Spec 030 (US2, T006) decay-arithmetic tests: EffectiveConfidence is a pure,
// computed-on-read half-life curve (nothing stored ever mutates), a revision
// re-anchors the clock only on direct evidence (US2-AC3), and a legacy belief
// with no stamp is grandfathered. Curve values are pinned to the tick per
// contracts/events-and-decay.md and quickstart §3. Model-free, deterministic.

import "testing"

// TestEffectiveConfidenceCurve pins the half-life curve to the tick (SC-002).
// day = 86400 ticks; half-life = BeliefHalfLifeDays (8). The day-4 point proves
// the curve is CONTINUOUS (fractional half-lives), not an integer 8-day step:
// integer stepping would still read the full 80 at day 4.
func TestEffectiveConfidenceCurve(t *testing.T) {
	const day = int64(86400)
	const R = int64(1000) // formation anchor

	cases := []struct {
		name  string
		conf  int
		reinf int64
		tick  int64
		want  int
	}{
		{"formation (0 elapsed) reads full", 80, R, R, 80},
		{"fractional half-life (day 4) reads continuous", 80, R, R + 4*day, 57},
		{"one half-life (day 8) halves", 80, R, R + 8*day, 40},
		{"one half-life on the Birch confidence 68", 68, R, R + 8*day, 34},
		{"day 12 between half-lives", 80, R, R + 12*day, 28},
		{"two half-lives (day 16) at the floor", 80, R, R + 16*day, 20},
		{"floor crossing (day 17) reads below floor", 80, R, R + 17*day, 18},
		{"day 24 three half-lives", 80, R, R + 24*day, 10},
		{"legacy grandfather (Reinforced 0) never decays", 80, 0, 9_999_999, 80},
	}
	for _, c := range cases {
		got := EffectiveConfidence(Belief{Confidence: c.conf, Reinforced: c.reinf}, c.tick)
		if got != c.want {
			t.Errorf("%s: EffectiveConfidence(conf=%d, reinf=%d, tick=%d) = %d, want %d",
				c.name, c.conf, c.reinf, c.tick, got, c.want)
		}
	}
}

// TestFloorCrossingBoundary pins the floor semantics the read sites (T008) key
// off: at two half-lives the effective value sits AT the floor (still live), one
// day later it drops below (fades to myth).
func TestFloorCrossingBoundary(t *testing.T) {
	const day = int64(86400)
	const R = int64(1000)
	b := Belief{Confidence: 80, Reinforced: R}

	if got := EffectiveConfidence(b, R+16*day); got < BeliefConfidenceFloor {
		t.Errorf("day 16 effective %d is below floor %d — expected at/above (still live)", got, BeliefConfidenceFloor)
	}
	if got := EffectiveConfidence(b, R+17*day); got >= BeliefConfidenceFloor {
		t.Errorf("day 17 effective %d is at/above floor %d — expected below (faded)", got, BeliefConfidenceFloor)
	}
}

// TestEffectiveConfidenceNeverMutates: EffectiveConfidence is read-only — the
// stored Confidence and Reinforced are untouched by any number of reads (the
// memory-recency precedent, FR-006).
func TestEffectiveConfidenceNeverMutates(t *testing.T) {
	b := Belief{Confidence: 80, Reinforced: 1000}
	for i := 0; i < 5; i++ {
		EffectiveConfidence(b, 1000+int64(i)*86400)
	}
	if b.Confidence != 80 || b.Reinforced != 1000 {
		t.Errorf("read mutated the belief: Confidence=%d Reinforced=%d", b.Confidence, b.Reinforced)
	}
}

// TestDirectRevisionRefreshesClock (US2-AC3): a held belief's revision re-anchors
// the decay clock iff it cites direct-perception evidence. A hearsay-only
// revision changes stored confidence but leaves Reinforced — so a myth retold
// nightly never refreshes; a directly-confirmed revision resets the curve. The
// post-reinforcement reset is proven by reading full conviction at the new anchor.
func TestDirectRevisionRefreshesClock(t *testing.T) {
	const day = int64(86400)
	s := NewState(42, testMap(42))

	// Form a belief at tick 1000 (Reinforced = 1000).
	if err := s.Apply(consolidationEvent(t, 1000, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: 0, Statement: "Tendrils lurk past the ridge.", Confidence: 80,
		Provenance: ProvenanceTold, Source: 3, Subject: -1,
		Evidence: []MemoryRef{{Tick: 100, Hash: "aa"}}, Direct: false,
	})); err != nil {
		t.Fatal(err)
	}
	id := s.Agents[0].Beliefs[0].ID

	// Hearsay-only revision at day 8: confidence changes, clock does NOT refresh.
	hearsayTick := int64(1000) + 8*day
	if err := s.Apply(consolidationEvent(t, hearsayTick, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: id, Statement: "Tendrils lurk past the ridge.", Confidence: 60,
		Provenance: ProvenanceTold, Source: 3, Subject: -1,
		Evidence: []MemoryRef{{Tick: 200, Hash: "bb"}}, Direct: false,
	})); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents[0].Beliefs[0].Reinforced; got != 1000 {
		t.Errorf("hearsay revision refreshed the clock: Reinforced = %d, want 1000 (unchanged)", got)
	}
	// The belief keeps decaying from formation: at day 8 the effective value is
	// still the halved stored confidence (60 → 30), not reset.
	if got := EffectiveConfidence(s.Agents[0].Beliefs[0], hearsayTick); got != 30 {
		t.Errorf("hearsay revision effective at day 8 = %d, want 30 (still decaying from formation)", got)
	}

	// Direct-evidence revision at day 10: clock refreshes to now.
	directTick := int64(1000) + 10*day
	if err := s.Apply(consolidationEvent(t, directTick, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: id, Statement: "I saw the tendrils myself.", Confidence: 70,
		Provenance: ProvenanceWitnessed, Source: -1, Subject: -1,
		Evidence: []MemoryRef{{Tick: directTick - 10, Hash: "cc"}}, Direct: true,
	})); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents[0].Beliefs[0].Reinforced; got != directTick {
		t.Errorf("direct revision did not refresh the clock: Reinforced = %d, want %d", got, directTick)
	}
	// Post-reinforcement reset: at the new anchor the belief reads full conviction.
	if got := EffectiveConfidence(s.Agents[0].Beliefs[0], directTick); got != 70 {
		t.Errorf("post-refresh effective = %d, want 70 (curve reset to full)", got)
	}
}

// TestPromptBeliefsExcludesBelowFloor (spec 030 US2, FR-007; T008): the
// general read-site exclusion rule for a model-facing prompt that lists
// beliefs as live convictions — distinct from the nightly consolidation
// held-beliefs block (mind/consolidate.go), which is the documented
// exception and does NOT call this (it lists faded beliefs too, marked).
func TestPromptBeliefsExcludesBelowFloor(t *testing.T) {
	const day = int64(86400)
	const R = int64(1000)
	tick := R + 17*day // matches TestEffectiveConfidenceCurve's floor-crossing day

	live := Belief{ID: 1, Confidence: 95, Reinforced: R}
	faded := Belief{ID: 2, Confidence: 80, Reinforced: R}
	if got := EffectiveConfidence(live, tick); got < BeliefConfidenceFloor {
		t.Fatalf("test setup: expected the confidence-95 belief to stay live at day 17, got effective %d", got)
	}
	if got := EffectiveConfidence(faded, tick); got >= BeliefConfidenceFloor {
		t.Fatalf("test setup: expected the confidence-80 belief to fall below the floor at day 17, got effective %d", got)
	}

	got := PromptBeliefs([]Belief{live, faded}, tick)
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("PromptBeliefs([live, faded], day17) = %+v, want only the live belief (id 1)", got)
	}

	// Every belief below the floor: excluded down to empty, no panic.
	if got := PromptBeliefs([]Belief{faded}, tick); len(got) != 0 {
		t.Errorf("PromptBeliefs([faded], day17) = %+v, want empty", got)
	}

	// Nothing below the floor (no elapsed time): passthrough, order preserved.
	stillLive := Belief{ID: 3, Confidence: 90, Reinforced: R}
	if got := PromptBeliefs([]Belief{live, stillLive}, R); len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("PromptBeliefs at formation = %+v, want both beliefs unchanged", got)
	}
}

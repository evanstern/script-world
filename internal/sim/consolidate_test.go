package sim

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

func consolidationEvent(t *testing.T, tick int64, typ string, payload any) store.Event {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return store.Event{Tick: tick, Type: typ, Payload: b}
}

func findMem(t *testing.T, s *State, agent int, text string) *Memory {
	t.Helper()
	for i := range s.Agents[agent].Memories {
		if s.Agents[agent].Memories[i].Text == text {
			return &s.Agents[agent].Memories[i]
		}
	}
	return nil
}

// TestConsolidationReducer is the T004 reducer table: promote (cap), fade
// (remove), vanished targets no-op, belief create/revise/clamp, narrative,
// marker semantics.
func TestConsolidationReducer(t *testing.T) {
	s := NewState(42, testMap(42))
	seed := []Memory{
		{Text: "Chopped the birch all morning.", Salience: 3, Tick: 100, Subject: -1},
		{Text: "Cedar lied about the wolf.", Salience: 9, Tick: 200, Subject: 2, Tone: -50},
	}
	s.Agents[0].Memories = append(s.Agents[0].Memories, seed...)

	apply := func(tick int64, typ string, p any) {
		t.Helper()
		if err := s.Apply(consolidationEvent(t, tick, typ, p)); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
	}

	// Promote: +3 from 3 → 6; cap at MaxSalience on the second boost.
	apply(300, "agent.memory_promoted", MemoryPromotedPayload{
		Agent: 0, MemTick: 100, TextHash: MemoryHash(seed[0].Text), Boost: 3})
	if m := findMem(t, s, 0, seed[0].Text); m.Salience != 6 {
		t.Errorf("promoted salience = %d, want 6", m.Salience)
	}
	apply(301, "agent.memory_promoted", MemoryPromotedPayload{
		Agent: 0, MemTick: 100, TextHash: MemoryHash(seed[0].Text), Boost: 9})
	if m := findMem(t, s, 0, seed[0].Text); m.Salience != MaxSalience {
		t.Errorf("salience cap = %d, want %d", m.Salience, MaxSalience)
	}

	// Vanished target (wrong hash): no-op, no error.
	apply(302, "agent.memory_faded", MemoryFadedPayload{Agent: 0, MemTick: 100, TextHash: "00000000"})
	if len(s.Agents[0].Memories) != 2 {
		t.Fatalf("no-op fade removed something: %d memories", len(s.Agents[0].Memories))
	}

	// Real fade removes.
	apply(303, "agent.memory_faded", MemoryFadedPayload{
		Agent: 0, MemTick: 100, TextHash: MemoryHash(seed[0].Text)})
	if findMem(t, s, 0, seed[0].Text) != nil {
		t.Error("faded memory still present")
	}

	// Belief create (id 0), confidence clamped.
	apply(400, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: 0, Statement: "Cedar breaks his word.",
		Confidence: 130, Provenance: ProvenanceWitnessed, Source: -1, Subject: 2})
	if n := len(s.Agents[0].Beliefs); n != 1 {
		t.Fatalf("beliefs = %d, want 1", n)
	}
	b := s.Agents[0].Beliefs[0]
	if b.ID != 1 || b.Confidence != 100 || b.Subject != 2 {
		t.Errorf("belief = %+v", b)
	}

	// Revise in place; unknown ID no-ops.
	apply(500, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: 1, Statement: "Cedar breaks his word.",
		Confidence: 40, Provenance: ProvenanceWitnessed, Source: -1, Subject: 2})
	apply(501, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 0, BeliefID: 99, Statement: "ghost", Confidence: 50, Provenance: ProvenanceInferred, Source: -1, Subject: -1})
	if got := s.Agents[0].Beliefs[0].Confidence; got != 40 {
		t.Errorf("revised confidence = %d, want 40", got)
	}
	if len(s.Agents[0].Beliefs) != 1 {
		t.Errorf("unknown-ID revision created a belief")
	}

	// Second agent's new belief gets the next monotonic ID.
	apply(502, "agent.belief_revised", BeliefRevisedPayload{
		Agent: 1, BeliefID: 0, Statement: "The woods are safe.", Confidence: 60,
		Provenance: ProvenanceInferred, Source: -1, Subject: -1})
	if id := s.Agents[1].Beliefs[0].ID; id != 2 {
		t.Errorf("second belief ID = %d, want 2", id)
	}

	// Narrative replacement.
	apply(600, "agent.narrative_set", NarrativeSetPayload{Agent: 0, Text: "I am the one who watches."})
	if s.Agents[0].Narrative != "I am the one who watches." {
		t.Errorf("narrative = %q", s.Agents[0].Narrative)
	}

	// Marker: accepted advances night + up_to; rejected bumps night only.
	apply(700, "agent.consolidated", ConsolidatedPayload{
		Agent: 0, Night: 1, UpTo: 650, Outcome: ConsolidationAccepted})
	a := &s.Agents[0]
	if a.LastConsolidatedNight != 1 || a.ConsolidatedUpTo != 650 || a.LastConsolidateMark != 700 {
		t.Errorf("accepted marker: night=%d upTo=%d mark=%d", a.LastConsolidatedNight, a.ConsolidatedUpTo, a.LastConsolidateMark)
	}
	apply(86500, "agent.consolidated", ConsolidatedPayload{
		Agent: 0, Night: 2, UpTo: 86400, Outcome: ConsolidationRejected, Reason: "drift"})
	if a.LastConsolidatedNight != 2 || a.ConsolidatedUpTo != 650 {
		t.Errorf("rejected marker must not advance up_to: night=%d upTo=%d", a.LastConsolidatedNight, a.ConsolidatedUpTo)
	}
}

// TestConsolidationLedger covers the once-per-night guards and the buffer
// boundary math (FR-001, D2).
func TestConsolidationLedger(t *testing.T) {
	s := NewState(7, testMap(7))
	a := &s.Agents[3]

	if got := NightIndex(0); got != 1 {
		t.Errorf("NightIndex(0) = %d, want 1 (1-based)", got)
	}
	if !a.ConsolidationDue(80000) {
		t.Error("fresh agent must be due (night 1 > never=0)")
	}

	// Judged night 1 → not due again the same night, due next night only
	// after the 12-game-hour gap.
	if err := s.Apply(consolidationEvent(t, 80000, "agent.consolidated", ConsolidatedPayload{
		Agent: 3, Night: 1, UpTo: 79000, Outcome: ConsolidationAccepted})); err != nil {
		t.Fatal(err)
	}
	if a.ConsolidationDue(82000) {
		t.Error("same night must not be due")
	}
	if a.ConsolidationDue(87000) {
		t.Error("post-midnight doze (night 2, gap 7000 < 43200) must not be due")
	}
	if !a.ConsolidationDue(80000 + ConsolidationGapTicks + 90000) {
		t.Error("next evening must be due")
	}

	// Dead agents never consolidate.
	a.Dead = true
	if a.ConsolidationDue(999999) {
		t.Error("the dead must not be due")
	}
	a.Dead = false

	// Buffer boundary: only memories past ConsolidatedUpTo.
	a.Memories = []Memory{
		{Text: "old", Salience: 5, Tick: 70000, Subject: -1},
		{Text: "new", Salience: 5, Tick: 79500, Subject: -1},
	}
	buf := a.EpisodicBuffer()
	if len(buf) != 1 || buf[0].Text != "new" {
		t.Errorf("buffer = %+v, want just the post-up_to memory", buf)
	}
}

// TestConsolidationReplayDeterminism: a timeline containing every
// consolidation event type replays to byte-identical state (SC-004).
func TestConsolidationReplayDeterminism(t *testing.T) {
	seed := uint64(11)
	m := testMap(seed)
	live := NewState(seed, m)

	events := []store.Event{
		consolidationEvent(t, 10, "agent.memory_added", MemoryAddedPayload{Agent: 0, Text: "saw a wolf", Salience: 7, Subject: -1}),
		consolidationEvent(t, 20, "agent.memory_added", MemoryAddedPayload{Agent: 0, Text: "ate berries", Salience: 2, Subject: -1}),
		consolidationEvent(t, 30, "agent.memory_promoted", MemoryPromotedPayload{Agent: 0, MemTick: 10, TextHash: MemoryHash("saw a wolf"), Boost: 2}),
		consolidationEvent(t, 31, "agent.memory_faded", MemoryFadedPayload{Agent: 0, MemTick: 20, TextHash: MemoryHash("ate berries")}),
		consolidationEvent(t, 32, "agent.memory_added", MemoryAddedPayload{Agent: 0, Text: "A day of wolves and hunger.", Salience: SalDayGist, Subject: -1}),
		consolidationEvent(t, 33, "agent.belief_revised", BeliefRevisedPayload{Agent: 0, BeliefID: 0, Statement: "Wolves hunt the ridge.", Confidence: 70, Provenance: ProvenanceWitnessed, Source: -1, Subject: -1}),
		consolidationEvent(t, 34, "agent.narrative_set", NarrativeSetPayload{Agent: 0, Text: "I survive by paying attention."}),
		consolidationEvent(t, 35, "agent.consolidated", ConsolidatedPayload{Agent: 0, Night: 1, UpTo: 32, Outcome: ConsolidationAccepted, Promoted: 1, Faded: 1, Beliefs: 1}),
		consolidationEvent(t, 86500, "agent.consolidated", ConsolidatedPayload{Agent: 1, Night: 2, Outcome: ConsolidationSkippedEmpty}),
	}
	for _, e := range events {
		if err := live.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	live.Tick = 86500

	replayed := NewState(seed, m)
	for _, e := range events {
		if err := replayed.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	replayed.Tick = 86500

	if !bytes.Equal(live.Marshal(), replayed.Marshal()) {
		t.Error("replayed state differs from live state")
	}

	// The whole state (beliefs, narrative, ledger) must round-trip the
	// snapshot path too.
	var thawed State
	if err := json.Unmarshal(live.Marshal(), &thawed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(live.Marshal(), thawed.Marshal()) {
		t.Error("snapshot round-trip changed state")
	}
}

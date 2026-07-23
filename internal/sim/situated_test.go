package sim

// Spec 019 (US1) unit tests: the situated-memory data path (reducer copy,
// intent-reason survival), the deterministic place helper, and the situated
// text grammar. Table-driven, model-free — the whole layer is deterministic.

import (
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// TestMemoryReducerCopiesContext (T004): the agent.memory_added arm copies
// Where/Why/Conv from payload to Memory unchanged; a pre-019 payload (fields
// absent) reduces to a pre-019-shaped Memory (nil/""/0) — FR-007, FR-014.
func TestMemoryReducerCopiesContext(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)

	// A situated (019) memory.
	where := &MemoryPlace{X: 7, Y: 12, Desc: "the rock outcrop"}
	if err := s.Apply(store.Event{Tick: 100, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{Agent: 0, Text: "Built a fire.", Salience: 5,
			Subject: -1, Where: where, Why: "keep the Gru away tonight.", Conv: 0})}); err != nil {
		t.Fatal(err)
	}
	// A pre-019 memory (no situated fields).
	if err := s.Apply(store.Event{Tick: 200, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{Agent: 0, Text: "an old memory", Salience: 3, Subject: -1})}); err != nil {
		t.Fatal(err)
	}

	got := s.Agents[0].Memories
	if len(got) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(got))
	}
	m0 := got[0]
	if m0.Where == nil || m0.Where.X != 7 || m0.Where.Y != 12 || m0.Where.Desc != "the rock outcrop" {
		t.Errorf("situated memory Where = %+v, want {7 12 the rock outcrop}", m0.Where)
	}
	if m0.Why != "keep the Gru away tonight." {
		t.Errorf("Why = %q, want the reason verbatim", m0.Why)
	}
	m1 := got[1]
	if m1.Where != nil || m1.Why != "" || m1.Conv != 0 {
		t.Errorf("pre-019 memory carried fabricated context: %+v", m1)
	}
}

// TestPre019RoundTripByteIdentical (T020 / FR-014, SC-007): a state built from
// pre-019 events (memories with no situated context, intents with no reason,
// agents that never journal) marshals with NONE of the spec-019 JSON keys — so
// a pre-019 snapshot round-trips byte-identically and a mixed-era log replays
// cleanly. Two independent replays of the same log agree byte-for-byte.
func TestPre019RoundTripByteIdentical(t *testing.T) {
	m := testMap(42)

	// A pre-019 event log: bare memories (no where/why/conv) and a bare intent
	// (no reason) — exactly what a log recorded before this feature carries.
	log := []store.Event{
		{Tick: 100, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{Agent: 0, Goal: "forage", Source: "reflex"})},
		{Tick: 200, Type: "agent.memory_added", Payload: mustPayload(MemoryAddedPayload{Agent: 0, Text: "Built a fire.", Salience: 5, Subject: -1})},
		{Tick: 300, Type: "agent.memory_added", Payload: mustPayload(MemoryAddedPayload{Agent: 1, Text: "Talked with Ash.", Salience: 3, Subject: 0})},
	}
	replay := func() []byte {
		s := NewState(42, m)
		for _, e := range log {
			if err := s.Apply(e); err != nil {
				t.Fatalf("apply %s: %v", e.Type, err)
			}
		}
		return s.Marshal()
	}

	a, b := replay(), replay()
	if string(a) != string(b) {
		t.Fatalf("two replays of the same pre-019 log diverged:\n%s\n---\n%s", a, b)
	}
	// None of the spec-019 keys may appear — the omitempty fields stay absent.
	for _, key := range []string{`"where"`, `"why"`, `"conv"`, `"reason"`, `"journal"`} {
		if strings.Contains(string(a), key) {
			t.Errorf("pre-019 state carries the spec-019 key %s — round-trip is not byte-identical:\n%s", key, a)
		}
	}
	// And the reduced memories are pre-019-shaped (no fabricated context).
	s := NewState(42, m)
	for _, e := range log {
		_ = s.Apply(e)
	}
	for _, mem := range s.Agents[0].Memories {
		if mem.Where != nil || mem.Why != "" || mem.Conv != 0 {
			t.Errorf("pre-019 memory gained context: %+v", mem)
		}
	}
}

// TestIntentReasonSurvivesToState (T005): a planner intent_set carrying a reason
// populates Intent.Reason; a reflex intent_set (no reason) leaves it "".
func TestIntentReasonSurvivesToState(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)

	if err := s.Apply(store.Event{Tick: 10, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 0, Goal: "build_fire", Source: "planner",
			Reason: "the night will be cold"})}); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents[0].Intent; got == nil || got.Reason != "the night will be cold" {
		t.Fatalf("planner Intent.Reason = %+v, want the reason carried", got)
	}

	if err := s.Apply(store.Event{Tick: 20, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 1, Goal: "forage", Source: "reflex"})}); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents[1].Intent; got == nil || got.Reason != "" {
		t.Fatalf("reflex Intent.Reason = %q, want \"\" (never fabricated)", got.Reason)
	}
}

// TestDescribePlace (T006): the feature scan is deterministic — a station on
// the tile wins; an adjacent station is found within the ring; a nil map (or no
// notable feature reachable) yields "". Structures are fully controlled here so
// the assertions do not depend on generated terrain.
func TestDescribePlace(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)

	// A station ON the tile is the most notable feature — it wins over whatever
	// terrain the generated map placed there.
	s.Structures = []Structure{{Kind: "oven", X: 20, Y: 20}}
	if d := describePlace(s, 20, 20); d != "the oven" {
		t.Errorf("on the oven tile: describePlace = %q, want \"the oven\"", d)
	}

	// Ring scan: find a 3×3 neighborhood the generated terrain leaves plain, then
	// plant a chest one tile away and confirm the scan reaches it (radius 1).
	cx, cy, found := plainCenter(s)
	if !found {
		t.Skip("no terrain-free 3×3 neighborhood found on this map")
	}
	s.Structures = []Structure{{Kind: "chest", X: cx + 1, Y: cy}}
	if d := describePlace(s, cx, cy); d != "the chest" {
		t.Errorf("adjacent to the chest at plain (%d,%d): describePlace = %q, want \"the chest\"", cx, cy, d)
	}

	// Determinism: identical inputs, identical output.
	if describePlace(s, cx, cy) != describePlace(s, cx, cy) {
		t.Error("describePlace not deterministic")
	}
	// No map ⇒ coords alone situate the memory.
	s.SetMap(nil)
	if d := describePlace(s, cx, cy); d != "" {
		t.Errorf("nil map: describePlace = %q, want \"\"", d)
	}
}

// plainCenter finds a tile whose whole ring-2 scan window is free of notable
// terrain (structures aside), so a planted structure is the only feature the
// scan can find — making the ring assertion independent of generated terrain.
func plainCenter(s *State) (int, int, bool) {
	bare := &State{m: s.m}
	for y := placeScanRadius; y < s.m.H-placeScanRadius; y++ {
		for x := placeScanRadius; x < s.m.W-placeScanRadius; x++ {
			if describePlace(bare, x, y) == "" {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// TestSituateText (T007): the situated text grammar, composed in the exact
// order pinned by contracts/memory-context.md — pins the strings the examples
// name, with and without desc and with and without why.
func TestSituateText(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		where *MemoryPlace
		why   string
		want  string
	}{
		{"where+why", "Built a fire.", &MemoryPlace{X: 23, Y: 41, Desc: "the rock outcrop"},
			"keep the Gru away from camp tonight.",
			"Built a fire at the rock outcrop (23,41) — keep the Gru away from camp tonight."},
		{"where only, no desc", "Raised a shelter with my own hands.", &MemoryPlace{X: 7, Y: 12},
			"", "Raised a shelter with my own hands at (7,12)."},
		{"where with desc, no why", "Talked with Mira.", &MemoryPlace{X: 3, Y: 4, Desc: "the woods"},
			"", "Talked with Mira at the woods (3,4)."},
		{"no where, no why", "A bare memory.", nil, "", "A bare memory."},
		{"why without a trailing period on base", "Foraged", &MemoryPlace{X: 1, Y: 2}, "hungry.",
			"Foraged at (1,2) — hungry."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := situateText(c.base, c.where, c.why); got != c.want {
				t.Errorf("situateText = %q\nwant             %q", got, c.want)
			}
		})
	}
}

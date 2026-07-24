package sim

// Spec 030 (T002) unit tests: the memory-origin substrate. The classifier is a
// pure function on the stored field; the constructors stamp origin at emission;
// the reducer copies it verbatim; a pre-030 memory (no origin) round-trips
// byte-identically. Model-free — the whole layer is deterministic.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// TestDirectPerceptionClassifier (FR-002): the closed vocabulary maps to direct
// perception deterministically — action/witness/omen are direct; report/gist/
// digest and the absent/legacy origin are secondhand.
func TestDirectPerceptionClassifier(t *testing.T) {
	cases := []struct {
		origin string
		direct bool
	}{
		{OriginAction, true},
		{OriginWitness, true},
		{OriginOmen, true},
		{OriginReport, false},
		{OriginGist, false},
		{OriginDigest, false},
		{"", false},         // legacy / unclassified → secondhand (conservative)
		{"nonsense", false}, // unknown value → secondhand
	}
	for _, c := range cases {
		if got := DirectPerception(c.origin); got != c.direct {
			t.Errorf("DirectPerception(%q) = %v, want %v", c.origin, got, c.direct)
		}
	}
}

// TestSituatedConstructorsStampOrigin (T002): each situated constructor writes
// the origin it is handed into the payload, and the classifier reads it back —
// personal acts are direct (action), a witnessed event is direct (witness), a
// chest-owner's learned-at-a-distance report is secondhand.
func TestSituatedConstructorsStampOrigin(t *testing.T) {
	where := &MemoryPlace{X: 1, Y: 2}
	cases := []struct {
		name       string
		ev         store.Event
		wantOrigin string
		wantDirect bool
	}{
		{"personal → action", situatedMemoryEvent(10, 0, 5, where, "", OriginAction, "Built a fire."), OriginAction, true},
		{"toned → action", situatedMemoryToned(10, 0, 5, 40, where, "", OriginAction, "Bathed."), OriginAction, true},
		{"about, seen → witness", situatedMemoryAboutEvent(10, 0, 1, -10, 5, where, OriginWitness, "Saw %s die.", "Rowan"), OriginWitness, true},
		{"about, learned → report", situatedMemoryAboutEvent(10, 0, 1, -10, 5, where, OriginReport, "%s took from my chest.", "Rowan"), OriginReport, false},
	}
	for _, c := range cases {
		var p MemoryAddedPayload
		if err := json.Unmarshal(c.ev.Payload, &p); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p.Origin != c.wantOrigin {
			t.Errorf("%s: Origin = %q, want %q", c.name, p.Origin, c.wantOrigin)
		}
		if got := DirectPerception(p.Origin); got != c.wantDirect {
			t.Errorf("%s: DirectPerception = %v, want %v", c.name, got, c.wantDirect)
		}
	}
}

// TestMemoryOriginReducerCopies (T002): the agent.memory_added arm copies Origin
// from payload to Memory unchanged; a pre-030 payload (field absent) reduces to
// Origin "" — never a fabricated classification.
func TestMemoryOriginReducerCopies(t *testing.T) {
	m := testMap(42)
	s := NewState(42, m)

	if err := s.Apply(store.Event{Tick: 100, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{Agent: 0, Text: "Built a fire.", Salience: 5, Subject: -1, Origin: OriginAction})}); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(store.Event{Tick: 200, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{Agent: 0, Text: "an old memory", Salience: 3, Subject: -1})}); err != nil {
		t.Fatal(err)
	}

	got := s.Agents[0].Memories
	if len(got) != 2 {
		t.Fatalf("memories = %d, want 2", len(got))
	}
	if got[0].Origin != OriginAction {
		t.Errorf("stamped memory Origin = %q, want %q", got[0].Origin, OriginAction)
	}
	if got[1].Origin != "" {
		t.Errorf("pre-030 memory gained an origin: %q", got[1].Origin)
	}
}

// TestEmittedSimMemoriesCarryOrigin (T002 checkpoint: "every landed memory
// carries an origin"): every episodic memory the sim emits over a driven
// game-day carries a valid origin from the closed vocabulary — the observable
// guarantee behind the compiler-enforced required parameter.
func TestEmittedSimMemoriesCarryOrigin(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	log := driveTicks(t, s, m, 24*3600, nil) // a full game-day: fires, forages, talks, a cold night

	valid := map[string]bool{
		OriginAction: true, OriginWitness: true, OriginReport: true,
		OriginOmen: true, OriginGist: true, OriginDigest: true,
	}
	seen := 0
	for _, e := range log {
		if e.Type != "agent.memory_added" {
			continue
		}
		var p MemoryAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		seen++
		if !valid[p.Origin] {
			t.Errorf("sim-emitted memory carries no/invalid origin %q: %q", p.Origin, p.Text)
		}
	}
	if seen == 0 {
		t.Fatal("a full game-day produced no memories — the coverage assertion would be vacuous")
	}
}

// TestPre030MemoryByteIdentical (FR-011 / no format bump): a memory landed with
// no origin marshals with NO "origin" key, so a pre-030 snapshot round-trips
// byte-identically and two replays agree.
func TestPre030MemoryByteIdentical(t *testing.T) {
	m := testMap(42)
	log := []store.Event{
		{Tick: 100, Type: "agent.memory_added", Payload: mustPayload(MemoryAddedPayload{Agent: 0, Text: "Built a fire.", Salience: 5, Subject: -1})},
		{Tick: 200, Type: "agent.memory_added", Payload: mustPayload(MemoryAddedPayload{Agent: 1, Text: "Talked with Ash.", Salience: 3, Subject: 0})},
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
		t.Fatalf("two replays of the same pre-030 log diverged:\n%s\n---\n%s", a, b)
	}
	if strings.Contains(string(a), `"origin"`) {
		t.Errorf("pre-030 state carries the spec-030 key \"origin\" — round-trip is not byte-identical:\n%s", a)
	}
}

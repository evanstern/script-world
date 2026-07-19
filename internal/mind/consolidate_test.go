package mind

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// setupConsol builds a mind whose replica has agent 0 (Ash) carrying a
// known episodic buffer, driven by the given scripted model.
func setupConsol(t *testing.T, model Submitter) (*harness, *Mind) {
	t.Helper()
	h := newHarness(t, "") // harness's own mock is unused; we build our own mind
	m := h.m
	state := sim.NewState(42, m)
	state.Agents[0].Memories = []sim.Memory{
		{Text: "Saw a wolf at the treeline.", Salience: 7, Tick: 100, Subject: -1},
		{Text: "Ate two berries.", Salience: 1, Tick: 200, Subject: -1},
		{Text: "Cedar promised me firewood.", Salience: 5, Tick: 300, Subject: 2, Tone: 20},
	}

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(md.Close)
	return h, md
}

func sleptEvent(t *testing.T, tick int64, agent int) store.Event {
	t.Helper()
	b, err := json.Marshal(sim.AgentPayload{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	return store.Event{Tick: tick, Type: "agent.slept", Payload: b}
}

// goodConsolidation is a valid output for setupConsol's buffer, in Ash's
// nature.
func goodConsolidation() string {
	return fmt.Sprintf(`{
  "nature": "%s",
  "gist": "A day of wolves, thin meals, and a promise from Cedar.",
  "promote": [{"tick": 100, "hash": "%s"}],
  "fade": [{"tick": 200, "hash": "%s"}],
  "beliefs": [{"id": 0, "statement": "Cedar keeps his word.", "confidence": 55, "provenance": "witnessed", "source": -1, "subject": 2}],
  "narrative": "I keep this village fed and I keep my head. The wolf worries me; the fire does not."
}`, persona.Anchors["Ash"],
		sim.MemoryHash("Saw a wolf at the treeline."),
		sim.MemoryHash("Ate two berries."))
}

// TestConsolidationAcceptedLands is US1 AC-1: one sleep → one atomic batch
// (promote, fade, gist, belief, narrative, marker), and a second sleep the
// same night does nothing (AC-2).
func TestConsolidationAcceptedLands(t *testing.T) {
	h, md := setupConsol(t, &scriptedModel{replies: []string{goodConsolidation()}})

	md.maybeConsolidate(sleptEvent(t, 80000, 0))

	markers := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "agent.consolidated"
	})
	if len(markers) != 1 {
		t.Fatalf("markers = %d, want 1", len(markers))
	}
	var mp sim.ConsolidatedPayload
	json.Unmarshal(markers[0].Payload, &mp)
	if mp.Outcome != sim.ConsolidationAccepted || mp.Night != 1 || mp.UpTo != 300 {
		t.Errorf("marker = %+v", mp)
	}

	all, _ := h.st.EventsSince(0, 0)
	counts := map[string]int{}
	for _, e := range all {
		counts[e.Type]++
	}
	for typ, want := range map[string]int{
		"agent.memory_promoted": 1, "agent.memory_faded": 1,
		"agent.belief_revised": 1, "agent.narrative_set": 1,
		"agent.consolidated": 1,
	} {
		if counts[typ] != want {
			t.Errorf("%s = %d, want %d", typ, counts[typ], want)
		}
	}

	// Feed the recorded marker back to the replica (in the daemon the loop
	// notify does this) — a second sleep the same night must not consolidate
	// again.
	md.absorb(markers)
	if md.replica.Agents[0].LastConsolidatedNight != 1 {
		t.Fatal("replica did not absorb the marker")
	}
	md.maybeConsolidate(sleptEvent(t, 81000, 0))
	time.Sleep(300 * time.Millisecond)
	all, _ = h.st.EventsSince(0, 0)
	n := 0
	for _, e := range all {
		if e.Type == "agent.consolidated" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("same-night second sleep consolidated again: %d markers", n)
	}
}

// TestConsolidationTransportFailureDefers is US1 AC-3: tier down → no
// marker, no events, buffer intact for the next sleep.
func TestConsolidationTransportFailureDefers(t *testing.T) {
	h, md := setupConsol(t, &scriptedModel{}) // no replies → every call errors

	md.maybeConsolidate(sleptEvent(t, 80000, 0))
	time.Sleep(500 * time.Millisecond)

	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if strings.HasPrefix(e.Type, "agent.consolidated") ||
			e.Type == "agent.narrative_set" || e.Type == "agent.belief_revised" {
			t.Fatalf("deferred consolidation leaked %s", e.Type)
		}
	}
	if md.replica.Agents[0].LastConsolidatedNight != 0 {
		t.Error("deferred attempt must not close the night")
	}
	if got := len(md.replica.Agents[0].EpisodicBuffer()); got != 3 {
		t.Errorf("buffer = %d memories, want 3 (intact)", got)
	}
}

// TestConsolidationMalformedRejected: garbage output lands ONLY a rejected
// marker; the buffer survives (retry next night).
func TestConsolidationMalformedRejected(t *testing.T) {
	h, md := setupConsol(t, &scriptedModel{replies: []string{"the model hums a tune with no json"}})

	md.maybeConsolidate(sleptEvent(t, 80000, 0))
	markers := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "agent.consolidated"
	})
	var mp sim.ConsolidatedPayload
	json.Unmarshal(markers[0].Payload, &mp)
	if mp.Outcome != sim.ConsolidationRejected || mp.Reason == "" {
		t.Errorf("marker = %+v, want rejected with reason", mp)
	}
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		switch e.Type {
		case "agent.memory_promoted", "agent.memory_faded", "agent.belief_revised", "agent.narrative_set", "agent.memory_added":
			t.Fatalf("rejected night leaked %s", e.Type)
		}
	}
}

// TestConsolidationEmptyBufferSkips: nothing to digest → skipped_empty
// marker, zero model calls.
func TestConsolidationEmptyBufferSkips(t *testing.T) {
	model := &scriptedModel{replies: []string{goodConsolidation()}}
	h := newHarness(t, "")
	state := sim.NewState(42, h.m)
	md, err := New(model, h.loop, h.loop, h.m, 42, state.Marshal(), [sim.AgentCount]string{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(md.Close)

	md.maybeConsolidate(sleptEvent(t, 80000, 1))
	markers := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "agent.consolidated"
	})
	var mp sim.ConsolidatedPayload
	json.Unmarshal(markers[0].Payload, &mp)
	if mp.Outcome != sim.ConsolidationSkippedEmpty {
		t.Errorf("outcome = %q", mp.Outcome)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	if calls != 0 {
		t.Errorf("empty night spent %d model calls", calls)
	}
}

// TestPersonaBytesSurviveConsolidation is FR-007's observable half: a full
// accepted cycle leaves every persona.md byte-identical (the consolidator
// has no filesystem access at all — this is the canary).
func TestPersonaBytesSurviveConsolidation(t *testing.T) {
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	sum := func() [sim.AgentCount][16]byte {
		var out [sim.AgentCount][16]byte
		for i, name := range sim.AgentNames {
			b, err := os.ReadFile(persona.PersonaPath(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			out[i] = md5.Sum(b)
		}
		return out
	}
	before := sum()

	h, md := setupConsol(t, &scriptedModel{replies: []string{goodConsolidation()}})
	md.maybeConsolidate(sleptEvent(t, 80000, 0))
	h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "agent.consolidated"
	})

	if before != sum() {
		t.Fatal("persona.md changed across a consolidation cycle")
	}
	// And the files are genesis-locked: not writable.
	if err := os.WriteFile(persona.PersonaPath(dir, "Ash"), []byte("mutiny"), 0o644); err == nil {
		t.Fatal("persona.md was writable (mode should be 0444)")
	}
}

package scribe

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

func mustPayloadJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSoulRendersFromEvents(t *testing.T) {
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

	scr.Observe([]store.Event{{
		Tick: 3600, Type: "agent.memory_added",
		Payload: mustPayloadJSON(t, map[string]any{"agent": 0, "text": "Built a fire.", "salience": 5}),
	}})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
		s := string(data)
		if strings.Contains(s, "Built a fire.") && strings.Contains(s, "(5★)") &&
			strings.Contains(s, "day 1 07:00") && strings.Contains(s, "1 memories") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
	t.Fatalf("soul.md never rendered the memory; content:\n%s", data)
}

func TestDeathFreezesSoulHeader(t *testing.T) {
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

	scr.Observe([]store.Event{{
		Tick: 7200, Type: "agent.died",
		Payload: mustPayloadJSON(t, map[string]any{"agent": 1, "cause": "exposure"}),
	}})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Birch"))
		if strings.Contains(string(data), "Dead") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("soul header never marked death")
}

// TestSoulShowsConsolidatedGrowth (TASK-9, US2): two synthetic nights of
// consolidation events render a changing narrative and beliefs with
// confidence + provenance referencing two distinct days.
func TestSoulShowsConsolidatedGrowth(t *testing.T) {
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

	night := func(tick int64, gist, narrative, belief string, conf int) []store.Event {
		return []store.Event{
			{Tick: tick, Type: "agent.memory_added", Payload: mustPayloadJSON(t,
				sim.MemoryAddedPayload{Agent: 0, Text: gist, Salience: sim.SalDayGist, Subject: -1})},
			{Tick: tick, Type: "agent.belief_revised", Payload: mustPayloadJSON(t,
				sim.BeliefRevisedPayload{Agent: 0, BeliefID: 0, Statement: belief,
					Confidence: conf, Provenance: sim.ProvenanceWitnessed, Source: -1, Subject: -1})},
			{Tick: tick, Type: "agent.narrative_set", Payload: mustPayloadJSON(t,
				sim.NarrativeSetPayload{Agent: 0, Text: narrative})},
			{Tick: tick, Type: "agent.consolidated", Payload: mustPayloadJSON(t,
				sim.ConsolidatedPayload{Agent: 0, Night: sim.NightIndex(tick), UpTo: tick,
					Outcome: sim.ConsolidationAccepted, Beliefs: 1})},
		}
	}
	scr.Observe(night(80000, "First night: wolves at the treeline.",
		"I am the one who watches the dark.", "Wolves hunt the ridge.", 70))
	scr.Observe(night(170000, "Second night: the fire held.",
		"I am the one who keeps the fire.", "Fire keeps the wolves honest.", 60))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
		s := string(data)
		if strings.Contains(s, "## Who I am becoming") &&
			strings.Contains(s, "I am the one who keeps the fire.") && // latest narrative won
			!strings.Contains(s, "watches the dark") && // replaced, not appended
			strings.Contains(s, "## Beliefs") &&
			strings.Contains(s, "Wolves hunt the ridge. *(70% sure — witnessed, day 2") &&
			strings.Contains(s, "Fire keeps the wolves honest. *(60% sure — witnessed, day 3") &&
			strings.Contains(s, "First night: wolves at the treeline.") &&
			strings.Contains(s, "Second night: the fire held.") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(persona.SoulPath(dir, "Ash"))
	t.Fatalf("soul.md never showed consolidated growth; content:\n%s", data)
}

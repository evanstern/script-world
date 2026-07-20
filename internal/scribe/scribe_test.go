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

// governedEvents is a small governance history: place, an enacted curfew, a
// rephrase, a violation, an amendment, an exile, and a repeal of the curfew.
func governedEvents(t *testing.T) []store.Event {
	t.Helper()
	enact := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 1, Kind: sim.ProposeCurfew,
			Target: -1, Param: 22 * 3600, Proposer: 1, Text: "No one out after nightfall."},
		Yeas: []int{0, 1, 2}, Nays: []int{3}, Passed: true,
	}
	amend := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 2, Kind: sim.ProposeAmend,
			NormID: 1, Target: -1, Param: 0, Proposer: 2, Text: "later curfew"},
		Yeas: []int{0, 1, 2}, Passed: true,
	}
	exile := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 3, Kind: sim.ProposeExile,
			Target: 3, Proposer: 0, Text: "Rowan is a danger to us all — cast them out."},
		Yeas: []int{0, 1, 2}, Passed: true,
	}
	repeal := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 4, Kind: sim.ProposeRepeal,
			NormID: 1, Target: -1, Proposer: 2, Text: "strike it"},
		Yeas: []int{0, 1, 2}, Passed: true,
	}
	return []store.Event{
		{Tick: 19800, Type: "meeting.place_designated", Payload: mustPayloadJSON(t, sim.MeetingPlacePayload{X: 12, Y: 34})},
		{Tick: 21960, Type: "meeting.proposal_resolved", Payload: mustPayloadJSON(t, enact)},
		{Tick: 21970, Type: "meeting.proposal_rephrased", Payload: mustPayloadJSON(t,
			sim.ProposalRephrasedPayload{ProposalID: 1, NormID: 1, Text: "Stay by the fire once the dark comes down."})},
		{Tick: 60000, Type: "norm.violated", Payload: mustPayloadJSON(t, sim.NormViolatedPayload{NormID: 1, Violator: 4, Witnesses: []int{5}})},
		{Tick: 108360, Type: "meeting.proposal_resolved", Payload: mustPayloadJSON(t, amend)},   // day 2
		{Tick: 194400, Type: "meeting.proposal_resolved", Payload: mustPayloadJSON(t, exile)},  // day 3 noon
		{Tick: 280800, Type: "meeting.proposal_resolved", Payload: mustPayloadJSON(t, repeal)}, // day 4 noon
	}
}

// TestVillageCharterRenders (TASK-13, US3): the charter file tracks the law —
// enactment with provenance, rephrased text, amendment, violation count,
// standing judgment, and repeal with strikethrough.
func TestVillageCharterRenders(t *testing.T) {
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

	path := dir + "/village_charter.md"
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "No rules yet") {
		t.Fatalf("fresh world should render an empty charter, got err=%v content:\n%s", err, data)
	}

	scr.Observe(governedEvents(t))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		s := string(data)
		if strings.Contains(s, "Meeting place: (12, 34)") &&
			strings.Contains(s, "## Standing judgments") &&
			strings.Contains(s, "Rowan is exiled. — proposed by Ash, passed day 3 (3-0)") &&
			strings.Contains(s, "## Repealed") &&
			strings.Contains(s, "~~Stay by the fire once the dark comes down.~~") &&
			strings.Contains(s, "proposed by Birch, passed day 1 (3-1)") &&
			strings.Contains(s, "repealed day 4") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("village_charter.md never rendered the governed history; content:\n%s", data)
}

// TestVillageCharterReconstructsFromLog (FR-007): a fresh scribe over a state
// replayed from the log renders byte-identical law to the live render.
func TestVillageCharterReconstructsFromLog(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	events := governedEvents(t)[:5] // through the violation + amendment

	build := func(dir string) []byte {
		state := sim.NewState(42, m)
		for _, e := range events {
			if err := state.Apply(e); err != nil {
				t.Fatal(err)
			}
		}
		if err := persona.Genesis(dir); err != nil {
			t.Fatal(err)
		}
		scr, err := New(dir, 42, m, state.Marshal())
		if err != nil {
			t.Fatal(err)
		}
		defer scr.Close()
		data, err := os.ReadFile(dir + "/village_charter.md")
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	a, b := build(t.TempDir()), build(t.TempDir())
	if string(a) != string(b) {
		t.Fatalf("replayed charters differ:\n%s\n---\n%s", a, b)
	}
	s := string(a)
	if !strings.Contains(s, "amended day 2") || !strings.Contains(s, "1 recorded violation(s)") {
		t.Fatalf("charter missing amendment/violation detail:\n%s", s)
	}
}

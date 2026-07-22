package mind

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// fakeSocial captures injected batches without a loop.
type fakeSocial struct {
	batches [][]store.Event
	err     error
}

func (f *fakeSocial) InjectSocial(events []store.Event) error {
	if f.err != nil {
		return f.err
	}
	f.batches = append(f.batches, events)
	return nil
}

// narrMind builds a bare Mind for collector/worker unit tests (no goroutines).
func narrMind(t *testing.T) (*Mind, *fakeSocial, *mockModel) {
	t.Helper()
	m := worldmap.Generate(42, 64, 64)
	social := &fakeSocial{}
	model := &mockModel{}
	md := &Mind{
		orch:      model,
		social:    social,
		replica:   sim.NewState(42, m),
		m:         m,
		narrQ:     make(chan narrJob, 8),
		narrRetry: make(chan narrCarry, 1),
	}
	return md, social, model
}

func mustEvent(t *testing.T, tick int64, typ string, p any) store.Event {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return store.Event{Tick: tick, Type: typ, Payload: b}
}

// TestChronicleNoteWindowing: notable events become named log lines; the
// night boundary closes the chapter as one job; a quiet chapter spends no job.
func TestChronicleNoteWindowing(t *testing.T) {
	md, _, _ := narrMind(t)

	md.chronicleNote(mustEvent(t, 1000, "agent.died", sim.DiedPayload{Agent: 0, Cause: "starvation"}))
	md.chronicleNote(mustEvent(t, 2000, "social.conversation", sim.ConversationPayload{
		Conv: 2000, A: 1, B: 7, Gist: "argued about firewood", Topics: []string{"firewood"},
		Participants: []int{1, 7},
	}))
	md.chronicleNote(mustEvent(t, 3000, "agent.moved", sim.AgentMovedPayload{Agent: 3, X: 1, Y: 1})) // not notable

	md.chronicleNote(mustEvent(t, 57600, "sim.night_started", sim.DayPayload{Day: 1}))
	var job narrJob
	select {
	case job = <-md.narrQ:
	default:
		t.Fatal("night boundary did not close a chapter")
	}
	if job.day != 1 || !strings.Contains(job.label, "day 1") {
		t.Errorf("job day/label: %d %q", job.day, job.label)
	}
	if len(job.lines) != 2 {
		t.Fatalf("lines = %d, want 2 (moves are not notable): %v", len(job.lines), job.lines)
	}
	if !strings.Contains(job.lines[0], "Ash died of starvation") {
		t.Errorf("death line: %q", job.lines[0])
	}
	if !strings.Contains(job.lines[1], "Birch, Sage talked about firewood") {
		t.Errorf("conversation line: %q", job.lines[1])
	}
	if job.fromTick != 1000 || job.toTick != 57600 {
		t.Errorf("window: %d..%d", job.fromTick, job.toTick)
	}

	// Nothing notable overnight: dawn closes no chapter.
	md.chronicleNote(mustEvent(t, 86400, "sim.day_started", sim.DayPayload{Day: 2}))
	select {
	case <-md.narrQ:
		t.Fatal("quiet chapter should spend nothing")
	default:
	}
}

// TestChronicleNoteChestTaken (T034, spec 013 FR-016): social.chest_taken —
// the theft story — becomes a notable log line naming taker and owner, the
// same narrative treatment as social.gave/social.promise_broken.
func TestChronicleNoteChestTaken(t *testing.T) {
	md, _, _ := narrMind(t)
	md.chronicleNote(mustEvent(t, 1000, "social.chest_taken", sim.ChestTakenPayload{
		Owner: 0, Taker: 3, X: 5, Y: 5,
	}))
	if len(md.narrLines) != 1 {
		t.Fatalf("lines = %d, want 1", len(md.narrLines))
	}
	if !strings.Contains(md.narrLines[0], "Rowan took from Ash's chest without asking.") {
		t.Errorf("chest_taken line: %q", md.narrLines[0])
	}
}

// TestRunNarrationLands: good model output becomes one atomic batch of
// chronicle.entry events with names resolved to indices and threads slugified.
func TestRunNarrationLands(t *testing.T) {
	md, social, model := narrMind(t)
	model.narrReply = `{"entries":[
		{"text":"Ash starved before the first fire was lit.","thread":"The Cold Start!","agents":["Ash","Nobody"]},
		{"text":"Birch and Sage argued over firewood.","thread":"firewood","agents":["Birch","Sage"]}]}`

	md.runNarration(narrJob{day: 1, label: "day 1, dawn to nightfall", fromTick: 1000, toTick: 57600,
		lines: []string{"[day 1 06:16] Ash died of starvation."}})

	if len(social.batches) != 1 {
		t.Fatalf("batches = %d, want 1 atomic injection", len(social.batches))
	}
	batch := social.batches[0]
	if len(batch) != 2 {
		t.Fatalf("entries = %d, want 2", len(batch))
	}
	var p sim.ChronicleEntryPayload
	if err := json.Unmarshal(batch[0].Payload, &p); err != nil || batch[0].Type != "chronicle.entry" {
		t.Fatalf("bad event: %s %v", batch[0].Type, err)
	}
	if p.Day != 1 || p.FromTick != 1000 || p.ToTick != 57600 {
		t.Errorf("window fields: %+v", p)
	}
	if p.Thread != "the-cold-start" {
		t.Errorf("thread not slugified: %q", p.Thread)
	}
	if len(p.Agents) != 1 || p.Agents[0] != 0 {
		t.Errorf("unknown names must drop, Ash resolves to 0: %v", p.Agents)
	}
}

// TestNarrationFailureCarries: a transport failure injects nothing and
// carries the chapter's lines into the next close; bad output is a gap.
func TestNarrationFailureCarries(t *testing.T) {
	md, social, model := narrMind(t)

	// narrReply empty → ErrTierDown from the mock.
	md.runNarration(narrJob{day: 1, label: "day 1", fromTick: 1000, toTick: 57600,
		lines: []string{"[day 1 06:16] Ash died of starvation."}})
	if len(social.batches) != 0 {
		t.Fatal("failed call must inject nothing")
	}

	// The next boundary merges the carried lines ahead of new ones.
	md.chronicleNote(mustEvent(t, 60000, "gru.emerged", sim.GruEmergedPayload{Night: 1, X: 5, Y: 5}))
	md.chronicleNote(mustEvent(t, 86400, "sim.day_started", sim.DayPayload{Day: 2}))
	var job narrJob
	select {
	case job = <-md.narrQ:
	default:
		t.Fatal("carry chapter did not close")
	}
	if len(job.lines) != 2 || !strings.Contains(job.lines[0], "Ash died") ||
		!strings.Contains(job.lines[1], "gru emerged") {
		t.Fatalf("carry order wrong: %v", job.lines)
	}
	if job.fromTick != 1000 {
		t.Errorf("carry fromTick = %d, want 1000", job.fromTick)
	}

	// Unusable output: a gap — no injection, no carry.
	model.narrReply = "I cannot write JSON today."
	md.runNarration(job)
	if len(social.batches) != 0 {
		t.Fatal("unusable output must inject nothing")
	}
	select {
	case <-md.narrRetry:
		t.Fatal("unusable output must not carry (it would loop forever)")
	default:
	}
}

// TestParseNarration covers the output contract: caps, empties, slugs.
func TestParseNarration(t *testing.T) {
	entries, err := parseNarration(`Here you go: {"entries":[
		{"text":"  A story.  ","thread":"Gru Attacks","agents":["Ash"]},
		{"text":"","thread":"empty-drops"},
		{"text":"B","thread":"b"},{"text":"C","thread":"c"},{"text":"D","thread":"d"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != narrMaxEntries {
		t.Fatalf("entries = %d, want cap %d", len(entries), narrMaxEntries)
	}
	if entries[0].Text != "A story." || entries[0].Thread != "gru-attacks" {
		t.Errorf("entry 0: %+v", entries[0])
	}
	for _, bad := range []string{"", "no json", `{"entries":[]}`, `{"entries":[{"text":"  "}]}`} {
		if _, err := parseNarration(bad); err == nil {
			t.Errorf("parseNarration(%q) should fail", bad)
		}
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"The Gru!":                           "the-gru",
		"  food  chain ":                     "food-chain",
		"":                                   "village",
		"???":                                "village",
		"a-very-long-thread-name-beyond-cap": "a-very-long-thread-name",
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestChronicleLandsThroughTheDoor is the integration proof: on a real loop
// at max speed, musings supply notable lines, the night boundary triggers the
// narrator, and chronicle.entry events land in the store and reduce into the
// State ring (AC#1's substrate).
func TestChronicleLandsThroughTheDoor(t *testing.T) {
	h := newHarness(t, `{"goal": "wander", "reason": "Stretching my legs."}`)
	h.model.mu.Lock()
	h.model.musingReply = "The woods feel patient today."
	h.model.narrReply = `{"entries":[{"text":"The village wandered and mused while the light lasted.","thread":"first-days","agents":["Ash"]}]}`
	h.model.mu.Unlock()

	entries := h.waitEvents(t, 30*time.Second, func(e store.Event) bool {
		return e.Type == "chronicle.entry"
	})
	if len(entries) == 0 {
		t.Fatal("no chronicle.entry landed through the injection door")
	}
	var p sim.ChronicleEntryPayload
	if err := json.Unmarshal(entries[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Text == "" || p.Thread != "first-days" || p.Day == 0 {
		t.Errorf("entry payload: %+v", p)
	}

	// The reducer folds it into the ring a fresh replica would receive.
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	evs, err := h.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		state.Apply(e)
	}
	if len(state.Chronicle) == 0 {
		t.Fatal("replayed state has an empty chronicle ring")
	}
}

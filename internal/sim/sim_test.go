package sim

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

func testMap(seed uint64) *worldmap.Map { return worldmap.Generate(seed, 64, 64) }

// driveTicks advances a state tick by tick exactly as the live loop does,
// injecting the given commands at their scheduled ticks (tick boundaries),
// and returns every event produced. This is the loop's semantics minus the
// real-time scheduler — determinism must hold at this layer (SC-006).
func driveTicks(t *testing.T, s *State, m *worldmap.Map, ticks int64, commands map[int64][]store.Event) []store.Event {
	t.Helper()
	var log []store.Event
	apply := func(evs []store.Event) {
		for _, e := range evs {
			if err := s.Apply(e); err != nil {
				t.Fatalf("apply %s at tick %d: %v", e.Type, s.Tick, err)
			}
			log = append(log, e)
		}
	}
	for s.Tick < ticks {
		apply(commands[s.Tick]) // commands land at the boundary before the tick
		next := s.Tick + 1
		evs := stepEvents(s, m, next)
		s.Tick = next
		apply(evs)
	}
	return log
}

func commandTimeline() map[int64][]store.Event {
	pl := func(v any) json.RawMessage { return mustPayload(v) }
	return map[int64][]store.Event{
		500:  {{Tick: 500, Type: "clock.speed_set", Payload: pl(SpeedSetPayload{Speed: clock.Speed16x})}},
		1200: {{Tick: 1200, Type: "clock.paused", Payload: pl(struct{}{})}, {Tick: 1200, Type: "clock.resumed", Payload: pl(struct{}{})}},
		7000: {{Tick: 7000, Type: "clock.speed_set", Payload: pl(SpeedSetPayload{Speed: clock.SpeedMax})}},
	}
}

func canonicalLog(t *testing.T, log []store.Event) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, e := range log {
		buf.WriteString(e.Type)
		buf.WriteByte(' ')
		b, err := json.Marshal(struct {
			Tick    int64           `json:"tick"`
			Payload json.RawMessage `json:"payload"`
		}{e.Tick, e.Payload})
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func TestDeterminismSameSeedSameTimeline(t *testing.T) {
	const seed, ticks = 7, 30_000
	m := testMap(seed)
	a, b := NewState(seed, m), NewState(seed, m)
	logA := driveTicks(t, a, m, ticks, commandTimeline())
	logB := driveTicks(t, b, m, ticks, commandTimeline())

	if len(logA) == 0 {
		t.Fatal("30k executor ticks should produce events")
	}
	if !bytes.Equal(canonicalLog(t, logA), canonicalLog(t, logB)) {
		t.Fatal("same seed + same command timeline produced different event sequences")
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("state hashes diverged: %s vs %s", a.Hash(), b.Hash())
	}
}

func TestDifferentSeedsDiverge(t *testing.T) {
	a, b := NewState(1, testMap(1)), NewState(2, testMap(2))
	logA := driveTicks(t, a, testMap(1), 5_000, nil)
	logB := driveTicks(t, b, testMap(2), 5_000, nil)
	if bytes.Equal(canonicalLog(t, logA), canonicalLog(t, logB)) {
		t.Fatal("different seeds produced identical histories — RNG not seed-dependent")
	}
}

// TestReplayRebuildsState is the recovery contract: reducing the logged
// events over genesis, then aligning the clock (recovery sets Tick =
// max(snapshot tick, last event tick) and re-lives quiet trailing ticks),
// must land on the exact live state.
func TestReplayRebuildsState(t *testing.T) {
	const seed, ticks = 99, 40_000
	m := testMap(seed)
	live := NewState(seed, m)
	log := driveTicks(t, live, m, ticks, commandTimeline())

	replayed := NewState(seed, m)
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	// Re-live the quiet tail deterministically, as recovery does.
	driveTicks(t, replayed, m, ticks, nil)

	if live.Hash() != replayed.Hash() {
		t.Fatalf("replayed state diverged from live state:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replayed.Marshal()))
	}
}

// TestMultiStepIntentExecution is AC#1: with zero external input, an agent
// forms an intent, walks a multi-tile path, works, and completes — the whole
// chain visible in the log.
func TestMultiStepIntentExecution(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	log := driveTicks(t, s, m, 8*3600, nil) // one working morning

	type counts struct{ intents, moves, completions int }
	perAgent := map[int]counts{}
	completionTypes := map[string]bool{
		"agent.foraged": true, "agent.chopped": true, "agent.hunted": true,
		"agent.built": true, "agent.intent_done": true, "agent.slept": true,
	}
	for _, e := range log {
		var p struct {
			Agent int `json:"agent"`
		}
		json.Unmarshal(e.Payload, &p)
		c := perAgent[p.Agent]
		switch {
		case e.Type == "agent.intent_set":
			c.intents++
		case e.Type == "agent.moved":
			c.moves++
		case completionTypes[e.Type]:
			c.completions++
		}
		perAgent[p.Agent] = c
	}
	for i := 0; i < agentCount; i++ {
		c := perAgent[i]
		if c.intents == 0 || c.moves == 0 || c.completions == 0 {
			t.Errorf("agent %d never ran a full intent chain: %+v", i, c)
		}
	}
	var worked bool
	for _, e := range log {
		if e.Type == "agent.foraged" || e.Type == "agent.chopped" {
			worked = true
			break
		}
	}
	if !worked {
		t.Error("no resource work in a full morning")
	}
}

// TestNeedsDecayAndSatisfaction is AC#2 (satisfiable half): needs fall over
// time and agents refill them from world resources unattended.
func TestNeedsDecayAndSatisfaction(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	log := driveTicks(t, s, m, 12*3600, nil)

	var ate, gathered, needsChanged bool
	for _, e := range log {
		switch e.Type {
		case "agent.ate":
			ate = true
		case "agent.foraged", "agent.hunted":
			gathered = true
		case "agent.needs_changed":
			needsChanged = true
		}
	}
	if !needsChanged {
		t.Fatal("needs never decayed")
	}
	if !gathered || !ate {
		t.Errorf("agents did not feed themselves from the world: gathered=%v ate=%v", gathered, ate)
	}
	for i, a := range s.Agents {
		if a.Dead {
			t.Errorf("agent %d (%s) died within 12h on a resource-rich map", i, a.Name)
		}
	}
}

// TestStarvationDeath is AC#2 (lethal half): zero food and failing health
// kill, with the cause recorded, and the dead stop acting.
func TestStarvationDeath(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	for i := range s.Agents {
		s.Agents[i].Needs.Food = 0
		s.Agents[i].Needs.Health = 3 // one heartbeat from death
	}
	log := driveTicks(t, s, m, 120, nil)

	died := 0
	for _, e := range log {
		if e.Type == "agent.died" {
			var p DiedPayload
			json.Unmarshal(e.Payload, &p)
			if p.Cause != "starvation" {
				t.Errorf("cause = %q, want starvation", p.Cause)
			}
			died++
		}
	}
	if died != agentCount {
		t.Fatalf("%d/%d agents died of starvation", died, agentCount)
	}
	for _, a := range s.Agents {
		if !a.Dead {
			t.Error("state should mark dead agents Dead")
		}
	}
	tail := driveTicks(t, s, m, 240, nil)
	for _, e := range tail {
		if e.Type == "agent.intent_set" || e.Type == "agent.moved" {
			t.Fatalf("dead agent still acting: %s", e.Type)
		}
	}
}

// TestNightWarmthMechanics is AC#3: night is mechanically distinct — cold
// drains warmth outdoors, fire restores it, day does not, and exposure kills.
func TestNightWarmthMechanics(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	nightTick := int64(16 * 3600) // 22:00 day 1

	s := NewState(seed, m)
	a0 := s.Agents[0]

	// The decay mechanic in isolation.
	coldNight := decayNeeds(a0.Needs, false, true, false)
	day := decayNeeds(a0.Needs, false, false, false)
	if coldNight.Warmth >= a0.Needs.Warmth {
		t.Error("night outdoors should drain warmth")
	}
	if day.Warmth < a0.Needs.Warmth {
		t.Error("daytime should not drain warmth")
	}

	// Fire warmth via world state.
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: a0.X + 1, Y: a0.Y})
	if !warmAt(s, a0.X, a0.Y) {
		t.Fatal("agent beside a fire should be warm")
	}
	byFire := decayNeeds(Needs{Health: 1000, Food: 500, Rest: 500, Warmth: 500, Morale: 500},
		false, true, warmAt(s, a0.X, a0.Y))
	if byFire.Warmth <= 500 {
		t.Error("fire should restore warmth at night")
	}

	// Exposure death end-to-end.
	freezing := NewState(seed, m)
	freezing.Tick = nightTick
	freezing.Night = true
	for i := range freezing.Agents {
		freezing.Agents[i].Needs.Warmth = 0
		freezing.Agents[i].Needs.Health = 3
		freezing.Agents[i].Needs.Food = 500 // isolate the exposure cause
	}
	log := driveTicks(t, freezing, m, nightTick+120, nil)
	var exposed bool
	for _, e := range log {
		if e.Type == "agent.died" {
			var p DiedPayload
			json.Unmarshal(e.Payload, &p)
			if p.Cause == "exposure" {
				exposed = true
			}
		}
	}
	if !exposed {
		t.Error("no exposure death despite zero warmth and critical health")
	}
}

// TestVillageSurvivesTwoDays: the reflex policy keeps everyone alive through
// two full day/night cycles on a fresh map — fire built, food found.
func TestVillageSurvivesTwoDays(t *testing.T) {
	if testing.Short() {
		t.Skip("two full game days")
	}
	for _, seed := range []uint64{42, 7} {
		m := testMap(seed)
		s := NewState(seed, m)
		log := driveTicks(t, s, m, 2*24*3600, nil)

		var fires int
		for _, e := range log {
			if e.Type == "agent.built" {
				var p BuiltPayload
				json.Unmarshal(e.Payload, &p)
				if p.Kind == "fire" {
					fires++
				}
			}
		}
		if fires == 0 {
			t.Errorf("seed %d: no fire built before the cold got them", seed)
		}
		alive := 0
		for _, a := range s.Agents {
			if !a.Dead {
				alive++
			}
		}
		if alive != agentCount {
			t.Errorf("seed %d: only %d/%d agents survived two days", seed, alive, agentCount)
		}
	}
}

func TestApplyClockEvents(t *testing.T) {
	s := NewState(1, testMap(1))
	if err := s.Apply(store.Event{Type: "clock.paused", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if !s.Paused {
		t.Error("clock.paused should set Paused")
	}
	if err := s.Apply(store.Event{Type: "clock.speed_set", Payload: mustPayload(SpeedSetPayload{Speed: clock.Speed1x})}); err != nil {
		t.Fatal(err)
	}
	if s.Speed != clock.Speed1x {
		t.Error("clock.speed_set should set Speed")
	}
	if err := s.Apply(store.Event{Type: "clock.resumed", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if s.Paused {
		t.Error("clock.resumed should clear Paused")
	}
	// Unknown types are recorded history but state no-ops.
	if err := s.Apply(store.Event{Type: "daemon.started", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Errorf("daemon.* events must be no-op, got %v", err)
	}
}

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
	const seed, ticks = 7, 10_000
	m := testMap(seed)
	a, b := NewState(seed, m), NewState(seed, m)
	logA := driveTicks(t, a, m, ticks, commandTimeline())
	logB := driveTicks(t, b, m, ticks, commandTimeline())

	if len(logA) == 0 {
		t.Fatal("10k ticks should produce events (minute moves, night at tick 57600? no — moves every 60)")
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
	const seed, ticks = 99, 20_000
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

func TestDayNightCycle(t *testing.T) {
	s := NewState(3, testMap(3))
	// Run through night start (tick 16h) and next day start (tick 24h).
	log := driveTicks(t, s, testMap(3), 24*3600+60, nil)

	var sawNight, sawDay, sleptDuringNight bool
	for _, e := range log {
		switch e.Type {
		case "sim.night_started":
			sawNight = true
		case "sim.day_started":
			sawDay = true
		case "agent.moved":
			if sec := clock.SecondOfDay(e.Tick); sec >= 22*3600 || sec < 6*3600 {
				t.Errorf("agent moved at night (tick %d, second-of-day %d)", e.Tick, sec)
			}
		case "agent.slept":
			sleptDuringNight = true
		}
	}
	if !sawNight || !sawDay || !sleptDuringNight {
		t.Errorf("cycle incomplete: night=%v day=%v slept=%v", sawNight, sawDay, sleptDuringNight)
	}
	if s.Night {
		t.Error("state should be daytime after 06:00 day start")
	}
	for i, w := range s.Wanderers {
		if w.Asleep {
			t.Errorf("wanderer %d still asleep after day start", i)
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

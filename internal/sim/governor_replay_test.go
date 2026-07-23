package sim

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
)

// governorTimeline scripts a governed run: the player asks 32x, the governor
// sheds twice under load, a pause/resume happens mid-governed, then it recovers
// notch-by-notch back to the 32x ceiling (leaving governed state). Applied at
// tick boundaries exactly as the loop's govern command and replay do.
func governorTimeline() map[int64][]store.Event {
	pl := func(v any) json.RawMessage { return mustPayload(v) }
	gov := func(typ string, req, from, to clock.Speed, debt float64, jobs int) store.Event {
		return store.Event{Type: typ, Payload: pl(GovernorPayload{Requested: req, From: from, To: to, Debt: debt, Jobs: jobs})}
	}
	stamp := func(tick int64, e store.Event) store.Event { e.Tick = tick; return e }
	return map[int64][]store.Event{
		500:  {{Tick: 500, Type: "clock.speed_set", Payload: pl(SpeedSetPayload{Speed: clock.Speed32x})}},
		1000: {stamp(1000, gov("clock.governor_shed", clock.Speed32x, clock.Speed32x, clock.Speed16x, 1.8, 3))},
		2000: {stamp(2000, gov("clock.governor_shed", clock.Speed32x, clock.Speed16x, clock.Speed8x, 1.5, 2))},
		2500: {
			{Tick: 2500, Type: "clock.paused", Payload: pl(struct{}{})},
			{Tick: 2500, Type: "clock.resumed", Payload: pl(struct{}{})},
		},
		3000: {stamp(3000, gov("clock.governor_recovered", clock.Speed32x, clock.Speed8x, clock.Speed16x, 0.3, 1))},
		4000: {stamp(4000, gov("clock.governor_recovered", clock.Speed32x, clock.Speed16x, clock.Speed32x, 0.1, 0))},
	}
}

// TestGovernorReplayByteIdentical (spec 028 SC-001, FR-014): a run containing
// governor sheds, recoveries, and a mid-governed pause replays byte-identically
// from genesis — the reducer re-applies the recorded governor events verbatim
// and never re-derives debt, so the governed state is a pure function of the log.
func TestGovernorReplayByteIdentical(t *testing.T) {
	const seed, ticks = 123, 6000
	m := testMap(seed)

	live := NewState(seed, m)
	log := driveTicks(t, live, m, ticks, governorTimeline())

	// Guard: the log must actually carry both governor event types, or the test
	// proves nothing.
	var sheds, recovers int
	for _, e := range log {
		switch e.Type {
		case "clock.governor_shed":
			sheds++
		case "clock.governor_recovered":
			recovers++
		}
	}
	if sheds != 2 || recovers != 2 {
		t.Fatalf("timeline carried sheds=%d recovers=%d, want 2 and 2", sheds, recovers)
	}

	// Replay from genesis: reduce the logged events, align the clock, re-live the
	// quiet tail — exactly the recovery contract (mirrors TestReplayRebuildsState).
	replayed := NewState(seed, m)
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil)

	if live.Hash() != replayed.Hash() {
		t.Fatalf("governed replay diverged from live:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replayed.Marshal()))
	}
	// The final recovery reached the 32x ceiling, so governed state cleared.
	if live.Speed != clock.Speed32x {
		t.Errorf("final Speed = %q, want 32x", live.Speed)
	}
	if live.RequestedSpeed != "" {
		t.Errorf("final RequestedSpeed = %q, want empty (recovered to the ceiling)", live.RequestedSpeed)
	}
}

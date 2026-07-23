package sim

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
)

// newGovernHarness builds a Loop over a real store whose goroutine is NOT
// running: handleCommand is invoked directly so the govern command's boundary
// validation and emission are exercised deterministically — no timers, no ticks.
// The state is unpaused at the chosen effective speed unless mutate says otherwise.
func newGovernHarness(t *testing.T, speed clock.Speed, mutate func(*State)) *Loop {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := testMap(7)
	s := NewState(7, m)
	s.Speed = speed
	s.EffectiveRate = speed.TicksPerSecond()
	if mutate != nil {
		mutate(s)
	}
	return NewLoop(s, m, st, nil)
}

// runGovern issues one govern command straight through handleCommand and returns
// the resulting status; a transport error (never expected for a well-formed
// drop or emit) fails the test.
func runGovern(t *testing.T, l *Loop, to clock.Speed, debt float64, jobs int) Status {
	t.Helper()
	cmd := command{name: "govern", govern: &governArgs{to: to, debt: debt, jobs: jobs}, reply: make(chan commandResult, 1)}
	if err := l.handleCommand(cmd); err != nil {
		t.Fatalf("handleCommand(govern -> %s): %v", to, err)
	}
	res := <-cmd.reply
	if res.err != nil {
		t.Fatalf("govern -> %s returned err: %v", to, res.err)
	}
	return res.status
}

// governorEvents pulls every clock.governor_* event out of the store in log order.
func governorEvents(t *testing.T, l *Loop) []store.Event {
	t.Helper()
	all, err := l.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Event
	for _, e := range all {
		if e.Type == "clock.governor_shed" || e.Type == "clock.governor_recovered" {
			out = append(out, e)
		}
	}
	return out
}

func decodeGovernor(t *testing.T, e store.Event) GovernorPayload {
	t.Helper()
	var p GovernorPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatalf("decode %s payload: %v", e.Type, err)
	}
	return p
}

// TestGovernShedEmitsAndPaces (spec 028 US2-AC1): a valid one-notch shed records
// a clock.governor_shed carrying the full arithmetic, drops the effective Speed
// (which is what Run paces against — the same state.Speed → Interval path a
// set_speed takes, so pacing follows immediately), and records the ceiling.
func TestGovernShedEmitsAndPaces(t *testing.T) {
	l := newGovernHarness(t, clock.Speed32x, nil)

	st := runGovern(t, l, clock.Speed16x, 1.4, 3)

	if l.state.Speed != clock.Speed16x {
		t.Errorf("effective Speed = %q, want 16x (pacing follows the governed speed)", l.state.Speed)
	}
	if l.state.RequestedSpeed != clock.Speed32x {
		t.Errorf("RequestedSpeed = %q, want 32x (ceiling recorded on first shed)", l.state.RequestedSpeed)
	}
	if l.state.EffectiveRate != clock.Speed16x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v", l.state.EffectiveRate, clock.Speed16x.TicksPerSecond())
	}
	if st.Speed != clock.Speed16x || st.RequestedSpeed != clock.Speed32x {
		t.Errorf("status = {Speed:%q Requested:%q}, want {16x 32x}", st.Speed, st.RequestedSpeed)
	}

	evs := governorEvents(t, l)
	if len(evs) != 1 || evs[0].Type != "clock.governor_shed" {
		t.Fatalf("governor events = %+v, want exactly one clock.governor_shed", evs)
	}
	p := decodeGovernor(t, evs[0])
	want := GovernorPayload{Requested: clock.Speed32x, From: clock.Speed32x, To: clock.Speed16x, Debt: 1.4, Jobs: 3}
	if p != want {
		t.Errorf("shed payload = %+v, want %+v", p, want)
	}
}

// TestGovernStaleDecisionDrops (spec 028 edge case "governor events racing a
// player speed change"): a decision whose target equals the current speed (the
// speed already moved) emits nothing and leaves state untouched.
func TestGovernStaleDecisionDrops(t *testing.T) {
	l := newGovernHarness(t, clock.Speed16x, nil)
	before := l.state.Marshal()

	runGovern(t, l, clock.Speed16x, 2.0, 4) // decided shed to 16x, but speed is already 16x

	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("stale decision emitted events: %+v", evs)
	}
	if got := l.state.Marshal(); string(got) != string(before) {
		t.Errorf("stale decision mutated state")
	}
}

// TestGovernPausedDrops (spec 028 FR-013): the governor never acts on a paused
// world — a shed decision while paused emits nothing and changes nothing.
func TestGovernPausedDrops(t *testing.T) {
	l := newGovernHarness(t, clock.Speed32x, func(s *State) { s.Paused = true })
	before := l.state.Marshal()

	runGovern(t, l, clock.Speed16x, 1.4, 3)

	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("paused world emitted governor events: %+v", evs)
	}
	if got := l.state.Marshal(); string(got) != string(before) {
		t.Errorf("govern mutated a paused world")
	}
}

// TestGovernTwoNotchDrops (contracts/internal-api.md): a decision that is not
// exactly one ladder notch from the current speed is a stale multi-notch jump —
// dropped silently.
func TestGovernTwoNotchDrops(t *testing.T) {
	l := newGovernHarness(t, clock.Speed32x, nil)

	runGovern(t, l, clock.Speed8x, 3.0, 5) // 32x -> 8x is two notches

	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("two-notch decision emitted events: %+v", evs)
	}
	if l.state.Speed != clock.Speed32x {
		t.Errorf("Speed = %q, want unchanged 32x", l.state.Speed)
	}
}

// TestGovernOffLadderDrops (spec 028 FR-004/FR-012): the governor never touches
// uncapped max — a govern targeting max, or from a max world, is dropped.
func TestGovernOffLadderDrops(t *testing.T) {
	l := newGovernHarness(t, clock.Speed32x, nil)
	runGovern(t, l, clock.SpeedMax, 1.4, 3) // target off the capped ladder
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("govern to max emitted events: %+v", evs)
	}

	lm := newGovernHarness(t, clock.SpeedMax, nil)
	runGovern(t, lm, clock.Speed32x, 1.4, 3) // current off the capped ladder
	if evs := governorEvents(t, lm); len(evs) != 0 {
		t.Errorf("govern from max emitted events: %+v", evs)
	}
}

// TestGovernRecoverAboveCeilingDrops (contracts/internal-api.md): a recover that
// would climb above the player's ceiling is dropped, but a recover up to the
// ceiling emits and clears governed state.
func TestGovernRecoverAboveCeilingDrops(t *testing.T) {
	// Ungoverned at 16x: the ceiling defaults to 16x, so a recover to 32x is
	// above the ceiling and dropped.
	l := newGovernHarness(t, clock.Speed16x, nil)
	runGovern(t, l, clock.Speed32x, 0.1, 0)
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("recover above ceiling emitted events: %+v", evs)
	}
	if l.state.Speed != clock.Speed16x {
		t.Errorf("Speed = %q, want unchanged 16x", l.state.Speed)
	}

	// Governed at 8x with a 16x ceiling: a recover to 16x is allowed and, since
	// it reaches the ceiling, clears governed state.
	g := newGovernHarness(t, clock.Speed8x, func(s *State) { s.RequestedSpeed = clock.Speed16x })
	runGovern(t, g, clock.Speed16x, 0.2, 1)
	evs := governorEvents(t, g)
	if len(evs) != 1 || evs[0].Type != "clock.governor_recovered" {
		t.Fatalf("governor events = %+v, want one clock.governor_recovered", evs)
	}
	if g.state.Speed != clock.Speed16x {
		t.Errorf("Speed = %q, want 16x", g.state.Speed)
	}
	if g.state.RequestedSpeed != "" {
		t.Errorf("RequestedSpeed = %q, want empty (recovered to the ceiling)", g.state.RequestedSpeed)
	}
	p := decodeGovernor(t, evs[0])
	want := GovernorPayload{Requested: clock.Speed16x, From: clock.Speed8x, To: clock.Speed16x, Debt: 0.2, Jobs: 1}
	if p != want {
		t.Errorf("recover payload = %+v, want %+v", p, want)
	}
}

// TestGovernSequenceLandsInStore (spec 028 SC-005): consecutive one-notch sheds
// record an ordered event sequence the log alone can reconstruct.
func TestGovernSequenceLandsInStore(t *testing.T) {
	l := newGovernHarness(t, clock.Speed32x, nil)

	runGovern(t, l, clock.Speed16x, 2.1, 3) // 32x -> 16x
	runGovern(t, l, clock.Speed8x, 1.8, 2)  // 16x -> 8x

	evs := governorEvents(t, l)
	if len(evs) != 2 {
		t.Fatalf("governor events = %d, want 2", len(evs))
	}
	first, second := decodeGovernor(t, evs[0]), decodeGovernor(t, evs[1])
	if first.From != clock.Speed32x || first.To != clock.Speed16x || first.Requested != clock.Speed32x {
		t.Errorf("first shed = %+v, want 32x->16x ceiling 32x", first)
	}
	// The ceiling stands across the second shed: still the player's 32x request.
	if second.From != clock.Speed16x || second.To != clock.Speed8x || second.Requested != clock.Speed32x {
		t.Errorf("second shed = %+v, want 16x->8x ceiling 32x", second)
	}
	if l.state.Speed != clock.Speed8x || l.state.RequestedSpeed != clock.Speed32x {
		t.Errorf("final state = {Speed:%q Requested:%q}, want {8x 32x}", l.state.Speed, l.state.RequestedSpeed)
	}
}

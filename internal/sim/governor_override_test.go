package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
)

// runClock issues a plain clock command (set_speed, pause, resume) straight
// through handleCommand and returns the resulting status — the player-command
// analogue of runGovern, exercising the same tick-boundary reducer path a live
// loop takes without any timers.
func runClock(t *testing.T, l *Loop, name string, speed clock.Speed) Status {
	t.Helper()
	cmd := command{name: name, speed: speed, reply: make(chan commandResult, 1)}
	if err := l.handleCommand(cmd); err != nil {
		t.Fatalf("handleCommand(%s %s): %v", name, speed, err)
	}
	res := <-cmd.reply
	if res.err != nil {
		t.Fatalf("%s %s returned err: %v", name, speed, res.err)
	}
	return res.status
}

// speedEvents pulls every clock.speed_set event out of the store in log order.
func speedEvents(t *testing.T, l *Loop) []store.Event {
	t.Helper()
	all, err := l.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Event
	for _, e := range all {
		if e.Type == "clock.speed_set" {
			out = append(out, e)
		}
	}
	return out
}

// TestPlayerOverrideBelowGovernedNotchClears (spec 028 US4-AC2, FR-009): a world
// governed to 8x with a 32x ceiling receives a player set_speed 4x through the
// real loop command path. The single applied clock.speed_set drops the effective
// Speed to 4x AND clears the governed ceiling in the same reducer step, and a
// governor decision still targeting the old ladder position (the shed to 4x it
// sampled while the world sat at 8x) is now stale and drops silently.
func TestPlayerOverrideBelowGovernedNotchClears(t *testing.T) {
	// Governed at 8x, player asked 32x — the state a shed sequence leaves behind.
	l := newGovernHarness(t, clock.Speed8x, func(s *State) { s.RequestedSpeed = clock.Speed32x })

	st := runClock(t, l, "set_speed", clock.Speed4x)

	// The override runs at 4x at once and clears governed state — one command,
	// one applied event, both facts true together (FR-009).
	if l.state.Speed != clock.Speed4x {
		t.Errorf("effective Speed = %q, want 4x (override runs immediately)", l.state.Speed)
	}
	if l.state.RequestedSpeed != "" {
		t.Errorf("RequestedSpeed = %q, want empty (player command collapses governed state)", l.state.RequestedSpeed)
	}
	if l.state.EffectiveRate != clock.Speed4x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v", l.state.EffectiveRate, clock.Speed4x.TicksPerSecond())
	}
	if st.Speed != clock.Speed4x || st.RequestedSpeed != "" {
		t.Errorf("status = {Speed:%q Requested:%q}, want {4x <empty>}", st.Speed, st.RequestedSpeed)
	}

	// Exactly one clock.speed_set landed (the override), and no governor event.
	if evs := speedEvents(t, l); len(evs) != 1 {
		t.Fatalf("clock.speed_set events = %d, want exactly 1 (the override)", len(evs))
	}
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Fatalf("override emitted governor events: %+v", evs)
	}

	// A governor decision sampled against the OLD 8x notch now targets 4x — the
	// speed already there — so it is stale and drops silently, changing nothing.
	before := l.state.Marshal()
	runGovern(t, l, clock.Speed4x, 2.0, 3) // the shed 8x->4x it decided pre-override
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("stale post-override shed emitted events: %+v", evs)
	}
	if string(l.state.Marshal()) != string(before) {
		t.Errorf("stale post-override shed mutated state")
	}

	// A stale recover that would climb to the old 16x notch is two notches from
	// 4x now, so it too drops silently.
	runGovern(t, l, clock.Speed16x, 0.2, 1)
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Errorf("stale post-override recover emitted events: %+v", evs)
	}
	if l.state.Speed != clock.Speed4x {
		t.Errorf("Speed = %q after stale decisions, want unchanged 4x", l.state.Speed)
	}
}

// TestPlayerRaisesCeilingCollapsesGoverned (spec 028 US4-AC3, FR-009): a governed
// world (8x, ceiling 32x) receives a player set_speed to a HIGHER speed than the
// governed notch. The request collapses governed state — the world runs at the
// new requested speed at once (ungoverned), leaving the sampler to re-evaluate on
// its normal cadence (proven at the daemon layer). One clock.speed_set applies;
// no governor event.
func TestPlayerRaisesCeilingCollapsesGoverned(t *testing.T) {
	l := newGovernHarness(t, clock.Speed8x, func(s *State) { s.RequestedSpeed = clock.Speed32x })

	st := runClock(t, l, "set_speed", clock.Speed16x)

	if l.state.Speed != clock.Speed16x {
		t.Errorf("effective Speed = %q, want 16x (raise runs immediately)", l.state.Speed)
	}
	if l.state.RequestedSpeed != "" {
		t.Errorf("RequestedSpeed = %q, want empty (the request collapses governed state)", l.state.RequestedSpeed)
	}
	if st.Speed != clock.Speed16x || st.RequestedSpeed != "" {
		t.Errorf("status = {Speed:%q Requested:%q}, want {16x <empty>}", st.Speed, st.RequestedSpeed)
	}
	if evs := speedEvents(t, l); len(evs) != 1 {
		t.Fatalf("clock.speed_set events = %d, want exactly 1", len(evs))
	}
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Fatalf("raise-ceiling emitted governor events: %+v", evs)
	}
}

// TestGovernPauseResumeSequencing (spec 028 US4-AC4, FR-013): the single ordered
// command path serializes pause, govern, and resume. A govern decision arriving
// while paused drops silently (nothing emitted, nothing changed); after a resume
// the very same decision applies as a fresh shed and emits. Proves the pause is a
// true suspension of governing, not a deferral — no accrued shed springs loose on
// resume, but a genuinely fresh decision lands normally.
func TestGovernPauseResumeSequencing(t *testing.T) {
	l := newGovernHarness(t, clock.Speed32x, func(s *State) { s.Paused = true })

	// Paused: the shed decision drops silently.
	before := l.state.Marshal()
	runGovern(t, l, clock.Speed16x, 1.8, 3)
	if evs := governorEvents(t, l); len(evs) != 0 {
		t.Fatalf("govern during pause emitted events: %+v", evs)
	}
	if string(l.state.Marshal()) != string(before) {
		t.Fatalf("govern during pause mutated state")
	}

	// Resume, then the fresh decision applies as a normal shed.
	runClock(t, l, "resume", "")
	if l.state.Paused {
		t.Fatalf("resume did not unpause")
	}
	runGovern(t, l, clock.Speed16x, 1.8, 3)

	evs := governorEvents(t, l)
	if len(evs) != 1 || evs[0].Type != "clock.governor_shed" {
		t.Fatalf("post-resume govern = %+v, want exactly one clock.governor_shed", evs)
	}
	if l.state.Speed != clock.Speed16x || l.state.RequestedSpeed != clock.Speed32x {
		t.Errorf("post-resume state = {Speed:%q Requested:%q}, want {16x 32x}", l.state.Speed, l.state.RequestedSpeed)
	}
	p := decodeGovernor(t, evs[0])
	want := GovernorPayload{Requested: clock.Speed32x, From: clock.Speed32x, To: clock.Speed16x, Debt: 1.8, Jobs: 3}
	if p != want {
		t.Errorf("post-resume shed payload = %+v, want %+v", p, want)
	}
}

package sim

import (
	"bytes"
	"testing"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
)

// governorEvent builds a reducer-applied governor event from a payload.
func governorEvent(typ string, p GovernorPayload) store.Event {
	return store.Event{Tick: 1, Type: typ, Payload: mustPayload(p)}
}

// TestGovernorShedReducer (spec 028 US2-AC1, contracts/events.md): applying a
// clock.governor_shed sets the effective Speed to `to`, records the player's
// ceiling in RequestedSpeed, and follows the new speed's tick rate.
func TestGovernorShedReducer(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Speed = clock.Speed32x
	s.EffectiveRate = clock.Speed32x.TicksPerSecond()

	ev := governorEvent("clock.governor_shed", GovernorPayload{
		Requested: clock.Speed32x, From: clock.Speed32x, To: clock.Speed16x, Debt: 1.4, Jobs: 3,
	})
	if err := s.Apply(ev); err != nil {
		t.Fatalf("apply governor_shed: %v", err)
	}
	if s.Speed != clock.Speed16x {
		t.Errorf("Speed = %q, want 16x (effective drops one notch)", s.Speed)
	}
	if s.RequestedSpeed != clock.Speed32x {
		t.Errorf("RequestedSpeed = %q, want 32x (the ceiling is preserved)", s.RequestedSpeed)
	}
	if s.EffectiveRate != clock.Speed16x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v", s.EffectiveRate, clock.Speed16x.TicksPerSecond())
	}
}

// TestGovernorShedDegradedKeepsRate (contracts/events.md, "unless Degraded"): a
// shed applied to a host already reporting a degraded pace updates Speed but
// leaves EffectiveRate under the auto-slow observer's control.
func TestGovernorShedDegradedKeepsRate(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Speed = clock.Speed32x
	s.Degraded = true
	s.EffectiveRate = 6.5 // the honest-slowdown observer's measured rate

	ev := governorEvent("clock.governor_shed", GovernorPayload{
		Requested: clock.Speed32x, From: clock.Speed32x, To: clock.Speed16x, Debt: 2.0, Jobs: 4,
	})
	if err := s.Apply(ev); err != nil {
		t.Fatalf("apply governor_shed: %v", err)
	}
	if s.Speed != clock.Speed16x {
		t.Errorf("Speed = %q, want 16x", s.Speed)
	}
	if s.RequestedSpeed != clock.Speed32x {
		t.Errorf("RequestedSpeed = %q, want 32x", s.RequestedSpeed)
	}
	if s.EffectiveRate != 6.5 {
		t.Errorf("EffectiveRate = %v, want 6.5 (degraded rate untouched by the governor)", s.EffectiveRate)
	}
}

// TestGovernorRecoveredReducer (spec 028 US3-AC1, contracts/events.md): a
// clock.governor_recovered raises Speed one notch; RequestedSpeed stays set
// while still below the ceiling and clears the instant it reaches the ceiling.
func TestGovernorRecoveredReducer(t *testing.T) {
	// Recover 8x -> 16x with a 32x ceiling: still governed, ceiling preserved.
	s := NewState(42, testMap(42))
	s.Speed = clock.Speed8x
	s.RequestedSpeed = clock.Speed32x
	s.EffectiveRate = clock.Speed8x.TicksPerSecond()
	if err := s.Apply(governorEvent("clock.governor_recovered", GovernorPayload{
		Requested: clock.Speed32x, From: clock.Speed8x, To: clock.Speed16x, Debt: 0.3, Jobs: 1,
	})); err != nil {
		t.Fatalf("apply governor_recovered: %v", err)
	}
	if s.Speed != clock.Speed16x {
		t.Errorf("Speed = %q, want 16x", s.Speed)
	}
	if s.RequestedSpeed != clock.Speed32x {
		t.Errorf("RequestedSpeed = %q, want 32x (still below the ceiling)", s.RequestedSpeed)
	}
	if s.EffectiveRate != clock.Speed16x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v", s.EffectiveRate, clock.Speed16x.TicksPerSecond())
	}

	// Recover 16x -> 32x reaching the 32x ceiling: governed state clears.
	if err := s.Apply(governorEvent("clock.governor_recovered", GovernorPayload{
		Requested: clock.Speed32x, From: clock.Speed16x, To: clock.Speed32x, Debt: 0.1, Jobs: 0,
	})); err != nil {
		t.Fatalf("apply governor_recovered (to ceiling): %v", err)
	}
	if s.Speed != clock.Speed32x {
		t.Errorf("Speed = %q, want 32x", s.Speed)
	}
	if s.RequestedSpeed != "" {
		t.Errorf("RequestedSpeed = %q, want empty (reached the ceiling, governed state cleared)", s.RequestedSpeed)
	}
	if s.EffectiveRate != clock.Speed32x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v", s.EffectiveRate, clock.Speed32x.TicksPerSecond())
	}
}

// TestSpeedSetClearsGovernedState (spec 028 FR-009): a player speed command
// collapses any standing governor ceiling — RequestedSpeed clears and the
// requested speed becomes both ceiling and effective.
func TestSpeedSetClearsGovernedState(t *testing.T) {
	s := NewState(42, testMap(42))
	s.Speed = clock.Speed8x
	s.RequestedSpeed = clock.Speed32x // governed: asked 32x, running 8x
	s.EffectiveRate = clock.Speed8x.TicksPerSecond()

	if err := s.Apply(store.Event{Tick: 1, Type: "clock.speed_set",
		Payload: mustPayload(SpeedSetPayload{Speed: clock.Speed4x})}); err != nil {
		t.Fatalf("apply speed_set: %v", err)
	}
	if s.Speed != clock.Speed4x {
		t.Errorf("Speed = %q, want 4x", s.Speed)
	}
	if s.RequestedSpeed != "" {
		t.Errorf("RequestedSpeed = %q, want empty (a player command clears governed state)", s.RequestedSpeed)
	}
	if s.EffectiveRate != clock.Speed4x.TicksPerSecond() {
		t.Errorf("EffectiveRate = %v, want %v", s.EffectiveRate, clock.Speed4x.TicksPerSecond())
	}
}

// TestUngovernedSnapshotOmitsRequestedSpeed (spec 028 R2/R3, SC-001): an
// ungoverned State marshals with NO requested_speed key, so every pre-028
// snapshot byte shape is preserved. A governed State does carry the key.
func TestUngovernedSnapshotOmitsRequestedSpeed(t *testing.T) {
	s := NewState(42, testMap(42))
	if got := s.Marshal(); bytes.Contains(got, []byte("requested_speed")) {
		t.Errorf("ungoverned snapshot leaked requested_speed key:\n%s", got)
	}

	s.RequestedSpeed = clock.Speed32x
	if got := s.Marshal(); !bytes.Contains(got, []byte(`"requested_speed":"32x"`)) {
		t.Errorf("governed snapshot missing requested_speed key:\n%s", got)
	}
}

// TestStructureHPOmitemptyStable (spec 032 T002, research R7): the additive
// Structure.HP field is omitempty, so a non-wall structure (or any pre-032
// structure, which never set HP) marshals with NO "hp" key — pre-032 snapshot
// bytes are unchanged. A standing wall (HP ≥ 1) does carry the key.
func TestStructureHPOmitemptyStable(t *testing.T) {
	s := NewState(42, testMap(42))
	// A pre-032-shaped structure (fire) never sets HP → the key must not appear.
	s.Structures = []Structure{{Kind: "fire", X: 1, Y: 1, FuelUntil: 8 * 3600}}
	if got := s.Marshal(); bytes.Contains(got, []byte(`"hp"`)) {
		t.Errorf("non-wall structure leaked an hp key:\n%s", got)
	}
	// A standing wall carries its current health.
	s.Structures = []Structure{{Kind: "wall_plank", X: 2, Y: 2, HP: wallPlankHP}}
	if got := s.Marshal(); !bytes.Contains(got, []byte(`"hp":200`)) {
		t.Errorf("wall snapshot missing hp key:\n%s", got)
	}
}

// TestAxesOmitemptyStable (spec 032 T011, research R7): Inventory.Axes and
// Pile.Axes are omitempty, so an agent or pile carrying no axes marshals with NO
// "axes" key — pre-032 snapshot bytes are unchanged. Carried axes do serialize,
// sorted ascending.
func TestAxesOmitemptyStable(t *testing.T) {
	s := NewState(42, testMap(42))
	if got := s.Marshal(); bytes.Contains(got, []byte(`"axes"`)) {
		t.Errorf("a fresh (axe-less) world leaked an axes key:\n%s", got)
	}
	// Carried axes serialize under the "axes" key.
	s.Agents[0].Inv.Axes = []int{3, 10}
	if got := s.Marshal(); !bytes.Contains(got, []byte(`"axes":[3,10]`)) {
		t.Errorf("inventory axes not serialized:\n%s", got)
	}
	// A pile carrying only axes is non-empty (empty() sees them).
	p := Pile{X: 1, Y: 1, Axes: []int{5}}
	if p.empty() {
		t.Error("a pile holding an axe must not report empty")
	}
	if got := s.Marshal(); !bytes.Contains(got, []byte(`"axes"`)) {
		t.Error("pile/inventory axes key missing after adding axes")
	}
}

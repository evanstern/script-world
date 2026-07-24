package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/worldmap"
)

// grassCorridor finds a horizontal run of at least n+1 consecutive grass tiles,
// returning the left end (x0,y). A straight grass corridor guarantees the BFS
// walks it in a straight line (Manhattan distance = n, no shorter route), so
// step timing is the only variable the path tests measure.
func grassCorridor(m *worldmap.Map, n int) (x0, y int, ok bool) {
	for yy := 0; yy < m.H; yy++ {
		run := 0
		for xx := 0; xx < m.W; xx++ {
			if m.At(xx, yy) == worldmap.Grass {
				run++
				if run >= n+1 {
					return xx - n, yy, true
				}
			} else {
				run = 0
			}
		}
	}
	return 0, 0, false
}

// traverseTicks drives agent 0 from the left end of a length-n corridor to the
// right end and returns the tick count to arrive. pavedTo paves the corridor
// tiles [x0, x0+pavedTo) as path structures (the step-FROM tiles that get the
// 2x phase-2 slot); pavedTo == 0 is fully unpaved.
func traverseTicks(t *testing.T, m *worldmap.Map, x0, y, n, pavedTo int) int64 {
	t.Helper()
	s := NewState(42, m)
	a := reviveAt(s, x0, y) // isolates the rest, revives agent 0 here
	for i := 0; i < pavedTo; i++ {
		s.Structures = append(s.Structures, Structure{Kind: "path", X: x0 + i, Y: y})
	}
	a.Intent = &Intent{Goal: "wander", TargetX: x0 + n, TargetY: y}
	start := s.Tick
	for s.Tick < start+2000 {
		next := s.Tick + 1
		evs := stepEvents(s, m, next)
		s.Tick = next
		for _, e := range evs {
			if err := s.Apply(e); err != nil {
				t.Fatalf("apply %s: %v", e.Type, err)
			}
		}
		if a.X == x0+n && a.Y == y {
			return s.Tick - start
		}
	}
	t.Fatalf("agent never crossed the %d-tile corridor", n)
	return 0
}

// TestPathDoublesTraversalSpeed is spec 032 US3 AC#2 / SC-003: a fully-paved
// corridor is crossed in half the ticks of the identical unpaved corridor
// (±1 movement step of rounding). Agent 0 (stagger index 0) makes the phase
// arithmetic clean: one step per 5-tick window off-path, two on-path.
func TestPathDoublesTraversalSpeed(t *testing.T) {
	const n = 8
	m := testMap(42)
	x0, y, ok := grassCorridor(m, n)
	if !ok {
		t.Skip("no straight grass corridor of the needed length on seed 42")
	}

	unpaved := traverseTicks(t, m, x0, y, n, 0)
	paved := traverseTicks(t, m, x0, y, n, n+1) // pave every step-from tile

	// One movement step is moveEveryTicks (5) ticks; SC-003 allows ±1 step.
	const tol = moveEveryTicks
	half := unpaved / 2
	if diff := paved - half; diff > tol || diff < -tol {
		t.Errorf("paved traversal = %d ticks, want ~half of unpaved %d (=%d, ±%d)", paved, unpaved, half, tol)
	}
	if paved >= unpaved {
		t.Errorf("paved (%d) must be strictly faster than unpaved (%d)", paved, unpaved)
	}
}

// TestPathPartialAcceleration is spec 032 US3 AC#3 / quickstart scenario 5: only
// steps taken FROM path tiles get the bonus, so a half-paved corridor lands
// strictly between the fully-paved and unpaved times.
func TestPathPartialAcceleration(t *testing.T) {
	const n = 8
	m := testMap(42)
	x0, y, ok := grassCorridor(m, n)
	if !ok {
		t.Skip("no straight grass corridor on seed 42")
	}
	unpaved := traverseTicks(t, m, x0, y, n, 0)
	paved := traverseTicks(t, m, x0, y, n, n+1)
	mixed := traverseTicks(t, m, x0, y, n, n/2) // pave only the first half

	if !(paved < mixed && mixed < unpaved) {
		t.Errorf("half-paved traversal (%d) should sit between paved (%d) and unpaved (%d)", mixed, paved, unpaved)
	}
}

// TestOffPathUnaffected is spec 032 US3: an agent whose route crosses no path
// moves at exactly the pre-032 baseline (one step per moveEveryTicks) — the
// dual-phase change is invisible off paths.
func TestOffPathUnaffected(t *testing.T) {
	const n = 6
	m := testMap(42)
	x0, y, ok := grassCorridor(m, n)
	if !ok {
		t.Skip("no straight grass corridor on seed 42")
	}
	// Agent 0 steps at phase 0 (nextTick%5==0) only: n steps land at 5,10,…,5n.
	unpaved := traverseTicks(t, m, x0, y, n, 0)
	if want := int64(moveEveryTicks * n); unpaved != want {
		t.Errorf("unpaved traversal = %d ticks, want %d (baseline one step / %d ticks)", unpaved, want, moveEveryTicks)
	}
}

// TestPathReplayDeterminism is SC-005 over the movement change: a run with paths
// laid replays to a byte-identical state hash (the dual-phase cadence is
// tick-pure and stateless).
func TestPathReplayDeterminism(t *testing.T) {
	const n = 6
	m := testMap(42)
	x0, y, ok := grassCorridor(m, n)
	if !ok {
		t.Skip("no straight grass corridor on seed 42")
	}
	genesis := func() *State {
		s := NewState(42, m)
		for i := 0; i <= n; i++ {
			s.Structures = append(s.Structures, Structure{Kind: "path", X: x0 + i, Y: y})
		}
		return s
	}
	const ticks = 4000
	live := genesis()
	log := driveTicks(t, live, m, ticks, nil)

	replay := genesis()
	for _, e := range log {
		if err := replay.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replay.Tick = e.Tick
	}
	driveTicks(t, replay, m, ticks, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("path replay diverged")
	}
}

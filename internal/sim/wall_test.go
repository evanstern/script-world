package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/worldmap"
)

// openBlock scans for a 3x3 fully base-passable block centered on (cx,cy) — a
// deterministic scenario setup for the wall pathing tests: with the center
// walled, a detour through the surrounding ring always exists.
func openBlock(m *worldmap.Map) (cx, cy int, ok bool) {
	for y := 1; y < m.H-1; y++ {
		for x := 1; x < m.W-1; x++ {
			all := true
			for dy := -1; dy <= 1 && all; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if !m.Passable(x+dx, y+dy) {
						all = false
						break
					}
				}
			}
			if all {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// TestWallBlocksAndReroutes is spec 032 US1 AC#2 / SC-002: a wall on the direct
// route makes its tile impassable, so nextStep detours around it and never
// steps onto the wall tile — while a route still exists.
func TestWallBlocksAndReroutes(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	cx, cy, ok := openBlock(m)
	if !ok {
		t.Fatal("no 3x3 open block found on seed 42")
	}
	// West-center → East-center, wall at the center: the direct step (the wall
	// tile) is barred, the detour through the ring is taken.
	fromX, fromY := cx-1, cy
	toX, toY := cx+1, cy

	// Without a wall the direct step is straight across (onto the center tile).
	if nx, ny := nextStep(m, s, fromX, fromY, toX, toY); nx != cx || ny != cy {
		t.Fatalf("baseline nextStep = (%d,%d), want the center tile (%d,%d)", nx, ny, cx, cy)
	}

	s.Structures = append(s.Structures, Structure{Kind: "wall_plank", X: cx, Y: cy, HP: wallPlankHP})
	if passable(m, s, cx, cy) {
		t.Fatal("a standing wall tile must be impassable")
	}
	nx, ny := nextStep(m, s, fromX, fromY, toX, toY)
	if nx == cx && ny == cy {
		t.Fatal("nextStep stepped onto the wall tile — it must detour")
	}
	if nx == fromX && ny == fromY {
		t.Fatal("nextStep froze — a detour exists through the open ring")
	}
	if !passable(m, s, nx, ny) {
		t.Errorf("nextStep chose an impassable tile (%d,%d)", nx, ny)
	}
}

// TestWallEnclosureUnreachable is spec 032 US1 (research R2): walls on all four
// neighbors of a tile enclose the agent standing on it — BFS finds no route, so
// nextStep returns the current tile (the caller resolves via intent_done).
func TestWallEnclosureUnreachable(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	cx, cy, ok := openBlock(m)
	if !ok {
		t.Fatal("no 3x3 open block found on seed 42")
	}
	// Wall the four orthogonal neighbors of the center: an agent on the center
	// is sealed in (diagonal moves are not allowed — 4-neighbor grid).
	for _, d := range neighborOrder {
		s.Structures = append(s.Structures, Structure{Kind: "wall_stone", X: cx + d[0], Y: cy + d[1], HP: wallStoneHP})
	}
	// A target two tiles east, outside the pen — unreachable.
	nx, ny := nextStep(m, s, cx, cy, cx+2, cy)
	if nx != cx || ny != cy {
		t.Errorf("enclosed agent's nextStep = (%d,%d), want its own tile (%d,%d) (no route)", nx, ny, cx, cy)
	}
}

// TestWallDetourReplayDeterminism is SC-005 over the passability change: a run
// with a wall placed (by direct injection, mirroring the structure-overlay
// idiom) replays to a byte-identical state hash — the wall scan in passable is
// tick-pure.
func TestWallDetourReplayDeterminism(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	cx, cy, ok := openBlock(NewState(seed, m).m)
	if !ok {
		t.Fatal("no open block on seed 42")
	}

	genesis := func() *State {
		s := NewState(seed, m)
		s.Structures = append(s.Structures, Structure{Kind: "wall_plank", X: cx, Y: cy, HP: wallPlankHP})
		return s
	}
	const ticks = 3000
	live := genesis()
	log := driveTicks(t, live, m, ticks, nil)

	replayed := genesis()
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil)
	if live.Hash() != replayed.Hash() {
		t.Fatalf("replayed state diverged with a wall present")
	}
}

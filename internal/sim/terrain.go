package sim

import "github.com/evanstern/promptworld/internal/worldmap"

// Effective terrain = static generated map + event-sourced overlays: chopped
// trees become clear ground, harvested forage is bare until it regrows.
// These are pure functions of (map, state) so the generator and any client
// replica agree exactly.

func effectiveKind(m *worldmap.Map, s *State, x, y int) worldmap.TileKind {
	k := m.At(x, y)
	switch k {
	case worldmap.Tree:
		for _, c := range s.Cleared {
			if c.X == x && c.Y == y {
				return worldmap.Grass
			}
		}
	case worldmap.Forage:
		for _, h := range s.Harvested {
			if h.X == x && h.Y == y {
				return worldmap.Grass
			}
		}
	case worldmap.Rock:
		// Quarried (spec 012): unlike Cleared/Harvested, a depleted outcrop
		// does NOT revert to Grass — it stays non-buildable and non-quarryable,
		// rendered distinctly (worldmap.Depleted), permanent in v1 (no regrow).
		for _, q := range s.Quarried {
			if q.X == x && q.Y == y {
				return worldmap.Depleted
			}
		}
	}
	return k
}

func passable(m *worldmap.Map, s *State, x, y int) bool {
	if !m.InBounds(x, y) {
		return false
	}
	// T004 (spec 032 US1, FR-002): a standing wall makes its tile impassable —
	// the first structure family to block pathing. A linear scan of the
	// structure overlay, matching effectiveKind's overlay scans; wallAt is nil
	// for every non-wall tile, so this is a no-op on pre-032 worlds.
	if wallAt(s, x, y) != nil {
		return false
	}
	k := effectiveKind(m, s, x, y)
	return k == worldmap.Grass || k == worldmap.Forage || k == worldmap.Depleted
}

// isWall names the wall family (spec 032, research R1): kind ∈ {wall_plank,
// wall_stone}. Paths are not walls (they never block).
func isWall(kind string) bool {
	return kind == "wall_plank" || kind == "wall_stone"
}

// wallMaxHP is a wall kind's derived maximum health — the single source the
// build stamp, repair clamp, and TUI damage styling all read (never stored, so
// it cannot drift; fire lit-ness precedent). Zero for non-wall kinds.
func wallMaxHP(kind string) int {
	switch kind {
	case "wall_plank":
		return wallPlankHP
	case "wall_stone":
		return wallStoneHP
	}
	return 0
}

// wallAt returns a pointer to the standing wall on (x,y), or nil when the tile
// holds none — the passable scan, the demolish/repair completions, and the
// reducer arms re-validate the wall by coord through it (chestAt sibling).
func wallAt(s *State, x, y int) *Structure {
	for i := range s.Structures {
		st := &s.Structures[i]
		if isWall(st.Kind) && st.X == x && st.Y == y {
			return st
		}
	}
	return nil
}

// pathAt reports whether a path structure stands on (x,y) — the movement
// dual-phase cadence's per-step predicate (research R3), a structure scan
// sibling of chestAt/wallAt.
func pathAt(s *State, x, y int) bool {
	for i := range s.Structures {
		st := &s.Structures[i]
		if st.Kind == "path" && st.X == x && st.Y == y {
			return true
		}
	}
	return false
}

// agentAt reports whether any living agent occupies (x,y) — the wall-build
// occupancy guard (spec 032 FR-007): a wall may never land on a tile holding an
// agent (entombment). Map-free; villagers may share a tile, so the first match
// suffices.
func agentAt(s *State, x, y int) bool {
	for i := range s.Agents {
		if !s.Agents[i].Dead && s.Agents[i].X == x && s.Agents[i].Y == y {
			return true
		}
	}
	return false
}

// buildSite: effective grass with no structure on it.
func buildSite(m *worldmap.Map, s *State, x, y int) bool {
	if !m.InBounds(x, y) || effectiveKind(m, s, x, y) != worldmap.Grass {
		return false
	}
	for _, st := range s.Structures {
		if st.X == x && st.Y == y {
			return false
		}
	}
	// T019 (spec 013 US2, FR-007): goods aren't buried — a tile holding a pile
	// is not buildable. buildSite backs both the resolveGoal buildable search
	// and the executor's completion re-validation, so every build_* goal
	// rejects pile tiles at both the search and the landing.
	if s.pileAt(x, y) != nil {
		return false
	}
	return true
}

// warmAt: within fireWarmRadius of a LIT fire, or exactly on a shelter tile.
// A fire is lit iff tick < FuelUntil (T019): a burned-out fire grants no
// warmth. Shelter warmth is unchanged (no fuel).
func warmAt(s *State, x, y int, tick int64) bool {
	for _, st := range s.Structures {
		switch st.Kind {
		case "fire":
			if tick < st.FuelUntil && abs(st.X-x)+abs(st.Y-y) <= fireWarmRadius {
				return true
			}
		case "shelter":
			if st.X == x && st.Y == y {
				return true
			}
		}
	}
	return false
}

func (s *State) structureAt(kind string, x, y int) bool {
	for _, st := range s.Structures {
		if st.Kind == kind && st.X == x && st.Y == y {
			return true
		}
	}
	return false
}

func (s *State) hasStructure(kind string) bool {
	for _, st := range s.Structures {
		if st.Kind == kind {
			return true
		}
	}
	return false
}

// chestAt returns a pointer to the chest structure on (x,y), or nil when the
// tile holds none — the deposit/withdraw completions and reducer cases
// re-validate the chest by coord (spec 013 US3), and Store is the mutable
// contents pointer they move goods through.
func (s *State) chestAt(x, y int) *Structure {
	for i := range s.Structures {
		st := &s.Structures[i]
		if st.Kind == "chest" && st.X == x && st.Y == y {
			return st
		}
	}
	return nil
}

// fireStructAt returns a pointer to the fire structure on (x,y), if any
// (T020/T021: refuel/cook re-validate the station at completion by coord).
func fireStructAt(s *State, x, y int) (*Structure, bool) {
	for i := range s.Structures {
		st := &s.Structures[i]
		if st.Kind == "fire" && st.X == x && st.Y == y {
			return st, true
		}
	}
	return nil, false
}

// litFireAt reports whether a LIT fire stands on (x,y) at tick — the cook
// station predicate (T021). Lit-ness is derived: tick < FuelUntil.
func litFireAt(s *State, x, y int, tick int64) bool {
	st, ok := fireStructAt(s, x, y)
	return ok && tick < st.FuelUntil
}

func denReadyAt(s *State, x, y int, tick int64) bool {
	for _, d := range s.DenUses {
		if d.X == x && d.Y == y && tick < d.Ready {
			return false
		}
	}
	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

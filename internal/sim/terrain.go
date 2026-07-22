package sim

import "github.com/evanstern/script-world/internal/worldmap"

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
	k := effectiveKind(m, s, x, y)
	return k == worldmap.Grass || k == worldmap.Forage || k == worldmap.Depleted
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

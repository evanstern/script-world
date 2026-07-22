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
	return true
}

// warmAt: within fireWarmRadius of a fire, or exactly on a shelter tile.
func warmAt(s *State, x, y int) bool {
	for _, st := range s.Structures {
		switch st.Kind {
		case "fire":
			if abs(st.X-x)+abs(st.Y-y) <= fireWarmRadius {
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

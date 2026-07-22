package sim

import "github.com/evanstern/promptworld/internal/worldmap"

// Deterministic grid search. Neighbor order is fixed (N, E, S, W) and the
// frontier is a FIFO queue, so shortest paths and "nearest match" results are
// identical on every run — a requirement, since intent targets and each
// movement step are derived from these.

var neighborOrder = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// bfs runs breadth-first search from (sx, sy) over passable terrain, calling
// visit for each reached tile in deterministic order (including the start).
// visit returns true to stop. cameFrom is returned for path reconstruction.
func bfs(m *worldmap.Map, s *State, sx, sy int, visit func(x, y int) bool) (stopX, stopY int, cameFrom map[Point]Point, found bool) {
	start := Point{X: sx, Y: sy}
	cameFrom = map[Point]Point{start: start}
	queue := []Point{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visit(cur.X, cur.Y) {
			return cur.X, cur.Y, cameFrom, true
		}
		for _, d := range neighborOrder {
			nx, ny := cur.X+d[0], cur.Y+d[1]
			np := Point{X: nx, Y: ny}
			if _, seen := cameFrom[np]; seen || !passable(m, s, nx, ny) {
				continue
			}
			cameFrom[np] = cur
			queue = append(queue, np)
		}
	}
	return 0, 0, cameFrom, false
}

// nextStep returns the next tile on a shortest path toward the target, or
// the current position if the target is unreachable (callers abandon the
// intent). The escape clause: from an impassable tile (pre-terrain saves),
// any in-bounds neighbor is a legal first step toward open ground.
func nextStep(m *worldmap.Map, s *State, fromX, fromY, toX, toY int) (int, int) {
	if fromX == toX && fromY == toY {
		return fromX, fromY
	}
	if !passable(m, s, fromX, fromY) {
		for _, d := range neighborOrder {
			if nx, ny := fromX+d[0], fromY+d[1]; passable(m, s, nx, ny) {
				return nx, ny
			}
		}
		for _, d := range neighborOrder {
			if nx, ny := fromX+d[0], fromY+d[1]; m.InBounds(nx, ny) {
				return nx, ny
			}
		}
		return fromX, fromY
	}
	_, _, cameFrom, found := bfs(m, s, fromX, fromY, func(x, y int) bool {
		return x == toX && y == toY
	})
	if !found {
		return fromX, fromY
	}
	// Walk back from the target to the tile adjacent to the start.
	cur := Point{X: toX, Y: toY}
	start := Point{X: fromX, Y: fromY}
	for cameFrom[cur] != start {
		cur = cameFrom[cur]
	}
	return cur.X, cur.Y
}

// nearest finds the closest reachable tile satisfying match, in BFS order.
func nearest(m *worldmap.Map, s *State, fromX, fromY int, match func(x, y int) bool) (Point, bool) {
	x, y, _, found := bfs(m, s, fromX, fromY, match)
	return Point{X: x, Y: y}, found
}

// nearestAdjacentTo finds the closest passable tile that neighbors a tile
// satisfying matchRes (e.g. stand beside a tree to chop it). Returns both
// the standing tile and the resource tile.
func nearestAdjacentTo(m *worldmap.Map, s *State, fromX, fromY int, matchRes func(x, y int) bool) (stand, res Point, ok bool) {
	sx, sy, _, found := bfs(m, s, fromX, fromY, func(x, y int) bool {
		for _, d := range neighborOrder {
			if matchRes(x+d[0], y+d[1]) {
				return true
			}
		}
		return false
	})
	if !found {
		return Point{}, Point{}, false
	}
	for _, d := range neighborOrder {
		if matchRes(sx+d[0], sy+d[1]) {
			return Point{X: sx, Y: sy}, Point{X: sx + d[0], Y: sy + d[1]}, true
		}
	}
	return Point{}, Point{}, false
}

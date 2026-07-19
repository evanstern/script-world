// Package worldmap generates and represents the village terrain: a pure
// function of (seed, width, height), so the map is never persisted — every
// process that knows the manifest regenerates it identically. Terrain is a
// flat tile slice (index y*W+x), the representation that scales to DF-style
// sizes later; only dynamic things (buildings, when they exist) will be
// event-sourced on top.
package worldmap

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

type TileKind uint8

// There is deliberately no structure/building tile kind: worlds start cold
// (Minecraft-style), and structures arrive later as event-sourced state
// layered over the terrain, never as generated terrain.
const (
	Grass TileKind = iota
	Water
	Tree
	Forage
)

// DefaultSize is the v1 village area; the representation itself is
// size-agnostic.
const DefaultSize = 64

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Map struct {
	W, H  int
	Tiles []TileKind
	// Dens are animal home sites (huntable wildlife territory); the animal
	// entities themselves are TASK-5's.
	Dens []Point
}

func (m *Map) At(x, y int) TileKind {
	return m.Tiles[y*m.W+x]
}

func (m *Map) InBounds(x, y int) bool {
	return x >= 0 && x < m.W && y >= 0 && y < m.H
}

// Passable is walkable terrain: grass and forage. Water and standing trees
// block movement.
func (m *Map) Passable(x, y int) bool {
	if !m.InBounds(x, y) {
		return false
	}
	k := m.At(x, y)
	return k == Grass || k == Forage
}

// Buildable sites are plain grass: flat, dry, unforested (AC#2 — worlds
// start with no structures but room to build them).
func (m *Map) Buildable(x, y int) bool {
	return m.InBounds(x, y) && m.At(x, y) == Grass
}

func (m *Map) CountKind(k TileKind) int {
	n := 0
	for _, t := range m.Tiles {
		if t == k {
			n++
		}
	}
	return n
}

// Hash fingerprints the full terrain + dens for determinism checks (AC#3).
func (m *Map) Hash() string {
	h := sha256.New()
	h.Write([]byte{byte(m.W), byte(m.W >> 8), byte(m.H), byte(m.H >> 8)})
	for _, t := range m.Tiles {
		h.Write([]byte{byte(t)})
	}
	for _, d := range m.Dens {
		h.Write([]byte{byte(d.X), byte(d.X >> 8), byte(d.Y), byte(d.Y >> 8)})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Generation tuning: fractions of the map, chosen so a 64×64 village has a
// real lake, real woods, and plenty of open ground.
const (
	waterFraction  = 0.18 // lowest-lying land floods
	treeFraction   = 0.24 // moistest dry land forests
	foragePerMille = 45   // ~4.5% of open grass carries forage
	denCount       = 4
	denMinDistance = 12
)

// Generate is deterministic: same (seed, w, h) → identical Map, on every
// platform (integer/hash noise only, no float randomness sources).
func Generate(seed uint64, w, h int) *Map {
	if w <= 0 {
		w = DefaultSize
	}
	if h <= 0 {
		h = DefaultSize
	}
	m := &Map{W: w, H: h, Tiles: make([]TileKind, w*h)}

	// Two independent fractal noise fields: elevation and moisture.
	height := make([]float64, w*h)
	moist := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			height[i] = fbm(seed, "elevation", x, y)
			moist[i] = fbm(seed, "moisture", x, y)
		}
	}

	// Water: flood everything below the waterFraction percentile of height.
	waterLine := percentile(height, waterFraction)
	for i, hv := range height {
		if hv <= waterLine {
			m.Tiles[i] = Water
		}
	}

	// Trees: the moistest dry tiles, as patches (moisture is spatially
	// correlated noise, so thresholding yields woods, not salt-and-pepper).
	var dryMoist []float64
	for i, t := range m.Tiles {
		if t != Water {
			dryMoist = append(dryMoist, moist[i])
		}
	}
	treeLine := percentileTop(dryMoist, treeFraction)
	for i, t := range m.Tiles {
		if t == Grass && moist[i] >= treeLine {
			m.Tiles[i] = Tree
		}
	}

	// Forage: scattered over remaining grass by per-tile hash.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if m.Tiles[i] == Grass && hash2(seed, "forage", x, y)%1000 < foragePerMille {
				m.Tiles[i] = Forage
			}
		}
	}

	// Animal dens: deterministic candidate stream; grass only, spread out.
	for n := 0; len(m.Dens) < denCount && n < 10_000; n++ {
		x := int(hash2(seed, "den-x", n, 0) % uint64(w))
		y := int(hash2(seed, "den-y", n, 0) % uint64(h))
		if m.At(x, y) != Grass {
			continue
		}
		ok := true
		for _, d := range m.Dens {
			if abs(d.X-x)+abs(d.Y-y) < denMinDistance {
				ok = false
				break
			}
		}
		if ok {
			m.Dens = append(m.Dens, Point{X: x, Y: y})
		}
	}

	return m
}

func percentile(values []float64, frac float64) float64 {
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	idx := int(frac * float64(len(s)))
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return s[idx]
}

// percentileTop returns the threshold above which frac of values lie.
func percentileTop(values []float64, frac float64) float64 {
	return percentile(values, 1-frac)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

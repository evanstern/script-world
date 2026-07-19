package worldmap

import "hash/fnv"

// Seeded integer-hash value noise: lattice values come from an FNV hash of
// (seed, purpose, lattice point), bilinearly interpolated and summed over
// octaves. Pure integer hashing keeps generation identical across
// platforms and Go versions — the same discipline as internal/sim's rngAt.

// hash2 hashes (seed, purpose, a, b) to a uint64.
func hash2(seed uint64, purpose string, a, b int) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(v uint64) {
		for i := 0; i < 8; i++ {
			buf[i] = byte(v >> (8 * i))
		}
		h.Write(buf[:])
	}
	put(seed)
	h.Write([]byte(purpose))
	put(uint64(int64(a)))
	put(uint64(int64(b)))
	return h.Sum64()
}

// latticeValue is the noise value at an integer lattice point, in [0, 1).
func latticeValue(seed uint64, purpose string, x, y int) float64 {
	return float64(hash2(seed, purpose, x, y)%1_000_000) / 1_000_000
}

// valueNoise samples smoothed noise at (x, y) with the given lattice cell
// size, using smoothstep-eased bilinear interpolation.
func valueNoise(seed uint64, purpose string, x, y, cell int) float64 {
	x0, y0 := floorDiv(x, cell), floorDiv(y, cell)
	fx := float64(x-x0*cell) / float64(cell)
	fy := float64(y-y0*cell) / float64(cell)
	fx = fx * fx * (3 - 2*fx)
	fy = fy * fy * (3 - 2*fy)

	v00 := latticeValue(seed, purpose, x0, y0)
	v10 := latticeValue(seed, purpose, x0+1, y0)
	v01 := latticeValue(seed, purpose, x0, y0+1)
	v11 := latticeValue(seed, purpose, x0+1, y0+1)

	top := v00 + (v10-v00)*fx
	bot := v01 + (v11-v01)*fx
	return top + (bot-top)*fy
}

// fbm is 3-octave fractal noise in [0, ~1): cells 16, 8, 4 with halving
// amplitude — broad landforms with local texture at village scale.
func fbm(seed uint64, purpose string, x, y int) float64 {
	return (valueNoise(seed, purpose, x, y, 16)*4 +
		valueNoise(seed, purpose+"/2", x, y, 8)*2 +
		valueNoise(seed, purpose+"/3", x, y, 4)) / 7
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b < 0 {
		q--
	}
	return q
}

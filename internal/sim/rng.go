package sim

import (
	"hash/fnv"
	"math/rand/v2"
)

// rngAt returns a PCG seeded purely from (world seed, purpose, tick, index).
// There is no long-lived RNG stream: every random decision is a pure function
// of its coordinates, so replay and recovery need no RNG state at all.
func rngAt(seed uint64, purpose string, tick int64, index int) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(purpose))
	sub := h.Sum64()
	return rand.New(rand.NewPCG(seed^sub, uint64(tick)*0x9e3779b97f4a7c15+uint64(index)))
}

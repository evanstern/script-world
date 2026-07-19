package sim

import (
	"fmt"
	"sort"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
)

// Episodic memory: deterministic emission heuristics (research R2) and the
// deterministic working-memory window (research R3). Generation is the
// executor's job; selection is a pure function shared by the mind's prompts
// and the tests.

// memoryEvent builds a personal agent.memory_added event (no gossip subject).
func memoryEvent(tick int64, agent int, salience int, format string, args ...any) store.Event {
	return store.Event{
		Tick: tick, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{
			Agent: agent, Text: fmt.Sprintf(format, args...), Salience: salience, Subject: -1,
		}),
	}
}

// memoryAboutEvent marks a gossip-worthy memory about another agent — the
// seed rumors are born from (TASK-8).
func memoryAboutEvent(tick int64, agent, subject, tone, salience int, format string, args ...any) store.Event {
	return store.Event{
		Tick: tick, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{
			Agent: agent, Text: fmt.Sprintf(format, args...), Salience: salience,
			Subject: subject, Tone: tone,
		}),
	}
}

// Salience table (1..10). Kept small and legible on purpose — consolidation
// (TASK-9) is the layer that reweighs and rewrites.
const (
	salTalk           = 3
	salHunt           = 4
	salFire           = 5
	salShelter        = 6
	salStarvingForage = 5
	salColdNight      = 5
	salNearDeath      = 9
	salWitnessDeath   = 10
)

// WindowK is the working-memory bound: prompts never carry more than this
// many memories (top K−tail by score + seeded tail picks).
const (
	WindowK        = 10
	windowTailPick = 2
	// recency half-life: a memory's weight halves every game-day.
	halfLifeTicks = 24 * 3600
)

// SelectMemories is the deterministic top-K window (AC#3): score = salience
// halved per day of age; top K−2 by score, plus 2 serendipity picks from the
// oldest half seeded per cadence bucket; presented reverse-chronologically.
func SelectMemories(a *Agent, seed uint64, agentIdx int, tick int64, k int) []Memory {
	n := len(a.Memories)
	if n == 0 || k <= 0 {
		return nil
	}
	if n <= k {
		out := append([]Memory(nil), a.Memories...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].Tick > out[j].Tick })
		return out
	}

	type scored struct {
		m     Memory
		score float64
		idx   int
	}
	all := make([]scored, n)
	for i, m := range a.Memories {
		age := tick - m.Tick
		if age < 0 {
			age = 0
		}
		// integer-friendly decay: halve per whole game-day of age
		w := float64(m.Salience)
		for d := age / halfLifeTicks; d > 0; d-- {
			w /= 2
		}
		all[i] = scored{m: m, score: w, idx: i}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].m.Tick > all[j].m.Tick // ties: newer wins
	})

	take := k - windowTailPick
	picked := map[int]bool{}
	var out []Memory
	for i := 0; i < take && i < n; i++ {
		out = append(out, all[i].m)
		picked[all[i].idx] = true
	}

	// Serendipity: seeded picks from the oldest half (by original position),
	// bucketed to the planner cadence so retries in one window agree.
	oldHalf := n / 2
	if oldHalf > 0 {
		r := rngAt(seed, "serendipity", tick/PlannerCadenceTicks, agentIdx)
		for t := 0; t < windowTailPick; t++ {
			for tries := 0; tries < 8; tries++ {
				i := int(r.Uint64N(uint64(oldHalf)))
				if !picked[i] {
					picked[i] = true
					out = append(out, a.Memories[i])
					break
				}
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick > out[j].Tick })
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// FormatMemory renders one memory line as prompts and soul.md show it.
func FormatMemory(m Memory) string {
	return fmt.Sprintf("%s (%d★) %s", clock.Format(m.Tick), m.Salience, m.Text)
}

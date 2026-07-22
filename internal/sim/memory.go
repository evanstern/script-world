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

// memoryEventToned is memoryEvent with an explicit tone — for a personal
// memory (no gossip subject, Subject stays -1) that still carries a
// positive/negative flavor, like a bath's contentment (spec 012 T032).
func memoryEventToned(tick int64, agent, salience, tone int, format string, args ...any) store.Event {
	return store.Event{
		Tick: tick, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{
			Agent: agent, Text: fmt.Sprintf(format, args...), Salience: salience, Subject: -1, Tone: tone,
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
	// SalDream: Metatron's dreams/omens (TASK-12) — exported for the
	// injection builder; between shelter and near-death so the divine
	// reliably surfaces without outranking real trauma.
	SalDream = 8
	// Governance (TASK-13): speaking is routine, outcomes matter, watching a
	// neighbor break the law sticks, being cast out is formative.
	salMeetingSpoke   = 3
	salMeetingOutcome = 5
	salNormViolation  = 6
	salExiled         = 9
	// Spec 012 (crafting economy, research R8): "high" here means memorable,
	// not generation-interrupting — both sit below GenerationBumpSalience (9),
	// the same band as SalDream, so a broken spear or a new oven doesn't
	// outrank real trauma at cognition-landing time.
	salSpearBroke = 8 // US3: the spear that spent its last use
	salBath       = 5 // US4: medium, positive tone
	salOvenBuilt  = 7 // US4: high, village-visible (builder + nearby witnesses)
	// Spec 013 (inventory & storage, research R5). Same "high = memorable, not
	// generation-interrupting" band as salOvenBuilt (below GenerationBumpSalience).
	salChestBuilt = 7 // US4/T030: high, village-visible (oven precedent)
	// salTaking: a taking from an owned chest — suffered by the owner and
	// witnessed by neighbors; high and negative, above rumorMinSalience so the
	// owner's subject-tagged memory is a live gossip seed (FR-012).
	salTaking = 7
	// salFireOut: low-salience — a fire going cold nearby is background
	// texture, not formative (contracts/events.md: "fire burned out while
	// agents nearby, low"). Purely personal (no gossip subject).
	salFireOut = 3
)

// Tone constants for the spec 012 memories above (governance.go/social.go/
// gru.go each declare their own tone band the same way).
const (
	toneBath       = 40 // positive, matching toneSaved's magnitude
	toneOvenBuilt  = 30 // positive; witnesses take pride, less personal than bathing
	toneChestBuilt = 20 // positive; a neighbor's larder is welcome but modestly so
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

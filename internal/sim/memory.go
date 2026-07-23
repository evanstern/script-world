package sim

import (
	"fmt"
	"sort"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Episodic memory: deterministic emission heuristics (research R2) and the
// deterministic working-memory window (research R3). Generation is the
// executor's job; selection is a pure function shared by the mind's prompts
// and the tests.

// --- spec 019 (US1): situated episodic memories ---
//
// Every episodic memory the sim emits is situated (SC-001): these constructors
// bake the where/why context into the agent.memory_added payload AND compose it
// into the memory text via the shared grammar helper (situateText). The
// salience/subject/tone semantics are unchanged — this layer situates memories,
// it does not re-weigh them. There are three, mirroring the memory shapes:
// personal (situatedMemoryEvent), personal-with-tone (situatedMemoryToned), and
// gossip/witness about another agent (situatedMemoryAboutEvent, which carries no
// Why — a witness never drove the act). Spec 019 T008b removed the pre-019 bare
// constructors once every emission site was migrated, so no sim memory can be
// emitted unsituated: a new memory site must pick a situated constructor and
// therefore a Where.

// placeScanRadius bounds describePlace's deterministic feature scan (Manhattan).
const placeScanRadius = 2

// describePlace returns a deterministic terrain/feature description for the tile
// at (x,y) — the nearest notable feature (station or terrain) within a small
// fixed radius, scanned in a fixed ring order so the same (state, x, y) always
// yields the same string. "" when nothing notable is near (coords alone
// situate the memory). Baked into the event at emission (research R3); the
// scribe renders what the payload carries and never re-derives, so replay is
// byte-identical with no map lookup.
func describePlace(s *State, x, y int) string { return describePlaceExcept(s, x, y, "") }

// describePlaceExcept is describePlace with one structure kind held out of the
// scan — the build-memory fix (T024): a build completion describes its tile as
// it was WITHOUT the thing just placed (and without any same-kind neighbour), so
// "Built a fire" never resolves to "at the fire". Excluding the kind is
// deterministic and needs no ordering dance with the not-yet-reduced built
// event. excludeKind == "" is the ordinary describePlace.
func describePlaceExcept(s *State, x, y int, excludeKind string) string {
	if s.m == nil {
		return ""
	}
	for r := 0; r <= placeScanRadius; r++ {
		for dy := -r; dy <= r; dy++ {
			dx := r - abs(dy)
			if d := featureDesc(s, x+dx, y+dy, excludeKind); d != "" {
				return d
			}
			if dx != 0 {
				if d := featureDesc(s, x-dx, y+dy, excludeKind); d != "" {
					return d
				}
			}
		}
	}
	return ""
}

// featureDesc names the notable feature on one tile — a station structure
// first (the most salient), then the terrain kind — as a noun phrase that reads
// after "at" ("the fire", "the rock outcrop"). A structure whose kind equals
// excludeKind is skipped (build fix, T024). "" for ordinary or off-map tiles.
func featureDesc(s *State, x, y int, excludeKind string) string {
	if s.m == nil || x < 0 || y < 0 || x >= s.m.W || y >= s.m.H {
		return ""
	}
	for _, st := range s.Structures {
		if st.X == x && st.Y == y && st.Kind != excludeKind {
			switch st.Kind {
			case "fire":
				return "the fire"
			case "shelter":
				return "the shelter"
			case "oven":
				return "the oven"
			case "chest":
				return "the chest"
			}
		}
	}
	switch effectiveKind(s.m, s, x, y) {
	case worldmap.Water:
		return "the water"
	case worldmap.Tree:
		return "the woods"
	case worldmap.Rock:
		return "the rock outcrop"
	case worldmap.Forage:
		return "the forage patch"
	}
	return ""
}

// PlaceAt returns the situated location of a memory formed at (x,y): the coords
// always (FR-001) plus a deterministic feature description (may be empty).
// Exported so the mind side (convo.go) situates conversation memories from the
// same helper the executor uses. Never nil — coords alone satisfy FR-001.
func PlaceAt(s *State, x, y int) *MemoryPlace {
	return &MemoryPlace{X: x, Y: y, Desc: describePlace(s, x, y)}
}

// placeForBuild situates a build-completion memory: the tile described as it was
// without the just-built structure kind, so a fire built by the woods reads
// "at the woods (x,y)", never "at the fire (x,y)" (T024).
func placeForBuild(s *State, x, y int, builtKind string) *MemoryPlace {
	return &MemoryPlace{X: x, Y: y, Desc: describePlaceExcept(s, x, y, builtKind)}
}

// situateText composes a situated memory text from a base template and its
// context, in the exact grammar order pinned by contracts/memory-context.md:
//
//	<base>[ at <desc> (x,y) | at (x,y)][ — <why>]
//
// The where-clause splices before the base's trailing period (preserved when
// there is no why); the why-clause is the intent reason verbatim, carrying its
// own terminal punctuation. Absent parts produce no clause — never a fabricated
// one. Implemented once here so every call site composes identically.
func situateText(base string, where *MemoryPlace, why string) string {
	stem := strings.TrimSuffix(base, ".")
	hadDot := stem != base
	var b strings.Builder
	b.WriteString(stem)
	if where != nil {
		if where.Desc != "" {
			fmt.Fprintf(&b, " at %s (%d,%d)", where.Desc, where.X, where.Y)
		} else {
			fmt.Fprintf(&b, " at (%d,%d)", where.X, where.Y)
		}
	}
	switch {
	case why != "":
		b.WriteString(" — ")
		b.WriteString(why)
	case hadDot:
		b.WriteByte('.')
	}
	return b.String()
}

// situatedMemoryEvent is memoryEvent with situated context (spec 019): the
// where/why are baked into the payload AND composed into the text. Where is the
// acting agent's tile; Why is the driving intent's reason ("" for reflex).
func situatedMemoryEvent(tick int64, agent, salience int, where *MemoryPlace, why string, format string, args ...any) store.Event {
	return store.Event{
		Tick: tick, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{
			Agent: agent, Text: situateText(fmt.Sprintf(format, args...), where, why),
			Salience: salience, Subject: -1, Where: where, Why: why,
		}),
	}
}

// situatedMemoryToned is memoryEventToned with situated context (spec 019).
func situatedMemoryToned(tick int64, agent, salience, tone int, where *MemoryPlace, why string, format string, args ...any) store.Event {
	return store.Event{
		Tick: tick, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{
			Agent: agent, Text: situateText(fmt.Sprintf(format, args...), where, why),
			Salience: salience, Subject: -1, Tone: tone, Where: where, Why: why,
		}),
	}
}

// situatedMemoryAboutEvent is memoryAboutEvent with situated context (spec 019):
// a gossip-worthy memory about another agent, situated by the WITNESS's own
// location. Witness memories carry no Why — the witness did not drive the act
// (contracts/memory-context.md rule 2).
func situatedMemoryAboutEvent(tick int64, agent, subject, tone, salience int, where *MemoryPlace, format string, args ...any) store.Event {
	return store.Event{
		Tick: tick, Type: "agent.memory_added",
		Payload: mustPayload(MemoryAddedPayload{
			Agent: agent, Text: situateText(fmt.Sprintf(format, args...), where, ""),
			Salience: salience, Subject: subject, Tone: tone, Where: where,
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

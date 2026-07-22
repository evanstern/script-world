package sim

// World migration — the migration-only seam (spec 012 US6 for v1→v2, research
// R10; spec 013 for v2→v3, research R3). This file holds the pure transforms:
// the typed v1 legacy decode + v1→v2 transform, and the v2→v3 transform.
// Neither runs on the live reducer path — the migrate command (internal/world)
// decodes a world's covering snapshot, transforms it here, and writes the
// result as a single world.migrated event whose reducer case (state.go)
// replaces state wholesale. A v1 world chains 1→2→3 in one run.
//
// The v1→v2 transform's contract is "keep the people, reset the land": every
// villager and the whole social/governance fabric carry over verbatim (tick
// continuity intact, so memory ticks, consolidation marks, and day counts stay
// meaningful); the map and everything bound to it is reborn under v2 rules. The
// v2→v3 transform (below) is people- AND land-preserving — spec 013 changed no
// terrain, so nothing resets; it only enforces the new bulk cap by spilling
// over-cap carry to ground piles.

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// legacyFoodToMeals converts a v1 legacy Food unit into v2 Meals. The design
// pin (spec Assumptions, research R10): 1 old food → 3 Meals — a mild haircut
// (350 → 300 restore) flavored as preserved meals crossing the break.
const legacyFoodToMeals = 3

// legacyInventory is the v1 carried-items shape: only wood and the coarse
// legacy Food unit existed. Decoding a v1 snapshot straight into the v2
// Inventory would SILENTLY DROP `food` (v2 has no such field), so migration
// must read it through this typed shape — the one field where v1 and v2 diverge
// incompatibly rather than v2 merely adding fields.
type legacyInventory struct {
	Wood int `json:"wood"`
	Food int `json:"food"`
}

// legacyAgent mirrors the v1 Agent exactly except for Inv (legacyInventory).
// Every other field either is unchanged from v1 or was v2-added (absent in v1
// JSON ⇒ decodes to its zero value), so the shared v2 sub-types decode a v1
// agent faithfully. Map-/session-bound agent fields (Intent, Plan, Hail,
// Asleep) are decoded but discarded by the transform — everyone wakes standing.
type legacyAgent struct {
	Name                  string          `json:"name"`
	X                     int             `json:"x"`
	Y                     int             `json:"y"`
	Needs                 Needs           `json:"needs"`
	Inv                   legacyInventory `json:"inv"`
	Dead                  bool            `json:"dead"`
	LastTalk              int64           `json:"last_talk"`
	LastGive              int64           `json:"last_give,omitempty"`
	Known                 []KnownRumor    `json:"known,omitempty"`
	Memories              []Memory        `json:"memories,omitempty"`
	NearDeath             bool            `json:"near_death,omitempty"`
	Generation            int64           `json:"generation,omitempty"`
	Beliefs               []Belief        `json:"beliefs,omitempty"`
	Narrative             string          `json:"narrative,omitempty"`
	LastConsolidatedNight int64           `json:"last_consolidated_night,omitempty"`
	ConsolidatedUpTo      int64           `json:"consolidated_up_to,omitempty"`
	LastConsolidateMark   int64           `json:"last_consolidate_mark,omitempty"`
}

// legacyState is the v1 reducer state as it decodes a v1 covering snapshot.
// It intentionally names ONLY the fields the migration carries across the break
// plus Agents (for the legacyInventory capture); v1's map-/session-bound fields
// (Structures, Cleared, Harvested, DenUses, Gru, Meeting, MeetingConvention,
// MeetingPlace) are deliberately not decoded — they are reset, not carried, so
// json.Unmarshal simply ignores them. Norms and the charter/governance state,
// by contrast, ARE the village's lived law and carry verbatim.
type legacyState struct {
	Tick   int64         `json:"tick"`
	Paused bool          `json:"paused"`
	Speed  clock.Speed   `json:"speed"`
	Seed   uint64        `json:"seed"`
	Night  bool          `json:"night"`
	Agents []legacyAgent `json:"agents"`
	// Social fabric (carried verbatim).
	Relations    []Relation `json:"relations,omitempty"`
	Debts        []Debt     `json:"debts,omitempty"`
	Rumors       []Rumor    `json:"rumors,omitempty"`
	NextDebtID   int        `json:"next_debt_id,omitempty"`
	NextRumorID  int        `json:"next_rumor_id,omitempty"`
	NextBeliefID int        `json:"next_belief_id,omitempty"`
	// Conversation ring, chronicle ring, Metatron's bank (carried verbatim).
	Conversations   []ConvoRecord    `json:"conversations,omitempty"`
	Chronicle       []ChronicleEntry `json:"chronicle,omitempty"`
	MetatronCharges int              `json:"metatron_charges"`
	// Governance/charter: the norms and their id counters carry; the in-flight
	// Meeting session and the MeetingConvention/Place are reset (re-seeded from
	// world.json on next boot, or re-emerge).
	Norms          []Norm `json:"norms,omitempty"`
	NextNormID     int    `json:"next_norm_id,omitempty"`
	NextProposalID int    `json:"next_proposal_id,omitempty"`
}

// decodeLegacyState reads a v1 covering-snapshot state JSON through the typed
// legacy shape. Migration-only: never the live reducer path.
func decodeLegacyState(data []byte) (*legacyState, error) {
	var ls legacyState
	if err := json.Unmarshal(data, &ls); err != nil {
		return nil, fmt.Errorf("decode v1 state: %w", err)
	}
	return &ls, nil
}

// MigrateState is the pure v1→v2 transform (research R10). It carries the
// people and the social/governance fabric verbatim (tick continuity intact),
// resets everything bound to the map, and re-places the carried souls on the v2
// regeneration of the same seed via the shared genesis placement (m must be
// worldmap.Generate(seed, w, h) for the v2 build). It is a pure function of
// (v1 state, v2 map): the migration tick is the carried v1 tick, so the clock
// simply continues.
func MigrateState(v1 *legacyState, m *worldmap.Map) *State {
	migTick := v1.Tick
	s := &State{
		// Clock: tick/night/speed/pause carry; the derived rate is recomputed
		// for a fresh, non-degraded start at the carried speed (a stopped world
		// carries no live drift across the break).
		Tick:          v1.Tick,
		Paused:        v1.Paused,
		Speed:         v1.Speed,
		Night:         v1.Night,
		Degraded:      false,
		EffectiveRate: v1.Speed.TicksPerSecond(),
		Seed:          v1.Seed,
		m:             m,
		Agents:        make([]Agent, len(v1.Agents)),
		// Social fabric — carried verbatim.
		Relations:    v1.Relations,
		Debts:        v1.Debts,
		Rumors:       v1.Rumors,
		NextDebtID:   v1.NextDebtID,
		NextRumorID:  v1.NextRumorID,
		NextBeliefID: v1.NextBeliefID,
		// Conversation ring, chronicle ring, Metatron bank — carried verbatim.
		Conversations:   v1.Conversations,
		Chronicle:       v1.Chronicle,
		MetatronCharges: v1.MetatronCharges,
		// Governance: norms + charter carry; the meeting session/convention are
		// reset (nil) — MeetingConvention/MeetingPlace/Meeting left zero.
		Norms:          v1.Norms,
		NextNormID:     v1.NextNormID,
		NextProposalID: v1.NextProposalID,
		// Map-bound overlays and the gru are RESET (nil zero values):
		// Structures, Cleared, Harvested, DenUses, Quarried, Gru,
		// MeetingConvention, MeetingPlace.
	}

	pos := genesisPlacement(v1.Seed, m, len(v1.Agents))
	for i := range v1.Agents {
		la := &v1.Agents[i]
		s.Agents[i] = Agent{
			// People-state carried verbatim.
			Name:                  la.Name,
			Needs:                 la.Needs,
			Memories:              la.Memories,
			Beliefs:               la.Beliefs,
			Narrative:             la.Narrative,
			Generation:            la.Generation,
			LastConsolidatedNight: la.LastConsolidatedNight,
			ConsolidatedUpTo:      la.ConsolidatedUpTo,
			LastConsolidateMark:   la.LastConsolidateMark,
			LastTalk:              la.LastTalk,
			LastGive:              la.LastGive,
			Known:                 la.Known,
			// NearDeath is people-state (a health collapse the villager lived
			// through), so it is preserved. Dead is likewise preserved — a
			// villager who died in the old world stays part of the village's
			// history, dead, rather than being resurrected by the break.
			NearDeath: la.NearDeath,
			Dead:      la.Dead,
			// Re-placed on the v2 map (map-bound position is reset).
			X: pos[i].X,
			Y: pos[i].Y,
			// Inventory: Wood 1:1; legacy Food → Meals at the pinned rate; every
			// new v2 kind starts empty.
			Inv: Inventory{
				Wood:  la.Inv.Wood,
				Meals: la.Inv.Food * legacyFoodToMeals,
			},
			// Reset (map-/session-bound): Intent/Plan/Hail nil, Asleep false,
			// WorkStart n/a (lives on the now-nil Intent). IdleSince is the
			// migration tick — everyone wakes standing, freshly idle.
			IdleSince: migTick,
		}
	}
	return s
}

// TransformV1Snapshot is the migrate command's entry point: decode a v1
// covering-snapshot state JSON and transform it to the v2 state, re-placing
// souls on m (the v2 regeneration of the world's seed). It returns the
// transformed state plus the carried source tick (the migration tick), so the
// command can stamp the world.migrated event and its initial snapshot.
func TransformV1Snapshot(v1StateJSON []byte, m *worldmap.Map) (*State, int64, error) {
	ls, err := decodeLegacyState(v1StateJSON)
	if err != nil {
		return nil, 0, err
	}
	return MigrateState(ls, m), ls.Tick, nil
}

// --- v2→v3 transform (spec 013 US, research R3) -----------------------------
//
// The v3 format break (spec 013: bulk cap, ground piles, chests, theft, rot)
// changes how the reducer/executor treat EXISTING event shapes (yield
// truncation, death spill, give-guard), so a v2 log replayed under v3 code
// would diverge — the format gate is the shield, and this transform is the
// door. Unlike the v1→v2 cut it is NOT a "reset the land" migration: spec 013
// changes no terrain generation and no map inputs, so everything carries
// VERBATIM — agents in place (NO re-placement), structures, overlays,
// memories, relations, rumors, governance, ticks. The one adjustment is the new
// bulk-cap invariant: any carried bulk over bulkCap spills to a ground pile at
// the agent's tile, and — taking the v3 death-spill invariant forward — a dead
// villager's entire carried inventory spills too (under v3, death spills; a v2
// world froze the dead's Inv, so carrying it forward keeps the migrated world
// consistent with what v3 would have produced).
//
// No distinct "v2 legacy decode" is needed: every v3 addition is additive and
// omitempty (State.Piles, Structure.Owner/Store, Intent.Kind/Qty), so a v2
// snapshot's JSON decodes into the current sim.State faithfully, all new fields
// landing on their zero values. A parallel legacy decoder would be redundant
// maintenance surface, so the transform runs against sim.State directly.

// TransformV2State is the pure v2→v3 transform. It carries the whole v2 state
// verbatim (positions, structures, overlays, the social/governance fabric, the
// clock — the migration tick is the carried tick, so the clock simply
// continues) and applies only the bulk-cap invariant: living agents over the
// cap spill their excess, dead agents spill everything, both into a ground pile
// at the agent's own tile (create-or-merge in agent-index order for
// determinism), spilled food stamped with a fresh rot deadline. It is a pure
// function of the input state and mutates no input slice.
func TransformV2State(v2 *State) *State {
	migTick := v2.Tick
	out := *v2 // carry every field verbatim (slice headers shared, read-only)
	// Derived clock fields start fresh & non-degraded, exactly as the v1→v2
	// transform does: a stopped world carries no live drift across the break.
	out.Degraded = false
	out.EffectiveRate = out.Speed.TicksPerSecond()
	// Own the Agents slice (we mutate Inv on spill) and the Piles slice (we
	// append spill piles) so the input is never mutated — the transform is pure.
	out.Agents = make([]Agent, len(v2.Agents))
	copy(out.Agents, v2.Agents)
	out.Piles = append([]Pile(nil), v2.Piles...)

	for i := range out.Agents {
		a := &out.Agents[i]
		switch {
		case a.Dead:
			// The v3 death-spill invariant, applied to the frozen v2 dead: the
			// entire carried inventory spills (research R7 idiom).
			if over := bulk(a.Inv); over > 0 {
				spillToPile(&out, a, over, migTick)
			}
		default:
			// FR-001: no living villager may carry over the cap. The excess
			// spills; the cap's worth of best goods stays carried.
			if over := bulk(a.Inv) - bulkCap; over > 0 {
				spillToPile(&out, a, over, migTick)
			}
		}
	}
	return &out
}

// spillToPile moves `over` units of an agent's carried goods into the ground
// pile at its tile (create-or-merge), removing in canonical kind order. Within
// food that order is least-nutritious-first (food_raw → food_cooked → meals,
// which IS canonical order), so a capped villager keeps its best food; spears
// move most-worn-first, mirroring the drop/deposit transfer idioms. Spilled
// food batches are stamped SpoilAt = migration tick + rotWindowTicks. `over` is
// clamped to what is actually carried, so a dead agent (over = full bulk)
// empties completely and a living one lands exactly at the cap.
func spillToPile(s *State, a *Agent, over int, migTick int64) {
	if over <= 0 {
		return
	}
	pile := s.pileFor(a.X, a.Y)
	for _, kind := range canonicalKinds {
		if over <= 0 {
			break
		}
		n := carriedCount(a.Inv, kind)
		if n > over {
			n = over
		}
		if n <= 0 {
			continue
		}
		switch {
		case kind == "spears":
			// Most-worn-first: the front of the ascending slice moves; both
			// sides stay sorted ascending.
			pile.Spears = append(pile.Spears, a.Inv.Spears[:n]...)
			sort.Ints(pile.Spears)
			rest := append([]int(nil), a.Inv.Spears[n:]...)
			if len(rest) == 0 {
				a.Inv.Spears = nil
			} else {
				a.Inv.Spears = rest
			}
		case isFoodKind(kind):
			pile.addFood(kind, n, migTick+rotWindowTicks)
			addItems(&a.Inv, []Item{{kind, n}}, -1)
		default:
			pile.addNonFood(kind, n)
			addItems(&a.Inv, []Item{{kind, n}}, -1)
		}
		over -= n
	}
}

// TransformV2Snapshot decodes a v2 covering-snapshot state JSON (structurally a
// subset of v3 — see TransformV2State) and applies the pure v2→v3 transform,
// returning the v3 state and the carried migration tick (the v2 tick continues
// unbroken). The migrate command's 2→3 entry point.
func TransformV2Snapshot(v2StateJSON []byte) (*State, int64, error) {
	var v2 State
	if err := json.Unmarshal(v2StateJSON, &v2); err != nil {
		return nil, 0, fmt.Errorf("decode v2 state: %w", err)
	}
	out := TransformV2State(&v2)
	return out, out.Tick, nil
}

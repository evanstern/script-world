// Package sim owns the deterministic simulation: world state, the event
// reducer (the single mutation path, used identically live and in replay),
// and the fixed-timestep loop.
package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// State is the whole reducer state: clock state + agents + the dynamic
// overlays on the static terrain (structures, cleared trees, harvested
// forage, den cooldowns). It marshals to canonical bytes (fixed struct field
// order) for snapshots and determinism hashing. Wall-clock time never
// appears here.
type State struct {
	Tick   int64       `json:"tick"`
	Paused bool        `json:"paused"`
	Speed  clock.Speed `json:"speed"`
	// RequestedSpeed (spec 028 US2) is the player's speed CEILING, carried only
	// while the adaptive-throttle governor holds the effective Speed below it;
	// empty means ungoverned (requested == effective). Speed stays the effective
	// speed the loop paces at, so the router and auto-slow observer need no
	// change (research R2). omitempty keeps every pre-028 snapshot byte-identical:
	// a field-absent snapshot is a valid ungoverned state.
	RequestedSpeed clock.Speed `json:"requested_speed,omitempty"`
	Degraded       bool        `json:"degraded"`
	EffectiveRate  float64     `json:"effective_rate"`
	Seed           uint64      `json:"seed"`
	Night          bool        `json:"night"`
	Agents         []Agent     `json:"agents"`
	Structures     []Structure `json:"structures,omitempty"`
	Cleared        []Point     `json:"cleared,omitempty"`
	Harvested      []Harvest   `json:"harvested,omitempty"`
	DenUses        []DenUse    `json:"den_uses,omitempty"`
	// Quarried (spec 012, US1) marks depleted rock outcrops — permanent in
	// v1, no regrow entry (unlike Harvested/Cleared, which do regrow). A
	// quarried tile is passable but NOT buildable and NOT quarryable again;
	// effectiveKind renders it as worldmap.Depleted, distinct from Grass.
	Quarried []Point `json:"quarried,omitempty"`
	// Piles (spec 013 US2) are the per-tile commons of dropped/spilled goods —
	// event-sourced overlay state like Quarried, appended in creation order,
	// one pile per tile (the reducer merges drops onto an existing pile), an
	// emptied pile removed in the same application. omitempty keeps pre-013
	// snapshots byte-identical.
	Piles []Pile `json:"piles,omitempty"`
	// Social fabric (TASK-8) — all event-sourced.
	Relations   []Relation `json:"relations,omitempty"`
	Debts       []Debt     `json:"debts,omitempty"`
	Rumors      []Rumor    `json:"rumors,omitempty"`
	NextDebtID  int        `json:"next_debt_id,omitempty"`
	NextRumorID int        `json:"next_rumor_id,omitempty"`
	// Nightly consolidation (TASK-9).
	NextBeliefID int `json:"next_belief_id,omitempty"`
	// The gru (TASK-10) — nil while it is not abroad; omitempty keeps
	// pre-TASK-10 snapshots valid.
	Gru *Gru `json:"gru,omitempty"`
	// Conversation records (TASK-22) — bounded ring, event-sourced.
	Conversations []ConvoRecord `json:"conversations,omitempty"`
	// The chronicle (TASK-11) — narrated story entries, bounded ring. Riding
	// State means every attaching client gets catch-up history in the snapshot.
	Chronicle []ChronicleEntry `json:"chronicle,omitempty"`
	// Metatron's charge bank (TASK-12) — event-sourced: executor regen,
	// injected spends. Genesis 1; pre-TASK-12 snapshots (field absent) gain
	// the default. Deliberately NOT omitempty: a spent-to-zero bank must
	// round-trip as 0, never resurrect as the genesis value.
	MetatronCharges int `json:"metatron_charges"`
	// Norms and votes (TASK-13) — all event-sourced. Pre-TASK-13 snapshots
	// unmarshal to zero values: no meeting place yet, no law, no meeting.
	MeetingPlace *Point       `json:"meeting_place,omitempty"`
	Meeting      MeetingState `json:"meeting"`
	// The meeting convention (TASK-36) — event-sourced, nil until a source
	// (config or emergence) establishes one. Pre-TASK-36 snapshots load nil:
	// a village with no standing agreement to meet.
	MeetingConvention *MeetingConvention `json:"meeting_convention,omitempty"`
	Norms             []Norm             `json:"norms,omitempty"`
	NextNormID        int                `json:"next_norm_id,omitempty"`
	NextProposalID    int                `json:"next_proposal_id,omitempty"`

	// m is the static generated map for this world (seed + dimensions). It is
	// unexported and never serialized — canonical state bytes are unchanged by
	// it (spec 016: "State marshals unchanged"). It is attached at construction
	// (NewState) and carried into the loop's dry-run probe and across a
	// world.migrated replacement, so miracle reducer arms can consult the
	// terrain vocabulary (passable/buildSite/effectiveKind) the same way live,
	// in the dry-run, and in replay — the reducer stays the single, map-aware
	// validator for the map-dependent miracle checks (spec 016 R1/R4).
	m *worldmap.Map
}

// NewState is genesis: day 1 06:00, default speed, named agents placed
// deterministically on distinct passable tiles (no long-lived RNG stream —
// see rng.go). Needs start imperfect on purpose: day 1 must demand foraging,
// wood, and a fire before the first night (the cold-start drama).
func NewState(seed uint64, m *worldmap.Map) *State {
	s := &State{
		Speed:           clock.DefaultSpeed,
		EffectiveRate:   clock.DefaultSpeed.TicksPerSecond(),
		Seed:            seed,
		Agents:          make([]Agent, agentCount),
		MetatronCharges: MetatronGenesisCharges,
		m:               m,
	}
	pos := genesisPlacement(seed, m, agentCount)
	for i := range s.Agents {
		s.Agents[i] = Agent{
			Name:  AgentNames[i],
			X:     pos[i].X,
			Y:     pos[i].Y,
			Needs: Needs{Health: 1000, Food: 600, Rest: 800, Warmth: 800, Morale: 600},
		}
	}
	return s
}

// genesisPlacement assigns each of count agents a distinct passable tile,
// deterministically from (seed, "genesis", …) with no long-lived RNG stream
// (rng.go). Shared by NewState (cold-start genesis) and MigrateState (spec
// 012 US6: re-placing carried souls on the regenerated v2 map) so the two use
// byte-identical placement — the migration lands villagers exactly where a
// fresh world of that seed would, on passable v2 tiles.
func genesisPlacement(seed uint64, m *worldmap.Map, count int) []Point {
	pos := make([]Point, count)
	taken := map[Point]bool{}
	for i := 0; i < count; i++ {
		for n := int64(0); ; n++ {
			r := rngAt(seed, "genesis", n, i)
			p := Point{X: int(r.Uint64N(uint64(m.W))), Y: int(r.Uint64N(uint64(m.H)))}
			if m.Passable(p.X, p.Y) && !taken[p] {
				taken[p] = true
				pos[i] = p
				break
			}
		}
	}
	return pos
}

// SetMap attaches the static world map to a State built outside NewState —
// the loop's dry-run probe and any replica reconstructed by unmarshalling into
// a bare State. The map is unexported and unserialized, so a State restored
// from bytes alone has none until this is called; miracle reducer arms need it
// for the terrain vocabulary (spec 016). Idempotent, never marshaled.
func (s *State) SetMap(m *worldmap.Map) { s.m = m }

// Marshal renders canonical state bytes (struct field order is fixed, so the
// bytes are deterministic for equal states).
func (s *State) Marshal() []byte {
	b, err := json.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("state marshal: %v", err)) // struct-only fields; cannot fail
	}
	return b
}

func (s *State) Hash() string {
	sum := sha256.Sum256(s.Marshal())
	return hex.EncodeToString(sum[:])
}

// Event payload shapes shared with the substrate. Structs (not maps) so JSON
// bytes are deterministic. Executor payloads live in agents.go.
type (
	WorldCreatedPayload struct {
		Name string `json:"name"`
		Seed uint64 `json:"seed"`
	}
	SpeedSetPayload struct {
		Speed clock.Speed `json:"speed"`
	}
	// GovernorPayload is the recorded arithmetic behind one governor speed change
	// (spec 028 FR-008): the shed/recover decision carries the player's ceiling,
	// the effective speed before and after, and the measured debt + contributing-
	// thought count that justified it — so the event log alone reconstructs every
	// governed interval (SC-005). Shared by clock.governor_shed and
	// clock.governor_recovered; no governor speed change is ever silent.
	GovernorPayload struct {
		Requested clock.Speed `json:"requested"`
		From      clock.Speed `json:"from"`
		To        clock.Speed `json:"to"`
		Debt      float64     `json:"debt"`
		Jobs      int         `json:"jobs"`
	}
	DegradedPayload struct {
		EffectiveRate float64 `json:"effective_rate"`
	}
	DayPayload struct {
		Day int64 `json:"day"`
	}
	AgentMovedPayload struct {
		Agent int `json:"agent"`
		X     int `json:"x"`
		Y     int `json:"y"`
	}
	AgentPayload struct {
		Agent int `json:"agent"`
	}
	DaemonStartedPayload struct {
		Tick       int64 `json:"tick"`
		RecoveryMs int64 `json:"recovery_ms"`
	}
	DaemonStoppedPayload struct {
		Tick int64 `json:"tick"`
	}
	// WorldMigratedPayload (spec 012, US6) carries the full transformed v2 state
	// of a migrated v1 world. Appended once to the fresh v2 log right after
	// world.created; the reducer replaces State wholesale (after validating
	// name/seed), so the log alone reproduces the migrated world with zero
	// snapshots. State is the full canonical sim.State (struct-embedded).
	WorldMigratedPayload struct {
		FromFormat   int   `json:"from_format"`
		SourceEvents int64 `json:"source_events"`
		SourceTick   int64 `json:"source_tick"`
		State        State `json:"state"`
	}
)

func mustPayload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("payload marshal: %v", err))
	}
	return b
}

// --- ground pile helpers (spec 013 US2) ---
//
// One pile per tile is a reducer invariant, so at most one pile ever matches a
// coordinate. These are the create-or-merge / drain-and-remove primitives the
// drop, pick_up, death-spill, and rot cases build on (wired in Phase 4/5/7).

// pileAt returns a pointer to the pile on (x,y), or nil when the tile holds none.
func (s *State) pileAt(x, y int) *Pile {
	for i := range s.Piles {
		if s.Piles[i].X == x && s.Piles[i].Y == y {
			return &s.Piles[i]
		}
	}
	return nil
}

// pileOnOrAdjacent returns the pile on (x,y) or, failing that, on a
// Manhattan-adjacent tile (neighbor order fixed for determinism) — the pile a
// pick_up completion may access (spec US2-AS3: on/adjacent). nil when none.
func (s *State) pileOnOrAdjacent(x, y int) *Pile {
	if p := s.pileAt(x, y); p != nil {
		return p
	}
	for _, d := range neighborOrder {
		if p := s.pileAt(x+d[0], y+d[1]); p != nil {
			return p
		}
	}
	return nil
}

// pileFor returns the pile on (x,y), creating an empty one in creation order
// (slice append) when the tile has none — the create-or-merge target of a drop
// or a death spill.
func (s *State) pileFor(x, y int) *Pile {
	if p := s.pileAt(x, y); p != nil {
		return p
	}
	s.Piles = append(s.Piles, Pile{X: x, Y: y})
	return &s.Piles[len(s.Piles)-1]
}

// removeEmptyPileAt drops the pile on (x,y) when its contents have reached zero,
// preserving the creation order of the survivors (the forage-regrown filter
// pattern). A no-op when the tile has no pile or a non-empty one.
func (s *State) removeEmptyPileAt(x, y int) {
	p := s.pileAt(x, y)
	if p == nil || !p.empty() {
		return
	}
	out := s.Piles[:0]
	for _, q := range s.Piles {
		if !(q.X == x && q.Y == y) {
			out = append(out, q)
		}
	}
	s.Piles = out
	if len(s.Piles) == 0 {
		s.Piles = nil
	}
}

// Apply is the reducer: the only mutation path for event-sourced state, used
// identically by the live loop and by recovery replay. Unknown and daemon.*
// event types are recorded history but no-ops on state.
//
// Tick itself is not event-sourced: quiet ticks (no events) advance the clock
// without a row. Recovery sets Tick = max(snapshot tick, last event tick), so
// at most one quiet stretch (bounded by the snapshot cadence) is re-lived —
// deterministically, since sim events are a pure function of seed + tick.
func (s *State) Apply(e store.Event) error {
	agent := func(p int) (*Agent, error) {
		if p < 0 || p >= len(s.Agents) {
			return nil, fmt.Errorf("apply %s: agent %d out of range", e.Type, p)
		}
		return &s.Agents[p], nil
	}

	switch e.Type {
	case "world.created":
		// Genesis marker; state already reflects genesis.

	case "clock.paused":
		s.Paused = true
	case "clock.resumed":
		s.Paused = false
	case "clock.speed_set":
		var p SpeedSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.Speed = p.Speed
		// A player speed command always collapses governed state (spec 028
		// FR-009): the request becomes the new ceiling AND the effective speed,
		// so any standing governor ceiling is cleared.
		s.RequestedSpeed = ""
		if !s.Degraded {
			s.EffectiveRate = p.Speed.TicksPerSecond()
		}
	case "clock.governor_shed":
		// The governor shed one notch (spec 028 US2): Speed becomes the governed
		// (lower) effective speed and RequestedSpeed records the player's ceiling.
		// EffectiveRate follows the new Speed unless the host is separately
		// reporting a degraded pace (the auto-slow observer owns that fact).
		var p GovernorPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.Speed = p.To
		s.RequestedSpeed = p.Requested
		if !s.Degraded {
			s.EffectiveRate = p.To.TicksPerSecond()
		}
	case "clock.governor_recovered":
		// The governor climbed one notch back (spec 028 US3): Speed becomes the
		// restored effective speed. When it reaches the ceiling the world leaves
		// governed state (RequestedSpeed cleared); otherwise the ceiling stands.
		var p GovernorPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.Speed = p.To
		if p.To != p.Requested {
			s.RequestedSpeed = p.Requested
		} else {
			s.RequestedSpeed = ""
		}
		if !s.Degraded {
			s.EffectiveRate = p.To.TicksPerSecond()
		}
	case "clock.degraded":
		var p DegradedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.Degraded = true
		s.EffectiveRate = p.EffectiveRate
	case "clock.recovered":
		s.Degraded = false
		s.EffectiveRate = s.Speed.TicksPerSecond()

	case "agent.memory_added":
		var p MemoryAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// Spec 019: the situated context (Where/Why/Conv) rides the payload and
		// is copied unchanged onto the reduced Memory — baked at emission, never
		// re-derived, so live and replay agree. Absent fields stay absent
		// (nil/""/0): a pre-019 payload produces a pre-019-shaped Memory (FR-007).
		a.Memories = append(a.Memories, Memory{
			Text: p.Text, Salience: p.Salience, Tick: e.Tick, Subject: p.Subject, Tone: p.Tone,
			Where: p.Where, Why: p.Why, Conv: p.Conv,
		})
		// Cognition horizon (TASK-32): a high-salience stimulus bumps the
		// agent's generation — in-flight thoughts snapshotted under the old
		// generation are superseded at landing (FR-014). The salience table
		// is the definition of "high": near-death (9), witnessed death (10),
		// exile (9); dreams (8) deliberately do not interrupt thought.
		if p.Salience >= GenerationBumpSalience {
			a.Generation++
		}
	case "journal.entry_written":
		// Spec 019 (US3): the ONLY journal-growth path. The budget rule lives
		// here in Apply — appendEntry returns an error when the write would
		// exceed it, and the InjectSocial dry-run turns that error into a door
		// rejection, so no over-budget event ever lands (Principle III, SC-005).
		var p JournalWrittenPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if a.Journal == nil {
			a.Journal = &Journal{}
		}
		return a.Journal.appendEntry(e.Tick, p.Text)
	case "journal.entry_deleted":
		var p JournalDeletedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if a.Journal == nil {
			return fmt.Errorf("no journal entry #%d", p.Entry)
		}
		return a.Journal.deleteEntry(p.Entry)
	case "agent.thought":
		// Chronicle material; no state effect.
	case "cog.thought", "cog.outcome", "cog.recalibration_recommended",
		"agent.intent_rejected":
		// Cognition-horizon telemetry (TASK-32): recorded observability,
		// no state effect.
	case "cog.tool_call":
		// The tool-use loop's call trace (spec 017): recorded observability,
		// no state effect — same as the cog.* arm above.

	case "agent.plan_set":
		var p PlanSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Plan = append([]PlanStep(nil), p.Steps...)
	case "agent.plan_step_started":
		var p PlanStepPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if len(a.Plan) > 0 {
			a.Plan = a.Plan[1:]
		}
		if len(a.Plan) == 0 {
			a.Plan = nil
		}
	case "agent.plan_expired":
		var p PlanStepPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// v1 semantics: a broken sequence is not resumed — the whole
		// remaining plan clears and the reflex floor covers.
		a.Plan = nil

	case "sim.night_started":
		s.Night = true
	case "sim.day_started":
		s.Night = false
	case "sim.forage_regrown":
		var p RegrownPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		out := s.Harvested[:0]
		for _, h := range s.Harvested {
			if !(h.X == p.X && h.Y == p.Y) {
				out = append(out, h)
			}
		}
		s.Harvested = out

	case "agent.intent_set":
		var p IntentSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Intent = &Intent{Goal: p.Goal, TargetX: p.TargetX, TargetY: p.TargetY, ResX: p.ResX, ResY: p.ResY, Kind: p.Kind, Qty: p.Qty, Reason: p.Reason}
		// TASK-56: remember the objective past its own lifetime (never
		// cleared) so the villagers tab can show it while idle.
		a.LastGoal = p.Goal
		a.LastGoalTick = e.Tick
	case "agent.work_started":
		var p WorkStartedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if a.Intent != nil {
			a.Intent.WorkStart = p.Tick
		}
	case "agent.intent_done":
		var p AgentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Intent = nil
		a.IdleSince = e.Tick

	case "agent.moved":
		var p AgentMovedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.X, a.Y = p.X, p.Y

	case "agent.foraged":
		var p HarvestPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// US1-AS2 (T010): the yield truncates to the taker's free bulk; the
		// remainder is forfeit, but the overlay/depletion still applies (the
		// forage tile is marked harvested regardless). A full pouch never
		// reaches here — the executor emits intent_done only at zero space
		// (T011), so depletion-at-zero-space never occurs.
		a.Inv.FoodRaw += minInt(forageYieldV2, freeBulk(a.Inv))
		a.Intent = nil
		a.IdleSince = e.Tick
		s.Harvested = append(s.Harvested, Harvest{X: p.X, Y: p.Y, Regrow: e.Tick + forageRegrowSec})
	case "agent.chopped":
		var p HarvestPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// US1-AS2 (T010): yield truncates to free bulk; the tree still clears.
		a.Inv.Wood += minInt(chopWood, freeBulk(a.Inv))
		a.Intent = nil
		a.IdleSince = e.Tick
		s.Cleared = append(s.Cleared, Point{X: p.X, Y: p.Y})
	case "agent.hunted":
		var p HarvestPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// T027: spear-aware hunting — carrying a spear yields huntYieldSpear
		// (12) and spends the most-worn spear's (Spears[0]) last use. This is
		// re-derived from the SAME pre-mutation state the emitter checked
		// (contracts/events.md: "no new types" for this — yield/spend are
		// state-derived, not payload-carried). A companion agent.spear_broke,
		// if any, applies right after this in the same batch and removes the
		// now-zero spear.
		// US1-AS2 (T010): the food yield truncates to pre-event free bulk; the
		// spear spend frees no bulk mid-event (decrementing a use leaves
		// len(Spears) — and thus bulk — unchanged), and the spear's removal on
		// break rides its own companion agent.spear_broke. So free space is
		// read once, before the food is added, with the spear still counted.
		yield := huntYieldBare
		if len(a.Inv.Spears) > 0 {
			yield = huntYieldSpear
		}
		a.Inv.FoodRaw += minInt(yield, freeBulk(a.Inv))
		if len(a.Inv.Spears) > 0 {
			a.Inv.Spears[0]--
		}
		a.Intent = nil
		a.IdleSince = e.Tick
		out := s.DenUses[:0]
		for _, d := range s.DenUses {
			if !(d.X == p.X && d.Y == p.Y) {
				out = append(out, d)
			}
		}
		s.DenUses = append(out, DenUse{X: p.X, Y: p.Y, Ready: e.Tick + denCooldownSec})
	case "agent.built":
		var p BuiltPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// T030/T036: costs come from the recipes table (the single source) by
		// its "build_<kind>" goal — shelter (planks, re-costed from wood) and
		// oven (refined stone + planks) both fall out of this the same way
		// fire always has.
		if r, ok := recipeFor("build_" + p.Kind); ok {
			addItems(&a.Inv, r.Inputs, -1)
		}
		a.Intent = nil
		a.IdleSince = e.Tick
		st := Structure{Kind: p.Kind, X: p.X, Y: p.Y}
		if p.Kind == "fire" {
			// T019: a fresh fire (2 wood) burns 2×fireBurnPerWood from now; the
			// fuel sweep burns it out and refuel pushes FuelUntil forward.
			st.FuelUntil = e.Tick + 2*fireBurnPerWood
		}
		if p.Kind == "chest" {
			// T023 (spec 013 US3, research R8): the builder is recorded as owner
			// permanently (no transfer/inheritance in v1), and the chest gets an
			// empty Store for its contents. The recipe delta above already spent
			// the chestPlankCost planks (recipes.go stays the single source).
			st.Owner = p.Agent
			st.Store = &Inventory{}
		}
		if isWall(p.Kind) {
			// T008 (spec 032 US1, research R1): a fresh wall stands at full health,
			// derived from its kind (never stored as a separate max — fire lit-ness
			// doctrine). The recipe delta above already spent the wall's material.
			st.HP = wallMaxHP(p.Kind)
		}
		s.Structures = append(s.Structures, st)
	case "agent.ate":
		// T018: outcome-payload eat — the emitter computed the
		// most-nutritious-first consumption (Meals → FoodCooked → FoodRaw) to
		// satiety; the reducer decrements the counts and sets the absolute
		// post-eat food need. No arithmetic that could drift on replay.
		var p AtePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Inv.Meals = maxInt(0, a.Inv.Meals-p.Meals)
		a.Inv.FoodCooked = maxInt(0, a.Inv.FoodCooked-p.Cooked)
		a.Inv.FoodRaw = maxInt(0, a.Inv.FoodRaw-p.Raw)
		a.Needs.Food = p.FoodAfter

	// --- spec 012 resources/food/crafting v2 event surface ---
	case "agent.quarried":
		var p HarvestPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// US1-AS2 (T010): yield truncates to free bulk; the outcrop still depletes.
		a.Inv.Stone += minInt(quarryYield, freeBulk(a.Inv))
		a.Intent = nil
		a.IdleSince = e.Tick
		s.Quarried = append(s.Quarried, Point{X: p.X, Y: p.Y})
	case "agent.collected_water":
		var p HarvestPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// US1-AS2 (T010): yield truncates to free bulk (water sources never deplete).
		a.Inv.Water += minInt(collectWaterYield, freeBulk(a.Inv))
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.crafted":
		// T026: recipe delta re-derived from recipes.go by the crafted kind's
		// goal (recipes.go stays the single source — no duplicated numbers
		// here). Spear durability doesn't fit a plain int field: a fresh
		// spear (spearDurability uses) is appended to Spears, kept sorted
		// ascending so hunts always spend the most-worn spear first.
		var p CraftedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		goal := craftGoalFor(p.Kind)
		r, ok := recipeFor(goal)
		if !ok {
			return fmt.Errorf("apply %s: no recipe for crafted kind %q", e.Type, p.Kind)
		}
		addItems(&a.Inv, r.Inputs, -1)
		if p.Kind == "spear" {
			a.Inv.Spears = append(a.Inv.Spears, spearDurability)
			sort.Ints(a.Inv.Spears)
		} else {
			addItems(&a.Inv, r.Outputs, 1)
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.cooked":
		// T021: a cook batch. FoodRaw -= consumed; the produced kind gains the
		// same count (fire → food_cooked; oven → meals, wired in Phase 6). The
		// oven also burns 1 Wood (T031). Station lit-ness was re-validated by
		// the emitter (contested pattern) — the reducer applies the outcome.
		var p CookedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Inv.FoodRaw = maxInt(0, a.Inv.FoodRaw-p.Consumed)
		switch p.Kind {
		case "food_cooked":
			a.Inv.FoodCooked += p.Produced
		case "meals":
			a.Inv.Meals += p.Produced
		}
		if p.Station == "oven" {
			a.Inv.Wood = maxInt(0, a.Inv.Wood-1)
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.bathed":
		// T032: water's only v1 consumer. Morale/Warmth are absolute post-cap
		// values (gru-pattern) — the emitter already applied the cap, so the
		// reducer never recomputes arithmetic that could drift on replay.
		var p BathedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Inv.Water = maxInt(0, a.Inv.Water-1)
		a.Inv.Wood = maxInt(0, a.Inv.Wood-1)
		a.Needs.Morale = p.MoraleAfter
		a.Needs.Warmth = p.WarmthAfter
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.refueled":
		// T020: 1 Wood spent; the fire's FuelUntil is set to the absolute
		// deadline the emitter already capped. Relighting a cold fire is the
		// same assignment (lit-ness is derived from FuelUntil vs tick).
		var p RefueledPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Inv.Wood = maxInt(0, a.Inv.Wood-1)
		for i := range s.Structures {
			st := &s.Structures[i]
			if st.Kind == "fire" && st.X == p.X && st.Y == p.Y {
				st.FuelUntil = p.FuelUntil
				break
			}
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.spear_broke":
		// T027: the batch's preceding agent.hunted already decremented
		// Spears[0] to zero (spent its last use) — this event just removes
		// the now-empty entry. The companion memory rides alongside as its
		// own agent.memory_added event, not part of this payload.
		var p SpearBrokePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if len(a.Inv.Spears) > 0 {
			a.Inv.Spears = a.Inv.Spears[1:]
		}
	case "sim.fire_burned_out":
		// T019: no state effect — lit-ness is derived from FuelUntil; the event
		// is the once-per-burnout chronicle/TUI signal (the sweep emits it).

	// --- spec 032 walls: multi-cycle demolish / repair ---
	case "agent.wall_chipped":
		// T008 (spec 032 US1, research R5): one demolish cycle chips the wall's
		// health, then re-arms the executor's work gate (Intent.WorkStart = 0) so
		// the next cycle runs under the same intent — the whole multi-cycle loop,
		// no new scheduling. The executor only emits this when HP − chip ≥ 1, so
		// the wall stays standing; the ≥1 clamp defends the invariant that a
		// standing wall never serializes ≤ 0 (data-model.md).
		var p WallWorkPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if w := wallAt(s, p.X, p.Y); w != nil {
			w.HP -= demolishChipHP
			if w.HP < 1 {
				w.HP = 1
			}
		}
		if a.Intent != nil {
			a.Intent.WorkStart = 0
		}
	case "agent.wall_destroyed":
		// T008: the final demolish cycle (or a chip that would reach ≤ 0) removes
		// the wall — its tile is passable again by construction — and clears the
		// demolisher's intent. Metatron's entity_removed reaches the same end
		// through the miracle path (contracts/events.md).
		var p WallWorkPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if i := s.structureIndexAt(p.X, p.Y); i >= 0 && isWall(s.Structures[i].Kind) {
			s.removeStructureAt(i)
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.wall_repaired":
		// T008 (spec 032 US1, research R5): one repair cycle consumes 1 matching
		// material and restores HP up to (never past) the derived max. If the wall
		// is still damaged AND material remains the work gate re-arms for another
		// cycle (WorkStart = 0, intent kept); otherwise the intent clears. The
		// emitter validated a damaged wall + material; the reducer clamps
		// defensively so replay never drifts.
		var p WallWorkPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if w := wallAt(s, p.X, p.Y); w != nil {
			mat := wallRepairMaterial(w.Kind)
			maxHP := wallMaxHP(w.Kind)
			if invField(a.Inv, mat) >= 1 {
				addItems(&a.Inv, []Item{{mat, 1}}, -1)
			}
			w.HP = minInt(maxHP, w.HP+repairHPPerUnit)
			if w.HP < maxHP && invField(a.Inv, mat) >= 1 {
				if a.Intent != nil {
					a.Intent.WorkStart = 0
				}
				break // still damaged with material in hand — keep repairing
			}
		}
		// Fully mended, out of material, or the wall vanished: the work is done.
		a.Intent = nil
		a.IdleSince = e.Tick

	// --- spec 013 inventory/storage v1 event surface ---
	case "agent.dropped":
		// T016 (spec 013 US2): move the recorded count of Kind from inventory
		// to the tile's pile (create-or-merge). The payload carries the actual
		// post-clamp count; the reducer clamps defensively to what is carried
		// and stays total (a missing kind ⇒ no-op, intent still cleared).
		var p DroppedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if p.Kind == "spears" {
			n := p.N
			if n > len(a.Inv.Spears) {
				n = len(a.Inv.Spears)
			}
			if n > 0 {
				pile := s.pileFor(p.X, p.Y)
				// Most-worn-first: the front of the agent's ascending slice
				// moves; both sides stay sorted ascending.
				pile.Spears = append(pile.Spears, a.Inv.Spears[:n]...)
				sort.Ints(pile.Spears)
				rest := append([]int(nil), a.Inv.Spears[n:]...)
				if len(rest) == 0 {
					a.Inv.Spears = nil
				} else {
					a.Inv.Spears = rest
				}
			}
		} else if n := p.N; n > 0 {
			if c := carriedCount(a.Inv, p.Kind); n > c {
				n = c
			}
			if n > 0 {
				pile := s.pileFor(p.X, p.Y)
				if isFoodKind(p.Kind) {
					pile.addFood(p.Kind, n, e.Tick+rotWindowTicks)
				} else {
					pile.addNonFood(p.Kind, n)
				}
				addItems(&a.Inv, []Item{{p.Kind, n}}, -1)
			}
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.picked_up":
		// T017 (spec 013 US2): move the recorded count of Kind from the tile's
		// pile into inventory (food drained oldest-batch-first), clamped
		// defensively to free bulk and to what the pile holds (contested
		// re-validation: a second same-tick taker finds only the remainder).
		// An emptied pile is removed; intent is cleared.
		var p PickedUpPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if pile := s.pileAt(p.X, p.Y); pile != nil {
			n := p.N
			if f := freeBulk(a.Inv); n > f {
				n = f
			}
			switch {
			case p.Kind == "spears":
				if taken := pile.takeSpears(n); len(taken) > 0 {
					a.Inv.Spears = append(a.Inv.Spears, taken...)
					sort.Ints(a.Inv.Spears)
				}
			case isFoodKind(p.Kind):
				addItems(&a.Inv, []Item{{p.Kind, pile.takeFood(p.Kind, n)}}, 1)
			default:
				addItems(&a.Inv, []Item{{p.Kind, pile.takeNonFood(p.Kind, n)}}, 1)
			}
			s.removeEmptyPileAt(p.X, p.Y)
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.deposited":
		// T024 (spec 013 US3): move the recorded count of Kind from inventory into
		// the chest's Store (chest food is plain counts — NO rot batches, FR-010).
		// The payload carries the actual post-clamp count; the reducer clamps
		// defensively to what is carried AND to the chest's free space, and stays
		// total (missing chest/Store ⇒ no-op, intent still cleared).
		var p DepositedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if ch := s.chestAt(p.X, p.Y); ch != nil && ch.Store != nil {
			free := chestCap - bulk(*ch.Store)
			if p.Kind == "spears" {
				n := p.N
				if n > len(a.Inv.Spears) {
					n = len(a.Inv.Spears)
				}
				if n > free {
					n = free
				}
				if n > 0 {
					// Most-worn-first: the front of the ascending slice moves;
					// both sides stay sorted ascending.
					ch.Store.Spears = append(ch.Store.Spears, a.Inv.Spears[:n]...)
					sort.Ints(ch.Store.Spears)
					rest := append([]int(nil), a.Inv.Spears[n:]...)
					if len(rest) == 0 {
						a.Inv.Spears = nil
					} else {
						a.Inv.Spears = rest
					}
				}
			} else if n := p.N; n > 0 {
				if c := carriedCount(a.Inv, p.Kind); n > c {
					n = c
				}
				if n > free {
					n = free
				}
				if n > 0 {
					addItems(ch.Store, []Item{{p.Kind, n}}, 1)
					addItems(&a.Inv, []Item{{p.Kind, n}}, -1)
				}
			}
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "agent.withdrew":
		// T024 (spec 013 US3): move the recorded count of Kind from the chest's
		// Store into inventory, clamped defensively to the taker's free bulk and to
		// what the chest holds. Spears leave most-worn-first carrying their
		// durabilities; both slices stay sorted ascending. Chest food is plain
		// counts (no batches). The Owner field feeds the US4 theft companion batch
		// (T029); this case only moves goods. Missing chest/Store ⇒ no-op.
		var p WithdrewPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		if ch := s.chestAt(p.X, p.Y); ch != nil && ch.Store != nil {
			n := p.N
			if f := freeBulk(a.Inv); n > f {
				n = f
			}
			if p.Kind == "spears" {
				if n > len(ch.Store.Spears) {
					n = len(ch.Store.Spears)
				}
				if n > 0 {
					taken := append([]int(nil), ch.Store.Spears[:n]...)
					rest := append([]int(nil), ch.Store.Spears[n:]...)
					if len(rest) == 0 {
						ch.Store.Spears = nil
					} else {
						ch.Store.Spears = rest
					}
					a.Inv.Spears = append(a.Inv.Spears, taken...)
					sort.Ints(a.Inv.Spears)
				}
			} else if n > 0 {
				if c := carriedCount(*ch.Store, p.Kind); n > c {
					n = c
				}
				if n > 0 {
					addItems(ch.Store, []Item{{p.Kind, n}}, -1)
					addItems(&a.Inv, []Item{{p.Kind, n}}, 1)
				}
			}
		}
		a.Intent = nil
		a.IdleSince = e.Tick
	case "sim.food_rotted":
		// T032 (spec 013 US5, FR-013): remove the pile's spoiled food batches —
		// batches whose SpoilAt has arrived (<= event tick) matching Kind, up to
		// the recorded N, draining oldest batches first (drop order). The reducer
		// stays total: it clamps to the spoiled units that remain (a same-tick
		// pickup that applied first drained the oldest batch, so this finds only
		// the remainder — the contested re-validation idiom), and an absent
		// pile/batch is a no-op. An emptied pile is removed in the same
		// application. Chest food never batches, so it is never reached (FR-010).
		var p FoodRottedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if pile := s.pileAt(p.X, p.Y); pile != nil {
			pile.takeSpoiled(p.Kind, p.N, e.Tick)
			s.removeEmptyPileAt(p.X, p.Y)
		}

	case "world.migrated":
		// T038 (spec 012 US6): the format-break migration event carries the FULL
		// transformed v2 state (research R10) — the reducer replaces State
		// wholesale so the log alone (world.created → world.migrated, zero
		// snapshots) reproduces the migrated world byte-identically. v1 events
		// are never replayed under v2 rules; this single event is the whole
		// history-before-the-break, distilled to one canonical state.
		var p WorldMigratedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		// Seed is the world's identity carried in State (Name lives in the
		// manifest, not State — the migrate command stamps it on the preceding
		// world.created and preserves the seed through the transform). A payload
		// whose seed disagrees with the world being replayed is a foreign
		// migration record: no-op, keeping the reducer total (never errors on a
		// well-formed but mismatched event, exactly like the contested-resource
		// completions).
		if p.State.Seed != s.Seed {
			return nil
		}
		// The payload's State was unmarshalled from the event and carries no
		// map (unexported, unserialized); preserve the map already attached to
		// the live/replay State across the wholesale replacement (spec 016).
		m := s.m
		*s = p.State
		s.m = m

	case "agent.slept":
		var p AgentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Asleep = true
		a.Intent = nil
		// A sleeper sheds any hail (TASK-47): un-interruptible, and leaving
		// the pointer set would silently freeze it on waking.
		a.Hail = nil
	case "agent.woke":
		var p AgentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Asleep = false
		a.IdleSince = e.Tick

	case "agent.needs_changed":
		var p NeedsPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Needs = Needs{Health: p.Health, Food: p.Food, Rest: p.Rest, Warmth: p.Warmth, Morale: p.Morale}
		if p.Health < nearDeathBelow {
			a.NearDeath = true
		} else if p.Health >= nearDeathResetAt {
			a.NearDeath = false
		}
	case "agent.died":
		var p DiedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Dead = true
		a.Asleep = false
		a.Intent = nil
		// The dead shed any hail (TASK-47); the hailer's seek proceeds or
		// fails exactly as today.
		a.Hail = nil
		// T018 (spec 013 US2, FR-006, research R7): the agent's entire carried
		// inventory spills as a ground pile at the death tile (create-or-merge),
		// food batches stamped with a fresh rot deadline, spears riding along
		// with their durabilities. Reducer-internal — no new event. An agent
		// carrying nothing leaves no pile.
		if bulk(a.Inv) > 0 {
			pile := s.pileFor(a.X, a.Y)
			pile.addNonFood("wood", a.Inv.Wood)
			pile.addNonFood("stone", a.Inv.Stone)
			pile.addNonFood("water", a.Inv.Water)
			pile.addNonFood("planks", a.Inv.Planks)
			pile.addNonFood("refined_stone", a.Inv.RefinedStone)
			pile.addFood("food_raw", a.Inv.FoodRaw, e.Tick+rotWindowTicks)
			pile.addFood("food_cooked", a.Inv.FoodCooked, e.Tick+rotWindowTicks)
			pile.addFood("meals", a.Inv.Meals, e.Tick+rotWindowTicks)
			if len(a.Inv.Spears) > 0 {
				pile.Spears = append(pile.Spears, a.Inv.Spears...)
				sort.Ints(pile.Spears)
			}
			a.Inv = Inventory{}
		}

	case "social.hailed":
		var p HailedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.To)
		if err != nil {
			return err
		}
		// Emitters never hail an already-hailed target (first-hail-wins); the
		// reducer applies what the log says — replay fidelity over
		// re-validation, like every other event.
		a.Hail = &AgentHail{By: p.From, Until: p.Until}
	case "social.hail_met":
		var p HailMetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.To)
		if err != nil {
			return err
		}
		// The accompanying agent.talked applies its own morale/LastTalk
		// effects; the sweep just lifts the pause.
		a.Hail = nil
	case "social.hail_expired":
		var p HailExpiredPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.To)
		if err != nil {
			return err
		}
		// Nothing else changes: intent, plan, and needs are exactly as the
		// pause left them (FR-005, SC-003).
		a.Hail = nil

	case "social.relation_changed", "social.gave", "social.promise_broken",
		"social.rumor_told", "social.secret_seeded",
		"social.conversation_turn", "social.conversation", "social.chest_taken":
		return s.applySocial(e)

	case "agent.memory_promoted", "agent.memory_faded", "agent.belief_revised",
		"agent.narrative_set", "agent.consolidated":
		return s.applyConsolidation(e)

	case "gru.emerged", "gru.moved", "gru.sighted", "gru.attacked", "gru.withdrew":
		return s.applyGru(e)

	case "chronicle.entry":
		return s.applyChronicle(e)

	case "metatron.charge_regenerated", "metatron.nudged":
		return s.applyMetatron(e)

	case "metatron.time_snapped", "metatron.item_granted",
		"metatron.entity_moved", "metatron.entity_removed":
		return s.applyMiracle(e)

	case "meeting.convention_established", "sim.gathering_observed",
		"meeting.place_designated", "meeting.convened", "meeting.opened",
		"meeting.turn_taken", "meeting.proposal_tabled", "meeting.proposal_resolved",
		"meeting.proposal_rephrased", "meeting.closed", "norm.violated":
		return s.applyGovernance(e)

	case "agent.talked":
		var p TalkedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.A)
		if err != nil {
			return err
		}
		b, err := agent(p.B)
		if err != nil {
			return err
		}
		a.Needs.Morale = minInt(1000, a.Needs.Morale+talkMoraleBonus)
		b.Needs.Morale = minInt(1000, b.Needs.Morale+talkMoraleBonus)
		a.LastTalk, b.LastTalk = e.Tick, e.Tick
	}
	return nil
}

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

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// State is the whole reducer state: clock state + agents + the dynamic
// overlays on the static terrain (structures, cleared trees, harvested
// forage, den cooldowns). It marshals to canonical bytes (fixed struct field
// order) for snapshots and determinism hashing. Wall-clock time never
// appears here.
type State struct {
	Tick          int64       `json:"tick"`
	Paused        bool        `json:"paused"`
	Speed         clock.Speed `json:"speed"`
	Degraded      bool        `json:"degraded"`
	EffectiveRate float64     `json:"effective_rate"`
	Seed          uint64      `json:"seed"`
	Night         bool        `json:"night"`
	Agents        []Agent     `json:"agents"`
	Structures    []Structure `json:"structures,omitempty"`
	Cleared       []Point     `json:"cleared,omitempty"`
	Harvested     []Harvest   `json:"harvested,omitempty"`
	DenUses       []DenUse    `json:"den_uses,omitempty"`
	// Quarried (spec 012, US1) marks depleted rock outcrops — permanent in
	// v1, no regrow entry (unlike Harvested/Cleared, which do regrow). A
	// quarried tile is passable but NOT buildable and NOT quarryable again;
	// effectiveKind renders it as worldmap.Depleted, distinct from Grass.
	Quarried []Point `json:"quarried,omitempty"`
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
		if !s.Degraded {
			s.EffectiveRate = p.Speed.TicksPerSecond()
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
		a.Memories = append(a.Memories, Memory{Text: p.Text, Salience: p.Salience, Tick: e.Tick, Subject: p.Subject, Tone: p.Tone})
		// Cognition horizon (TASK-32): a high-salience stimulus bumps the
		// agent's generation — in-flight thoughts snapshotted under the old
		// generation are superseded at landing (FR-014). The salience table
		// is the definition of "high": near-death (9), witnessed death (10),
		// exile (9); dreams (8) deliberately do not interrupt thought.
		if p.Salience >= GenerationBumpSalience {
			a.Generation++
		}
	case "agent.thought":
		// Chronicle material; no state effect.
	case "cog.thought", "cog.outcome", "cog.recalibration_recommended",
		"agent.intent_rejected":
		// Cognition-horizon telemetry (TASK-32): recorded observability,
		// no state effect.

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
		a.Intent = &Intent{Goal: p.Goal, TargetX: p.TargetX, TargetY: p.TargetY, ResX: p.ResX, ResY: p.ResY}
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
		a.Inv.FoodRaw += forageYieldV2
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
		a.Inv.Wood += chopWood
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
		if len(a.Inv.Spears) > 0 {
			a.Inv.FoodRaw += huntYieldSpear
			a.Inv.Spears[0]--
		} else {
			a.Inv.FoodRaw += huntYieldBare
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
	// Registered as explicit no-ops in Phase 2 so each later story fills its own
	// case without merge collisions, and so the reducer documents the v2 event
	// vocabulary. Until wired, behavior is identical to the unknown-type
	// fall-through: recorded history, zero state effect (contracts/events.md).
	case "agent.quarried":
		var p HarvestPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		a.Inv.Stone += quarryYield
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
		a.Inv.Water += collectWaterYield
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
		*s = p.State

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
		"social.conversation_turn", "social.conversation":
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

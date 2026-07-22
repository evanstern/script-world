// Package sim owns the deterministic simulation: world state, the event
// reducer (the single mutation path, used identically live and in replay),
// and the fixed-timestep loop.
package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

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
	taken := map[Point]bool{}
	for i := range s.Agents {
		for n := int64(0); ; n++ {
			r := rngAt(seed, "genesis", n, i)
			x, y := int(r.Uint64N(uint64(m.W))), int(r.Uint64N(uint64(m.H)))
			if m.Passable(x, y) && !taken[Point{X: x, Y: y}] {
				taken[Point{X: x, Y: y}] = true
				s.Agents[i] = Agent{
					Name:  AgentNames[i],
					X:     x,
					Y:     y,
					Needs: Needs{Health: 1000, Food: 600, Rest: 800, Warmth: 800, Morale: 600},
				}
				break
			}
		}
	}
	return s
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
		// TODO(T017): rescale to +forageYieldV2 (2) FoodRaw. Phase 2 keeps the
		// legacy yield, only re-expressed over FoodRaw (behavior-equivalent).
		a.Inv.FoodRaw += forageYield
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
		// TODO(T017/T027): rescale to +huntYieldBare (8) / +huntYieldSpear (12)
		// and spend Spears[0]. Phase 2 keeps the legacy yield over FoodRaw.
		a.Inv.FoodRaw += huntYield
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
		cost := fireWoodCost
		if p.Kind == "shelter" {
			cost = shelterWoodCost
		}
		a.Inv.Wood = maxInt(0, a.Inv.Wood-cost)
		a.Intent = nil
		a.IdleSince = e.Tick
		s.Structures = append(s.Structures, Structure{Kind: p.Kind, X: p.X, Y: p.Y})
	case "agent.ate":
		var p AgentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		a, err := agent(p.Agent)
		if err != nil {
			return err
		}
		// TODO(T018): rewrite to AtePayload (most-nutritious-first to satietyAt,
		// absolute food_after). Phase 2 keeps the legacy one-unit +eatFoodValue
		// eat, re-expressed over FoodRaw so behavior is unchanged.
		if a.Inv.FoodRaw > 0 {
			a.Inv.FoodRaw--
			a.Needs.Food = minInt(1000, a.Needs.Food+eatFoodValue)
		}

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
		// TODO(T026): apply recipeFor(kind) delta (spear appends spearDurability); clear intent.
	case "agent.cooked":
		// TODO(T021/T031): FoodRaw -= consumed; Kind += produced; oven also Wood -= 1; clear intent.
	case "agent.bathed":
		// TODO(T032): Water -1, Wood -1; set Morale/Warmth to the absolute after-values.
	case "agent.refueled":
		// TODO(T020): Wood -1; set the fire's FuelUntil to the absolute deadline (relights if cold).
	case "agent.spear_broke":
		// TODO(T027): remove Spears[0] (spent to zero); companion memory rides the same batch.
	case "sim.fire_burned_out":
		// TODO(T019): no state effect — lit-ness is derived from FuelUntil; chronicle/TUI signal only.
	case "world.migrated":
		// TODO(T038): validate name/seed match, then replace State wholesale from the payload.

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

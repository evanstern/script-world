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
	// Social fabric (TASK-8) — all event-sourced.
	Relations   []Relation `json:"relations,omitempty"`
	Debts       []Debt     `json:"debts,omitempty"`
	Rumors      []Rumor    `json:"rumors,omitempty"`
	NextDebtID  int        `json:"next_debt_id,omitempty"`
	NextRumorID int        `json:"next_rumor_id,omitempty"`
	// Conversation records (TASK-22) — bounded ring, event-sourced.
	Conversations []ConvoRecord `json:"conversations,omitempty"`
}

// NewState is genesis: day 1 06:00, default speed, named agents placed
// deterministically on distinct passable tiles (no long-lived RNG stream —
// see rng.go). Needs start imperfect on purpose: day 1 must demand foraging,
// wood, and a fire before the first night (the cold-start drama).
func NewState(seed uint64, m *worldmap.Map) *State {
	s := &State{
		Speed:         clock.DefaultSpeed,
		EffectiveRate: clock.DefaultSpeed.TicksPerSecond(),
		Seed:          seed,
		Agents:        make([]Agent, agentCount),
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
	case "agent.thought":
		// Chronicle material; no state effect.

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
		a.Inv.Food += forageYield
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
		a.Inv.Food += huntYield
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
		if a.Inv.Food > 0 {
			a.Inv.Food--
			a.Needs.Food = minInt(1000, a.Needs.Food+eatFoodValue)
		}

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
	case "social.relation_changed", "social.gave", "social.promise_broken",
		"social.rumor_told", "social.secret_seeded",
		"social.conversation_turn", "social.conversation":
		return s.applySocial(e)

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

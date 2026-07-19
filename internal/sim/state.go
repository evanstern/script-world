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
)

type Wanderer struct {
	X      int  `json:"x"`
	Y      int  `json:"y"`
	Asleep bool `json:"asleep"`
}

// State is the whole reducer state: clock state + placeholder sim state.
// It marshals to canonical bytes (fixed struct field order) for snapshots and
// determinism hashing. Wall-clock time never appears here.
type State struct {
	Tick          int64       `json:"tick"`
	Paused        bool        `json:"paused"`
	Speed         clock.Speed `json:"speed"`
	Degraded      bool        `json:"degraded"`
	EffectiveRate float64     `json:"effective_rate"`
	Seed          uint64      `json:"seed"`
	Night         bool        `json:"night"`
	Wanderers     []Wanderer  `json:"wanderers"`
}

// NewState is genesis: day 1 06:00, default speed, wanderer positions derived
// from the seed (no long-lived RNG stream — see rng.go).
func NewState(seed uint64) *State {
	s := &State{
		Speed:         clock.DefaultSpeed,
		EffectiveRate: clock.DefaultSpeed.TicksPerSecond(),
		Seed:          seed,
		Wanderers:     make([]Wanderer, wandererCount),
	}
	for i := range s.Wanderers {
		r := rngAt(seed, "genesis", 0, i)
		s.Wanderers[i] = Wanderer{X: int(r.Uint64N(gridSize)), Y: int(r.Uint64N(gridSize))}
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

// Event payload shapes. Structs (not maps) so JSON bytes are deterministic.
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
	case "sim.night_started":
		s.Night = true
	case "sim.day_started":
		s.Night = false
		for i := range s.Wanderers {
			s.Wanderers[i].Asleep = false
		}
	case "agent.moved":
		var p AgentMovedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if p.Agent < 0 || p.Agent >= len(s.Wanderers) {
			return fmt.Errorf("apply %s: agent %d out of range", e.Type, p.Agent)
		}
		s.Wanderers[p.Agent].X, s.Wanderers[p.Agent].Y = p.X, p.Y
	case "agent.slept":
		var p AgentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if p.Agent < 0 || p.Agent >= len(s.Wanderers) {
			return fmt.Errorf("apply %s: agent %d out of range", e.Type, p.Agent)
		}
		s.Wanderers[p.Agent].Asleep = true
	}
	return nil
}

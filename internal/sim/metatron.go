package sim

import (
	"encoding/json"
	"fmt"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/tool"
)

// Metatron's world-visible surface (TASK-12): the charge economy and the
// nudge event. Everything else about the angel (charter, soul, console)
// lives outside deterministic space; only recorded events reach here.

const (
	// chargeRegenTicks: one charge per 6 game hours, at absolute boundaries
	// (multiples of 21600 ticks) — a pure function of the clock.
	chargeRegenTicks = 6 * 3600
	// MetatronChargeCap bounds the bank; MetatronGenesisCharges is day-1
	// grace (a reign begins with one favor).
	MetatronChargeCap      = 3
	MetatronGenesisCharges = 1
)

// NudgeTextMax caps the villager-bound rendering — read from the tool registry
// (spec 014 T021/R7; re-pointed at send_vision when spec 029 retired the nudges):
// the influence tools' TextCapBytes (400). The reducer dry-run stays the
// enforcer; the registry is the single source of the cap, so the enforcer and
// the metatron-side truncation can never carry divergent literals.
var NudgeTextMax = func() int {
	t, _ := tool.Lookup("send_vision")
	return t.Cost.TextCapBytes
}()

type (
	// MetatronNudgedPayload is the injected spend + record: form "dream"
	// (exactly one living target) or "omen" (every villager alive at
	// landing, recorded explicitly).
	MetatronNudgedPayload struct {
		Form    string `json:"form"`
		Targets []int  `json:"targets"`
		Text    string `json:"text"`
	}
	// ChargeRegeneratedPayload is empty — the event row's tick is the
	// boundary crossed.
	ChargeRegeneratedPayload struct{}
)

// applyMetatron is the reducer arm for metatron.* events. The nudged arm
// validates rather than clamps: the InjectSocial dry-run runs this on a
// state copy, so invalid spends are rejected at the door and recorded
// events always re-apply cleanly at the same position in replay.
func (s *State) applyMetatron(e store.Event) error {
	switch e.Type {
	case "metatron.charge_regenerated":
		if s.MetatronCharges < MetatronChargeCap {
			s.MetatronCharges++
		}
	case "metatron.nudged":
		var p MetatronNudgedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if s.MetatronCharges <= 0 {
			return fmt.Errorf("apply %s: no charges banked", e.Type)
		}
		// Form validation (spec 029): the metatron.nudged form domain is
		// {vision, omen, dream}. A vision reaches exactly one living villager at
		// any hour; an omen reaches ≥1 living villagers and lands ONLY at night
		// (State.Night); dream is the RETIRED legacy form, grandfathered here
		// (exactly one target) so pre-029 histories replay to identical state —
		// but no tool, handler, or roster entry can produce a NEW one, so
		// structural absence is the guarantee dreams cannot land afresh. This
		// explicit form switch REPLACES the spec-014 OnRoster(RosterMetatron,
		// "nudge_"+form) check, which could no longer hold once nudge_dream/
		// nudge_omen left the registry (contracts/events.md).
		switch p.Form {
		case "vision":
			if len(p.Targets) != 1 {
				return fmt.Errorf("apply %s: vision needs exactly one target, got %d", e.Type, len(p.Targets))
			}
		case "omen":
			if len(p.Targets) == 0 {
				return fmt.Errorf("apply %s: omen needs targets", e.Type)
			}
			if !s.Night {
				return fmt.Errorf("apply %s: an omen may land only at night", e.Type)
			}
		case "dream":
			// Legacy (pre-029), replay-only — grandfathered so recorded histories
			// reproduce identically; unreachable from any live tool.
			if len(p.Targets) != 1 {
				return fmt.Errorf("apply %s: dream needs exactly one target, got %d", e.Type, len(p.Targets))
			}
		default:
			return fmt.Errorf("apply %s: unknown form %q", e.Type, p.Form)
		}
		for _, t := range p.Targets {
			if t < 0 || t >= len(s.Agents) {
				return fmt.Errorf("apply %s: unknown target %d", e.Type, t)
			}
			if s.Agents[t].Dead {
				return fmt.Errorf("apply %s: target %s is dead", e.Type, s.Agents[t].Name)
			}
		}
		if p.Text == "" || len(p.Text) > NudgeTextMax {
			return fmt.Errorf("apply %s: text length %d outside 1..%d", e.Type, len(p.Text), NudgeTextMax)
		}
		s.MetatronCharges--
	}
	return nil
}

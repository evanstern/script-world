package sim

import (
	"encoding/json"
	"fmt"

	"github.com/evanstern/script-world/internal/store"
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
	// NudgeTextMax caps the villager-bound rendering.
	NudgeTextMax = 400
)

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
		switch p.Form {
		case "dream":
			if len(p.Targets) != 1 {
				return fmt.Errorf("apply %s: dream needs exactly one target, got %d", e.Type, len(p.Targets))
			}
		case "omen":
			if len(p.Targets) == 0 {
				return fmt.Errorf("apply %s: omen needs targets", e.Type)
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

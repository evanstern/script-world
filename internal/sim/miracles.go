package sim

import (
	"fmt"

	"github.com/evanstern/promptworld/internal/store"
)

// Metatron's miracles (spec 016): four recorded, charge-priced world edits that
// land through the same InjectSocial door the nudge uses. Like applyMetatron,
// these arms validate rather than clamp — the door's dry-run runs them on a
// state copy, so an invalid miracle is rejected before recording and a recorded
// miracle always re-applies cleanly at the same log position in replay
// (spec 016 R1). Charge pricing lives here in the reducer (not the console) so
// replay re-validates every spend identically (R2). `gratis` waives the charge
// and nothing else; it is unreachable from model output by construction (the
// angel's turn contract has no gratis field).

// Miracle event payloads (canonical JSON, struct-ordered). No new persistent
// entities: miracles mutate existing fields only (data-model.md).
type (
	// TimeSnappedPayload jumps the clock forward to ToTick, re-basing the
	// relative-duration fields so remaining times are preserved (FR-008/009).
	TimeSnappedPayload struct {
		ToTick int64 `json:"to_tick"`
		Gratis bool  `json:"gratis"`
	}
	// ItemGrantedPayload provisions a living villager with known items,
	// reject-never-clamp at the bulk cap (FR-011).
	ItemGrantedPayload struct {
		Agent  int    `json:"agent"`
		Kind   string `json:"kind"`
		Qty    int    `json:"qty"`
		Gratis bool   `json:"gratis"`
	}
	// EntityMovedPayload relocates a villager, structure, or pile from (X,Y) to
	// (ToX,ToY) (FR-014).
	EntityMovedPayload struct {
		Class  string `json:"class"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		ToX    int    `json:"to_x"`
		ToY    int    `json:"to_y"`
		Gratis bool   `json:"gratis"`
	}
	// EntityRemovedPayload deletes a structure, pile, or terrain overlay target
	// at (X,Y); villagers may never be removed (v1 doctrine).
	EntityRemovedPayload struct {
		Class  string `json:"class"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Gratis bool   `json:"gratis"`
	}
)

// miracleCost is the doctrine cost table (data-model.md): the time snap is the
// expensive one (2 charges), every other miracle costs 1. Pricing is doctrine,
// not caller input — a payload never carries its own price. Keyed lookup only;
// never iterated into state (determinism).
var miracleCost = map[string]int{
	"metatron.time_snapped":   2,
	"metatron.item_granted":   1,
	"metatron.entity_moved":   1,
	"metatron.entity_removed": 1,
}

// spendMiracleCharge is the shared validate/spend helper for every miracle arm.
// It checks the bank against the event's cost and decrements it — UNLESS gratis,
// which waives the charge (and only the charge; every other validation still
// runs). It must be called only after all other validation has passed and
// before any mutation, so a rejected miracle spends nothing and leaves no
// partial application (validate-not-clamp, reject-whole).
func (s *State) spendMiracleCharge(eventType string, gratis bool) error {
	cost, ok := miracleCost[eventType]
	if !ok {
		return fmt.Errorf("apply %s: no cost defined", eventType)
	}
	if gratis {
		return nil
	}
	if s.MetatronCharges < cost {
		return fmt.Errorf("apply %s: need %d charge(s), only %d banked", eventType, cost, s.MetatronCharges)
	}
	s.MetatronCharges -= cost
	return nil
}

// applyMiracle is the reducer dispatcher for the four metatron.* miracle event
// types, routed here from State.Apply. Each arm validates, prices via
// spendMiracleCharge, and mutates — atomically, or errors with nothing changed.
func (s *State) applyMiracle(e store.Event) error {
	switch e.Type {
	case "metatron.time_snapped":
		return s.applyTimeSnapped(e)
	case "metatron.item_granted":
		return s.applyItemGranted(e)
	case "metatron.entity_moved":
		return s.applyEntityMoved(e)
	case "metatron.entity_removed":
		return s.applyEntityRemoved(e)
	}
	return fmt.Errorf("apply %s: unknown miracle type", e.Type)
}

// applyTimeSnapped — stub (spec 016 US3, T016). Rejected cleanly until wired.
func (s *State) applyTimeSnapped(e store.Event) error {
	return fmt.Errorf("apply %s: not implemented", e.Type)
}

// applyItemGranted — stub (spec 016 US4, T020). Rejected cleanly until wired.
func (s *State) applyItemGranted(e store.Event) error {
	return fmt.Errorf("apply %s: not implemented", e.Type)
}

// applyEntityMoved — stub (spec 016 US1, T007). Rejected cleanly until wired.
func (s *State) applyEntityMoved(e store.Event) error {
	return fmt.Errorf("apply %s: not implemented", e.Type)
}

// applyEntityRemoved — stub (spec 016 US1, T008). Rejected cleanly until wired.
func (s *State) applyEntityRemoved(e store.Event) error {
	return fmt.Errorf("apply %s: not implemented", e.Type)
}

// VillagerAt returns the index of the first living villager standing on (x,y),
// or -1 when none does. Villagers may share a tile, so "first by agent index"
// is the deterministic choice; the miracle move arm and the perception-memory
// builder both resolve through this one helper so they can never disagree on
// which villager a tile-addressed move refers to. Map-free.
func (s *State) VillagerAt(x, y int) int {
	for i := range s.Agents {
		if !s.Agents[i].Dead && s.Agents[i].X == x && s.Agents[i].Y == y {
			return i
		}
	}
	return -1
}

// LivingAgents returns the indices of every living villager, ascending — the
// recipients of a time-snap perception memory (every villager feels the lurch,
// data-model.md). Map-free.
func (s *State) LivingAgents() []int {
	var out []int
	for i := range s.Agents {
		if !s.Agents[i].Dead {
			out = append(out, i)
		}
	}
	return out
}

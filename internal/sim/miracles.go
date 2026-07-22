package sim

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
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

// applyEntityMoved relocates a villager, structure, or pile (spec 016 US1,
// FR-014). The source class MUST be present at (x,y) and the destination MUST
// satisfy the class's placement rule (villager/pile → passable; structure →
// buildSite) — validated here so the dry-run rejects a bad move at the door and
// replay re-applies a recorded move cleanly. All validation precedes the charge
// spend and the mutation, so a rejected move spends nothing and leaves no
// partial application. Villagers may share a tile, so no destination-exclusivity
// check applies to a villager move.
func (s *State) applyEntityMoved(e store.Event) error {
	var p EntityMovedPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("apply %s: %w", e.Type, err)
	}
	switch p.Class {
	case "villager":
		idx := s.VillagerAt(p.X, p.Y)
		if idx < 0 {
			return fmt.Errorf("apply %s: no living villager at (%d,%d)", e.Type, p.X, p.Y)
		}
		if !passable(s.m, s, p.ToX, p.ToY) {
			return fmt.Errorf("apply %s: (%d,%d) is not passable", e.Type, p.ToX, p.ToY)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		a := &s.Agents[idx]
		a.X, a.Y = p.ToX, p.ToY
		// Cancel-and-replan (clarified): the moved villager's in-flight objective
		// is dropped and it becomes idle at the landing tick, exactly like every
		// other intent-clearing path.
		a.Intent = nil
		a.IdleSince = e.Tick
	case "structure":
		i := s.structureIndexAt(p.X, p.Y)
		if i < 0 {
			return fmt.Errorf("apply %s: no structure at (%d,%d)", e.Type, p.X, p.Y)
		}
		if !buildSite(s.m, s, p.ToX, p.ToY) {
			return fmt.Errorf("apply %s: (%d,%d) is not a valid build site", e.Type, p.ToX, p.ToY)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		// The struct moves whole — FuelUntil/Owner/Store ride along in the value.
		s.Structures[i].X, s.Structures[i].Y = p.ToX, p.ToY
	case "pile":
		if s.pileAt(p.X, p.Y) == nil {
			return fmt.Errorf("apply %s: no pile at (%d,%d)", e.Type, p.X, p.Y)
		}
		if !passable(s.m, s, p.ToX, p.ToY) {
			return fmt.Errorf("apply %s: (%d,%d) is not passable", e.Type, p.ToX, p.ToY)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		s.movePile(p.X, p.Y, p.ToX, p.ToY)
	default:
		return fmt.Errorf("apply %s: cannot move class %q", e.Type, p.Class)
	}
	return nil
}

// applyEntityRemoved deletes a structure, pile, or terrain overlay target
// (spec 016 US1). Villagers are never removable (v1 doctrine). A chest spills
// its Store to a ground pile before deletion (goods are never silently
// destroyed); a pile is removed with its contents (explicit, operator-visible
// destruction of the named target); terrain routes through the existing overlay
// vocabulary. All validation precedes the charge spend and the mutation.
func (s *State) applyEntityRemoved(e store.Event) error {
	var p EntityRemovedPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("apply %s: %w", e.Type, err)
	}
	switch p.Class {
	case "villager":
		return fmt.Errorf("apply %s: a villager can never be removed", e.Type)
	case "structure":
		i := s.structureIndexAt(p.X, p.Y)
		if i < 0 {
			return fmt.Errorf("apply %s: no structure at (%d,%d)", e.Type, p.X, p.Y)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		st := s.Structures[i]
		if st.Kind == "chest" && st.Store != nil {
			// Reuse the death-spill vocabulary: contents become a ground pile on
			// the tile, food stamped with a fresh rot deadline (never lost).
			s.spillInventory(p.X, p.Y, st.Store, e.Tick)
		}
		s.removeStructureAt(i)
	case "pile":
		if s.pileAt(p.X, p.Y) == nil {
			return fmt.Errorf("apply %s: no pile at (%d,%d)", e.Type, p.X, p.Y)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		s.removePileAt(p.X, p.Y)
	case "terrain":
		return s.removeTerrain(e, p)
	default:
		return fmt.Errorf("apply %s: cannot remove class %q", e.Type, p.Class)
	}
	return nil
}

// removeTerrain overlays a tree/forage/rock tile through the SAME vocabulary the
// executor uses (chop→Cleared, forage→Harvested with regrow, quarry→Quarried),
// so a removed tile is a state the executor could already produce. A tile that
// is already overlaid is a no-op target → rejected. The charge is spent only
// after the base kind and the not-already-overlaid check pass.
func (s *State) removeTerrain(e store.Event, p EntityRemovedPayload) error {
	switch s.m.At(p.X, p.Y) {
	case worldmap.Tree:
		if effectiveKind(s.m, s, p.X, p.Y) != worldmap.Tree {
			return fmt.Errorf("apply %s: the tree at (%d,%d) is already cleared", e.Type, p.X, p.Y)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		// Mirror agent.chopped: Cleared is a bare Point (reverts to grass).
		s.Cleared = append(s.Cleared, Point{X: p.X, Y: p.Y})
	case worldmap.Forage:
		if effectiveKind(s.m, s, p.X, p.Y) != worldmap.Forage {
			return fmt.Errorf("apply %s: the forage at (%d,%d) is already harvested", e.Type, p.X, p.Y)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		// Mirror agent.foraged: standard forage regrow window.
		s.Harvested = append(s.Harvested, Harvest{X: p.X, Y: p.Y, Regrow: e.Tick + forageRegrowSec})
	case worldmap.Rock:
		if effectiveKind(s.m, s, p.X, p.Y) != worldmap.Rock {
			return fmt.Errorf("apply %s: the rock at (%d,%d) is already quarried", e.Type, p.X, p.Y)
		}
		if err := s.spendMiracleCharge(e.Type, p.Gratis); err != nil {
			return err
		}
		// Mirror agent.quarried: permanent depletion (no regrow entry).
		s.Quarried = append(s.Quarried, Point{X: p.X, Y: p.Y})
	default:
		return fmt.Errorf("apply %s: (%d,%d) holds no removable terrain", e.Type, p.X, p.Y)
	}
	return nil
}

// structureIndexAt returns the index of the structure on (x,y), or -1. At most
// one structure ever stands on a tile (buildSite forbids stacking), so the
// first match is the tile's structure.
func (s *State) structureIndexAt(x, y int) int {
	for i := range s.Structures {
		if s.Structures[i].X == x && s.Structures[i].Y == y {
			return i
		}
	}
	return -1
}

// removeStructureAt deletes structure index i, preserving the creation order of
// the survivors. Empties to nil so canonical bytes stay stable.
func (s *State) removeStructureAt(i int) {
	s.Structures = append(s.Structures[:i], s.Structures[i+1:]...)
	if len(s.Structures) == 0 {
		s.Structures = nil
	}
}

// removePileAt deletes the pile on (x,y) and its contents outright (the explicit
// destruction a remove pile miracle names). Preserves survivor creation order.
func (s *State) removePileAt(x, y int) {
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

// movePile relocates the pile on (fromX,fromY) to (toX,toY), merging onto any
// pile already there (one-pile-per-tile doctrine). Food batches keep their own
// SpoilAt; spears keep their durabilities, sorted ascending. Contents are copied
// by value before the source slice is mutated, so the merge is pointer-safe even
// when the destination append reallocates the pile slice.
func (s *State) movePile(fromX, fromY, toX, toY int) {
	src := s.pileAt(fromX, fromY)
	if src == nil {
		return
	}
	moved := *src
	s.removePileAt(fromX, fromY)
	dest := s.pileFor(toX, toY)
	dest.addNonFood("wood", moved.Wood)
	dest.addNonFood("stone", moved.Stone)
	dest.addNonFood("water", moved.Water)
	dest.addNonFood("planks", moved.Planks)
	dest.addNonFood("refined_stone", moved.RefinedStone)
	for _, b := range moved.Food {
		dest.addFood(b.Kind, b.N, b.SpoilAt)
	}
	if len(moved.Spears) > 0 {
		dest.Spears = append(dest.Spears, moved.Spears...)
		sort.Ints(dest.Spears)
	}
}

// spillInventory pours an inventory (a removed chest's Store) onto the ground
// pile at (x,y), create-or-merge, food stamped with a fresh rot deadline — the
// exact death-spill vocabulary (state.go agent.died), so a removed chest can
// never silently destroy goods (spec 016 R4).
func (s *State) spillInventory(x, y int, inv *Inventory, tick int64) {
	if bulk(*inv) == 0 {
		return
	}
	pile := s.pileFor(x, y)
	pile.addNonFood("wood", inv.Wood)
	pile.addNonFood("stone", inv.Stone)
	pile.addNonFood("water", inv.Water)
	pile.addNonFood("planks", inv.Planks)
	pile.addNonFood("refined_stone", inv.RefinedStone)
	pile.addFood("food_raw", inv.FoodRaw, tick+rotWindowTicks)
	pile.addFood("food_cooked", inv.FoodCooked, tick+rotWindowTicks)
	pile.addFood("meals", inv.Meals, tick+rotWindowTicks)
	if len(inv.Spears) > 0 {
		pile.Spears = append(pile.Spears, inv.Spears...)
		sort.Ints(pile.Spears)
	}
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

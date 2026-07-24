package sim

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

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

const (
	// ticksPerGameDay is a game day in ticks (1 tick = 1 game second, so
	// clock.secondsPerDay = 24×3600). Standing-order TTLs are game days (spec 029).
	ticksPerGameDay = 24 * 3600
	// MetatronOrderTTLMinDays / MaxDays bound a standing order's lifetime
	// (spec 029 FR-007): player-specifiable, default 3, capped 1..7 game days.
	MetatronOrderTTLMinDays = 1
	MetatronOrderTTLMaxDays = 7
	// MetatronPlayerOrderCap is the concurrent ACTIVE player-placed order cap
	// (FR-007); system-origin deferral orders are exempt (FR-012).
	MetatronPlayerOrderCap = 3
	// metatronOrderRetain bounds retained NON-ACTIVE orders (data-model §1): the
	// slice keeps every active order plus the most recent 32 consumed ones, so
	// the status/trail shows recent history without unbounded growth.
	metatronOrderRetain = 32
)

// MetatronOrder is one event-sourced standing order (spec 029, data-model §1): a
// pre-authorized watch-and-act instruction placed via monitor_and_act. Its
// lifecycle (active → triggered | cancelled | expired) is driven entirely by
// recorded events, so it reconstructs identically through snapshots, restart,
// and from-genesis replay; replay only reconstructs state — it never triggers.
type MetatronOrder struct {
	ID          string   `json:"id"`                 // "ord-<placedTick>-<seq>" (research R7)
	Origin      string   `json:"origin"`             // "player" | "system"
	Condition   string   `json:"condition"`          // original NL, ≤300 chars
	Action      string   `json:"action"`             // NL action instruction, ≤400 chars
	EventTypes  []string `json:"event_types"`        // structural predicate: non-empty
	Agent       int      `json:"agent"`              // villager index, -1 = any
	Keywords    []string `json:"keywords,omitempty"` // coarse text filter, lowercase
	Confirm     bool     `json:"confirm,omitempty"`  // fuzzy: needs the watch confirm
	PlacedTick  int64    `json:"placed_tick"`
	ExpiresTick int64    `json:"expires_tick"` // placed + ttl_days game days
	Status      string   `json:"status"`       // "active" | "triggered" | "cancelled" | "expired"
}

// OrderTriggeredPayload records a matched order's one-shot consumption (spec
// 029): the matched event's type + tick ride along for the trail. Injected by
// the trigger worker (Batch B), NEVER emitted during replay.
type OrderTriggeredPayload struct {
	ID          string `json:"id"`
	MatchedType string `json:"matched_type"`
	MatchedTick int64  `json:"matched_tick"`
}

// OrderIDPayload is the bare-id payload shared by metatron.order_cancelled
// (injected — cancel_order) and metatron.order_expired (executor-emitted, a pure
// function of state + tick, like charge_regenerated).
type OrderIDPayload struct {
	ID string `json:"id"`
}

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
	case "metatron.order_placed":
		var o MetatronOrder
		if err := json.Unmarshal(e.Payload, &o); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if o.ID == "" {
			return fmt.Errorf("apply %s: empty order id", e.Type)
		}
		// Duplicate id in ANY status is rejected — ids are assigned once and
		// consumed orders are retained, so a reused id would corrupt the trail.
		for i := range s.MetatronOrders {
			if s.MetatronOrders[i].ID == o.ID {
				return fmt.Errorf("apply %s: duplicate order id %q", e.Type, o.ID)
			}
		}
		switch o.Origin {
		case "player", "system":
		default:
			return fmt.Errorf("apply %s: unknown origin %q", e.Type, o.Origin)
		}
		if len(o.EventTypes) == 0 {
			return fmt.Errorf("apply %s: order has no event_types (uncompilable condition)", e.Type)
		}
		if ttl := o.ExpiresTick - o.PlacedTick; ttl < MetatronOrderTTLMinDays*ticksPerGameDay || ttl > MetatronOrderTTLMaxDays*ticksPerGameDay {
			return fmt.Errorf("apply %s: ttl %d ticks outside %d..%d game days", e.Type, ttl, MetatronOrderTTLMinDays, MetatronOrderTTLMaxDays)
		}
		if o.Agent < -1 || o.Agent >= len(s.Agents) {
			return fmt.Errorf("apply %s: agent index %d out of range", e.Type, o.Agent)
		}
		if utf8.RuneCountInString(o.Condition) > 300 {
			return fmt.Errorf("apply %s: condition over 300 chars", e.Type)
		}
		if utf8.RuneCountInString(o.Action) > 400 {
			return fmt.Errorf("apply %s: action over 400 chars", e.Type)
		}
		// Concurrent cap: at most 3 ACTIVE player-origin orders; system-origin
		// deferral orders are exempt (already-authorized acts, FR-012).
		if o.Origin == "player" {
			active := 0
			for i := range s.MetatronOrders {
				if s.MetatronOrders[i].Origin == "player" && s.MetatronOrders[i].Status == "active" {
					active++
				}
			}
			if active >= MetatronPlayerOrderCap {
				return fmt.Errorf("apply %s: %d player orders already active (cap %d)", e.Type, active, MetatronPlayerOrderCap)
			}
		}
		// The status field is IGNORED on the payload — an order always lands
		// active (data-model §2), then the retention prune runs.
		o.Status = "active"
		s.MetatronOrders = pruneMetatronOrders(append(s.MetatronOrders, o))
	case "metatron.order_triggered":
		var p OrderTriggeredPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		return s.transitionMetatronOrder(e.Type, p.ID, "triggered")
	case "metatron.order_cancelled":
		var p OrderIDPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		return s.transitionMetatronOrder(e.Type, p.ID, "cancelled")
	case "metatron.order_expired":
		var p OrderIDPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		return s.transitionMetatronOrder(e.Type, p.ID, "expired")
	}
	return nil
}

// transitionMetatronOrder moves the order named id from active to a terminal
// status (spec 029, one-way). An unknown id or an order not currently active is
// rejected at the door — this is where the cancel/expiry/trigger races resolve:
// exactly one terminal lands, and the loser hits a non-active order and refuses
// (contracts/events.md edge cases).
func (s *State) transitionMetatronOrder(eventType, id, to string) error {
	for i := range s.MetatronOrders {
		if s.MetatronOrders[i].ID != id {
			continue
		}
		if s.MetatronOrders[i].Status != "active" {
			return fmt.Errorf("apply %s: order %q is not active (status %q)", eventType, id, s.MetatronOrders[i].Status)
		}
		s.MetatronOrders[i].Status = to
		return nil
	}
	return fmt.Errorf("apply %s: unknown order %q", eventType, id)
}

// pruneMetatronOrders retains every active order plus the most recent
// metatronOrderRetain (32) non-active ones, dropping the oldest consumed orders
// first while preserving slice order (data-model §1). Deterministic — a pure
// function of the append-ordered slice, so replay prunes identically.
func pruneMetatronOrders(orders []MetatronOrder) []MetatronOrder {
	nonActive := 0
	for i := range orders {
		if orders[i].Status != "active" {
			nonActive++
		}
	}
	drop := nonActive - metatronOrderRetain
	if drop <= 0 {
		return orders
	}
	out := make([]MetatronOrder, 0, len(orders)-drop)
	for i := range orders {
		if orders[i].Status != "active" && drop > 0 {
			drop--
			continue
		}
		out = append(out, orders[i])
	}
	return out
}

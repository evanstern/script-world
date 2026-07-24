# Data Model: Metatron Agency

## §1 MetatronOrder (new, event-sourced on `sim.State`)

```go
// State field: MetatronOrders []MetatronOrder `json:"metatron_orders,omitempty"`
// Pre-agency snapshots unmarshal to nil — upgrade-free (the TASK-12 precedent).
type MetatronOrder struct {
    ID         string   `json:"id"`          // "ord-<placedTick>-<seq>" (research R7)
    Origin     string   `json:"origin"`      // "player" | "system"
    Condition  string   `json:"condition"`   // original NL, ≤300 chars
    Action     string   `json:"action"`      // NL action instruction, ≤400 chars
    EventTypes []string `json:"event_types"` // structural predicate: non-empty
    Agent      int      `json:"agent"`       // villager index, -1 = any
    Keywords   []string `json:"keywords,omitempty"` // coarse text filter, lowercase
    Confirm    bool     `json:"confirm,omitempty"`  // fuzzy: needs watch confirm
    PlacedTick int64    `json:"placed_tick"`
    ExpiresTick int64   `json:"expires_tick"` // placed + ttl_days game days
    Status     string   `json:"status"`       // "active" | "triggered" | "cancelled" | "expired"
}
```

Validation rules (reducer arms, enforced at the InjectSocial dry-run and identical
in replay):

- `order_placed`: id non-empty and not already present in ANY status; origin ∈
  {player, system}; player-origin active count < 3 (system exempt); `event_types`
  non-empty; ttl ⇒ `ExpiresTick` within 1..7 game days after `PlacedTick`; agent
  index -1 or valid; condition/action within caps.
- `order_triggered` / `order_cancelled` / `order_expired`: id names an order with
  status `active`; transition is one-way; anything else rejected at the door.

State transitions: `active → triggered | cancelled | expired` (one-shot; no
re-arming). Consumed orders are retained in the slice (bounded: pruned to the
most recent 32 non-active orders on placement) so the trail and status can show
recent history without unbounded growth.

## §2 Event types (new, whitelisted)

| Type | Payload | Emitter |
|------|---------|---------|
| `metatron.order_placed` | full `MetatronOrder` (status field ignored on apply — always lands active) | metatron turn handler via InjectSocial |
| `metatron.order_triggered` | `{id, matched_type, matched_tick}` | metatron trigger worker via InjectSocial (never replay) |
| `metatron.order_cancelled` | `{id}` | metatron turn handler (cancel_order) via InjectSocial |
| `metatron.order_expired` | `{id}` | executor (pure function of state + tick), like `charge_regenerated` |

`metatron.nudged` (existing) — `form` domain becomes `{dream (legacy, replay-only),
omen, vision}`; `omen` additionally requires `State.Night == true`; `vision`
requires exactly one living target. Charge spend unchanged (−1, validated > 0).

## §3 Tools (registry deltas — full schemas in contracts/tools.md)

Retired: `nudge_dream`, `nudge_omen` (registry entries removed; cap-literal readers
re-point at `send_vision`).

Added (all on `LoopRosterMetatron` / `RosterMetatron`, all manifest-gateable):

| Tool | Effect | Gate | Cost | Notes |
|------|--------|------|------|-------|
| `send_vision` | Expressive (`metatron.nudged`, `agent.memory_added`) | Charge | 1 charge | target AgentName, text ≤400 |
| `send_omen` | Expressive (same events) | Charge | 1 charge | targets = comma list or `everyone`; night-only at door; day ⇒ deferral order |
| `monitor_and_act` | Expressive (`metatron.order_placed`) | None | free | authored InputSchemaJSON (arrays) |
| `cancel_order` | Expressive (`metatron.order_cancelled`) | None | free | id Text required |
| `pause` | Expressive (no events) | None | free | LoopControl.Do("pause") |
| `start` | Expressive (no events) | None | free | optional speed Enum; Do("resume") |
| `adjust_speed` | Expressive (no events) | None | free | required speed Enum; Do("set_speed") |

## §4 LLM kind (new)

`KindMetatronWatch = "metatron_watch"` — single bare Submit per confirm (no tool
loop), reply contract `yes`/`no`, MaxTokens small (16), default route chain
`["local", "cloud"]`, missing-route backfill on config load (research R8).

## §5 Metatron component state (not event-sourced)

- Order mirror: `orders []MetatronOrder` refreshed in `mirrorState` from the
  replica (absorb-owned; turn worker reads under `stateMu` like charges/alive).
- Trigger queue: buffered channel + one worker goroutine (FIFO; single-flight via
  the shared `turnBusy` with bounded wait for system turns).
- Confirm rate tracker: `lastConfirmTick map[string]int64` (absorb-owned;
  not world state — a skipped confirm is an economy decision, not history).
- `LoopControl` seam: interface over `sim.Loop.Do`, injected at `metatron.New`.

## §6 Status surface (additive)

`Status.Orders []OrderStatus` — `{id, condition, origin, fuzzy, expires_day,
status}` for active + recent orders; omitempty. Turn prompt gains the standing-
orders block (FR-017).

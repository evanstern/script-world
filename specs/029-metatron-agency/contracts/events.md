# Contract: Event Types & Whitelist Delta (spec 029)

## New event types (all four join `injectSocialWhitelist`; order_expired is
executor-emitted and needs no whitelist entry — verify at implementation: only
injected types need the whitelist, mirroring `charge_regenerated`)

### metatron.order_placed  (injected — metatron turn handler)
```json
{"id":"ord-123456-0","origin":"player","condition":"when Rowan next falls asleep",
 "action":"send her a comforting vision about the harvest",
 "event_types":["agent.slept"],"agent":3,"keywords":[],"confirm":false,
 "placed_tick":123456,"expires_tick":382656}
```
Reducer arm rejects: duplicate id (any status), unknown origin, player-origin
active count ≥ 3 (system-origin exempt), empty `event_types`, ttl outside
1..7 game days, agent index invalid (accepts -1 = any), condition > 300 chars,
action > 400 chars.

### metatron.order_triggered  (injected — trigger worker; NEVER emitted in replay)
```json
{"id":"ord-123456-0","matched_type":"agent.slept","matched_tick":201600}
```
Reducer arm rejects: id not found or status ≠ active. Applies: status →
triggered (one-shot consumption).

### metatron.order_cancelled  (injected — cancel_order handler)
```json
{"id":"ord-123456-0"}
```
Reducer arm rejects: id not found or status ≠ active. Applies: status → cancelled.

### metatron.order_expired  (executor-emitted — pure function of state + tick)
```json
{"id":"ord-123456-0"}
```
Emitted when `tick ≥ expires_tick` for an active order (the
`charge_regenerated` pattern). Same reducer arm shape: active → expired.

## Changed: metatron.nudged

`form` domain: `"vision"` (new), `"omen"` (kept), `"dream"` (legacy —
grandfathered for replay only; no tool can produce a new one).

Reducer arm validation:
- charge > 0 (unchanged, spends −1);
- `vision`: exactly 1 target, alive;
- `omen`: ≥1 targets, all alive, `State.Night == true`;
- `dream`: legacy shape (exactly 1 target, alive) — accepted so historical
  events replay; the roster-membership check (`OnRoster(RosterMetatron,
  "nudge_"+form)`) is REPLACED by this explicit form-set validation (the old
  check would fail once `nudge_dream`/`nudge_omen` leave the registry);
- text 1..400 bytes (cap re-pointed at `send_vision`'s registry entry).

Memory prefixes: dream `"You dreamed: "` (legacy), omen
`"You witnessed an omen: "` (unchanged), vision `"You saw a vision: "` (new).

## Replay invariants

- From-genesis replay of a pre-029 world reproduces identical state (dream
  events apply; `MetatronOrders` stays nil).
- Replay of a post-029 world applies order lifecycle events as pure state
  transitions; no trigger execution, no model calls, no Observe-path matching
  during replay (the angel component isn't running).
- Snapshots: `metatron_orders,omitempty` — absent field unmarshals to nil;
  a spent-to-zero bank precedent (`metatron_charges` non-omitempty) does NOT
  apply here because an empty order set is genuinely zero-value.

## Coverage gates

`sim.ValidateToolCoverage` (boot): every new Expressive tool's declared Events ⊆
the whitelist — automatic once entries and whitelist land; meta tools declare no
events (trivially covered). `tool.Validate` (boot): new entries pass the internal
consistency matrix (authored schema is valid JSON object shape, enum params
non-empty, roster names resolve).

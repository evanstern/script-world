# Contract: Governor Event Types

New recorded event types — the SOLE channel by which governing enters the deterministic space (FR-014). Both are
reducer-applied and versioned with the world format (no bump required; see research R3).

## `clock.governor_shed`

Emitted by the sim loop when it applies a governor shed command at a tick boundary.

```json
{ "requested": "32x", "from": "32x", "to": "16x", "debt": 1.4, "jobs": 3 }
```

- `requested`: the player's ceiling at decision time (`clock.Speed` string).
- `from` / `to`: effective speed before/after; `to` is exactly one capped-ladder notch below `from`; `to ≥ 1x`.
- `debt`: measured budget-fraction sum that justified the decision.
- `jobs`: number of pending thoughts contributing to `debt`.

Reducer: `Speed = to`; `RequestedSpeed = requested`; `EffectiveRate = to.TicksPerSecond()` unless `Degraded`.

## `clock.governor_recovered`

Same payload shape. `to` is exactly one notch above `from` and never above `requested`; when `to == requested`
the world leaves governed state.

Reducer: `Speed = to`; `RequestedSpeed = requested` if `to != requested`, else cleared; same `EffectiveRate` rule.

## Amended: `clock.speed_set`

Payload unchanged. Reducer gains: clears `RequestedSpeed` (player command collapses governed state, FR-009).

## Replay obligations

- Replay applies these events verbatim and NEVER re-derives debt (SC-001, FR-014).
- Logs without these types replay byte-identically to pre-028 (additive change).
- Unknown-type convention: pre-028 binaries reading a 028 log are out of scope (forward-only, standing policy).

## Emission rules

- Only the loop's `govern` command emits them; idempotent semantics match other clock commands (a no-op decision
  emits nothing).
- Never emitted while paused (FR-013) and never with an LLM-less daemon (FR-003).

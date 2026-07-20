# Contract: Metatron event family

New namespaced family `metatron.*`; payload structs live in `internal/sim/metatron.go`
(structs, never maps — canonical JSON). Unknown to older binaries → reducer no-ops
(backward-compatible replay, the established convention).

## `metatron.charge_regenerated`

- **Payload**: `{}` (empty struct; the event row's `tick` is the boundary crossed)
- **Emitted by**: the executor, live, when `tick` crosses an absolute 6-game-hour
  boundary (multiples of 21600 ticks) AND `State.MetatronCharges < 3`. Pure function of
  (state, tick): replay of a recorded log reproduces it by applying the recorded event;
  a re-simulation from the same seed re-emits it identically.
- **Reducer**: `MetatronCharges = min(3, MetatronCharges+1)`
- **Injectable**: NO — not in the `InjectSocial` whitelist. Regeneration is world
  physics, not model output.

## `metatron.nudged`

- **Payload**: `MetatronNudgedPayload{ form string, targets []int, text string }`
  - `form`: `"dream"` | `"omen"`
  - `targets`: dream → exactly one living villager index; omen → every villager alive at
    landing (computed at injection, recorded explicitly so replay needs no aliveness
    reconstruction)
  - `text`: Metatron's rendering, ≤ 400 chars; the only villager-bound text
- **Emitted by**: injection only (`InjectSocial`), as the head of one atomic batch per
  landed nudge
- **Reducer**: `MetatronCharges = max(0, MetatronCharges−1)`; total (no error paths —
  validation happens at the door)
- **Injectable**: YES — whitelisted. Dry-run rejection cases (whole batch aborts):
  charges = 0; unknown form; dream with targets ≠ 1; any dead/unknown target index;
  empty or over-cap text.

## Batch shape (per landed nudge)

```
[ metatron.nudged{form, targets, text},
  agent.memory_added{Agent: t, Text: prefix+text, Salience: salDream, Subject: -1}   × each target ]
```

- `salDream = 8` (new constant in `internal/sim/memory.go`, between shelter 6 and
  near-death 9)
- prefix: `"You dreamed: "` (dream) / `"You witnessed an omen: "` (omen)
- Atomicity: `InjectSocial` all-or-nothing at a tick boundary (existing contract) —
  a spend can never land without its memories, nor memories without the spend.

## Moment triggers (drama rule v1 — no new events)

`agent.died`, `gru.attacked`, `social.promise_broken` are consumed (not emitted) by the
Metatron component: each appends a moment line to `metatron/soul.md` and queues for
console surfacing. No event, no model call, no autonomous action.

## Status surface

The IPC `status` payload's `llm` block gains `metatron_charges` (int, from the loop's
state snapshot) so clients can display the bank without a state fetch.

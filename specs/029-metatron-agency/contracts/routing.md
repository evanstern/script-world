# Contract: KindMetatronWatch Routing (spec 029)

## New kind

- `llm.KindMetatronWatch = "metatron_watch"` joins `acceptedKinds` and `Kinds()`.
- Call shape: ONE bare `Orchestrator.Submit` per confirm — no tool loop, no tools
  declared. `MaxTokens: 16`. Reply contract: first token `yes` or `no`
  (case-insensitive, whitespace-trimmed); anything else, including empty or
  error, is a NO (unconfirmed — the order stays armed, no retry).
- Metering: rides the existing per-kind ledger/estimator unchanged — each confirm
  is one metered call attributable from the meter alone (this is SC-001/SC-008's
  countable trail).
- Admission: normal ladder. `ErrBudgetExhausted`/`ErrTierDown`/transport failure
  ⇒ unconfirmed, no retry, logged.

## Default route

`defaultRoutes()` gains: `"metatron_watch": {Chain: ["local", "cloud"]}` —
cheap-first with reliable fallback. Operators re-route like any kind (e.g. a
dedicated haiku provider entry).

## Config-load backfill (compatibility carve-out)

`validateV2` completeness ("every accepted kind has a route", both directions)
would brick every existing v2 `llm.json` the moment a new kind ships. Carve-out,
scoped and documented in code:

- Unknown route keys: still a boot error (typo protection, unchanged).
- A MISSING route for `metatron_watch` (and only for kinds introduced after the
  config was written — implemented as a per-kind default-backfill set): backfilled
  from `defaultRoutes()` at load with one boot log line naming the backfill.
  Post-load invariant (every kind routed) still holds.

Legacy (v1) configs already resolve entirely through `defaultRoutes()` and pick
the new kind up for free.

## Surfaces that follow automatically

- `promptworld calibrate` and any kind-enumerating status surface iterate
  `llm.Kinds()` — the new kind appears without extra wiring; confirm the
  calibrate reference-sample path tolerates a kind with no tools (it submits
  bare requests — expected fine, verify in tests).

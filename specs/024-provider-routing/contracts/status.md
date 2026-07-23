# Contract: per-provider status (protocol + TUI)

Replaces the fixed `local`/`cloud` pair in `llm.Status`; one shape for legacy and v2
worlds. Daemon, IPC, and TUI ship together — no dual-publishing.

## Wire shape (folded into protocol status as today)

```json
{
  "llm": {
    "providers": [
      {
        "name": "cogito",
        "model": "cogito:3b",
        "endpoint": "http://localhost:11434/v1",
        "up": true,
        "queue": 3,
        "inflight": 4,
        "slots": 4,
        "contended": false,
        "spent_usd": 0
      },
      {
        "name": "anthropic",
        "model": "claude-opus-4-8",
        "up": true,
        "queue": 0,
        "inflight": 0,
        "slots": 1,
        "contended": false,
        "spent_usd": 12.41
      }
    ],
    "month": "2026-07",
    "spent_usd": 12.41,
    "budget_usd": 100
  }
}
```

Rules:
- `providers` sorted by `name` (deterministic marshal).
- `up` = breaker not open; `queue` = normal-lane depth (as today); `inflight`/`slots`
  expose worker occupancy; `contended` = lease-wait flag (always false when
  `endpoint_capacity` undeclared).
- `spent_usd` per row is this month's attribution; Σ rows ≤ global `spent_usd`
  (difference = legacy unattributed spend, shown by the TUI as `(unattributed)` when
  nonzero).
- Legacy worlds show exactly two rows named `local` and `cloud`.

## Response additions (llm_call proof path + telemetry)

`Response.Provider` (serving provider name) and `Response.Skipped`
(`[{"provider": "...", "reason": "circuit-open" | "wallet-exhausted" | "queue-full" |
"busy"}]`) ride the existing response JSON; `promptworld llm` prints them.

## TUI

The pane that today renders tier health + spend renders the provider table: one row per
provider — name, model, up/down glyph, queue, inflight/slots, contended marker, spend.
Row order matches the wire (by name).

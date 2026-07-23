# Contract: Status Protocol & TUI Surface

## Protocol `Status` (internal/ipc/protocol.go) — additive fields

| Field | JSON | Source | Semantics |
|---|---|---|---|
| `RequestedSpeed` | `requested_speed,omitempty` | sim state | player's ceiling; empty = ungoverned |
| `GovernorDebt` | `governor_debt,omitempty` | daemon governor snapshot | current budget-fraction sum |
| `GovernorJobs` | `governor_jobs,omitempty` | daemon governor snapshot | pending thoughts contributing to debt |

Pre-028 clients ignore the new fields; 028 clients reading a pre-028 daemon see zero values (both directions are
compatible). No-LLM worlds always report empty/zero (FR-003, SC-004).

## `set_speed` command — semantics unchanged, one clarification

The requested speed takes effect immediately as both requested and effective (governed state collapses); `max`
remains refused when an LLM is configured (FR-012 — refusal site untouched). The governor re-evaluates on its
normal cadence afterward (FR-009).

## TUI (internal/tui)

- Header (views.go speed segment): ungoverned renders exactly as today. Governed appends plain language, e.g.
  `speed 16x  asked 32x — 3 minds in flight, debt 140%` (FR-015). Debt renders as a percentage of the shed
  threshold.
- Digest (digest.go): `clock.governor_shed` / `clock.governor_recovered` render one-line entries with
  `from→to`, debt, and jobs — same style as the `clock.degraded` line.
- Saturation (governed at 1x, debt still over threshold) is visible: the header line shows the floor speed with
  the over-threshold debt; no special casing beyond rendering the numbers.

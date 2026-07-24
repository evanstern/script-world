# Data Model: Governor Accrued-Drift Debt

No entity shape changes — this feature revises one arithmetic rule.

## PendingDebtInput (internal/cognition, unchanged shape)

| Field | Meaning | Source (unchanged) |
|---|---|---|
| `Kind` | call kind → decision class via registry | pending registry snapshot |
| `PredictedSec` | class points × serving provider's CURRENT live sec/pt estimate | computed at READ time in internal/llm/pending.go |
| `ElapsedSec` | 0 while queued; wall-clock since dispatch while in flight | computed at READ time in internal/llm/pending.go |

## Debt (internal/cognition/governor.go — the changed rule)

Before (spec 028): `remaining = max(0, PredictedSec − ElapsedSec)` — overdue
thoughts contribute zero ("an overdue thought invents no debt it cannot ground").

After (spec 033): piecewise per contracts/debt-formula.md — within prediction the
remaining work drains exactly as before; at/beyond prediction the thought
contributes its full accrued `ElapsedSec` ("an overdue thought's elapsed time IS
its grounded debt"). Jobs counter rule unchanged (positive fraction counts), so
overdue thoughts become visible in the status jobs figure as a side effect.

## Untouched entities

- `Governor` (hysteresis state machine), `Decision`, `Action` — unchanged.
- `GovernorPayload` / `clock.governor_shed` / `clock.governor_recovered` events —
  unchanged shapes; expected to actually occur under saturation now.
- `PendingThought` (internal/llm/pending.go) — unchanged; verified pass-through.

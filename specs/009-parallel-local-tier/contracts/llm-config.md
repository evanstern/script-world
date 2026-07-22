# Contract: llm.json — `local.parallel`

The world-facing configuration surface this feature adds. `llm.json` lives in the
world save directory; this extends the `local` object only.

## Shape

```json
{
  "monthly_budget_usd": 100,
  "local": {
    "endpoint": "http://localhost:11434/v1",
    "model": "gemma4:12b-mlx",
    "parallel": 4
  },
  "cloud": { "...": "unchanged by this feature" }
}
```

## Semantics

| Aspect | Contract |
|---|---|
| Field | `local.parallel`, integer, optional |
| Meaning | Maximum simultaneous in-flight calls against the local tier |
| Default | 1 — absent field ⇒ behavior indistinguishable from pre-feature builds |
| Valid range | 1–16 |
| Out-of-range | Never an error. Negative ⇒ 1; above 16 ⇒ 16. Effective value + warning printed on the daemon boot line. The world always starts. |
| Cloud tier | No `parallel` field; always 1 in-flight call. A `parallel` key under `cloud` is ignored (unknown fields are not errors, matching existing config behavior). |
| Queueing | Requests beyond N wait in today's priority-then-FIFO order (conversations jump via the prio lane); nothing is reordered or lost by concurrency |
| Best-effort | Drop-when-busy calls (musings) are refused only when no slot is free; contract of "dropped, never retried" unchanged |
| `promptworld new` | Generated default llm.json omits the field (default 1) |

## Boot-line surface (operator-visible)

The daemon's existing `daemon: llm orchestrator on (...)` line reports the
effective local parallelism when > 1, and a clamp warning when the configured
value was normalized, e.g.:

```
daemon: llm.json local.parallel 64 out of range — clamped to 16
daemon: llm orchestrator on (local gemma4:12b-mlx @ http://localhost:11434/v1, parallel 16, cloud ..., budget $100/mo)
```

(Exact wording is implementation detail; the contract is: effective value visible,
clamping visible, never fatal.)

# Data Model: Conversation Robustness

No new entities and no schema changes — the feature extends one existing telemetry
payload and adds in-memory retry state local to a scene run.

## Extended: `cog.outcome` event payload (`internal/mind/telemetry.go`)

Existing terminal per-thought record; gains one optional field.

| Field | Type | New? | Semantics |
|---|---|---|---|
| `job`, `class`, `agent`, `outcome`, `reason`, `predicted_wall_ms`, `actual_wall_ms`, `staleness_ticks`, … | (existing) | no | unchanged |
| `raw` | string, omitempty | **yes** | Verbatim model reply text that failed to parse. Populated ONLY on parse-failure outcomes (utterance or summary site). Truncated at 2048 bytes with `…[truncated]` suffix. Absent on success, transport errors, and staleness rejections. |
| `retried` | bool, omitempty | **yes** | True on any terminal outcome (landed or unusable) whose scene consumed a retry at either site. Lets recovery rates be measured from the event log alone (FR-005). |

Validation rules:
- `raw` MUST be valid UTF-8 after truncation (truncate on a rune boundary).
- `raw` never appears on `outcome: "landed"` events (landed means parsing succeeded;
  a landed-after-retry scene records the failure on the interim unusable emission? —
  **No**: to keep all-or-nothing intact, a retry-then-success scene emits ONE terminal
  `cog.outcome landed` with `retried: true`; the first attempt's raw text is emitted
  as a separate non-terminal `cog.outcome` with `outcome: "retried"` — see contract).

## New outcome constant: `sim.OutcomeRetried`

`"retried"` — a non-terminal cog.outcome marker: one reply failed to parse and the
scene continued (retry admitted). Carries `raw`. Distinct from terminal
`unusable`/`landed`/`rejected-*` so existing consumers that treat cog.outcome as
terminal-per-job can filter it by value. (Telemetry counters that sum outcomes must
treat `retried` as informational, not terminal — verified against
`internal/mind/telemetry.go` consumers in tasks.)

## In-memory (no persistence): scene retry state

Local variables in `runConversation` / helpers — not a struct field, not shared:

- utterance site: scene-level `utteranceRetried bool` — ONE utterance retry
  total per scene (FR-002/FR-007), NOT one per turn. A parse failure while the
  budget is already spent aborts the scene even on a later, non-consecutive
  turn; a retry that itself fails aborts too.
- outcome site: single `retriedOutcome bool` (its own one-retry budget).
- both feed the terminal event's `retried` flag.

## State transitions (scene lifecycle)

```
running ──utterance ok──▶ running ──…──▶ transcript complete
   │                                          │
   │ utterance parse-fail                     │ outcome ok ──▶ stale-check ──▶ land (atomic batch)
   ▼                                          │
 emit cog.outcome{retried,raw}                │ outcome parse-fail
   │ retry same speaker                       ▼
   ├─ ok ──▶ running (retried=true)         emit cog.outcome{retried,raw}
   └─ fail ─▶ abandon: terminal               │ retry outcome once
      cog.outcome{unusable,raw}               ├─ ok ──▶ stale-check ──▶ land (retried=true)
                                              └─ fail ─▶ abandon: terminal cog.outcome{unusable,raw}
```

Invariants preserved:
- All-or-nothing landing: the atomic batch (convo.go:225-292) is built only after a
  successful outcome; no partial state ever lands (FR-003).
- Stale-at-landing check still runs after the (possibly retried) outcome, so retry
  wall-time cannot smuggle a stale scene past the budget (FR-007).
- Transport/admission errors at either site abandon immediately — never retried.

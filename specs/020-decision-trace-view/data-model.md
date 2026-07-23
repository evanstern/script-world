# Data Model: Decision-Trace View

Client-side only; nothing here is persisted or sent over the wire. Source events are
the existing spec-017 payloads (`internal/sim/cognition.go`) — unchanged.

## Entities

### decisionChain

One cognition's causal record, keyed by job ID.

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| Job | string | all three event types | correlation key |
| Agent | int | `cog.thought.agent`, else `cog.outcome.agent`, else job-ID parse | `-1` = unattributed; Metatron uses a dedicated sentinel key, not an agent index |
| Class | string | `cog.thought.class` (or `cog.outcome.class` on fragments) | e.g. `planner` |
| Tick | int64 | first-seen event's tick | display ordering + "when" |
| TriggerSeq | int64 | `cog.thought.trigger_seq` | 0 = cadence |
| Stimulus | string | resolved at ingest (research D3) | digest text, cadence phrase, or neutral reference |
| Calls | []decisionCall | `cog.tool_call` events | ordinal order; see below |
| Outcome | string | `cog.outcome.outcome` | empty = in flight (FR-008) |
| OutcomeReason | string | `cog.outcome.reason` | router arithmetic for suppressions |
| Suppressed | bool | outcome present with no thought and no calls | renders "didn't think because…" |

### decisionCall

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| Ordinal | int | payload | 1-based, dense per job; insert-sorted |
| Tool | string | payload | tool name |
| Verdict | string | payload | raw enum, stored as-is; rendered ONLY via glossary |
| Reason | string | payload | verbatim prose, may be empty for landed/read_ok |
| Args | string | payload (already ≤ 2 KiB upstream) | compact single-line display form |

### decisionTraces (the projection)

| Field | Type | Notes |
|-------|------|-------|
| byJob | map[string]*decisionChain | ingest join point |
| byAgent | map[int][]string | job keys per agent, append-ordered; Metatron under its sentinel |

**Bounds**: `decisionChainCap = 20` chains per agent list; evicting a job key deletes
its `byJob` entry. Conversation-prefixed jobs are never ingested.

**Lifecycle**: created empty at connect; reset wholesale on `connectedMsg` (reconnect);
mutated only inside `applyEvent` after the seq-skip guard (so snapshot-folded events
never double-ingest).

## New Model (TUI) state

| Field | Type | Notes |
|-------|------|-------|
| traces | decisionTraces | the projection |
| villDecisions | bool | decisions sub-view open (only meaningful with `villDetail`) |
| villDecisionsScroll | int | scroll offset; reset on villager change, detail close, reconnect; render-time clamped like `chronDetailScroll` |

## State transitions

```
cog.thought  ──▶ chain created (or fragment upgraded): header + stimulus resolved
cog.tool_call ─▶ chain created if absent (fragment); call appended in ordinal order
cog.outcome  ──▶ chain terminal set; if no thought/calls ever arrive → suppression
eviction     ──▶ per-agent list > cap: oldest job dropped from both indexes
reconnect    ──▶ projection reset empty alongside replica swap
```

## Verdict glossary (rendering authority)

Complete domain (sweep-tested; raw strings never rendered):
`landed`, `rejected_gate`, `rejected_cardinality`, `rejected_unknown`,
`rejected_malformed`, `read_ok`, `read_error`, `unlanded` (toolloop verdicts) and
`landed`, `adapted`, `rejected-stale`, `rejected-guard`, `superseded`, `expired`,
`rejected-unavailable`, `unusable`, `suppressed`, `retried` (outcome vocabulary —
`retried` is non-terminal and only ever renders as an annotation, per
contracts/telemetry.md rule 1).

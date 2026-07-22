# Contract: Events & byte-stability (spec 017)

The event-log deltas this feature makes, and the rules that keep every existing log
replaying byte-identically. Replay NEVER re-runs any part of a tool loop — transcripts
and tool results are ephemeral; only events are facts.

## New event type: `cog.tool_call`

One per tool call the loop sees — landed, rejected, read, or unlanded. Reducer no-op
(observability record, like all `cog.*`). Added to `injectSocialWhitelist`
(internal/sim/loop.go:152) — the whitelist's ONLY change.

Payload (canonical field order; `internal/sim` payload struct):

```json
{
  "job": "planner-3-412800",
  "ordinal": 2,
  "tool": "set_plan",
  "args": {"steps": [{"goal": "chop"}, {"goal": "build_fire"}]},
  "verdict": "landed",
  "reason": "",
  "tier": "local",
  "snapshot_tick": 412800
}
```

Rules:
- `job` + `ordinal` are the correlation key; ordinals are 1-based, dense per job, in
  model-emission order.
- `args` is the raw arguments JSON, copied verbatim up to 2048 bytes; larger payloads
  are truncated to a valid JSON string field `{"_truncated": true, "prefix": "…"}`;
  `args` omitted (omitempty) for calls with no arguments.
- `reason` omitempty; REQUIRED (non-empty) for every `rejected_*` and `read_error`
  verdict — it is the queryable rejection explanation (AC#5 "including rejected/never-
  grounded calls").
- `verdict` enum exactly as data-model §5: `landed | rejected_gate |
  rejected_cardinality | rejected_unknown | rejected_malformed | read_ok | read_error |
  unlanded`.
- Emission transport: batched through the social door with the cognition's other
  telemetry (mind: `emitCog`; metatron: its existing telemetry landing). Dry-run
  no-op application, all-or-nothing batch — same guarantees as `cog.thought` today.

## Changed payload: `agent.intent_set` (`IntentSetPayload`, internal/sim/agents.go:624)

```go
// added field, LAST in the struct, TASK-32 pattern:
Job string `json:"job,omitempty"`
```

- Set from `InjectArgs.JobID` at the inject-landing emission site only
  (internal/sim/loop.go intent-landing arm). Reflex-authored (`decideIntent`) and
  executor-authored (adapt/continue) intent_set events carry no job → field absent →
  **pre-feature logs and reflex emissions marshal byte-identically to today**.
- `agent.plan_set` already carries `Job` (loop.go:545) — unchanged.
- Snapshot compatibility: `IntentSetPayload` is an event payload, not snapshot state;
  no snapshot schema change. (Verify: snapshots serialize `State`, and `State` gains no
  field from this feature.)

## Correlation chain (the AC#5 query contract)

For any grounding event, the full chain resolves by identifier alone:

```
agent.intent_set{job=J}  ─┐
agent.plan_set{job=J}     ├─→ cog.tool_call{job=J, ordinal=k, verdict=landed}
agent.thought (muse)      │      ↑ sibling records: ordinals 1..k-1 (reads, rejections)
                          └─→ cog.thought{job=J} (trigger) / cog.outcome{job=J} (close)
```

- Every landed acting call has ≥ 1 grounding event carrying its `job`.
- Every rejected/malformed/unknown/unlanded call exists as `cog.tool_call` with NO
  grounding event — present and queryable, grounding nothing.
- **Outcome multiplicity (as-built note, T012)**: the intent door records one
  `cog.outcome` per acting inject (its existing non-silent-rejection contract), so an
  intra-cognition retry (gate-reject → retry → land) yields multiple `cog.outcome`
  events sharing the job — rejection verdict(s) then the landing. The mind adds its own
  `OutcomeUnusable` only when NO acting call reached a door. Chain-walking by job is
  unaffected; consumers must not assume exactly one outcome per job.
- **Chain granularity (as-built note, T019)**: grounding events carry `job` but not
  `ordinal`, so "a rejected call grounds nothing" is job-resolvable only when NOTHING
  in that job landed (a rejected-then-retried-then-landed job holds both a rejected
  record and a grounding event). Call-level attribution within a landed job comes from
  the records' verdicts — sufficient for SC-003; pinned by
  TestToolCallCorrelationChainSC003.
- Muse landings: `agent.thought` carries no job field today; the muse handler's
  `cog.tool_call{verdict:landed}` plus the batch's shared `cog.outcome{job}` closes the
  chain without touching `agent.thought`'s payload (byte-stability preferred over
  redundant threading; revisit only if a consumer needs direct thought→job linkage).

## Byte-stability obligations (test-pinned)

1. Pre-feature fixture logs replay to byte-identical state under new code (existing
   suite: whole_feature_test.go:32, sim_test.go:68,99, per-capability replay tests —
   pass UNMODIFIED).
2. `IntentSetPayload` marshaling without `Job` is byte-identical to pre-feature output
   (unit-pinned, the TASK-32 precedent test shape).
3. New-code live run vs replay-from-genesis vs replay-from-snapshot: byte-identical
   states with the model/loop absent during replay (SC-002).
4. `cog.tool_call` payload field order is canonical and covered by a marshal-order
   test (future additive fields go last, omitempty).

## Retired emissions

None. The scheduled-musing *channel* dies, but its event vocabulary (`agent.thought`,
`cog.thought`, `cog.outcome`) remains, now emitted via the muse tool handler inside a
loop cognition. Event-type census: +1 type (`cog.tool_call`), -0.

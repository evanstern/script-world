# Contract: cog.outcome retry & raw-reply semantics

Consumers: `promptworld tail`/`attach` readers, the TUI cognition panel, telemetry
aggregation (`internal/mind/telemetry.go`), and offline analysis over `world.db`.

## Event shapes

### Non-terminal retry marker (new outcome value)

```json
{
  "type": "cog.outcome",
  "payload": {
    "job": "convo-<tick>",
    "class": "conversation",
    "outcome": "retried",
    "reason": "outcome: bad outcome JSON: invalid character 'H' looking for beginning of value",
    "raw": "{\"gist\": Hazel and Rowan talked about the fire, \"topics\": [\"fire\"]}",
    "actual_wall_ms": 41200
  }
}
```

- Emitted when a scene reply fails to parse AND the scene continues via retry.
- `raw` is the verbatim failed reply, ≤2048 bytes, rune-boundary truncated with
  `…[truncated]` suffix when cut.
- `reason` is prefixed `"utterance turn <t>: "` or `"outcome: "` to locate the site.

### Terminal outcomes (existing values, extended)

- `outcome: "landed"` — MAY carry `retried: true` (scene consumed ≥1 retry). NEVER
  carries `raw`.
- `outcome: "unusable"` — abandonment. On parse-failure abandonment carries `raw`
  (the RETRY attempt's reply — the first attempt's raw already rode the `retried`
  marker). On transport-error abandonment carries no `raw`.
- `outcome: "rejected-stale"` — unchanged; MAY carry `retried: true`.

## Compatibility rules

1. One terminal `cog.outcome` per scene, exactly as today. `retried` markers are
   additional non-terminal events; consumers counting job completions MUST filter
   `outcome == "retried"` out (it is informational).
2. New fields are additive JSON — absent means false/none. No existing field
   changes type or meaning.
3. Bound: a scene emits at most 2 `retried` markers (one per site) + 1 terminal.

## Measurability contract (FR-005 / SC-001..003)

From `world.db` alone:
- first-try success: terminal `landed` without `retried`
- retry success: terminal `landed` with `retried: true`
- double failure: `retried` marker followed by terminal `unusable` for the same job
- every parse failure's verbatim reply: `SELECT json_extract(payload,'$.raw') FROM
  events WHERE type='cog.outcome' AND json_extract(payload,'$.raw') IS NOT NULL`

## Behavioral contract (scene runner)

- Parse/validation failure → eligible for exactly one retry per site.
- Transport/admission failure (Submit error, ctx expiry, queue-full) → immediate
  abandonment, never retried (backpressure doctrine).
- Retry requests are identical `KindConversation` submissions on the same scene ctx
  (same prio lane, same `convoDeadline`, same staleness budget at landing).
- Happy path (no failures): zero additional Submit calls; emitted events are
  byte-identical to the pre-change system except that... they are byte-identical
  (no `retried`, no `raw`). (SC-004 golden test.)

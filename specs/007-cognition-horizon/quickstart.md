# Quickstart: Validating the Cognition Horizon

Prerequisites: Go toolchain, a local Ollama serving the configured model (or any
OpenAI-compatible endpoint in `llm.json`). Cloud tier optional throughout.

## 1. Unit + integration tests

```sh
go test ./internal/cognition/ ./internal/mind/ ./internal/sim/ ./internal/llm/
go test ./...   # full suite, includes replay byte-equality
```

Expected: registry completeness and Fibonacci validation, router purity table,
estimator spike/drift behavior, staleness/guard/generation rejection at the door,
pause-landing-at-zero-staleness, and byte-identical replay all green.

## 2. Calibrate a world (US1)

```sh
promptworld new --dir /tmp/horizon-w --name horizon
promptworld calibrate --dir /tmp/horizon-w
cat /tmp/horizon-w/calibration.json
```

Expected: profile written; summary prints seconds-per-point and the speed ladder
verdict per class ("planner suppressed above 16x" on a ~17 s/pt host). See
[contracts/cli.md](contracts/cli.md).

## 3. Telemetry + causality (US1, SC-002, SC-007)

```sh
promptworld start --dir /tmp/horizon-w
promptworld speed --dir /tmp/horizon-w 4x
sleep 300
sqlite3 /tmp/horizon-w/world.db \
  "SELECT type, count(*) FROM events WHERE type LIKE 'cog.%' GROUP BY type"
```

Expected: `cog.thought` and `cog.outcome` present; every `cog.thought`'s `job` has
exactly one `cog.outcome` (audit query in the e2e test); outcomes carry
`snapshot_tick`/`landing_tick`/`trigger_seq` — pick any `agent.intent_set`, walk
`job` → `cog.thought` → `trigger_seq` back to the stimulus event.

## 4. The horizon at speed (US2, SC-001, SC-006)

```sh
promptworld speed --dir /tmp/horizon-w 32x   # slow local model
sleep 300
sqlite3 /tmp/horizon-w/world.db \
  "SELECT json_extract(payload,'$.outcome'), count(*) FROM events
   WHERE type='cog.outcome' GROUP BY 1"
```

Expected at 32x on a ~17 s/pt host: planner jobs show `suppressed` with the
arithmetic in `reason`; musings still land; **zero** outcomes execute with
`staleness_ticks` over their class budget (SC-001 audit). Back at 1x, planners route
to the model again (SC-006).

## 5. Pause semantics (US5)

```sh
promptworld speed --dir /tmp/horizon-w 4x
# wait for a conversation to found (tail the log), then:
promptworld pause --dir /tmp/horizon-w
sleep 120
sqlite3 /tmp/horizon-w/world.db \
  "SELECT type, tick FROM events ORDER BY seq DESC LIMIT 20"
```

Expected: the in-flight scene lands at the frozen tick (`staleness_ticks: 0` on its
outcome); no new `cog.thought` rows appear while paused; resume brings cadence back
with no burst.

## 6. Replay determinism (SC-003)

```sh
promptworld stop --dir /tmp/horizon-w
go test ./e2e/ -run Replay   # replays the log, byte-compares derived state
```

Expected: byte-identical. Telemetry payloads (wall durations, predictions) are
recorded data — replay reads them; nothing recomputes.

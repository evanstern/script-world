# Quickstart: validating the agent tool-use loop (spec 017)

Runnable checks proving the feature end-to-end. Contracts referenced:
[loop-api.md](contracts/loop-api.md), [events.md](contracts/events.md),
[provider-wire.md](contracts/provider-wire.md).

## Prerequisites

- Go toolchain (repo-pinned); repo root on the task worktree
  (`.worktrees/task-52`).
- Local tier: an OpenAI-compatible endpoint (Ollama with the configured gemma-class
  model) at `llm.json: local.endpoint`.
- Cloud tier (optional for unit/replay checks, required for SC-001 cloud leg):
  `ANTHROPIC_API_KEY` set; budget headroom in `llm.json`.

## 1. Unit + wire + replay gates (no network)

```bash
go test ./internal/toolloop/...   # driver: cap, cardinality, records, observe
go test ./internal/llm/...        # wire fixtures per mode; nil-Tools regression pin
go test ./internal/tool/...       # Number kind, InputSchema derivation, set_plan, Validate
go test ./internal/sim/... ./internal/mind/... ./internal/metatron/...
go test ./...                     # full suite — pre-feature replay fixtures UNMODIFIED
```

Expected: all green; the pre-existing replay/byte-identity tests pass with zero edits
(events.md obligation 1–2).

## 2. Live smoke — local tier, native mode (Story 1 + 4)

```bash
promptworld init --world /tmp/pw017 && promptworld run --world /tmp/pw017 &
sleep 300 && promptworld stop --world /tmp/pw017   # or the project's usual drive commands
```

Then inspect the log (project's event query tooling / sqlite):

- `cog.tool_call` events exist; each planner-class `cog.outcome{job}` has ≥1 sibling
  `cog.tool_call{job}`.
- Landed cognitions: `agent.intent_set` / `agent.plan_set` carrying `job` that resolves
  to a `cog.tool_call{verdict:landed}` — no adjacency needed (SC-003 spot check).
- Some cognitions choose `muse` → `agent.thought` present, and NO scheduled-musing
  telemetry pattern remains.

## 3. Fallback mode (Story 4 scenario 3)

Set `llm.json: local.tool_mode: "json"`, repeat §2. Expected: identical event-log
shape (verdicts, correlation, outcomes) — the wire mode is invisible in artifacts.

## 4. Cloud leg + metatron (Story 4 scenario 1, R13)

With the API key set, drive a metatron turn (existing CLI/UI path). Expected: turn
completes via native tool use; a nudge call lands `metatron.nudged` +
`agent.memory_added` with the turn's `cog.tool_call{verdict:landed}`; a
charge-exhausted nudge shows `verdict:rejected_gate` with reason, and no world events.

## 5. Replay determinism (Story 2, SC-002)

```bash
promptworld replay --world /tmp/pw017 --verify   # or the project's replay-verify path
```

Expected: byte-identical terminal state; zero model/tool-handler invocations (replay
binary has no live orchestrator wired).

## 6. Budget & governor sanity (Story 5, SC-004)

- Set `monthly_budget_usd` to a value already consumed → drive a cloud cognition →
  expect refusal BEFORE any spend; `cog.outcome` failure family; `unlanded` verdicts.
- Soak ≥100 local cognitions → estimator telemetry shows sec/pt converging on
  whole-loop wall time; no permanent breach state; route verdict strings remain pure
  arithmetic.

## 7. Cap behavior (Story 1 scenario 3)

Unit-level (stub model that never acts): loop ends at `loop_max_rounds`, termination
`cap_exhausted`, zero grounding events, all calls recorded. Config knob: set
`loop_max_rounds: 2` in a scratch world and observe earlier caps in telemetry.

## Success = board ACs

| Board AC (TASK-52) | Proven by |
|---|---|
| #1 loop + registry + cap | §1 driver tests, §2 smoke |
| #2 events + replay identity | §1 replay gates, §5 |
| #3 local + cloud + fallback | §2, §3, §4 |
| #4 metering/governor | §6 |
| #5 first-class correlatable trace | §2 log inspection, correlation test in §1 |

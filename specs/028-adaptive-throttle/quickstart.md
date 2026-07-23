# Quickstart Validation: Adaptive Time Throttling

Runnable proof that the governor works end-to-end. Prerequisites: Go toolchain per `go.mod`, a configured local
model (see `docs/llm-providers.md`) for the live scenarios, `export PATH="/opt/homebrew/bin:$PATH"`.

## 1. Unit + race proof (no model needed)

```sh
go test ./internal/cognition/ ./internal/sim/ ./internal/llm/ ./internal/ipc/ ./internal/tui/
go test -race ./internal/llm/ -run PendingCognition
go test ./...        # full suite green is the phase gate
```

Expected: controller table tests prove shed at sustained breach, multi-notch descent, 1x-floor saturation,
projected-debt parking (no oscillation), window resets on pause/player-change; replay tests prove a log containing
sheds, recoveries, player overrides, and a mid-governed pause replays byte-identical (SC-001).

## 2. Debt is visible before it governs (US1)

In a scratch home (`export PROMPTWORLD_HOME=$(mktemp -d)`): create a world with a slow local model configured,
start the daemon, set speed 32x, and let villagers think.

- `promptworld status` (or the TUI header) shows `governor_debt` / `governor_jobs` rising while planner thoughts
  are in flight and draining to exactly 0 when the world quiesces (US1-AC1/AC2).
- With no `llm.json`, the same commands show empty/zero governor fields at every speed (US1-AC4, SC-004).

## 3. The crisis scenario (US2 + US3)

At requested 32x, provoke a burst of concurrent cognition (several villagers + a conversation; the spec-023-style
scripted-latency stub makes this deterministic in tests).

- Watch the event log (TUI digest or `promptworld events`): a `clock.governor_shed` lands carrying
  `{requested: 32x, from: 32x, to: 16x, debt, jobs}` after the breach window (US2-AC1); sustained load sheds
  further, never below 1x and never to max (US2-AC2/AC4).
- TUI header reads `asked 32x — N minds in flight, debt P%` while governed (US4-AC1, FR-015).
- When the burst ends, `clock.governor_recovered` events climb back notch-by-notch, with observably longer gaps
  than the sheds took (asymmetry, US3-AC4); at requested speed the governor goes quiet (US3-AC3).
- SC-002 check: compare `rejected-stale` cog outcomes for the same scripted burst with the governor on vs off
  (test harness flag) — the governed run discards at most half as many landings.

## 4. Player override + pause (US4)

While governed at 8x (requested 32x):

- `promptworld speed 4x` → pacing changes immediately, status shows requested == effective == 4x, governed state
  cleared (US4-AC2).
- `promptworld speed 32x` → ceiling raised instantly; governor re-sheds within one cadence if debt still breaches
  (US4-AC3).
- `promptworld pause` mid-governed → no governor events while paused; in-flight thoughts land at the frozen tick;
  resume restarts windows fresh — verify no shed/recover fires in the first breach/recovery window after resume
  without a fresh accrual (US4-AC4).
- `promptworld speed max` with an LLM configured → refused exactly as pre-028 (US4-AC5, FR-012).

## 5. Determinism (SC-001)

Copy the world dir from scenario 3/4 and replay from genesis (`go test ./internal/sim -run Replay` covers the
synthetic case; for the live world, the standing replay-verify tooling): byte-identical state required, on any
host speed — replay applies recorded governor events and never re-derives debt.

## Contract references

Event shapes: [contracts/events.md](contracts/events.md) · status fields:
[contracts/status-protocol.md](contracts/status-protocol.md) · APIs:
[contracts/internal-api.md](contracts/internal-api.md) · entities: [data-model.md](data-model.md).

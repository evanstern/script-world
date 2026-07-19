# Quickstart validation results — 2026-07-18

Manual run of every quickstart.md scenario against the built binary
(darwin/arm64, Apple Silicon MacBook), plus the automated suites.
Branch `001-world-daemon`, validated at the T028 polish pass.

## Automated suites

- `go build ./...`, `go vet ./...`, `gofmt -l .` — clean.
- `go test -race ./...` — all green, including `e2e/` (8 scenarios) and the
  10k/20k-tick determinism harnesses in `internal/sim`.

## Scenario A — always-on world (SC-001 sample, SC-002) ✅

- `new` → `start` → detached daemon answered status in well under 2 s.
- 35 real seconds at default 4x → tick 143 (≈4.08 ticks/s, within 5% of spec).
- `tail --follow` streamed live events; killing the tail client abruptly left
  the world running (detach ≠ pause), verified by continued tick advance.

## Scenario B — time is a dial (SC-004, SC-005) ✅

- `pause` froze the clock at tick 143 across 3 real seconds; `resume`
  continued from exactly tick 143.
- `speed max` on this machine: **~1.65M ticks/sec** — 58 game-days simulated
  in ~3 real seconds (95k events). `speed 4x` returned to 4.0 ticks/s.
- Local-model throughput, not this substrate, will set the real max-speed cap
  (as the grounding session predicted).

## Scenario C — kill -9 and lossless resume (SC-003) ✅

- `kill -9` at tick 4,969,980 with 95,347 events logged.
- Restart: **recovery in 18 ms** (snapshot + replay), well under the 10 s
  budget; clock resumed at the recorded tick; speed 4x preserved; seq
  contiguous (`daemon.started {"recovery_ms":18}` in the log).
- Restart-while-paused wakes paused (covered by e2e).

## Scenario D — determinism (SC-006) ✅

- Unit harness: two states, same seed, same command timeline, 10,000 ticks →
  byte-identical event sequences and equal state hashes; replay rebuilds the
  live state hash exactly (20,000 ticks).
- Full-binary e2e: two daemons, seed 777, ~20k+ ticks at max speed →
  identical sim histories over the common tick prefix.

## Scenario E — separable runs (FR-009) ✅

- `stop` → `cp -R w1 w1-archive` → `start w1-archive`: the copy is a
  complete, runnable world continuing from its archived tick.

## Drift found between contracts and behavior

- `status` at max speed can display the previous measured effective rate for
  up to one 5 s measurement window after a speed change (display lag only;
  bookkeeping is exact). Acceptable within FR-012's "surface it" intent.
- contracts/cli.md offered `attach` re-sync and `tail --since`; both work,
  but `attach` currently subscribes live-only (from the current head), which
  matches the contract's intent for an interactive viewer.

## Verdict

All scenarios pass. Backlog TASK-2 ACs map: Scenario A → AC#1, B → AC#2,
C → AC#3.

# Quickstart: validating the parallel local tier

Validation scenarios proving the feature end-to-end. Contracts:
[contracts/llm-config.md](contracts/llm-config.md); entity semantics:
[data-model.md](data-model.md).

## Prerequisites

- Go 1.26+ (`go build ./...` from the repo/worktree root)
- Local model server up with the always-on model loaded:
  `curl -s http://localhost:11434/v1/models` should list `gemma4:12b-mlx`
- Known-good operator configs (skips a fresh calibrate run):
  - calibration profile: `~/.scratch/calibration.json` (gemma4:12b-mlx, 2026-07-21)
  - llm.json template: `~/worlds/village03/llm.json`

## 1. Unit + race suite (SC-003, FR-002/003/005/006)

```bash
go test ./internal/llm/ ./internal/cognition/ -race -count=1
go test ./... -count=1        # full suite unchanged with the field absent (SC-003)
```

Expected: all green. The new tests cover N-in-flight concurrency, slot-aware
best-effort admission, exactly-once breaker/meter accounting under parallel load,
and clamp normalization.

## 2. Live world at parallel 4 (US1/US2, SC-001/002/004)

```bash
# Fresh world in the worktree scratch area
scriptworld new paralleltest            # or the current world-creation path
cp ~/.scratch/calibration.json <world-dir>/calibration.json
cp ~/worlds/village03/llm.json <world-dir>/llm.json
# edit <world-dir>/llm.json: add  "parallel": 4  to the "local" object

scriptworld start paralleltest          # watch the boot lines
```

Expected boot line includes the effective parallelism (`parallel 4`) and the
calibration-seeded line (`local 8.1s/pt`).

Then, at speed (8×–32×), compare against a `parallel`-absent run of the same world:

- planner herd after restart: rejected-stale share collapses (SC-001)
- musings appear while planners are in flight (SC-002 — previously near-total silence)
- `scriptworld tail` / TUI status shows queue depth no longer pinned at cap

## 3. Clamp behavior (FR-007)

Set `"parallel": 64`, restart the daemon. Expected: boot warning naming the clamp
to 16, world starts normally. Repeat with `"parallel": -2` → effective 1 + warning.

## 4. Concurrency micro-benchmark (SC-004)

With the world quiet, fire 4 short local calls at once (test hook or the
calibrate command's probe path) and compare wall clock to a single warm call:
4-wide must finish in ≤2× the single-call wall time (reference measurement:
4-in-0.98s vs 3.8s cold single).

## 5. Post-run accounting (SC-005)

After a sustained run: spend meter total equals the sum of per-call costs (cloud
calls only — local is free), and `scriptworld` status shows both tiers healthy
(no stuck breaker/probe state).

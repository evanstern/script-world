# Quickstart: validating Governor Accrued-Drift Debt

## Unit-level validation (primary)

```sh
go test ./internal/cognition/ -run 'Debt|Governor' -v
go test ./internal/daemon/ -run 'Governor' -v
```

Expected:
- **World-01 regression (SC-001, red-first)**: 8 planner thoughts predicted 1.573 s
  / elapsed 30 s at 8 t/s yield debt 1.6 (was 0.0) and a shed decision on the 5th
  consecutive sample (see contracts/debt-formula.md worked example).
- **Monotonic stuck thought (SC-002)**: one thought sampled at growing ElapsedSec
  past its prediction contributes a non-decreasing, never-zero fraction.
- **Within-prediction bit-identical (SC-003)**: table of healthy pending sets
  (elapsed < predicted) produces output equal to the spec 028 arithmetic to full
  float64 equality.
- Existing hysteresis tests (shed/recover windows, pause reset, ladder saturation)
  green unmodified.

Full gates:

```sh
go test ./...
gofmt -l internal/   # only the 5 pre-existing TASK-83 files may appear
go vet ./...
```

## Live/operational verification (US2 — FR-006, recorded on TASK-87)

1. **Binary check** (mandatory — world-01's running daemon was built before the
   task-33 merge): rebuild `go build -o promptworld ./cmd/promptworld`, stop and
   restart world-01, confirm the governor sampler is active (status carries
   governor debt/jobs fields).
2. **Saturation probe**: run at 8x+ with a deliberately slow/underprovisioned
   provider serving the planner (or shrink its `parallel`), so thoughts overrun
   their predictions.
3. Observe:

```sh
sqlite3 ~/.promptworld/worlds/world-01/world.db \
  "SELECT tick, type, payload FROM events WHERE type LIKE 'clock.governor%' ORDER BY seq DESC LIMIT 5;"
```

Expected: at least one `clock.governor_shed` with its debt arithmetic; the TUI /
status shows effective speed below requested during the governed interval; speed
recovers per the asymmetric hysteresis once the backlog drains. Record the result
(event seq + tick) on TASK-87.

## Post-merge gates (not part of this run)

- `/grounding-wiki:wiki-update` re-pins wiki notes sourcing
  internal/cognition/governor.go.
- `node .claude/skills/player-docs/scripts/check-freshness.mjs --check`.

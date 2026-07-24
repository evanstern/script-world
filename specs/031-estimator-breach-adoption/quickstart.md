# Quickstart: validating Estimator Breach Adoption

## Prerequisites

- Go toolchain per go.mod; repo root.
- For the live probe: a world with an LLM provider configured (e.g. world-01) and a
  way to load the provider (running at 8x+ with several villagers suffices).

## Unit-level validation (primary)

```sh
go test ./internal/cognition/ -run 'Estimator' -v
```

Expected:
- **Freeze regression (SC-001)**: estimator seeded 0.52 s/pt fed 20 consecutive
  ~12 s/pt samples adopts the window median at the 20th sample; estimate ≈ 12, an
  adoption evidence value is returned exactly once.
- **One-shot preservation (SC-002)**: windows containing ≤2 spikes leave the
  estimate identical to pre-change EWMA arithmetic; no adoption evidence returned.
- **Re-arm (US3)**: after adoption, stable samples at the new level produce no
  second adoption; a second sustained >3× step does.

Full gates:

```sh
go test ./...        # includes TestCatalogSweep (digest catalog) and
                     # TestEstimatorSampleCountUnderConcurrency (mutex path)
gofmt -l .           # must print nothing
```

## Event-contract validation

```sh
go test ./internal/tui/ -run CatalogSweep -v
go test ./internal/sim/ ./internal/mind/
```

Expected: `cog.recalibration_recommended` decodes with and without the new
`prior_s_per_pt` / `adopted_s_per_pt` fields (see
[contracts/adoption-event.md](contracts/adoption-event.md)); historical fixtures
replay unchanged.

## Live-world probe (secondary, optional)

1. Build and restart the daemon: `go build -o promptworld ./cmd/promptworld`, stop
   and start the world.
2. Drive load: run at 8x with all villagers active on a deliberately small
   `parallel` for the planner's provider (or route planner to a slow local model).
3. Watch the log/event stream:

```sh
sqlite3 ~/.promptworld/worlds/<world>/world.db \
  "SELECT tick, payload FROM events WHERE type='cog.recalibration_recommended' ORDER BY seq DESC LIMIT 5;"
```

Expected: on sustained saturation, ONE event per breach episode now carries
`prior_s_per_pt` → `adopted_s_per_pt`, and subsequent `cog.outcome` rows show
`predicted_wall_ms` tracking `actual_wall_ms` within the spike factor (SC-003) —
rejected-stale landings for admitted thoughts fall off correspondingly.

## Post-merge gates (not part of this run)

- `/grounding-wiki:wiki-update` re-pins docs/wiki/cognition.md (sources include
  internal/cognition/estimate.go).
- `node .claude/skills/player-docs/scripts/check-freshness.mjs --check` for the
  player docs projection.

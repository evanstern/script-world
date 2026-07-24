# Quickstart Validation: Walls, Axes, and Paths (spec 032)

How to prove the feature works end-to-end. Contracts: [recipes](contracts/recipes.md),
[events](contracts/events.md); entities: [data-model.md](data-model.md).

## Prerequisites

- Go toolchain (repo root builds with `go build ./...`)
- No new dependencies, no migrations (additive omitempty state only)

## Gate 0 — build, boot coverage, determinism

```sh
go build ./... && go vet ./...
go test ./internal/sim/ ./internal/tool/ ./internal/tui/
```

Must pass: `ValidateToolCoverage` (six new verbs each have resolver + duration),
`tool.Validate`, recipe-mirror test against contracts/recipes.md literals, and the
existing replay/determinism suites (snapshot round-trip with new omitempty fields).

## Scenario checks (unit/integration tests in internal/sim)

1. **Axe economics (US2)**: agent with 1 plank + 1 stone → `craft_axe` → axe with 10 uses;
   chop yields 1 bare / 3 with axe; quarry the same; 10th axe-assisted harvest co-emits
   `agent.axe_broke` in the same batch and the axe leaves inventory; 11th harvest yields 1.
2. **Wall blocks & reroutes (US1)**: build a `wall_plank` between an agent and its target
   on the only direct corridor → next `nextStep` BFS detours; a full wall line across the
   corridor → intent resolves as unreachable (`agent.intent_done`); no agent ever occupies
   a wall tile (assert over a long run).
3. **Wall lifecycle (US1)**: plank wall HP 200, stone 600; demolish chips 100/cycle under
   ONE intent (WorkStart-reset loop) — plank falls in 2 cycles (`agent.wall_chipped` then
   `agent.wall_destroyed`), stone in 6; tile passable after collapse; repair on a chipped
   wall consumes 1 matching material per cycle, +100 HP clamped at max; repair at full HP
   does not resolve.
4. **Occupancy guard**: wall build whose Res tile holds an agent at completion resolves via
   `intent_done` (no wall, no spend); builder never builds under itself (adjacent-stand).
5. **Path speed (US3)**: fully-paved straight corridor traversed in half the ticks of the
   identical unpaved corridor (±1 step); mixed route: only steps FROM path tiles get the
   phase-2 slot.
6. **Storage symmetry**: drop/pick_up/deposit/withdraw with kind `"axes"` moves axes with
   uses preserved, sorted ascending; pile with only axes is non-empty; taking the last axe
   removes the pile.
7. **Replay determinism**: record a session exercising all of the above; replay produces a
   byte-identical state hash. A pre-032 snapshot (no hp/axes fields) loads unchanged.

## Live smoke (TUI)

```sh
go run ./cmd/promptworld   # or the project's usual daemon+TUI launch
```

- Map shows ▤ (plank wall), ▩ (stone wall), · (path); damaged walls render dim.
- Via set_plan or Metatron prompting, a villager: crafts an axe, chops (observe +3 wood),
  builds a wall, demolishes it, lays a path and is visibly faster along it.

## Post-merge (PDLC)

- `/grounding-wiki:wiki-update` — touched sources appear in `docs/wiki/executor.md`,
  `reflex-policy.md` (pathfinding), `sim-state-reducer.md`, `tool-registry.md`,
  `worldmap-generation.md` (passability language).
- `player-docs` freshness check, then regenerate.

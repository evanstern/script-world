# Implementation Plan: Walls, Axes, and Paths

**Branch**: `032-walls-axes-paths` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/032-walls-axes-paths/spec.md`

## Summary

Three build-system additions to the deterministic villager sim: **walls** (`wall_plank` /
`wall_stone` Structure kinds — the first movement-blocking structures, with HP, a
multi-cycle `demolish` verb, and a `repair` verb), **axes** (`Inventory.Axes`, a spear-clone
tool gating chop AND quarry yields: 1 bare / 3 with axe, replacing today's flat 2), and
**paths** (a `path` Structure granting exactly 2x movement via a second stateless cadence
phase slot; routing stays unweighted BFS per the spec clarification). All state is additive
`omitempty` (no format-version bump); all new arithmetic is integer, reducer-applied, and
replay-deterministic. Full design rationale in [research.md](research.md) (R1–R8), entities
in [data-model.md](data-model.md), literal numbers in
[contracts/recipes.md](contracts/recipes.md) and behavior in
[contracts/events.md](contracts/events.md).

## Technical Context

**Language/Version**: Go (module `github.com/evanstern/promptworld`, existing toolchain)

**Primary Dependencies**: none new — stdlib + existing internal packages
(`internal/sim`, `internal/tool`, `internal/worldmap`, `internal/tui`, `internal/store`)

**Storage**: event-sourced canonical-JSON state (existing store); additive omitempty
fields only, no migration, no format-version bump (research R7)

**Testing**: `go test ./internal/sim/ ./internal/tool/ ./internal/tui/` — table/scenario
tests beside code (repo convention: `*_test.go` per feature area)

**Target Platform**: the promptworld daemon + TUI (darwin/linux; byte-deterministic across
platforms via integer math)

**Project Type**: single Go project, multi-package feature

**Performance Goals**: `passable` gains a structure scan inside BFS — same linear-scan
shape as existing overlay checks; negligible at current map/structure counts

**Constraints**: byte-identical replay (determinism hash); pre-032 snapshots must load
unchanged; boot coverage gate (`ValidateToolCoverage`) must pass for all six new verbs

**Scale/Scope**: 3 new structure kinds, 1 new tool item, 6 new world verbs, 4 new event
types, 2 reducer-arm rebalances, 1 movement-cadence change, TUI glyphs; ~16 constants

## Constitution Check

*Constitution v1.1.0 (.specify/memory/constitution.md). Checked pre-Phase-0; re-checked
post-Phase-1 design — PASS on both.*

- **I. Artifact-Grounded Action** — PASS: spec → clarifications (recorded in spec) →
  plan/research/data-model/contracts on disk; board task via `spec-bridge:link` before
  implementation; decisions traceable to spec 012/013/014 precedents cited by file:line.
- **II. One Task, One PR** — PASS: one linked TASK, one worktree
  (`.worktrees/task-<N>`), one branch, one PR; user stories are internal phases, not PR
  boundaries.
- **III. Gates Over Assertions** — PASS: boot coverage gate + recipe-mirror test +
  determinism/replay suites are the physical evidence; spec-bridge gate holds board status
  to artifacts; no derived state hand-edited.
- **IV. Grounding Freshness** — PASS (planned): touched wiki notes (executor,
  reflex-policy, sim-state-reducer, tool-registry) re-pinned via
  `/grounding-wiki:wiki-update` before the TASK closes; player-docs freshness check after.
- **V. Model-Tiered Workflow** — PASS: this plan is planning-tier work; implementation
  delegates to the `spec-implementer` agent. **Tier: Opus 4.8** per rubric — cross-package
  (sim + tool + tui), and it changes core movement/pathability semantics (`passable`,
  movement cadence) plus introduces new executor scheduling shape (WorkStart-reset
  multi-cycle work); this is exactly the "architectural / core-loop" band, not a routine
  single-package slice. Justification to be recorded on the board task at delegation.

No violations → Complexity Tracking not needed.

## Project Structure

### Documentation (this feature)

```text
specs/032-walls-axes-paths/
├── spec.md              # feature spec (+ Clarifications session 2026-07-23)
├── plan.md              # this file
├── research.md          # Phase 0 — decisions R1–R8
├── data-model.md        # Phase 1 — Structure.HP, Inventory.Axes, Pile.Axes, helpers
├── quickstart.md        # Phase 1 — validation gates & scenarios
├── checklists/
│   └── requirements.md  # spec quality checklist (all pass)
├── contracts/
│   ├── recipes.md       # Phase 1 — recipe/registry literal numbers
│   └── events.md        # Phase 1 — new/reused event contracts
└── tasks.md             # Phase 2 (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/sim/
├── agents.go       # Inventory.Axes, Pile.Axes, Structure.HP, canonicalKinds+"axes",
│                   #   bulk(), spec-032 constants block, delete chopWood/quarryYield
├── recipes.go      # craft_axe, build_wall_plank, build_wall_stone, build_path;
│                   #   craftKindFor/craftGoalFor "axe"; wallRepairMaterial
├── terrain.go      # passable() wall scan; isWall/wallMaxHP/wallAt/pathAt/agentAt
├── policy.go       # 6 goalResolvers arms (wall builds adjacent-stand; demolish;
│                   #   repair; build_path; craft_axe joins craft closure)
├── executor.go     # movement dual-phase cadence; wall-build completion (Res-tile
│                   #   validation + occupancy guard); demolish/repair work cycles;
│                   #   chop/quarry axe branch + agent.axe_broke co-emit
├── state.go        # reducer arms: built (HP stamp), chopped/quarried rebalance +
│                   #   axe spend, crafted "axe", axe_broke, wall_chipped,
│                   #   wall_destroyed, wall_repaired; storage arms move axes
├── miracles.go     # give_item grants axes; entity_removed already covers walls/paths
└── *_test.go       # wall_test.go, axe_test.go, path_speed_test.go + updated
                    #   craft/quarry/toolcheck/state suites

internal/tool/
└── registry.go     # 6 worldToolsBase rows + glosses; itemKinds+"axes"

internal/tui/
└── views.go        # glyphs: ▤ wall_plank, ▩ wall_stone (dim when damaged), · path
```

**Structure Decision**: existing single-project layout; the feature is a multi-package
slice over `internal/sim` (core), `internal/tool` (registry), `internal/tui` (rendering).
No new packages, files added only for tests.

## Phase summary

- **Phase 0 (research.md)**: 8 decisions, all NEEDS CLARIFICATION resolved (two were
  settled in the spec's clarification session). Key calls: walls as two Structure kinds
  with derived max HP (R1); adjacent-stand wall builds + `passable` structure scan, free
  re-routing via per-step BFS (R2); paths as structures + stateless dual-phase cadence for
  exact 2x (R3); axe as full spear clone including storage plumbing, yields 1/3 replacing
  flat 2 (R4); demolish/repair as multi-cycle work via reducer `WorkStart` reset (R5);
  registry/coverage-gate integration, planner-only (R6); TUI + no version bump (R7);
  constants table (R8).
- **Phase 1 (data-model.md, contracts/, quickstart.md)**: entities and invariants (wall
  HP never serializes ≤ 0; axes sorted ascending; one improvement per tile), event
  contracts (4 new types, batch-ordering rules), validation gates (build/coverage/
  determinism, 7 scenario checks, TUI smoke).
- **Phase 2**: `/speckit-tasks` generates tasks.md; then `spec-bridge:link` puts the spec
  on the board before implementation (constitution Development Workflow).

## Post-plan agent context

No `update-agent-context.sh` ships in this repo's Spec Kit install (`.specify/scripts/bash/`
has only prerequisite/setup scripts) — agent grounding lives in CLAUDE.md + `docs/wiki/`,
re-pinned post-merge per Constitution IV; step intentionally skipped.

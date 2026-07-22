# Implementation Plan: Inventory & Storage v1

**Branch**: `task-51-inventory-storage` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/013-inventory-storage/spec.md`

## Summary

Add the storage layer over spec 012's resource economy: a single derived bulk cap
(24, everything costs 1) on every villager; emergent ground piles from a new drop
action (one pile per tile, adjacency read as stockpile zones at render time —
no zone state, no player zoning); death spills as reducer-internal pile creation;
builder-owned finite chests (6 planks, 48 bulk) as a `Structure` extension; theft
recorded-never-prevented through the existing relation/memory/rumor machinery via
a companion event batch; and per-batch food rot on the ground (2 game days) with
chests as the larder. All five new goals (`drop`, `pick_up`, `deposit`,
`withdraw`, `build_chest`) are planner/plan-only — the reflex is untouched and
degraded-mode survival is proven cap-safe. Because yield truncation, death
spills, and the give-guard change replay behavior of existing event shapes, the
world format bumps 2→3 with a people-preserving, **no-land-reset** migration
reusing 012's `world.migrated` machinery (over-cap carry spills to a pile).
Decisions R1–R9 in [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.x (existing module; no new language features)

**Primary Dependencies**: stdlib only, per existing `internal/sim` discipline
(canonical-JSON structs, FNV-64a state hash). No new dependencies.

**Storage**: event log + snapshots via existing `internal/store` /
`internal/world`; manifest `format_version` 2→3 (existing rejection + migrate
machinery)

**Testing**: `go test ./...`; replay byte-identity over a storage-exercising
script, bulk-audit table test, degraded-mode 3-day survival (zero storage
events), theft companion-batch test, rot window test, 2→3 migration fixture test
(see [quickstart.md](quickstart.md))

**Target Platform**: darwin/linux dev machines (daemon + TUI), byte-deterministic
across platforms per integer-math discipline

**Project Type**: single Go module — sim substrate (`internal/sim`), minds
(`internal/mind`), manifest (`internal/world`), rendering (`internal/tui`), CLI
(`cmd/promptworld`)

**Performance Goals**: preserve executor throughput headroom; new per-tick cost
is zero — the rot sweep rides the existing per-game-minute heartbeat, and bulk
is an O(1) sum computed at completion edges only

**Constraints**: byte-determinism (structs-never-maps, fixed iteration orders:
pile slice order, batch drop order, canonical kind order); outcome-only payloads
(actual post-clamp counts); unknown event types no-op in old replay code; reflex
degraded-mode contract untouched (FR-003/FR-014: zero storage in the reflex);
`stepEvents` stays a pure function of (pre-tick state, map, next tick)

**Scale/Scope**: 8 agents, 64×64 map; 6 new event types + 1 new `agent.built`
kind; 5 new goals; 1 new state slice (`Piles`), 2 new `Structure` fields, 2 new
intent/plan-step fields; 7 tuning constants; format bump + one migration step

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I. Artifact-grounded action | Spec from TASK-26's recorded session decisions; plan/research/data-model/contracts/quickstart in `specs/013-*`; implementation tracked on TASK-51 | PASS |
| II. One task, one PR | TASK-51 is the single deliverable: one branch (`task-51-inventory-storage` worktree under `.worktrees/task-51`), one PR; spec phases are internal breakdown | PASS |
| III. Gates over assertions | spec-bridge links TASK-51 ↔ specs/013 (Spec marker on the task); status driven by artifacts; Stop gate active | PASS |
| IV. Grounding freshness | Post-merge wiki re-pin list pinned in quickstart.md (executor, event-types, sim-state-reducer, reflex-policy, agent-mind, social-fabric, world-migration, world-save-directory, snapshots, tui-client, cli-promptworld, testing-strategy) | PASS (planned) |
| V. Model-tiered workflow | Planning on Fable 5 (this session); implementation via `spec-implementer` subagents — tier per slice in research.md R9 (Opus 4.8: substrate + executor/social wiring; Sonnet: planner vocabulary, TUI); tier + rubric justification to be recorded on TASK-51 at dispatch | PASS |

**Post-Phase-1 re-check**: design adds no new packages, no config-driven
behavior, no cross-plugin coupling; chest rides the existing `Structure`
lifecycle, rot rides the existing heartbeat, theft rides existing social types.
Violations: none. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/013-inventory-storage/
├── spec.md              # Feature spec (TASK-26 design session output)
├── plan.md              # This file
├── research.md          # Phase 0: decisions R1–R9
├── data-model.md        # Phase 1: state/entity shapes
├── quickstart.md        # Phase 1: validation guide
├── contracts/
│   └── events.md        # New event types, companion batches, changed v3 semantics
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/sim/
├── agents.go            # Pile/FoodBatch structs, Structure.Owner/Store,
│                        #   Intent/IntentSetPayload Kind+Qty, bulk(), tuning
│                        #   consts, new payload structs, intentDuration entries
├── recipes.go           # +build_chest row
├── state.go             # Reducer cases: dropped/picked_up/deposited/withdrew/
│                        #   food_rotted; built{chest}; yield clamps; died spill;
│                        #   gave clamp; pile create/merge/remove helpers
├── social.go            # ChestTakenPayload + social.chest_taken record case
├── executor.go          # Completion handling for the five goals (instant +
│                        #   timed chest build), theft companion batch, rot sweep
│                        #   on the minute heartbeat, give-guard, zero-space
│                        #   gather guard, build-site pile exclusion
├── policy.go            # resolveGoal cases for the five goals (planner-only;
│                        #   reflex ladder untouched)
├── plan.go              # planGoals additions; PlanStep Kind+Qty
├── memory.go            # Salience entries: chest built, taking suffered/witnessed
├── migrate.go           # v2 legacy decode + pure v2→v3 transform (over-cap
│                        #   spill); 1→2→3 chaining
└── *_test.go            # Replay, bulk-audit, degraded-mode, theft, rot,
                         #   migration fixtures alongside

internal/world/
└── world.go             # FormatVersion 2→3; migrate orchestration reuse

cmd/promptworld/
└── (migrate command)    # 2→3 step; chained 1→2→3

internal/mind/
└── prompt.go            # goalVocabulary + kind/qty guidance for storage goals

internal/tui/
└── views.go             # Pile/chest glyphs, adjacency zone grouping (render-
                         #   side), contents/owner inspection, carried bulk n/24
```

**Structure Decision**: no new packages, no new files beyond what 012 already
established (`migrate.go` gains a step, `recipes.go` a row); every change
extends an existing file in its established pattern. The event-sourced substrate
(loop, store, ipc, cognition) is untouched; the reflex is untouched by
construction.

## Complexity Tracking

No constitution violations; table intentionally empty.

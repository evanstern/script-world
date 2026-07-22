# Implementation Plan: Resources, Food, and Crafting v1

**Branch**: `task-50-resources-food-crafting` | **Date**: 2026-07-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/012-resources-food-crafting/spec.md`

## Summary

Add the resource economy to the deterministic sim: rock-outcrop terrain + quarrying and
water collection (new gatherables), fine-grained raw/cooked/meal food replacing +350
meals, fuel-burning fires that cook, and a crafting layer (planks, refined stone, spear
with durability, plank shelter with rest bonus, wood-fueled oven with meals and baths).
Every mechanic lands in the established shape — (goal, duration, completion event,
reducer case) through the executor's intent state machine — with a world format-version
bump (1→2) as the pinned refuse-don't-migrate compatibility story. The reflex gains
exactly one rule (refuel fires); everything else is planner-initiated. Storage/carry
caps are explicitly deferred to spec 013 (designed, linked as TASK-51): 012 ships with
unbounded inventories.

## Technical Context

**Language/Version**: Go 1.x (existing module; no new language features required)

**Primary Dependencies**: stdlib only, per existing `internal/sim` / `internal/worldmap`
discipline (FNV-64a hashing, canonical-JSON structs). No new dependencies.

**Storage**: event log + snapshots via existing `internal/store` / `internal/world`;
world manifest `format_version` bumped 1→2 (existing rejection machinery, world.go:125)

**Testing**: `go test ./...`; determinism tests (same-seed map hash, replay
byte-identity), executor behavior tests alongside code, degraded-mode survival test
(3 game days, no planner, zero crafting events)

**Target Platform**: darwin/linux dev machines (daemon + TUI), byte-deterministic across
platforms per integer-math discipline

**Project Type**: single Go module — sim substrate (`internal/sim`), terrain
(`internal/worldmap`), minds (`internal/mind`), rendering (`internal/tui`), manifest
(`internal/world`)

**Performance Goals**: preserve current executor throughput headroom (>200k ticks/sec
in harness); new per-tick cost is one O(#structures) fuel sweep + unchanged BFS usage

**Constraints**: byte-determinism (integer math, structs-never-maps, canonical JSON,
fixed iteration orders); outcome-only event payloads (no dice rolls); unknown event
types no-op in old replay code; reflex degraded-mode contract (planner-less survival);
`stepEvents` stays a pure function of (pre-tick state, map, next tick)

**Scale/Scope**: 8 agents, 64×64 default map, ~9 new event types, ~9 new goals, 5 new
inventory fields + spear slice, 1 new TileKind, 1 new structure kind

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I. Artifact-grounded action | Spec, plan, research, contracts, tasks all in `specs/012-*`; decisions recorded on TASK-25; implementation tracked on TASK-50 | PASS |
| II. One task, one PR | TASK-50 is the single deliverable: one branch (`task-50-*` worktree under `.worktrees/`), one PR; spec phases are internal breakdown | PASS |
| III. Gates over assertions | spec-bridge links TASK-50 ↔ specs/012; status driven by artifacts; Stop gate active | PASS |
| IV. Grounding freshness | Post-merge wiki re-pin required for: executor, event-types, worldmap-generation, reflex-policy, sim-state-reducer, tui-client, agent-mind (goal vocabulary), snapshots (inventory shape) — called out in quickstart.md | PASS (planned) |
| V. Model-tiered workflow | Planning on Fable 5 (this session); implementation via `spec-implementer` subagents — tier recommendation per slice in research.md R9 (Opus 4.8: substrate + executor/reflex slices; Sonnet: TUI, vocabulary, recipes); tier + justification to be recorded on TASK-50 at dispatch | PASS |

**Post-Phase-1 re-check**: design introduces no new packages, no config-driven behavior,
no cross-plugin coupling; all violations: none. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/012-resources-food-crafting/
├── spec.md              # Feature spec (TASK-25 design session output)
├── plan.md              # This file
├── research.md          # Phase 0: decisions R1–R9
├── data-model.md        # Phase 1: state/entity shapes
├── quickstart.md        # Phase 1: validation guide
├── contracts/
│   ├── events.md        # New event types: payload structs, emitters, reducer effects
│   └── recipes.md       # The recipe/tuning table (mirrors internal/sim/recipes.go)
├── checklists/requirements.md
├── design-summary.html  # Session summary artifact
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/worldmap/
├── worldmap.go          # +Rock TileKind, outcrop placement in Generate, tuning consts
└── worldmap_test.go     # +outcrop presence/determinism/buildable-floor assertions

internal/sim/
├── agents.go            # Inventory expansion, Structure.FuelUntil, tuning consts,
│                        #   new payload structs, intentDuration additions
├── recipes.go           # NEW: the authoritative recipe table (R6)
├── state.go             # Reducer cases for new events; agent.ate rewrite
├── executor.go          # Fuel sweep (burnout events), completion handling for new
│                        #   goals, warmAt lit-ness, shelter rest bonus in decayNeeds
├── policy.go            # resolveGoal cases for new goals; reflex refuel rule
├── terrain.go           # Quarried overlay (effectiveKind/passable merge)
├── memory.go            # Salience entries: spear broke, bath, oven built, fire died
└── *_test.go            # Behavior + replay-determinism tests alongside

internal/mind/
└── prompt.go            # goalVocabulary + prompt guidance for new goals

internal/world/
└── world.go             # FormatVersion 1→2 (existing rejection path)

internal/tui/
└── views.go             # Rock/quarried glyphs, oven glyph, cold-fire styling,
                         #   inventory pane expansion
```

**Structure Decision**: no new packages beyond one new file (`internal/sim/recipes.go`);
every change extends an existing file in its established pattern. The event-sourced
substrate (loop, store, ipc, cognition) is untouched.

## Complexity Tracking

No constitution violations; table intentionally empty.

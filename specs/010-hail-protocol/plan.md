# Implementation Plan: Hail Protocol

**Branch**: `task-47-hail-protocol` | **Date**: 2026-07-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/010-hail-protocol/spec.md`

## Summary

A talk_to landing hails its target: a deterministic, zero-LLM sim event
(`social.hailed`) that pauses the target in place for a bounded game-tick window so
the hailer can close distance and the pair can actually meet. The landing ladder is
relaxed for hailable targets — a target beyond `presentRadius` that can be hailed
lands the intent as **adapted** instead of rejecting it — and the meeting itself is
founded deterministically on adjacency (`social.hail_met` + `agent.talked`),
bypassing the ambient talk cooldown. Expiry (`social.hail_expired`) restores the
target untouched. All state transitions are event-sourced through the reducer;
replay and snapshots are unaffected by construction.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**: stdlib only in the touched packages (`internal/sim`);
`internal/store` (SQLite event log) for event shapes

**Storage**: SQLite event log + JSON snapshots via `internal/store` (existing; new
event types only)

**Testing**: `go test ./...` — table-driven unit tests in `internal/sim`, replay
determinism harness (existing `sim_test.go` patterns)

**Target Platform**: macOS/Linux daemon (`scriptworld` binary)

**Project Type**: single Go module, layered CLI daemon + TUI

**Performance Goals**: per-tick hail sweep is O(agents)=O(8) with no allocation on
the quiet path; no measurable tick-rate impact at max speed

**Constraints**: strict determinism (no wall clock, no unseeded randomness, reducer
is the only mutation path); canonical state JSON must stay byte-stable for
pre-feature snapshots (new Agent field is a pointer with `omitempty`)

**Scale/Scope**: 8 agents, ~5 files in `internal/sim` + event contract doc + tests;
no TUI changes required (generic event rendering resolves `from`/`to` payload
fields to agent names already)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|-----------|-------|--------|
| I. Artifact-Grounded Action | Spec, plan, research, data-model, contracts, tasks all land in `specs/010-hail-protocol/`; board task TASK-47 linked via spec-bridge marker; before/after measurement recorded on the task | PASS |
| II. One Task, One PR | TASK-47 → one branch (`task-47-hail-protocol`) in `.worktrees/task-47`, one PR; spec phases are internal breakdown | PASS |
| III. Gates Over Assertions | spec-bridge sync moves TASK-47 only as artifacts prove phases; SC-001 measurement gates the "substantially reduced" claim | PASS |
| IV. Grounding Freshness | Touched files (`internal/sim/loop.go`, `executor.go`, `guard.go`, `state.go`, `agents.go`) are pinned sources of wiki notes (sim-loop, executor, sim-state-reducer, event-types, cognition); `/grounding-wiki:wiki-update` is a required post-merge step before Done | PASS (tracked as explicit task) |
| V. Model-Tiered Workflow | Plan/tasks authored on planning tier; implementation delegated to `spec-implementer`. Tier: **Opus 4.8** — the slice changes the cognition-horizon landing ladder (doctrine-adjacent, `internal/sim/loop.go` guard semantics) and executor scheduling behavior; rubric justification recorded on TASK-47 | PASS |

**Post-Phase-1 re-check**: design adds one nullable Agent field, three event types,
one predicate, and two tunables — no new packages, no new abstraction layers, no
violations. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/010-hail-protocol/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── events.md        # Hail event contracts (payloads, reducer semantics)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/sim/
├── hail.go              # NEW: tunables, hailable predicate, hail sweep (expiry + met)
├── hail_test.go         # NEW: unit tests (hailable matrix, pause, expiry, met, mutual)
├── agents.go            # Agent.Hail field + payload types
├── state.go             # reducer cases: social.hailed / hail_met / hail_expired; clear on death
├── guard.go             # (unchanged closed set — relaxation happens in the loop, not the guard)
├── loop.go              # inject_intent: hail rung on target_present failure; hail emission on talk_to landings
├── executor.go          # pause enforcement in per-agent step; hail sweep call
├── plan.go              # plan-step talk_to firing also hails
└── sim_test.go          # replay determinism covers hail events (existing harness)

internal/mind/           # no changes (hail path is sim-only; agent.talked already
                         # triggers maybeStartConversation)
internal/tui/            # no changes required (classDefault + from/to name resolution)
```

**Structure Decision**: all behavior lands in `internal/sim` (single package), new
logic concentrated in a new `hail.go` so the diff to load-bearing files stays
surgical. The mind layer is deliberately untouched: the hail changes what the world
does with a landing, not how thoughts are formed.

## Complexity Tracking

No constitution violations; table intentionally empty.

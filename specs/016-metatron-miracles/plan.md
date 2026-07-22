# Implementation Plan: Metatron Miracles

**Branch**: `016-metatron-miracles` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/016-metatron-miracles/spec.md`

## Summary

Give Metatron three charge-priced intervention families — time snap (shift semantics),
item grant, entity move/remove — landed as four new recorded event types through the
existing `InjectSocial` whitelist door, exactly on the `metatron.nudged` pattern
(dry-run at the door, validate-not-clamp, charge economy in the reducer). Add a
player-only operator surface (`promptworld miracle …` CLI → new `miracle` IPC command)
that can mark miracles gratis; the angel's model contract structurally cannot express
gratis. Affected villagers perceive miracles as recorded memories. Everything replays
byte-identically; a drift test proves the frozen-world snap semantics.

## Technical Context

**Language/Version**: Go 1.26.4 (single module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib + existing internal packages only — `internal/sim`
(reducer, loop, terrain), `internal/metatron` (turn), `internal/ipc`, `internal/clock`,
`internal/store`, `cmd/promptworld`. No new external dependencies.

**Storage**: existing SQLite event log + snapshots (`internal/store`); additive event
types only — no schema change, no format-version bump.

**Testing**: `go test ./...`; established patterns: reducer unit tests, replay
byte-identity (per `chest_test.go`/`craft_test.go` SC pattern), IPC round-trip tests
(`ipc_test.go`), metatron turn-parsing tests.

**Target Platform**: the `promptworld` daemon/CLI (macOS/Linux), unchanged.

**Project Type**: single Go module, CLI + daemon.

**Performance Goals**: none new — miracles are rare, human-initiated events; the only
hot-path consideration is that `rebaseTicks` is O(state) once per snap (negligible).

**Constraints**: replay determinism is inviolable (canonical state bytes, reducer as
single mutation path); the `injectSocialWhitelist` is the model-isolation boundary and
grows by exactly four entries; gratis must be structurally unreachable from model output.

**Scale/Scope**: 8 agents, 64×64 map, single world per daemon — unchanged. Feature
touches 5 packages; est. ~600–900 LOC including tests.

## Constitution Check

*GATE: evaluated against constitution v1.1.0 before Phase 0; re-checked after Phase 1.*

| Principle | Status | Evidence |
|---|---|---|
| I. Artifact-Grounded Action | PASS | Board task TASK-59 (decisions recorded); spec + clarifications + this plan on disk; motivating incident documented in TASK-59 description. |
| II. One Task, One PR | PASS | TASK-59 ↔ branch `task-59-metatron-miracles` in `.worktrees/task-59` ↔ one PR. Spec phases are internal breakdown only. |
| III. Gates Over Assertions | PASS | Spec will be linked via `spec-bridge:link` before implementation; bridge gate then caps TASK-59's status at what artifacts prove. |
| IV. Grounding Freshness | PASS (planned) | Post-merge `/grounding-wiki:wiki-update` required: wiki notes sourcing `internal/sim`, `internal/metatron`, `internal/ipc`, `cmd/promptworld` (at minimum agent-mind, llm-orchestrator adjacents, metatron note) must re-pin. |
| V. Model-Tiered Workflow | PASS (planned) | Implementation delegated to `spec-implementer`. **Tier: Opus 4.8** — rubric: doctrine-adjacent (isolation whitelist + charge economy), cross-package (sim/metatron/ipc/cmd), and the re-base helper touches replay determinism. Justification to be recorded on TASK-59 at implementation start. |

**Post-Phase-1 re-check**: PASS — design introduces no new projects, no new dependencies,
no parallel mutation paths (single reducer arm file + shared batch-builder), no state
struct changes. No Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/016-metatron-miracles/
├── plan.md              # This file
├── spec.md              # Feature spec (clarified 2026-07-22, 5 Qs)
├── research.md          # Phase 0: R1–R8 decisions
├── data-model.md        # Phase 1: event payloads, cost table, re-base taxonomy
├── quickstart.md        # Phase 1: runnable validation scenarios A–F
├── contracts/
│   └── interfaces.md    # Phase 1: turn contract, IPC cmd, CLI grammar, whitelist delta
├── checklists/
│   └── requirements.md  # Spec quality checklist (20/20)
└── tasks.md             # Phase 2 (/speckit-tasks — not yet created)
```

### Source Code (repository root)

```text
internal/sim/
├── miracles.go          # NEW: reducer arms for the 4 events, cost table, rebaseTicks
├── miracles_test.go     # NEW: reducer units, replay byte-identity, drift test,
│                        #      re-base guard test (unclassified-field detector)
├── state.go             # dispatch: route metatron.time_snapped/item_granted/
│                        #   entity_moved/entity_removed to miracles.go arms
└── loop.go              # injectSocialWhitelist: +4 entries

internal/metatron/
├── turn.go              # turnReply.Miracle field; landMiracle(); prompt cost table
├── miracle_batch.go     # NEW (or in turn.go): shared batch-builder (event + memories)
└── metatron_test.go     # + adversarial gratis-strip test, landMiracle units

internal/ipc/
├── protocol.go          # MiracleArgs / MiracleData
├── server.go            # case "miracle": build batch (shared builder), InjectSocial
└── ipc_test.go          # + miracle round-trip, gratis-only-here test

cmd/promptworld/
└── main.go              # miracle subcommand family + help text

internal/clock/          # (read-only use: day/HH:MM ↔ tick conversion; extend only
                         #  if no suitable parse helper exists)
```

**Structure Decision**: single-module layout unchanged; one new sim file pairs the four
reducer arms with the re-base helper so the whole miracle doctrine reads in one place,
mirroring how `metatron.go` holds the nudge/charge doctrine. The batch-builder lives
with the metatron package but is exported for the IPC server arm — one builder, two
doors, no drift (research R6).

## Complexity Tracking

No constitution violations; table intentionally empty.

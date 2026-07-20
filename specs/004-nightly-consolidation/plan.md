# Implementation Plan: Nightly Consolidation + Persona Firewall

**Branch**: `task-9-consolidation` | **Date**: 2026-07-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-nightly-consolidation/spec.md`

## Summary

When an agent sleeps (`agent.slept`, already emitted by the executor), the mind driver runs
one cloud-tier consolidation call for that agent per game night: the episodic buffer
(memories since the last consolidation) is compressed into promotions/fades, a day-gist
memory, belief revisions (confidence + provenance), and a replaced self-narrative. The
output passes a deterministic validator (structural checks + persona-anchor echo +
per-agent drift lexicon) and lands as ONE atomic whitelisted injection batch — the same
pattern conversations use — capped by a recorded `agent.consolidated` marker that the
reducer folds into state, making once-per-night idempotent across restarts and replay
model-free. persona.md keeps its existing genesis-only 0444 write path; the consolidator
touches nothing but events.

## Technical Context

**Language/Version**: Go 1.22+ (toolchain go1.24.x), same module

**Primary Dependencies**: existing internal packages only — `internal/sim` (state,
reducer, loop injection door), `internal/mind` (driver, prompts, parsing, validation),
`internal/llm` (KindConsolidation → cloud tier, anthropic or openai_compat router),
`internal/persona` (authored temperament anchors + drift lexicons), `internal/scribe`
(soul.md rendering). No new external dependencies.

**Storage**: the existing append-only SQLite event log + snapshots; new event types and
new `Agent` state fields ride the existing reducer/snapshot machinery.

**Testing**: `go test ./... -race`; scripted mock model for the driver; fixture outputs
for the validator; live smoke via the 9router cloud tier per quickstart.md.

**Target Platform**: macOS homelab (darwin/arm64), single operator, localhost daemon

**Project Type**: single Go binary (daemon + CLI), extending existing packages

**Performance Goals**: ≈ 8 cloud calls per game night (one per living agent), ≈ 32/real
day at 4x — negligible load; consolidation is latency-tolerant (whole night is the window)

**Constraints**: deterministic replay (no model calls in reducer path); atomic batch
landing; persona.md write-path unchanged (genesis-only, 0444); cloud-tier failure degrades
to skip-and-retain, never blocks the loop

**Scale/Scope**: 8 agents, buffer ≈ 20–60 memories/night at current emission rates;
oversized buffers truncated oldest-first for the call only

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` is the unfilled template (no project constitution
adopted); the standing project rules from CLAUDE.md apply instead: artifact-grounded
action, one TASK = one PR, statuses never exceed artifacts, wiki re-pinned before done.
No violations: this plan extends existing packages, keeps determinism and the firewall
structural, and lands as a single PR on `task-9-consolidation`. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/004-nightly-consolidation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── consolidation-events.md   # new event types + reducer semantics
│   └── consolidation-output.md   # model output JSON contract + validator rules
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/sim/
├── consolidate.go       # NEW: Belief type, event payloads, reducer cases, night guard
├── state.go             # Agent gains Beliefs/Narrative/LastConsolidatedNight/ConsolidatedUpTo/NextBeliefID
├── loop.go              # injection whitelist extended with consolidation event types
└── consolidate_test.go  # NEW: reducer, ledger idempotence, replay determinism

internal/mind/
├── consolidate.go       # NEW: trigger on agent.slept, night queue, prompt, batch build
├── validate.go          # NEW: deterministic validator (structure + anchor echo + drift lexicon)
├── parse.go             # parseConsolidation (strict JSON)
└── consolidate_test.go  # NEW: scripted-model driver tests, validator fixtures

internal/persona/
├── personas.go          # NEW per-agent: Anchor (temperament line) + DriftMarkers lexicon
└── files.go             # unchanged write path (genesis-only 0444) — asserted by test

internal/scribe/
└── scribe.go            # soul.md gains "Who I am becoming" + "Beliefs" sections
```

**Structure Decision**: extend the four existing packages listed above; no new packages,
no new binaries, no new storage. The mind driver owns everything model-facing; the sim
package owns everything state-facing; the boundary between them is the whitelisted
event batch, exactly as TASK-8 established for conversations.

## Complexity Tracking

No constitution violations to justify.

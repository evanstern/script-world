# Implementation Plan: Epistemic Hygiene for Emergent Lore

**Branch**: `task-79-epistemic-hygiene` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/030-epistemic-hygiene/spec.md`

## Summary

Three mechanisms that stop the epistemic bookkeeping from laundering invented lore into fact while keeping the lore
alive. (1) Memories gain an emission-stamped `Origin`; the consolidation contract gains per-belief evidence refs;
the deterministic validator coerces "witnessed" claims that lack direct-perception evidence to told/inferred —
never rejecting the night. (2) Beliefs gain a `Reinforced` tick; effective confidence is computed-on-read with an
8-game-day half-life and a floor of 20, below which beliefs leave prompts and render hedged in souls; a
`agent.belief_reinforced` event is the consumer-side seam for the future grounded-observation channel. (3) The
conversation outcome prompt preserves attribution and never asserts unperformed actions — shipped only through a
TASK-73-style eval gate with recorded numbers. All shapes additive-`omitempty`; replay byte-identity holds; no
format bump.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib only; existing internal packages (`sim`, `mind`, `scribe`, plus prompt/eval
tooling). No new third-party dependencies.

**Storage**: existing event log + snapshots; additive payload/state fields only

**Testing**: `go test ./...`; table-driven pure-function tests; replay determinism suite; judge-scored prompt eval
(scripts/eval-prompt-79.sh) per the TASK-73 precedent

**Target Platform**: existing daemon/CLI/TUI on macOS/Linux

**Project Type**: single Go project (existing layout)

**Performance Goals**: EffectiveConfidence is O(1) arithmetic at read sites; validator gains one pass over ≤4
evidence refs per belief; zero new steady-state work

**Constraints**: byte-identical replay (FR-011); validator stays deterministic (no model in enforcement); stored
state never mutates from decay; provenance labels never rewritten (FR-012); prompt change gated on eval numbers
(FR-010)

**Scale/Scope**: 3 packages of substance (`sim`, `mind`, `scribe`) + eval assets; 1 new event type; 2 new state
fields; 1 memory-payload field; no migration

## Constitution Check

*Constitution v1.1.0. Gate: PASS (pre-Phase-0) · re-checked PASS (post-Phase-1). No Complexity Tracking entries.*

- **I. Artifact-Grounded Action — PASS**: the board task's authored scope + Thornspire evidence are the inputs;
  open design points resolved from standing artifacts (validator absorb-slack doctrine, memory half-life, rumor
  floor, TASK-73 eval precedent) and recorded in research.md; constants land on TASK-79 per its own AC.
- **II. One Task, One PR — PASS**: implementation on `task-79-epistemic-hygiene` in `.worktrees/task-79`; one PR;
  planning artifacts ride main (027/028 precedent).
- **III. Gates Over Assertions — PASS**: spec-bridge mirrors phases; the eval gate is itself an artifact gate
  (decision.md + task numbers before the prompt ships).
- **IV. Grounding Freshness — PASS (obligation scheduled)**: touched sources appear in wiki notes
  nightly-consolidation, agent-mind, social-fabric, sim-state-reducer, event-types, testing-strategy — post-merge
  `wiki-update` + player-docs refresh are polish tasks.
- **V. Model-Tiered Workflow — PASS**: plan implements nothing. Tier ruling: validator/reducer/replay and the
  origin-stamp sweep (`internal/sim`, `internal/mind` doctrine-adjacent) → **Opus 4.8**; the gist-prompt + eval
  slice → **Opus 4.8** (prompt-behavior-affecting, the TASK-73 precedent tier); scribe/prompt rendering of
  effective confidence and doc reconciliation → Sonnet. Recorded on TASK-79 at dispatch.

## Project Structure

### Documentation (this feature)

```text
specs/030-epistemic-hygiene/
├── plan.md              # This file
├── research.md          # R1–R8
├── data-model.md        # Origin, Belief.Reinforced, contract, read sites
├── quickstart.md        # validation scenarios
├── checklists/requirements.md
├── contracts/
│   ├── consolidation-contract.md   # evidence refs + validator enforcement
│   ├── events-and-decay.md         # belief_revised/belief_reinforced + curve + read sites
│   └── eval-protocol.md            # TASK-73-style gate for the gist prompt
├── eval/                # created during implementation (fixtures, old/new, decision.md)
└── tasks.md             # /speckit-tasks output
```

### Source Code (repository root)

```text
internal/
├── sim/
│   ├── agents.go            # Memory.Origin field
│   ├── memory.go            # situated constructors take origin; emission sites stamp
│   ├── social.go            # conversation-gist injection stamps origin=gist
│   ├── consolidate.go       # Belief.Reinforced, EffectiveConfidence, constants,
│   │                        #   belief_revised payload fields, belief_reinforced reducer arm
│   └── *_test.go
├── mind/
│   ├── consolidate.go       # prompt: evidence instruction + held-beliefs effective values
│   ├── validate.go          # evidence resolution + provenance coercion + coercion telemetry
│   ├── convo.go             # outcome prompt attribution rules (eval-gated)
│   └── *_test.go
├── scribe/
│   └── scribe.go            # Beliefs section: effective values + hedged below-floor form
└── metatron/ (or the dream-delivery site)  # origin=omen stamp

scripts/eval-prompt-79.sh    # eval runner (modeled on eval-prompt-73.sh)
```

**Structure Decision**: no new packages; the classifier, curve, and constants live beside the belief reducer in
`internal/sim` (model-free half), enforcement lives in the existing validator (`internal/mind`), rendering in the
scribe — the same split spec 004/019 established.

## Complexity Tracking

No constitution violations; table intentionally empty.

# Implementation Plan: Decision-Trace View

**Branch**: `task-63-decision-trace-view` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/020-decision-trace-view/spec.md`

## Summary

Render the already-persisted `cog.*` verdict trail as a teaching surface: a bounded
per-agent **decision-trace projection** fed from the TUI's existing `applyEvent` path
(joining `cog.thought` / `cog.tool_call` / `cog.outcome` on their shared job ID), a
**decisions sub-view** inside the villager detail pane rendering per-cognition causal
chains (stimulus → thought → tool calls with verdicts → outcome), **inline verdict
rows** in the Metatron transcript for `turn-metatron-*` jobs, and a **plain-language
verdict glossary** both surfaces render from. Pure client-side feature: no daemon, sim,
event, or payload changes.

## Technical Context

**Language/Version**: Go 1.x (module `promptworld`, matches repo toolchain)

**Primary Dependencies**: Bubble Tea + Lipgloss (existing TUI stack); `internal/store`
(events), `internal/sim` (payload types) — all already imported by `internal/tui`

**Storage**: none — in-memory client-side projection, event-sourced from the existing
IPC subscription; lost on reconnect by design (matches raw-feed behavior)

**Testing**: `go test ./internal/tui/` — table-driven unit tests alongside code,
following the existing digest/villagers test patterns

**Target Platform**: terminal client (`promptworld ui`), same platforms as today

**Project Type**: single project — one package touched (`internal/tui`), plus a
read-only import of `internal/toolloop` verdict constants in tests for the glossary
sweep

**Performance Goals**: no render-path regression — projection ingest is O(1) per event;
rendering derives fresh per frame like every other pane body

**Constraints**: projection memory bounded (≤ 20 chains per agent × capped args);
`View()` output stays exactly terminal-height (existing clipping invariants);
focus-contract key routing preserved

**Scale/Scope**: ~4 files in `internal/tui` (new `decisions.go` + `decisions_test.go`;
edits to `tui.go`, `views.go`, and their tests); zero API surface change

## Constitution Check

*GATE: evaluated against constitution v1.1.0 before Phase 0; re-checked after Phase 1.*

- **I. Artifact-Grounded Action** — PASS: spec/plan/tasks under
  `specs/020-decision-trace-view/`, linked to board TASK-63 via spec-bridge before
  implementation; progress recorded on the board.
- **II. One Task, One PR** — PASS: TASK-63 → one branch (`task-63-decision-trace-view`)
  in `.worktrees/task-63`, one PR; spec phases are internal breakdown.
- **III. Gates Over Assertions** — PASS: board status advances only via
  `spec-bridge:sync` against spec artifacts; ACs ticked only with proof (tests, PR).
- **IV. Grounding Freshness** — PASS (with follow-through): touches
  `internal/tui/tui.go`, `views.go` — sources of the `tui-client` wiki note; TASK-63
  AC #5 mandates `/grounding-wiki:wiki-update` re-pin before Done.
- **V. Model-Tiered Workflow** — PASS: planning on Fable 5 (this document);
  implementation delegated to `spec-implementer`. Tier: **Sonnet (default)** — the
  rubric's routine profile fits: single-package feature, view/rendering code, tests
  alongside code, no concurrency/scheduling/governor logic, no doctrine-adjacent
  behavior change (the feature is read-only over persisted events). Escalation to Opus
  only if a Sonnet attempt fails gates.

**Post-Phase-1 re-check**: PASS — design introduces no new packages, no daemon surface,
no cross-package writes; Complexity Tracking empty.

## Project Structure

### Documentation (this feature)

```text
specs/020-decision-trace-view/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: design decisions + rationale
├── data-model.md        # Phase 1: projection entities & bounds
├── quickstart.md        # Phase 1: validation guide
├── contracts/
│   └── decision-trace-ui.md   # Phase 1: UI/projection contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (passed)
└── tasks.md             # Phase 2 (/speckit-tasks — not created by plan)
```

### Source Code (repository root)

```text
internal/tui/
├── decisions.go         # NEW: projection (ingest, bounds, attribution),
│                        #      verdict glossary, decisions/inline renderers
├── decisions_test.go    # NEW: ingest joins, bounds, fragments, glossary sweep,
│                        #      rendering, key routing
├── tui.go               # EDIT: Model fields (projection, sub-view state),
│                        #      applyEvent hook, handleVillagersKey 'd' + esc chain
├── views.go             # EDIT: villagerDetailBody hint + decisions body dispatch,
│                        #      metatron transcript verdict rows
├── villagers_test.go    # EDIT: detail-view key grammar additions
└── tui_test.go          # EDIT (if needed): reconnect/reset coverage
```

**Structure Decision**: single-package feature inside `internal/tui`, the existing TUI
client package. New logic concentrates in `decisions.go` so the projection + glossary
are testable pure functions in the digest.go style; `tui.go`/`views.go` edits are
wiring only.

## Complexity Tracking

No constitution violations — table intentionally empty.

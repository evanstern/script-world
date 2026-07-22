# Implementation Plan: Chronicle Digest Grammar & Selection Detail

**Branch**: `018-chronicle-digest` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/018-chronicle-digest/spec.md`

## Summary

Replace the chronicle raw feed's default raw-JSON payload dump with a per-event-type digest grammar: every cataloged event renders as an aligned `TICK  HH:MM  event.name  summary` line in a hybrid voice (natural phrase for narrative families, labeled fields for telemetry), with family color roles and token emphasis. In paused inspect mode, an always-on detail pane below the list replaces the ⏎-triggered inline inspector, showing the selected event verbatim (annotated, never rewritten) with a documented extension point for future jump-off actions. A sweep test fails if any cataloged type falls through to raw JSON.

Technical approach: extend the existing pure formatting layer (`internal/tui/grammar.go`) with a digest registry (`map[string]digestFunc`) returning styled *segments* (text + role) so formatting stays ANSI-free and table-driven-testable; the view layer (`internal/tui/views.go`) renders segments through new family/emphasis style tokens and splits the inspect panel into list + detail pane. View-layer only — no change to stored events, emission, or the reducer.

## Technical Context

**Language/Version**: Go 1.26 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: Bubble Tea (TUI runtime), Lipgloss (styling) — both already in use; no new dependencies

**Storage**: N/A (view-layer; reads the existing in-memory event ring `Model.events`, cap 256, and the replica roster for names)

**Testing**: `go test ./internal/tui/` — table-driven pure-function tests (pattern: existing `grammar_test.go`), plus the new catalog sweep test

**Target Platform**: terminal (same as existing TUI client; darwin/linux)

**Project Type**: single Go project, one package touched (`internal/tui`) + two design docs + wiki re-pin

**Performance Goals**: no perceptible feed lag at max time compression (SC-005); formatting cost bounded by the 256-event ring and per-render windowing that already exists

**Constraints**: pure formatting layer stays ANSI-free (segments, not styled strings); detail pane must bound oversized payloads (`world.migrated` embeds full world state); payload bytes never rewritten in the detail view

**Scale/Scope**: ~70 event types across 12 namespaces; 1 new source file + edits to 2 existing files + 2 test files; 2 design docs reconciled

## Constitution Check

*GATE: constitution v1.1.0 — evaluated pre-Phase 0, re-checked post-Phase 1.*

- **I. Artifact-Grounded Action** — PASS. Spec, plan, research, contracts, quickstart live in `specs/018-chronicle-digest/`; board task TASK-60 exists; tier decision recorded on the task before implementation.
- **II. One Task, One PR** — PASS. TASK-60 → branch `task-60-chronicle-digest` in `.worktrees/task-60` → one PR. Spec phases are internal breakdown.
- **III. Gates Over Assertions** — PASS. spec-bridge gate governs TASK-60's status; the sweep test is the mechanical coverage gate (SC-001).
- **IV. Grounding Freshness** — PASS with follow-up. `docs/wiki/tui-client.md` and `docs/wiki/event-types.md` list touched files as sources; `/grounding-wiki:wiki-update` runs after merge as part of Done.
- **V. Model-Tiered Workflow** — PASS. Planning (this document) on Fable 5. Implementation tier: **Sonnet** — routine per the rubric: single-package view/rendering code (`internal/tui` only), tests alongside code, doc reconciliation; no concurrency/governor logic, not doctrine-adjacent. Escalation one-way to Opus 4.8 if a slice fails gates.
- **Development Workflow / spec rigor** — PASS. Full Spec Kit run; spec linked to board via spec-bridge before implementation.

**Post-Phase-1 re-check**: PASS — design adds no projects, no dependencies, no new packages; Complexity Tracking empty.

## Project Structure

### Documentation (this feature)

```text
specs/018-chronicle-digest/
├── plan.md              # This file
├── research.md          # Phase 0 output — decisions R1–R8
├── data-model.md        # Phase 1 output — entities & state changes
├── quickstart.md        # Phase 1 output — validation guide
├── contracts/
│   └── digest-grammar.md  # Phase 1 output — per-type digest contract, pane contract, keymap delta
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/tui/
├── grammar.go          # existing pure layer: classifyEvent, formatChronicleLine, formatInspector — extended (family classification, segment type, column assembly)
├── digest.go           # NEW: digest registry — per-type digestFunc table + family helpers
├── digest_test.go      # NEW: per-family digest tests + catalog sweep test (SC-001 gate)
├── grammar_test.go     # existing tests updated (line format change: tick column replaces #seq)
├── views.go            # style tokens (family/emphasis roles), chronicleRawBody (columns), chronicleInspectBody (list + detail pane split)
├── tui.go              # inspect keymap: ⏎ freed, detail-pane scroll keys; chronExpanded/chronExpIdx state replaced by pane scroll state
└── render_test.go      # golden/render assertions updated where the feed shape changed

docs/design/tui/
├── patterns/chronicle-grammar.md   # reconciled: digest line format, hybrid voice, family color roles
├── patterns/keymap.md              # reconciled: inspect-mode key changes
└── panels/chronicle.md             # reconciled: Mode 2 detail pane, mockups

docs/wiki/                          # re-pinned post-merge via /grounding-wiki:wiki-update
```

**Structure Decision**: single existing package `internal/tui`; the digest registry gets its own file (`digest.go`) because it is a ~70-entry table that would drown `grammar.go`, but it stays in the same package sharing the pure-function conventions.

## Complexity Tracking

No constitution violations — table intentionally empty.

---
id: TASK-60
title: >-
  Chronicle digest grammar: readable per-event summaries, TICK column, selection
  detail pane
status: In Progress
assignee: []
created_date: '2026-07-22 18:53'
updated_date: '2026-07-22 19:15'
labels:
  - events
  - tui
dependencies: []
ordinal: 53000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Re-grounded 2026-07-22: the chronicle raw feed already formats lines via internal/tui/grammar.go (#seq HH:MM type payload) and pause-inspect mode already exists (j/k/g/G selection, enter-to-expand annotated inspector — tui.go handleInspectKey, views.go chronicleInspectBody). But only speech/scene/clock/narration classes get readable treatment — every other event type (~65 of ~70 in docs/wiki/event-types.md) falls through to the default class and renders as a compact raw-JSON payload dump. That fallthrough is the legibility complaint: the feed is a wall of JSON at speed.

Scope: (1) per-event-type digest grammar — every event.name renders as 'TICK | HH:MM | event.name | structured human-readable summary' extracting the fields that matter for that type (family-by-family coverage of the event catalog); (2) scanability at speed — aligned columns, per-family color roles, emphasis (bold/underline/reverse) on key tokens per the chronicle-grammar color-role contract; (3) selection-driven detail — in paused inspect mode, the selected entry shows its full detail (all logged fields, pretty JSON, resolved agent names) automatically on selection/highlight instead of requiring enter, structured as a future jumping-off surface to related menus/controls.

Baseline to extend: docs/design/tui/patterns/chronicle-grammar.md and docs/design/tui/panels/chronicle.md. Related: TASK-17 (event format carries agent names — complementary, format-level; this task is view-level).

Spec: specs/018-chronicle-digest
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Every event type in the catalog (docs/wiki/event-types.md) renders a structured readable digest line TICK | timestamp | event.name | summary — raw-JSON fallthrough remains only for unknown/future types
- [ ] #2 Digest lines use aligned columns and per-family styling (color, bold/underline emphasis on key tokens) so the feed is parseable as it flies by
- [ ] #3 Paused inspect mode navigates entries item by item (up/down, first/last) — existing j/k/g/G behavior preserved or improved
- [ ] #4 The selected entry shows its full detail view (all logged fields, pretty JSON, resolved names) on selection, no extra keypress required
- [ ] #5 Detail view leaves a documented extension point for jump-off actions to other menus/controls (no actions need to ship in this task)
- [ ] #6 chronicle-grammar.md and panels/chronicle.md design docs updated to match the shipped grammar and interaction
- [ ] #7 Spec phase: Setup
- [ ] #8 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #9 Spec phase: User Story 1 — Reading the live feed without decoding JSON (P1) 🎯 MVP
- [ ] #10 Spec phase: User Story 2 — Inspecting an entry in full on pause (P2)
- [ ] #11 Spec phase: User Story 3 — Scanning the feed by eye at speed (P3)
- [ ] #12 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Tier decision (constitution V rubric): Sonnet — routine slice: single-package view/rendering code (internal/tui only), tests alongside code, doc reconciliation; no concurrency/governor logic, not doctrine-adjacent. Escalation to Opus 4.8 only if a slice fails gates. Plan of record: specs/018-chronicle-digest (plan.md, research R1-R8, contracts/digest-grammar.md, tasks.md 27 tasks). Implementation delegated to spec-implementer in .worktrees/task-60 (branch task-60-chronicle-digest), slice 1 = T001-T015 (Setup+Foundational+US1 MVP), slice 2 = T016-T026 (US2+US3+Polish).
<!-- SECTION:NOTES:END -->

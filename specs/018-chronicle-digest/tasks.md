# Tasks: Chronicle Digest Grammar & Selection Detail

**Input**: Design documents from `/specs/018-chronicle-digest/`

**Prerequisites**: plan.md, spec.md, research.md (R1–R8), data-model.md, contracts/digest-grammar.md, quickstart.md

**Tests**: included — the sweep test is itself a spec requirement (SC-001, contract §7), and the package's table-driven test convention (grammar_test.go) is load-bearing.

**Organization**: grouped by user story; US1 (digest lines) is the MVP increment. All source work happens on branch `task-60-chronicle-digest` in `.worktrees/task-60` (one task, one PR).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: branch/worktree per constitution Principle II.

- [x] T001 Create the task worktree: `git fetch origin && git worktree add .worktrees/task-60 -b task-60-chronicle-digest origin/main`; all subsequent tasks execute inside `.worktrees/task-60`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the segment/family/registry substrate every story builds on (research R1, R2, R4; data-model).

- [X] T002 Add `seg`/`segRole` types (`segText|segName|segSpeech|segEmphasis|segLabel`) and a `plainSegs([]seg) string` helper to internal/tui/grammar.go, ANSI-free per R4
- [X] T003 Add `eventFamily` enum and namespace-prefix derivation with the `meeting`+`norm`→governance merge (R2) to internal/tui/grammar.go
- [X] T004 Create internal/tui/digest.go with `digestFunc`, `digestRegistry map[string]digestFunc`, and rework `formatChronicleLine` in internal/tui/grammar.go to chronicleLine v2 (`Tick`, `Time`, `Type`, `Family`, `Summary []seg`; `Seq` kept for the detail pane) — registry hit → digest segs, miss or `!ok` → compact `resolvePayloadNames` fallback as one `segText` (FR-002); fold the existing speech/scene privileges into registry entries so `classifyEvent`'s roles migrate rather than duplicate
- [X] T005 Update existing tests in internal/tui/grammar_test.go to the v2 line shape (tick column replaces `#seq` in `plainChronicleLine`; speech/scene expectations move to registry-driven output; wrap/truncate tests unchanged)

**Checkpoint**: `go test ./internal/tui/` green with only fallback digests — feed behavior equivalent to today plus tick column.

---

## Phase 3: User Story 1 — Reading the live feed without decoding JSON (P1) 🎯 MVP

**Goal**: every cataloged event type renders a readable per-type digest; aligned `TICK HH:MM type summary` at solo width, tick-less short-type at dock width.

**Independent Test**: run/replay a world and watch the raw feed — no cataloged type shows a JSON dump; the sweep test enforces it mechanically.

- [X] T006 [US1] Implement world/clock/daemon/sim digests per contract §3 tables 1–2 in internal/tui/digest.go (labeled voice for clock/daemon; `world.migrated` elides the embedded state)
- [X] T007 [US1] Implement agent acts & needs digests per contract §3 table 3 in internal/tui/digest.go (verify payload fields against internal/sim structs; `agent.needs_changed` phrase-prefixed labeled; `agent.died` alert-flagged)
- [X] T008 [US1] Implement agent mind & plan digests per contract §3 table 4 in internal/tui/digest.go (consolidation-family fields verified against internal/sim/consolidate.go, plans against internal/sim/plan.go)
- [X] T009 [US1] Implement social digests per contract §3 table 5 in internal/tui/digest.go (speech privilege for conversation_turn/rumor_told preserved as segName+segSpeech; `social.secret_seeded` fields verified against internal/sim/social.go; `social.chest_taken` alert-flagged)
- [X] T010 [US1] Implement governance digests per contract §3 table 6 in internal/tui/digest.go (fields verified against internal/sim/governance.go and specs/006-norms-and-votes/contracts/governance-events.md; `norm.violated` alert-flagged)
- [X] T011 [US1] Implement gru/chronicle/metatron/cog digests per contract §3 tables 7–8 in internal/tui/digest.go (cog labeled voice per specs/007-cognition-horizon/contracts/events.md field order; `gru.attacked` alert-flagged; `metatron.nudged` resolves target indices to names)
- [X] T012 [US1] Implement column assembly per R5 in internal/tui/grammar.go + internal/tui/views.go: tick right-aligned to widest visible, time width 5, type padded to widest visible cap 26 (solo) / last-segment short name cap 10 with no tick (dock); rework `chronicleRawBody` to window-first-then-format (R8)
- [X] T013 [US1] Write per-family digest unit tests in internal/tui/digest_test.go — one sample payload per type asserting plain text and role spans (contract §3 templates are the expected values)
- [X] T014 [US1] Write the catalog sweep test in internal/tui/digest_test.go per contract §7: fixture of all ~70 types × sample payloads; asserts registry coverage both directions plus backticked-type cross-check against docs/wiki/event-types.md (SC-001)
- [X] T015 [US1] Update internal/tui/render_test.go and internal/tui/tui_test.go expectations that assert on old feed strings

**Checkpoint**: US1 shippable — feed readable end-to-end, sweep green, inspect mode still the old inline inspector.

---

## Phase 4: User Story 2 — Inspecting an entry in full on pause (P2)

**Goal**: always-on detail pane in inspect mode; navigation unchanged; ⏎ freed as the documented extension point.

**Independent Test**: pause, move selection with j/k/g/G — full verbatim detail visible at every step with zero extra keypresses; J/K scroll long payloads; ⏎ does nothing.

- [X] T016 [US2] Model state changes in internal/tui/tui.go: remove `chronExpanded`/`chronExpIdx` and `chronToggleExpand`; add `chronDetailScroll int` reset on selection move, pause exit, and reconnect (data-model "Interaction state")
- [X] T017 [US2] Keymap changes in internal/tui/tui.go `handleInspectKey`: ⏎ → reserved no-op (comment cites contract §5 extension point); `J`/`K` → detail pane scroll; title-row hint string updated to `j/k select · J/K scroll detail`
- [X] T018 [US2] Rework `chronicleInspectBody` in internal/tui/views.go per contract §5: bottom detail pane `paneRows = min(rows/2, 14)` with rule line `DETAIL · seq N`, list keeps ≥5 rows, pane renders `formatInspector` windowed by `chronDetailScroll` with `… (+N more — J to scroll)` footer, oversized payloads styled only for the visible slice (FR-011); add the `detailActions(e store.Event) []detailAction` stub returning nil with the `[future: actions]` slot (FR-009)
- [X] T019 [US2] Inspect-mode tests in internal/tui/tui_test.go + internal/tui/render_test.go: pane present immediately on pause, follows selection, scroll clamps to content, selection move resets scroll, `world.migrated`-sized payload stays within row budget, ⏎ is a no-op

**Checkpoint**: US2 shippable independently on top of Phase 2 (works with fallback digests too).

---

## Phase 5: User Story 3 — Scanning the feed by eye at speed (P3)

**Goal**: family color roles, token emphasis, alert lines.

**Independent Test**: mixed ≥50-event window — columns aligned, families distinguishable by treatment alone, deaths/attacks/thefts/violations pop without reading.

- [X] T020 [US3] Add family/emphasis/alert style tokens to the token block in internal/tui/views.go per data-model "Style tokens" (roles not raw colors; `styleFeedClock` becomes the clock family tint; palette recorded for T023)
- [X] T021 [US3] Rework `renderChronicleRow` in internal/tui/views.go to style segment-wise after wrap (R4): family tint on type column, segName/segSpeech/segEmphasis/segLabel roles, whole-line alert style for the four alert types, selection reverse preserved
- [X] T022 [US3] Style-role tests in internal/tui/render_test.go: role→token mapping per family, alert lines styled whole, pure layer asserted ANSI-free (no `\x1b` in any digest output)

**Checkpoint**: all three stories complete.

---

## Phase 6: Polish & Cross-Cutting

- [X] T023 Reconcile docs/design/tui/patterns/chronicle-grammar.md: line format (tick column, seq to detail), hybrid voice by family, per-type table pointer to contract, color roles incl. family/emphasis/alert (FR-012)
- [X] T024 [P] Reconcile docs/design/tui/panels/chronicle.md: Mode 2 mockup with detail pane, always-on detail semantics, extension point, updated key hints (FR-012)
- [X] T025 [P] Reconcile docs/design/tui/patterns/keymap.md inspect-mode rows: ⏎ reserved, J/K scroll (contract §6)
- [x] T026 Run quickstart.md end-to-end in the worktree: `go build ./... && go vet ./... && go test ./...`, live-world feed/inspect walkthrough, gate-proves-itself check (delete one registry entry → sweep fails → restore) — all done: mechanical gates green (implementer + orchestrator re-runs); live walkthrough executed 2026-07-22 by the orchestrator via an expect-driven PTY against a throwaway world (digest rows with tick/time/type/summary rendering live; pause → inspect with ▌ selection, `DETAIL · seq N` pane showing type/payload, footer `j/k select · J/K scroll detail`; resume + quit clean). Family colors and full-width column alignment carry unit-test coverage but were not eyeballed in a real terminal (ANSI stripped in capture) — worth a human glance at PR review
- [x] T027 After the TASK-60 PR merges: run `/grounding-wiki:wiki-update` to re-pin wiki notes sourcing internal/tui (tui-client.md, event-types.md, chronicle.md — Principle IV), then remove the worktree

## Dependencies

- Phase 2 blocks everything; T004 blocks T005 and all of US1/US2/US3.
- US1: T006–T011 depend on T004 (same file, sequential); T012 depends on T004; T013–T014 depend on T006–T011; T015 depends on T012.
- US2: T016→T017→T018→T019; independent of US1 (pane renders fallback digests fine).
- US3: T020→T021→T022; T021 touches `renderChronicleRow` which T012 also edits — run US3 after US1.
- Polish: T023–T025 after their behavior lands; T026 last before PR; T027 post-merge.

**Story order**: Phase 2 → US1 (MVP) → US2 → US3 → Polish. US2 could swap before US1 if needed — no hard dependency either way.

## Parallel Opportunities

- Same-file registry work (T006–T011) is sequential by design; the genuinely parallel seams are T013 (tests) starting once its family's digests exist, and the three doc reconciliations T023/T024/T025.
- Within Polish: T024 and T025 are [P] against T023.

## Implementation Strategy

MVP = Phase 2 + Phase 3 (US1): the feed becomes readable and the sweep gate exists. US2 (detail pane) and US3 (styling) are independent increments on top; all land as commits on the single `task-60-chronicle-digest` branch and merge in TASK-60's one PR.

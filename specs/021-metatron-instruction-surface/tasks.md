# Tasks: Metatron Instruction Surface — Staged Charter + Skill Files + Gated Tool Roster

**Input**: Design documents from `/specs/021-metatron-instruction-surface/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED — the spec's success criteria are test-shaped (SC-002 adversarial
battery, SC-003 byte-compat, SC-004 drift-forcing, FR-012 determinism); Go convention:
tests alongside code.

**Organization**: single implementer, one branch (`task-64-metatron-instruction-surface`
in `.worktrees/task-64`), one PR. [P] marks parallel-safe tasks (different files).

## Phase 1: Setup

- [X] T001 Verify green baseline in the task worktree: `go build ./... && go test ./...`
      from `.worktrees/task-64` (fresh off origin/main); record the baseline commit in
      the PR description

---

## Phase 2: Foundational (blocking prerequisites in `internal/tool` + `internal/sim`)

**⚠️ CRITICAL**: US1 and US2 both consume these; complete before story phases.

- [X] T002 Add the authoritative miracle cost table to internal/tool/registry.go beside
      `miracleKinds`: per-kind `miracleCosts` map, kind↔event-type mapping, exported
      `MiracleCost(kind string) (int, bool)` and `MiracleCostsByEvent() map[string]int`
      (fresh map per call; deterministic — keyed lookups only, per data-model.md §5)
- [X] T003 Unit tests for the cost table in internal/tool/registry_test.go: kinds ≡
      `MiracleKinds()`, time_snap=2 / others=1, `MiracleCostsByEvent` covers exactly the
      four `metatron.*` miracle event types
- [X] T004 Derive `sim.miracleCost` from `tool.MiracleCostsByEvent()` in
      internal/sim/miracles.go (replace the literal map; `spendMiracleCharge` unchanged);
      update the existing kinds-mirror test (internal/metatron/metatron_test.go
      TestMiracleKindsMirrorTool and any sim-side pin) from "two copies equal" to
      "derivation holds" (SC-004)
- [X] T005 [P] Add `RestrictEnum(t Tool, param string, allowed []string) Tool` to
      internal/tool/derive.go — copy-on-write (registry never mutated), preserves
      allowed-value order from the tool's own Enum order, unknown allowed names dropped;
      unit tests in internal/tool/derive_test.go incl. InputSchema of a restricted
      work_miracle declaring only the granted kinds
- [X] T006 Add `MetatronToolGuidance(roster []Tool) string` to internal/tool/derive.go:
      renders per granted tool its name, argument surface (from Params — same source as
      InputSchema), and charge cost (nudges from Cost.Charges; work_miracle per-kind from
      `MiracleCost`, honoring a restricted kind enum); deterministic output; roster order
      preserved (research R6)
- [X] T007 Drift/derivation tests for guidance in internal/tool/derive_test.go: every
      roster tool name appears; every rendered cost equals the authoritative table; no
      non-roster tool or ungranted kind appears; byte-identical across two calls (SC-004,
      FR-008, INV-3)

**Checkpoint**: `go test ./internal/tool/ ./internal/sim/` green; one cost edit in T002's
table provably propagates (quickstart §4).

---

## Phase 3: User Story 1 — Player-authored skill files (Priority: P1) 🎯 MVP

**Goal**: `skills/*.md` compose into the turn prompt beneath the charter, per-read
hot-reloaded, with charter-style caps/notices; fixed frame provably last.

**Independent Test**: quickstart §2 — add/edit/delete skill files between turns; the next
reply tracks disk; adversarial fixtures cannot displace the fixed frame.

- [X] T008 [US1] Implement `loadSkills(worldDir)` in internal/metatron/charter.go:
      eligibility (regular `.md` direct children of `skills/`, no recursion, dotfiles and
      other extensions silently excluded), ascending bytewise filename sort, caps (4,000
      chars/file via persona.CharterMaxChars, max 8 files), per-issue notices (truncate /
      skip-beyond-8 / unreadable-skip) matching loadCharter's notice voice
      (contracts/instruction-surface.md rules 3–5)
- [X] T009 [P] [US1] Unit tests for loadSkills in internal/metatron/metatron_test.go:
      ordering, eligibility filtering, at-cap vs over-cap, 9-files skip, unreadable file,
      empty/missing skills dir (no notice), notice wording
- [X] T010 [US1] Restructure `turnSystemPrompt` in internal/metatron/turn.go per
      data-model.md §2: signature takes charter + skills + granted roster; assembly =
      charter → `--- skill: <name> ---` blocks in order → fixed frame appended LAST as a
      compile-time constant on every path; fixed frame = the two non-negotiables verbatim
      + doctrine prose + `tool.MetatronToolGuidance(roster)` replacing the hand-written
      tool list (turn.go:396-425); `Turn()` calls loadSkills per-read and merges skill
      notices into the reply-notice channel alongside charter notices (this phase passes
      the full `tool.LoopRosterMetatron()` as roster; gating lands in US2)
- [X] T011 [US1] Adversarial fixture battery test in internal/metatron/metatron_test.go:
      the 7-row table from contracts/instruction-surface.md — for each fixture assert the
      assembled prompt ends with the fixed frame verbatim and per-file truncation happened
      pre-assembly (SC-002, INV-1)
- [X] T012 [P] [US1] Prompt determinism test in internal/metatron/metatron_test.go: two
      identical world dirs (incl. multiple skills) ⇒ byte-identical composed prompt;
      repeated composition ⇒ byte-identical (FR-012, INV-2)

**Checkpoint**: US1 fully functional — skills hot-reload live in a running world
(quickstart §2); all existing metatron tests still green (prose-list replacement is the
one intended prompt change).

---

## Phase 4: User Story 2 — World-scoped capability grants (Priority: P2)

**Goal**: `capabilities.json` gates the declared roster, the derived guidance, and the
door; no-manifest worlds byte-compatible with today.

**Independent Test**: quickstart §3 — subset manifests change what is declared/landable
next turn; missing/malformed manifests fall back per contract.

- [X] T013 [US2] Implement `loadManifest(worldDir)` in internal/metatron/charter.go
      returning the effective grant set + notice: parse `capabilities.json` per
      contracts/capability-manifest.md — missing → full grant no notice; malformed →
      full grant + notice; unknown tool/kind names → ignored + notice; `tools: []` →
      conversation-only; `miracle_kinds` omitted → all kinds (data-model.md §3)
- [X] T014 [P] [US2] Unit tests for loadManifest in internal/metatron/metatron_test.go
      covering every row of the contract's semantics table
- [X] T015 [US2] Build the granted roster per-read in `Turn()`
      (internal/metatron/turn.go): filter `tool.LoopRosterMetatron()` by grants; apply
      `tool.RestrictEnum(work_miracle, "kind", kinds)` when restricted; pass granted
      roster to `toolloop.Job.Roster` AND to `MetatronToolGuidance` (declaration + prose
      layers, research R5.1–R5.2)
- [X] T016 [US2] Door-layer enforcement: build `turnHandlers` (internal/metatron/
      toolcalls.go) from the granted set only, and add grant checks to
      `landNudge`/`landMiracle` (internal/metatron/turn.go) — ungranted form/kind refused
      with an in-fiction reason exactly like existing refusals (research R5.3); manifest
      notice joins the reply-notice channel
- [X] T017 [US2] Gating tests in internal/metatron/metatron_test.go: dream-only world ⇒
      declared schemas contain only nudge_dream, guidance mentions no omen/miracle, omen
      call refused at door; kinds-restricted world ⇒ kind enum + guidance restricted,
      time_snap refused; empty-tools world ⇒ no tools declared, converse still lands;
      revoked-mid-world ⇒ next turn ungranted (per-read); charges untouched by grants
- [X] T018 [P] [US2] No-manifest byte-compat test: absent `capabilities.json` ⇒ declared
      roster ≡ `tool.LoopRosterMetatron()` and composed prompt identical to the
      full-grant prompt; full `go test ./...` stays green (SC-003)
- [X] T019 [P] [US2] Stage-preset fixtures test (SC-006): two TASK-68-shaped manifests
      (stage-1 `{"tools":["nudge_dream"]}`; stage-3 full) load into the expected grant
      sets — presets are pure data, in internal/metatron/metatron_test.go

**Checkpoint**: US1 + US2 independently verifiable; quickstart §3 passes end-to-end.

---

## Phase 5: User Story 3 — Provenance + grants in the TUI (Priority: P3)

**Goal**: `metatron.Status` reports skills, granted tools, manifest provenance; TUI
console header renders it.

**Independent Test**: quickstart §5 — status JSON and header track disk changes on next
read.

- [X] T020 [US3] Extend `Status` in internal/metatron/turn.go per contracts/status.md:
      `Skills []string` (effective files, composition order), `GrantedTools []string`
      (registry order; `work_miracle(move,give_item)` form when restricted),
      `ManifestDefault bool`; computed fresh per call via loadSkills/loadManifest; unit
      tests in internal/metatron/metatron_test.go (incl. restricted-kinds rendering)
- [X] T021 [US3] Render provenance in the TUI console header in internal/tui/tui.go:
      extend the `consoleStatusMsg` handling (tui.go:335-343) and header line to
      `custom charter · 2 skills · tools: dream, omen` form per contracts/status.md —
      quiet default (tools part omitted when `manifest_default`), `tools: none` for
      conversation-only; TOUCH ONLY the consoleStatusMsg/header region (TASK-63 owns
      digest/villager-detail/transcript regions)

**Checkpoint**: all three stories functional; quickstart §§2–5 pass.

---

## Phase 6: Polish & Cross-Cutting

- [X] T022 Run the full quickstart validation (quickstart.md §§1–6) in the worktree:
      `go build ./... && go test ./...`, live smoke of hot-reload + gating + status in a
      scratch world; fix anything found
- [X] T023 Reconcile doc comments touched by the change (charter.go header comment now
      covers skills+manifest; roster.go/registry.go comments where cost table moved);
      note for post-merge: wiki re-pin via /grounding-wiki:wiki-update (sources:
      metatron.md, metatron-miracles.md, tool-registry.md, tool-loop.md, tui-client.md,
      ipc-protocol.md) — the re-pin itself runs on main after merge, per PDLC

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2 → story phases**: T002–T007 block both US1 (T010 needs T006) and
  US2 (T015 needs T005+T006).
- **US1 (P3 phase) before US2 (P4 phase)**: both rewrite turn.go's prompt path — the
  single implementer lands composition first, gating second (no cross-story file
  parallelism; [P] applies within phases only).
- **US3 last**: consumes loadSkills (T008) + loadManifest (T013).
- Within phases: [P]-marked test tasks touch different files/table-rows than their
  implementation pair and can interleave freely.

## Implementation Strategy

MVP = Phases 1–3 (US1): a playable skills surface with the fixed frame proven. US2 makes
it the TASK-68 substrate; US3 makes it legible. Land as ONE PR per constitution II —
phases are commits on `task-64-metatron-instruction-surface`, checkpoint-by-checkpoint,
not separate PRs. Commit after each checkpoint at minimum.

## Notes

- turn.go is the contention file (T010, T015, T016 all touch it) — sequential by design.
- The prompt text WILL change vs today (derived guidance replaces prose) — existing tests
  asserting the old prose block must be updated deliberately, never loosened to regex
  slop; the new pinned surface is the derived guidance (T007).
- Never edit files under backlog/ by hand; board updates go through the orchestrator.

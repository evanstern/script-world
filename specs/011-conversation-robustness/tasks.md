# Tasks: Conversation Robustness

**Input**: Design documents from `/specs/011-conversation-robustness/`
**Prerequisites**: plan.md, research.md, data-model.md, contracts/telemetry.md, quickstart.md
**Board**: TASK-42 (one PR from `.worktrees/task-42`, branch `task-42-conversation-robustness`)
**Tests**: included — the spec's success criteria (SC-001..004) are test-shaped (fault injection + golden path), so tests ride alongside implementation per story.

## Phase 1: Setup

- [X] T001 Create worktree `.worktrees/task-42` on branch `task-42-conversation-robustness` from fresh `origin/main`; verify `go test ./internal/mind/` is green pre-change (baseline)

## Phase 2: Foundational (blocking prerequisites for all stories)

- [X] T002 Add `sim.OutcomeRetried = "retried"` alongside the existing outcome constants in internal/sim/cognition.go, with a comment marking it non-terminal (contract §Compatibility rule 1)
- [X] T003 Extend the `cog.outcome` payload constructor in internal/mind/telemetry.go:109 (`cogOutcomeEvent` or a sibling variant) with optional `raw` (string, omitempty; 2048-byte rune-boundary truncation + `…[truncated]` marker) and `retried` (bool, omitempty) fields per data-model.md; verify existing telemetry consumers in internal/mind/telemetry.go treat unknown outcome values as pass-through (contract §Compatibility)
- [X] T004 [P] Add truncation unit tests (oversized raw reply → rune-boundary cut with marker; UTF-8 validity preserved) in internal/mind/telemetry_test.go

## Phase 3: User Story 1 — A completed scene survives one bad summary reply (P1)

**Goal**: outcome-site retry — one malformed summary reply no longer discards a completed scene.
**Independent test**: fault-inject a malformed first summary reply via the fake `Submitter`; scene lands whole with `retried:true`; two consecutive malformed replies abandon exactly as today.

- [X] T005 [US1] Implement outcome retry in `runConversation` in internal/mind/convo.go:204-210: on `parseOutcome` parse/validation failure (NOT on `Submit` transport error), emit non-terminal `cog.outcome{outcome:"retried", raw, reason:"outcome: …"}` and re-submit the identical outcome request once; second parse failure → abandon with terminal `unusable` carrying the retry's `raw`; transport errors abandon immediately with no retry (research.md R2)
- [X] T006 [US1] Thread the `retried` flag into the scene's terminal `cog.outcome` (landed at convo.go:286, stale at :216, unusable at :207) so retry consumption is measurable (FR-005); stale-at-landing check MUST remain after the retried outcome (edge case: retry cannot bypass staleness)
- [X] T007 [US1] Add fault-injection tests in internal/mind/convo_test.go: (a) malformed-then-valid summary → scene lands whole, terminal `landed{retried:true}`, one `retried` marker with verbatim `raw`; (b) malformed-twice → abandons, no partial state (extend `TestConversationFailureInjectsNothing` pattern), terminal `unusable` carries `raw`; (c) transport error on summary → immediate abandon, Submit call count == expected (no retry)

**Checkpoint**: US1 independently shippable — the dominant live loss (4 observed scenes) is recovered.

## Phase 4: User Story 2 — A scene survives one bad utterance (P2)

**Goal**: utterance-site tolerance — one bad say no longer kills the dialogue.
**Independent test**: inject one bad utterance mid-scene → scene completes and lands; two consecutive bad utterances → abandons as today.

- [X] T008 [US2] Implement single same-speaker utterance retry in `runConversation` in internal/mind/convo.go:191-199: on `parseSay`/empty-say failure, emit non-terminal `cog.outcome{retried, raw, reason:"utterance turn <t>: …"}` and retry that turn once (retry-not-skip, research.md R1 — round-robin transcript invariant); retry failure → abandon as today with `raw`; transport errors → immediate abandon, no retry; at most one utterance retry per scene feeds `retried:true` on the terminal event
- [X] T009 [US2] Add fault-injection tests in internal/mind/convo_test.go: (a) one bad say then valid → scene completes, transcript alternation intact (assert speaker order), lands with `retried:true`; (b) two consecutive bad says → abandons, nothing injected; (c) bad say on the final turn then valid retry → outcome step receives a well-formed transcript

**Checkpoint**: both loss sites tolerant; scenes only die on double failure or backpressure.

## Phase 5: User Story 3 — Parse failures are inspectable (P3)

**Goal**: every parse failure's verbatim reply recoverable from world.db in one query.
**Independent test**: force parse failures at both sites; `json_extract(payload,'$.raw')` returns the exact reply text.

- [X] T010 [US3] Verify/complete raw propagation at every parse-failure emission in internal/mind/convo.go (both sites, first attempt AND retry-failure paths) — the plumbing lands in T005/T008; this task closes gaps and asserts none of the transport-error or staleness paths carry `raw` (contract §Terminal outcomes)
- [X] T011 [P] [US3] Add an integration-style test in internal/mind/convo_test.go running a scene through the store round-trip: emitted events marshal → unmarshal → `raw` recoverable verbatim and attributable to the conversation job id (SC-003)

## Phase 6: User Story 4 — Fewer malformed summaries (P4)

**Goal**: cut the malformed-summary base rate with zero extra model calls.
**Independent test**: replay the observed malformed shapes against the lenient parser; hardened prompt shipped.

- [X] T012 [P] [US4] Implement lenient unquoted-string recovery in `parseOutcome` in internal/mind/parse.go:98-130: on `json.Unmarshal` failure, quote bare `gist`/`retold` values in the extracted span and re-unmarshal once; original error returned if recovery fails (research.md R3); lenient success counts as parse success (no retry consumed)
- [X] T013 [P] [US4] Harden the outcome instruction in internal/mind/convo.go:358-365: `gist`/`retold` MUST be double-quoted JSON strings; `retold` empty-string alternative replaces bare-null instruction (research.md R5)
- [X] T014 [P] [US4] Add table tests in internal/mind/parse_test.go covering the four observed malformed shapes (unquoted gist starting with `F`/`H`/`S` initials, unquoted retold) plus non-recoverable shapes that must still fail (prose-only, unterminated object)

## Phase 7: User Story 5 — MLX reasoning_effort probe (P5)

**Goal**: recorded answer on whether the endpoint honors `reasoning_effort:none` under `max_tokens=128`.
**Independent test**: probe artifacts + board note exist.

- [X] T015 [P] [US5] Write probe script specs/011-conversation-robustness/probe-mlx-reasoning.sh: utterance-shaped request to http://localhost:11434/v1 at max_tokens=128 with reasoning_effort none/unset/low, N=10 each, reporting reply length + empty-rate per config (research.md R6)
- [X] T016 [US5] Run the probe against the live endpoint and record findings on board TASK-42 via `backlog task edit TASK-42 --append-notes` (AC #5); reference the note from the PR description

## Phase 8: Polish & Cross-Cutting

- [X] T017 Golden happy-path test in internal/mind/convo_test.go: all-valid scripted scene → emitted batch identical to pre-change shape — no `retried`, no `raw`, no extra Submit calls (SC-004 / FR-009)
- [X] T018 Run quickstart.md §1-2 (`go vet ./... && go test ./...`) green; optional §3 live smoke on a throwaway world
- [ ] T019 Update wiki sources post-merge: `/grounding-wiki:wiki-update` re-pins agent-mind, social-fabric, event-types (+ any note listing convo.go/parse.go/telemetry.go/cognition.go as sources) — Constitution IV
- [ ] T020 `spec-bridge:sync` after each phase completes and at Done; tick board ACs as their tests land

## Dependencies

- Phase 1 → Phase 2 → everything else.
- US1 (P3..T007) and US2 (T008-T009) both depend on T002/T003; US2 is independent of US1 but touches the same function — implement sequentially (T005-T007 then T008-T009) in the worktree to avoid conflict churn.
- US3 (T010-T011) depends on US1+US2 plumbing existing.
- US4 (T012-T014) independent of US1-US3 (different functions); parallelizable any time after Phase 2.
- US5 (T015-T016) fully independent; parallelizable from the start.
- Polish tasks last; T019/T020 post-merge/at-phase-boundaries.

## Parallel Execution Examples

- After Phase 2: {T005-T007 (US1)} ∥ {T012-T014 (US4)} ∥ {T015 (US5)} — different files/functions.
- T004, T011, T014, T015 are [P]-safe at their phase entry.
- Single-implementer reality (one Opus agent, one worktree): execute phases in order, using [P] markers as commit-batching guidance rather than true concurrency.

## Implementation Strategy

**MVP = Phase 1 + 2 + US1 (T001-T007)**: recovers the observed live loss (the outcome
site) and ships the telemetry/evidence trail. Each subsequent story is an independent,
testable increment; US4/US5 can trail into the same PR without blocking US1-US3
review. One PR total (TASK-42) — phases are commits, not PRs.

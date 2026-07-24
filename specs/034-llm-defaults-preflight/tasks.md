# Tasks: Working fresh-world LLM defaults + loud dead-tier surfacing

**Input**: Design documents from `/specs/034-llm-defaults-preflight/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: included — the spec's SC-003 (no false positives) and the boot-never-fails
invariant are exactly the kind of behavior that regresses silently; project convention
is tests alongside code.

**Organization**: grouped by user story; US1/US2 share the Foundational condition
plumbing. Implementation tier per constitution Principle V is noted per phase.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [x] T001 Create worktree `.worktrees/task-84` with branch `task-84-llm-defaults-preflight` from fresh `origin/main` (root stays on main)

---

## Phase 2: Foundational (Blocking Prerequisites) — tier: Opus 4.8 (orchestrator internals)

**Purpose**: the ProviderCondition slot, its transitions, and the wire export — both
US1 and US2 raise/clear through this. See data-model.md + contracts/provider-conditions.md.

- [ ] T002 Add ProviderCondition (kind/detail/remedy/since, precedence unreachable > missing > tool-silent) to the `provider` struct beside `tierHealth`, with mutex-guarded raise/reclassify/clear methods and a condition-transition hook field on the Orchestrator (SetConditionHook, pattern of SetRecalibrateHook llm.go:633-636) in internal/llm/llm.go
- [ ] T003 Clear-on-success: in the worker success path (llm.go:840 vicinity) clear the provider's active condition (traffic is truth, FR-004); export Condition/ConditionDetail/ConditionRemedy (omitempty) on ProviderStatus and fill them in StatusSnapshot() in internal/llm/llm.go
- [ ] T004 Wire the daemon side: on hook fire, print `daemon: WARNING llm provider …` to the boot/daemon log and append+broadcast a `daemon.llm_warning` event (payload per contracts/provider-conditions.md) via the appendDaemonEvent pattern in internal/daemon/daemon.go, with event-kind whitelist + no-op reducer arm in internal/sim/loop.go and internal/sim/state.go
- [ ] T005 Foundational tests: condition raise/reclassify/clear precedence, clear-on-success, StatusSnapshot field export, hook firing on transitions only, in internal/llm/llm_test.go

**Checkpoint**: conditions exist, transition loudly, and ride the status wire — nothing raises them yet

---

## Phase 3: User Story 1 — A dead local tier is loud, not silent (Priority: P1) 🎯 MVP

**Goal**: boot + periodic preflight classifies model-missing / endpoint-unreachable and
the operator sees it in daemon log, `promptworld status`, and the TUI without restart-to-clear.

**Independent Test**: quickstart V1/V2 — world with absent model boots, warns on all
three surfaces within 30s; pulling the model clears within 60s without restart.

**Tier**: T006–T008 Opus 4.8 (llm/daemon internals); T009–T010 Sonnet (rendering).

- [ ] T006 Implement the preflight probe in internal/llm/preflight.go (net-new): GET `{endpoint}/models` (auth header per provider key rules, ≤5s timeout), classify per contracts/provider-conditions.md (unreachable / missing / listing-unsupported→skip / healthy), openai_compat only
- [ ] T007 Preflight lifecycle in internal/llm/preflight.go + internal/llm/llm.go: run once for all providers at start (async), then re-probe every 60s only while a preflight condition is active, re-logging via the hook each active re-probe; clear/reclassify per data-model.md state machine
- [ ] T008 Boot wiring in internal/daemon/daemon.go + internal/sim/loop.go: add the narrow `Loop.InjectOperator` command door (InjectSocial pattern, whitelisted to daemon.llm_warning — research R8) and route the condition hook's durable event through it (log-line-only fallback when the loop isn't running); start the preflight goroutine (shutdown ctx, pattern `go sampler.run(ctx)` daemon.go:148) after llm.New inside the LLM-gated block; boot must never block or fail on probe results (FR-002)
- [ ] T009 [P] Render active conditions in `promptworld status` human output (WARNING line per affected provider after the clock line; healthy output byte-identical) in cmd/promptworld/commands.go (cmdStatus, clockLine vicinity 490-537)
- [ ] T010 [P] TUI surfaces: red `[llm: <provider> <kind>]` header badge while any condition is active (pattern: `[degraded]` views.go:121-122) and condition+remedy annotation on the metatron-pane provider lines (llmProviderLines views.go:1468-1497) in internal/tui/views.go
- [ ] T011 US1 tests: httptest fake endpoints for missing-model / unreachable / listing-unsupported / healthy; boot-never-fails; re-probe clears and reclassifies; status render golden for warning block, in internal/llm/preflight_test.go and cmd/promptworld tests

**Checkpoint**: US1 fully demoable — quickstart V1/V2 pass

---

## Phase 4: User Story 2 — Consistently tool-silent planner calls are loud (Priority: P2)

**Goal**: a provider whose tool-carrying calls keep returning zero tool calls raises a
`tool-silent` condition (threshold 8 consecutive) with a mode-appropriate remedy.

**Independent Test**: quickstart V3 — native-mode cogito flags within minutes; healthy
provider with occasional tool-free completions never flags.

**Tier**: Opus 4.8 (worker hot path).

- [ ] T012 [US2] Per-provider consecutiveToolFree counter in the worker (llm.go:828-866): increment on completed call with `len(req.Tools) > 0` and zero tool calls, reset on any returned tool call, ignore transport failures and non-tool calls; at ≥8 raise `tool-silent` (never overwrite an active preflight condition), remedy text by resolved tool mode per contracts/provider-conditions.md, in internal/llm/llm.go
- [ ] T013 [US2] Detector tests: raise at exactly 8, reset on tool call, non-tool kinds never count, transport failures don't count, preflight condition precedence, clear on first landed tool call, in internal/llm/llm_test.go

**Checkpoint**: US1 + US2 both independently demoable

---

## Phase 5: User Story 3 — Fresh-world defaults work out of the box (Priority: P2)

**Goal**: fresh worlds get the live-proven local config; `promptworld new` says what to
pull; docs and README tell the same story. See contracts/fresh-world-defaults.md.

**Tier**: Sonnet (config default + CLI output + doc reconciliation).

- [ ] T014 [P] [US3] Change DefaultConfig() local provider to `{model: "cogito:3b", tool_mode: "json", parallel: 4}` (comment updated; cloud/routes untouched) in internal/llm/config.go, with golden assertions updated in internal/llm/config_test.go
- [ ] T015 [P] [US3] Append the expected-model + `ollama pull cogito:3b` guidance line to cmdNew stdout (after commands.go:195-196) in cmd/promptworld/commands.go
- [ ] T016 [P] [US3] Align docs: docs/llm-providers.md presents cogito:3b+json+parallel-4 as the fresh-world default and gemma-class as the upgrade path; README.md (~line 86) names cogito:3b with the pull command (FR-009, SC-004)

**Checkpoint**: quickstart V4/V5 pass

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T017 Run quickstart V0 (full `go test ./...`), gofmt (touched files only — pre-existing drift is TASK-83) and `go vet` on touched packages; fix fallout
- [ ] T018 Execute quickstart V1–V5 live where the environment allows; record outcomes in specs/034-llm-defaults-preflight/quickstart-results.md
- [ ] T019 Post-merge re-grounding (root, after the PR lands): `/grounding-wiki:wiki-update` for notes sourcing internal/llm/*, internal/daemon/daemon.go, docs/llm-providers.md, README.md; then regenerate player docs via the player-docs skill (constitution Principle IV)

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → US1 (Phase 3) → US2 (Phase 4); US3 (Phase 5) depends only on
  Phase 1 and can run in parallel with Phases 2–4 (different files except a disjoint
  region of config.go vs llm.go).
- Within US1: T006/T007 → T008; T009/T010 [P] once T003's wire fields exist; T011 last.
- US2 (T012) needs Phase 2 only — independent of US1's preflight files, but sequenced
  after US1 in single-implementer flow because both edit internal/llm/llm.go.
- Polish after all stories; T019 strictly post-merge.

## Parallel Opportunities

- T009 + T010 (different files) once Phase 2 lands.
- T014 + T015 + T016 all [P] (config.go / commands.go / docs).
- US3 as a whole can proceed while Phases 2–4 are in flight.

## Implementation Strategy

MVP = Phases 1–3 (US1): the reported failure becomes loud. US2 adds the
success-shaped-failure net; US3 makes fresh worlds not need the net. Single PR
(TASK-84) regardless — phases are internal breakdown, commits land per task/logical
group on the one branch.

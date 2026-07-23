# Tasks: Multi-Provider Routing — Registry and Ordered Chains

**Input**: Design documents from `/specs/024-provider-routing/`

**Prerequisites**: plan.md, spec.md, research.md (R1–R8 binding), data-model.md,
contracts/ (llm-config, status, endpoint-lease), quickstart.md

**Tests**: required — `internal/llm` is concurrency-critical engine code; every story
lands behavior tests beside code (`go test -race` green is the bar throughout). The
legacy-equivalence suite (US1) is the standing regression gate for every later phase.

**Organization**: grouped by the spec's six user stories. Delegation per plan.md slice
map: Phases 1–8 (Setup through US5) → spec-implementer on **Opus 4.8** (concurrency/
scheduling in `internal/llm`, cross-package seams — constitution V rubric); Phase 9
(US6) → spec-implementer on **Sonnet** (view/rendering + output surfaces).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no ordering dependency)

## Path Conventions

Single Go module, packages evolved in place per plan.md. All work on branch
`task-35-provider-routing` in worktree `.worktrees/task-35`.

---

## Phase 1: Setup

**Purpose**: regression anchor before any refactor

- [x] T001 Record the pre-change baseline: run `go test -race ./...` in the worktree and
  note the green run (and any pre-existing flakes) in the task-35 board notes; this is
  the "unchanged semantics" reference for every later phase

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the registry/config model and the mechanical tier→provider generalization
everything else sits on. After this phase the tree compiles and behaves identically for
legacy configs (proven in US1).

- [x] T002 Rework internal/llm/config.go per data-model.md + contracts/llm-config.md:
  add `ProviderConfig` (transport, endpoint, model, pricing, api_key_env/api_key,
  parallel, reasoning_effort, tool_mode, endpoint_capacity) and `RouteConfig` (custom
  UnmarshalJSON accepting bare-array shorthand and `{chain, no_fallback}`); `Config`
  gains `Providers map[string]ProviderConfig` + `Routes map[string]RouteConfig`;
  LoadConfig implements the full validation matrix (v2/legacy mutual exclusion, unknown/
  missing/duplicate route entries, no_fallback contradiction, per-provider required
  fields) with warn-and-clamp preserved per provider; legacy `local`/`cloud` derivation
  exactly per contract; DefaultConfig/WriteDefault emit the v2 default shape
- [x] T003 Generalize internal/llm/llm.go: rename `tier` struct to `provider` (name,
  cfg, caller, health, queue, prio, slots, inflight, est); replace the `routing` map and
  `tiers` map with a validated `providers map[string]*provider` + `routes map[Kind]route`
  built in `New()` (workers spawned per provider's clamped parallel); `Submit` dispatches
  to the kind's chain head with today's admission semantics; `Response` gains `Provider`
  (always set) and cost uses the serving provider's pricing; `Request` gains the
  `Provider` pin field (validated, admission-honoring — full walk semantics arrive in
  US3); keep `Kinds()` static and boot-validated against routes
- [x] T004 Propagate mechanically so the tree compiles with unchanged behavior: meter
  `Add(provider, cost)` signature (attribution lands in US4), `llm.Status` →
  `{Providers []ProviderStatus, Month, Spent, Budget}` per contracts/status.md with
  internal/ipc/server.go (+ protocol type if typed) and the TUI pane updated to render
  rows minimally (full table polish in US6), cmd/promptworld llm one-shot and
  internal/mind/telemetry.go compiling against the new exports (`ProviderNames`,
  `EstimateForKind`, `ResolveProvider` — semantics proven in US2/US3)

**Checkpoint**: `go test -race ./...` compiles and passes with tests updated only where
they named tiers structurally, never where they pinned behavior

---

## Phase 3: User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP

**Goal**: registry + routes live; legacy equivalence proven; invalid configs die at boot
with named errors.

**Independent Test**: quickstart §1–§2.

- [x] T005 [P] [US1] Legacy-equivalence suite in internal/llm/config_test.go +
  llm_test.go: a legacy-shape llm.json derives providers `local`/`cloud` with today's
  routes, defaults, reasoning-effort/tool-mode resolution, parallel clamp warnings, and
  admission errors byte-identical to the pre-change suite (port the existing httptest
  mock-provider tests to prove routing/ceiling/breaker/queue behavior unchanged)
- [x] T006 [P] [US1] Validation-matrix tests in internal/llm/config_test.go: every row of
  contracts/llm-config.md's matrix fails LoadConfig with an error naming the offending
  entry; v2 default from WriteDefault round-trips loadable and semantically equals
  today's defaults
- [x] T007 [US1] Chain-head dispatch tests in internal/llm/llm_test.go: v2 config with
  two providers routes each kind to its chain head; `Response.Provider` names it; worker
  counts per provider match clamped `parallel` (extend the TASK-45 concurrency tests to
  two simultaneous providers under `-race`)

**Checkpoint**: US1 shippable alone — pure generalization, zero behavior change

---

## Phase 4: User Story 2 — Division of labor: per-provider speed truth (Priority: P2)

**Goal**: per-provider estimators, calibration seeding by provider name, mind seam reads
chain-head estimates.

**Independent Test**: quickstart §3 (live) + estimator tests.

- [x] T008 [P] [US2] internal/cognition/calibration.go: key `SeedFor` by provider name
  with pricing-class bootstrap fallback (zero-priced → local constant, priced → cloud
  constant) per research.md R5; legacy profile keys `local`/`cloud` keep matching by
  name; update cmd/promptworld/calibrate.go to write per-provider profile entries
- [x] T009 [US2] Orchestrator estimator seam in internal/llm/llm.go: per-provider
  estimators seeded via `SeedCalibration`; `EstimateForKind(kind)` returns the current
  admissible chain head's name + estimate (deterministic, falls back to chain head when
  none admissible); `ObserveCognition(kind, provider, millis)` feeds the named serving
  provider (empty provider → chain head) — update internal/toolloop/loop.go to pass
  `Response.Provider` through; recalibrate hook fires per provider; adapt
  internal/mind/telemetry.go to the seam; tests: estimator attribution under concurrent
  two-provider load (`-race`), whole-loop observation reaching exactly the serving
  provider, seed fallback classes

**Checkpoint**: a fast small model is never averaged with a slow quality model

---

## Phase 5: User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3)

**Goal**: admission-time chain-walk with recorded skips; no_fallback; pin field; scene
pinning in the conversation layer.

**Independent Test**: quickstart §4–§5.

- [x] T010 [US3] Chain-walk admission in internal/llm/llm.go per data-model.md: walk
  skips only circuit-open / wallet-exhausted (priced) / queue-full (best-effort
  additionally requires idle slot + empty queues per candidate); `Response.Skipped`
  records ordered `{Provider, Reason}`; all-inadmissible returns the head's refusal
  error; `no_fallback` and `Request.Provider` pin bypass walking while honoring the
  target's admission; `ErrUnknownProvider` for a pin naming an undeclared provider;
  post-dispatch failure is final (no re-dispatch). Tests in internal/llm/llm_test.go for
  each skip reason, walk order, no_fallback refusal, pin admission, and no-redispatch
  under `-race`
- [x] T011 [US3] Scene pinning in internal/mind/convo.go: resolve the scene's provider
  once at scene start via `ResolveProvider(KindConversation)` and stamp
  `Request.Provider` on both conversation Submit sites (convo.go:417, convo.go:465 —
  the scene struct carries the pin); mid-scene failure flows into the TASK-42 tolerance
  path unchanged. Tests in internal/mind: every turn of a scene carries the same
  provider even when a preferable candidate frees mid-scene; pinned-provider death →
  tolerance absorbs, never a provider switch (assert zero mid-scene switches)

**Checkpoint**: SC-004/SC-005 provable

---

## Phase 6: User Story 4 — One wallet, per-provider attribution (Priority: P4)

**Goal**: global ceiling unchanged; spend attributed per provider, persisted, summing.

**Independent Test**: quickstart §6.

- [x] T012 [US4] internal/llm/meter.go per research.md R4: `Add(provider, cost)` writes
  the unchanged total key AND `llm_spend_YYYY-MM:<provider>` under one lock; `Snapshot`
  returns per-provider attribution; month rollover clears both; legacy months surface
  the total-minus-attributed remainder as unattributed. Tests: Σ(providers) +
  unattributed == total on every path, persistence across store reopen, ceiling refusal
  still skips priced candidates while zero-priced serve (ties into the US3 walk tests),
  concurrent Add attribution under `-race`

**Checkpoint**: SC-007 provable

---

## Phase 7: User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5)

**Goal**: opt-in advisory flock lease pool bounding cross-world endpoint concurrency;
contended surfaced.

**Independent Test**: quickstart §7.

- [x] T013 [US5] New internal/llm/lease.go per contracts/endpoint-lease.md: endpoint
  normalization, pool dir `~/.promptworld/endpoint-leases/<sha256[:16]>/` with lazy slot
  files + `endpoint` name file, non-blocking flock sweep with jittered ~100 ms retry
  bounded by ctx, release-on-close; provider `contended` atomic set on wait > 2 s /
  cleared on wait < 2 s
- [x] T014 [US5] Worker integration in internal/llm/llm.go: lease-enabled providers
  acquire after the stale-skip check, inside `callCtx` (2-min cap), before
  `caller.call`; estimator measures from post-acquisition start; lease-wait expiry never
  strikes the breaker; `contended` flows into `ProviderStatus`. Tests in
  internal/llm/lease_test.go: two orchestrators (in-process pools, temp HOME) sharing
  one endpoint never exceed capacity C combined in-flight under saturation (`-race`),
  no breaker opens from contention alone, contended flag sets/clears, slot files
  reclaimed after a released pool (close), undeclared capacity ⇒ zero lease syscalls

**Checkpoint**: SC-006 provable (automated portion)

---

## Phase 8: Polish for the engine slices (Opus tier wrap-up)

- [x] T015 [P] Reconcile package docs and comments: internal/llm package comment
  (llm.go:1) and config.go/meter.go/health.go headers describe providers + chains
  instead of two tiers; drop retired exports (`TierFor`, `SecondsPerPoint(tier)`,
  `Tier` in routing contexts) and confirm no stragglers via `go vet ./...` + grep
- [x] T016 Full-suite gate: `go test -race ./...` green in the worktree; run quickstart
  §1/§2/§4/§5/§6/§7 automated commands verbatim and record outputs in task-35 board
  notes

---

## Phase 9: User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]

**Goal**: the status/TUI/CLI surfaces render what the engine already produces.

**Independent Test**: quickstart §3 + §8.

- [ ] T017 [P] [US6] TUI provider table per contracts/status.md: the pane rendering tier
  health/spend today renders one row per provider (name, model, up/down glyph, queue,
  inflight/slots, contended marker, spend share incl. `(unattributed)` when nonzero),
  sorted by name; view tests beside the existing TUI tests
- [ ] T018 [P] [US6] CLI surfaces in cmd/promptworld: `promptworld llm` one-shot prints
  serving provider and any `Skipped` reasons; `promptworld status` JSON passes the
  per-provider table through verbatim; calibrate output names providers; update command
  help strings that say local/cloud
- [ ] T019 [US6] End-to-end proof per quickstart §3/§8 against a live world (v2 config
  with two local providers): capture the one-shot naming each expected provider, a
  forced-fallback skip reason, and the TUI table; record evidence in task-35 board notes

- [ ] T020 [US6] v2-registry calibration in cmd/promptworld/calibrate.go: replace the
  legacy `--tier local|cloud|all` iteration with iteration over the declared providers
  (legacy configs iterate their two derived providers — unchanged UX), pinning each
  reference call via `Request.Provider` and writing one profile entry per provider name
  (the shape `cognition.SeedFor` already reads, slice-2 seam comment marks the site);
  `--provider <name>` narrows to one. Tests: a v2 three-provider config produces three
  named profile entries; legacy config output is byte-identical to today's
---

## Phase 10: Composition with spec 025 (post-merge reconciliation)

**Purpose**: TASK-72 (spec 025: in-loop retry + per-kind max_tokens) merged to main
mid-implementation; research.md R9 records the composition rulings. Opus tier
(toolloop/llm concurrency + conflict-bearing rebase).

- [ ] T021 Rebase task-35-provider-routing onto origin/main across the spec-025 merge
  and reconcile internal/llm/config.go: the v2 registry Config keeps
  `MaxTokens *TokenBudgets` + normalizer methods and `LoopMaxRounds` top-level in both
  shapes, shape-aware MarshalJSON round-trips them byte-for-byte (omitempty preserved),
  legacy derivation untouched by them; re-run the full -race suite including spec 025's
  retry/token tests and the legacy-equivalence suite
- [ ] T022 Run-level provider pinning in internal/toolloop/loop.go per research.md R9 /
  FR-008 extension: resolve ResolveProvider(kind) once at Run start, stamp
  Request.Provider on every round INCLUDING the spec-025 in-loop retry;
  ObserveCognition attribution uses the pinned provider (exact by construction). Tests:
  retry lands on the pinned provider even when the chain's fallback is admissible and
  preferable; a multi-round run never changes provider mid-transcript; pinned-provider
  hard-down fails the run per spec 025 semantics and the NEXT run resolves to the
  fallback

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 2 (Foundational) ← Phase 1; blocks everything
- Phase 3 (US1) ← Phase 2 — MVP, independently shippable
- Phase 4 (US2) ← Phase 3 (equivalence suite standing)
- Phase 5 (US3) ← Phase 3 (chain machinery); convo pinning also ← T009 (ResolveProvider)
- Phase 6 (US4) ← Phase 5 (walk's wallet-skip tests reference attribution)
- Phase 7 (US5) ← Phase 3 (worker structure stable); independent of US2–US4
- Phase 8 ← Phases 3–7
- Phase 9 (US6) ← Phase 8 (engine facts final before surfacing)

### Parallel Opportunities

- T005 ∥ T006 (different test files); T008 ∥ T009-prep (cognition vs llm packages)
- Phase 7 (US5) can proceed in parallel with Phases 4–6 once Phase 3 lands
- T015 ∥ T016; T017 ∥ T018

## Implementation Strategy

MVP = Phases 1–3 (US1): pure generalization, legacy worlds untouched — shippable and
reviewable on its own. Then US2 (the measured division-of-labor win), US3 (resilience +
persona integrity), US4 (money), US5 (TASK-24 closure), engine polish, and finally the
US6 surfacing slice on the default implementation tier. One branch, one PR (TASK-35);
spec-bridge sync after each phase checkpoint moves the board.

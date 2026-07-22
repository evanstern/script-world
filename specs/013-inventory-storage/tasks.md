# Tasks: Inventory & Storage v1

**Input**: Design documents from `/specs/013-inventory-storage/`

**Prerequisites**: plan.md, research.md (R1–R9), data-model.md,
contracts/events.md, quickstart.md

**Tests**: included — the spec's success criteria are test-shaped (SC-001–SC-006),
and determinism/replay gates are project doctrine (testing-strategy wiki). Tests
land alongside code within each story, per constitution workflow.

**Organization**: tasks grouped by user story (spec.md US1–US5) so each story is
independently implementable and testable on the single task branch. All work on
`task-51-inventory-storage` in `.worktrees/task-51` — one task, one PR.

**Tier map (research R9)**: Phases 2–6 and 8 (substrate, executor, social wiring,
migration) → Opus 4.8 `spec-implementer`; planner-vocabulary and TUI tasks
(marked Sonnet-ok) → Sonnet `spec-implementer`. Tier + justification recorded on
TASK-51 at dispatch.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: branch isolation and a green baseline.

- [X] T001 Fetch/ff-pull root main, then create worktree: `git worktree add .worktrees/task-51 -b task-51-inventory-storage origin/main`
- [X] T002 Baseline `go test ./...` green in the worktree (planning artifacts ride main per house pattern — the branch forks after they land)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: state shapes, payloads, recipe row, format gate — everything every
story compiles against.

- [X] T003 Add `Pile`/`FoodBatch` structs + `State.Piles []Pile` per data-model.md in `internal/sim/agents.go` + `internal/sim/state.go`, with pile helpers (find-by-tile, create-or-merge, remove-when-empty; batch merge on identical `(Kind, SpoilAt)`)
- [X] T004 Extend `Structure` with `Owner int` + `Store *Inventory` (omitempty) and add `Kind string`/`Qty int` (omitempty) to `Intent` + `IntentSetPayload` in `internal/sim/agents.go` and to `PlanStep` in `internal/sim/plan.go`; fix compile-touched references
- [X] T005 [P] Add derived `bulk(Inventory) int` + tuning constants per data-model.md (`bulkCap` 24, `chestCap` 48, `chestPlankCost` 6, `rotWindowTicks` 172800, `theftTrustDelta` −120, `theftAffectionDelta` −40, `theftMemoryTone` −60) in `internal/sim/agents.go`
- [X] T006 [P] Add new payload structs with canonical field order per contracts/events.md: `DroppedPayload`, `PickedUpPayload`, `DepositedPayload`, `WithdrewPayload`, `FoodRottedPayload` in `internal/sim/agents.go`; `ChestTakenPayload` in `internal/sim/social.go`
- [X] T007 [P] Add `build_chest` recipe row ({planks 6} → structure "chest", on_site, fire-comparable duration) in `internal/sim/recipes.go`; extend the mirror test in `internal/sim/recipes_test.go`
- [X] T008 [P] Bump `FormatVersion` 2→3 in `internal/world/world.go`; extend `internal/world/world_test.go`: v2 manifest refused with the unsupported-version error naming `scriptworld migrate`
- [X] T009 Reducer scaffolding in `internal/sim/state.go`: register `agent.dropped`, `agent.picked_up`, `agent.deposited`, `agent.withdrew`, `social.chest_taken`, `sim.food_rotted` as explicit no-ops-for-now with TODO-per-story markers — then `go test ./...` green

**Checkpoint**: v3 world boots; nothing behaves differently yet.

---

## Phase 3: User Story 1 — Villagers can only carry so much (P1) 🎯 MVP

**Goal**: the bulk cap constrains every acquisition; the raw survival loop never
jams.

**Independent test**: fill a villager to the cap → gathering yields nothing and
depletes nothing; partial space truncates with forfeit; eating frees bulk; a
planner-less village survives 3+ game days (quickstart §Automated gates 2–3).

- [X] T010 [US1] Reducer yield clamps in `internal/sim/state.go`: `agent.foraged`/`agent.chopped`/`agent.hunted`/`agent.quarried`/`agent.collected_water` truncate to `bulkCap − bulk(Inv)` (overlay/depletion still applies — remainder forfeit, US1-AS2)
- [X] T011 [US1] Executor zero-space guard in `internal/sim/executor.go`: gather completions with zero free bulk emit `agent.intent_done` only — no harvest event, no depletion (US1-AS1, contested-resource pattern)
- [X] T012 [US1] Craft/give cap rules in `internal/sim/executor.go` + `internal/sim/state.go`: craft completion re-validation extends to net bulk delta (no fit ⇒ no event, intent cleared); give rule skips a full receiver, reducer clamps `social.gave` defensively (research R2)
- [X] T013 [US1] Bulk-audit table test in `internal/sim`: every edge from the R2 table — truncation at partial space, zero-space no-event/no-depletion, craft no-fit ⇒ `intent_done`, give skipped at full receiver, eat frees bulk, cook/bathe/build asserted net ≤ 0
- [X] T014 [US1] Degraded-mode regression test in `internal/sim`: 8 agents, no planner, ≥3 game days — all alive, ZERO storage events in the log, cap never deadlocks the raw loop (SC-001, FR-003) — the doctrine gate for this feature
- [X] T015 [P] [US1] TUI: carried bulk `n/24` per villager in the agent pane in `internal/tui/views.go` (SC-006; Sonnet-ok)

**Checkpoint**: the cap is real and provably survival-safe — MVP demonstrable.

---

## Phase 4: User Story 2 — Ground piles and emergent stockpiles (P2)

**Goal**: drop/pick_up create and drain per-tile commons piles; death spills;
adjacent piles read as one zone in the TUI.

**Independent test**: direct a drop → pile appears, second villager picks up,
adjacent drops cluster on the map, a death spills a lootable pile (quickstart
§Automated gates 1).

- [X] T016 [US2] `drop` goal in `internal/sim/policy.go` (target = current tile, instant) + `planGoals`/inject_intent validation in `internal/sim/plan.go` + `internal/sim/loop.go`; executor completion emits `agent.dropped{agent,x,y,kind,n}` with clamped actual counts in `internal/sim/executor.go`; reducer case (pile create-or-merge; food batch stamped `tick + rotWindowTicks`; spears most-worn-first) in `internal/sim/state.go`
- [X] T017 [US2] `pick_up` goal in `internal/sim/policy.go` (target = nearest pile on/adjacent, instant): executor emits one `agent.picked_up` per kind moved (truncated to free bulk; Kind "" = all kinds canonical order, food oldest-batch-first) in `internal/sim/executor.go`; reducer case incl. emptied-pile removal in `internal/sim/state.go`
- [X] T018 [US2] Death spill: reducer-internal on `agent.died` in `internal/sim/state.go` — entire `Inv` moves to the death-tile pile (created/merged; food stamped), `Inv` emptied (research R7, FR-006)
- [X] T019 [US2] Build-site validation in `internal/sim/policy.go`/`internal/sim/executor.go`: all `build_*` goals reject tiles holding a pile (FR-007)
- [X] T020 [US2] Tests in `internal/sim`: drop/merge one-pile-per-tile, pickup truncation, same-tick contested pickup (deterministic agent-order arbitration, second taker finds remainder), death spill incl. spear durabilities, build-on-pile refused, replay byte-identity over a drop/pickup/death run
- [X] T021 [P] [US2] TUI: pile glyph, adjacent-pile stockpile-zone grouping (render-side flood fill, no state), pile contents inspection in `internal/tui/views.go` (US2-AS5, SC-006; Sonnet-ok)
- [X] T022 [P] [US2] Planner vocabulary: `drop`, `pick_up` with kind/qty syntax guidance in `internal/mind/prompt.go` (Sonnet-ok)

**Checkpoint**: the commons exists; the village's layout can emerge.

---

## Phase 5: User Story 3 — Chests: the village learns to keep things (P3)

**Goal**: builder-owned finite chests; deposits/withdrawals truncate, never
destroy; chest food keeps.

**Independent test**: build a chest from 6 planks → owner recorded; deposit to
capacity 48; withdraw bounded by free bulk; full chest leaves excess carried;
chest food never spoils.

- [X] T023 [US3] `build_chest` goal in `internal/sim/policy.go` (build_shelter pattern + pile-tile exclusion); `agent.built{kind: chest}` reducer case in `internal/sim/state.go` consumes `chestPlankCost` planks and adds the structure with `Owner = builder`, `Store = &Inventory{}`
- [X] T024 [US3] `deposit` + `withdraw` goals in `internal/sim/policy.go` (nearest chest / nearest chest containing Kind, instant on arrival); executor completions truncate to chest space (`chestCap − bulk(*Store)`) / taker's free bulk and emit `agent.deposited`/`agent.withdrew` with actual counts in `internal/sim/executor.go`; reducer cases in `internal/sim/state.go`
- [X] T025 [US3] Tests in `internal/sim`: build cost + permanent owner, deposit/withdraw truncation both sides, full-chest partial deposit (US3-AS4), chest food unaffected by any time passage (FR-010), spear durability round-trip through a chest, replay byte-identity over a chest run
- [X] T026 [P] [US3] TUI: chest glyph + contents/owner inspection in `internal/tui/views.go` (SC-006; Sonnet-ok)
- [X] T027 [P] [US3] Planner vocabulary: `build_chest`, `deposit`, `withdraw` + larder guidance in `internal/mind/prompt.go` (Sonnet-ok)

**Checkpoint**: ownership exists mechanically; the larder works.

---

## Phase 6: User Story 4 — Theft is a story, not an error (P4)

**Goal**: non-owner withdrawals always work and always leave the full social
mark through existing machinery.

**Independent test**: SC-003 — 100% of non-owner withdrawals produce record +
owner memory + trust drop; 0% blocked; owner withdrawals produce none.

- [ ] T028 [US4] `social.chest_taken` record case in `internal/sim/social.go` + `internal/sim/state.go` (reducer effect: the record itself only — chronicle/TUI material, FR-011)
- [ ] T029 [US4] Theft companion batch in `internal/sim/executor.go`: non-owner `agent.withdrew` co-emits `social.chest_taken`, `social.relation_changed` (owner→taker, `theftTrustDelta`/`theftAffectionDelta`, reason `"theft"`), owner `agent.memory_added` (subject = taker, tone `theftMemoryTone`, high salience, any distance, skipped if owner dead), and witness `agent.memory_added` for living awake villagers within `witnessRadius` excluding the taker — one atomic batch; owner-from-own-chest emits `agent.withdrew` alone (FR-012, research R5)
- [ ] T030 [US4] Salience table entries in `internal/sim/memory.go`: chest built (high, village-visible — oven precedent), taking suffered/witnessed (high, negative)
- [ ] T031 [US4] Tests in `internal/sim`: SC-003 full-batch assertions, own-chest silence (US4-AS4), dead-owner rule (record + witnesses, no owner memory), rumor birth from the subject-tagged owner memory via existing machinery, replay byte-identity over a theft run

**Checkpoint**: taking is never blocked and never free.

---

## Phase 7: User Story 5 — Rot: the ground is not a larder (P5)

**Goal**: ground food spoils on schedule, visibly; chests preserve; the player
can read the whole storage picture.

**Independent test**: SC-004 — pile food gone within `rotWindowTicks` + 1 game
minute; chest food and all non-food immortal; rot events chronicle-visible.

- [ ] T032 [US5] Rot sweep on the per-game-minute heartbeat in `internal/sim/executor.go`: pile slice order, `SpoilAt ≤ tick` batches emit `sim.food_rotted{x,y,kind,n}` (same-kind merged per pile per sweep); reducer removes spoiled batches + empties piles in `internal/sim/state.go` (research R6)
- [ ] T033 [US5] Tests in `internal/sim`: rot inside window +1 minute (SC-004), non-food immortal, chest immunity, death-spill batches inherit fresh deadlines, rot-vs-pickup same-tick re-validation (spec edge case), replay byte-identity incl. `spoil_at` deadlines
- [ ] T034 [P] [US5] TUI/chronicle observability pass in `internal/tui/views.go`: verify SC-006 answerable end-to-end (piles, chests, owner, bulk) and storage happenings (first chest, taking, rot, death-site recovery) appear in the chronicle feed; fill any gaps found (Sonnet-ok)

**Checkpoint**: chests have their mechanical job; the loop closes.

---

## Phase 8: Migration & Format Door (Cross-Cutting)

**Purpose**: existing v2 worlds cross the behavior break with their people —
and their belongings — intact.

- [ ] T035 v2 legacy decode + pure v2→v3 transform in `internal/sim/migrate.go`: everything carries verbatim (NO land reset — no map inputs changed, research R3); carried bulk over `bulkCap` spills to a pile at the agent's tile (food batches stamped `migration tick + rotWindowTicks`); wire 1→2→3 chaining so a v1 world migrates in one run
- [ ] T036 `scriptworld migrate` 2→3 orchestration in `cmd/scriptworld` + `internal/world/world.go`: archive `world.db` → `world.v2.db` (existing-archive guard), fresh log `world.created` + `world.migrated{from_format: 2}` + initial snapshot, manifest → 3; same refusal set as 1→2 (running daemon, uncovered tail)
- [ ] T037 Migration tests: v2 fixture (agents incl. over-cap inventory, structures, overlays, mid-flight intents, memories/relations) migrates with people + land state verbatim, spill pile present; v1 fixture chains 1→2→3; snapshot-free replay from genesis ⇒ byte-identical; refusal cases

**Checkpoint**: the format break has a door; no goods are lost crossing it.

---

## Phase 9: Polish & Cross-Cutting

**Purpose**: whole-feature verification, live proof, DoD tail.

- [ ] T038 [P] Whole-feature replay test in `internal/sim`: one scripted run exercising EVERY new event type + death spill + rot + theft, byte-identical state hash on replay (SC-005); confirm new types no-op under the unknown-type convention
- [ ] T039 Quickstart live smoke (quickstart.md §Live validation) against a fresh v3 world in a scratch worlds home: SC-002 (full storage loop within 2 game days of planks) + SC-006 (TUI answers) observed; record observations on TASK-51
- [ ] T040 `go build ./... && go vet ./... && go test ./...` green; open TASK-51's single PR from `.worktrees/task-51`
- [ ] T041 Post-merge DoD tail: `/grounding-wiki:wiki-update` (re-pin list in quickstart.md §Post-merge), `spec-bridge:sync`, worktree cleanup, root ff-pull

---

## Dependencies & Execution Order

- **Phase 2 blocks everything** (shapes, payloads, recipe row, format gate).
- **US1 (Phase 3)** depends only on Phase 2 → MVP; it is also the reason the
  format bumps, so it must land before migration is finalized.
- **US2 (Phase 4)** needs Phase 2 only, but shares reducer/executor files with
  US1 — execute after US1 to keep the branch clean.
- **US3 (Phase 5)** needs US2 conceptually (pile-tile build exclusion, commons
  contrast) and planks from the existing 012 economy.
- **US4 (Phase 6)** needs US3 (chests to steal from).
- **US5 (Phase 7)** needs US2 (batches exist from drops) and US3 (chest
  immunity contrast).
- **Phase 8** needs the full v3 state shape settled — after US1–US5, so the
  transform targets the final format.
- **Phase 9** last.

Story completion order: US1 → US2 → US3 → US4 → US5 → migration → polish.

## Parallel Opportunities

- Phase 2: T005, T006, T007, T008 in parallel after T003/T004.
- Within each story: TUI + vocabulary tasks ([P]) parallel to each other after
  the mechanics land (T015; T021∥T022; T026∥T027; T034; T038).
- Cross-story parallelism is intentionally NOT recommended: reducer/policy/
  executor are shared files; sequential stories keep the single branch clean.

## Implementation Strategy

MVP = Phase 2 + Phase 3 (US1): the cap exists and is provably survival-safe —
demonstrable alone, guarded by the degraded-mode regression (T014). Then the
commons (US2), the larder (US3), the social payoff (US4), the pressure loop
(US5) — each checkpoint independently testable. The migration door (Phase 8,
Opus tier — determinism-critical, cross-package) comes after the format
stabilizes, then whole-feature polish and TASK-51's one PR. Tier per slice:
research R9 (Opus 4.8 for Phases 2–6 + 8; Sonnet for the marked TUI/vocabulary
tasks).

# Tasks: Resources, Food, and Crafting v1

**Input**: Design documents from `/specs/012-resources-food-crafting/`

**Prerequisites**: plan.md, spec.md, research.md (R1–R9), data-model.md, contracts/events.md, contracts/recipes.md

**Tests**: included — the spec's success criteria (SC-001..SC-006) and the project's
determinism discipline explicitly require them; tests land alongside code per
constitution tier guidance.

**Organization**: grouped by user story; each phase is an independently testable
increment. All work happens on TASK-50's single branch in `.worktrees/task-50`
(constitution II); phases are internal breakdown, never separate PRs.

**Model tiers (research R9)**: Phases 2, 4, and 8 → Opus 4.8 (cross-package substrate;
degraded-mode doctrine; migration is determinism-critical across sim/store/world).
Phases 3, 5, 6, 7, 9 → Sonnet unless a gate fails. Record tier + justification on
TASK-50 at each dispatch.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: branch/worktree and a green baseline.

- [ ] T001 Fetch/ff-pull root main, then create worktree: `git worktree add .worktrees/task-50 -b task-50-resources-food-crafting origin/main`
- [X] T002 Baseline `go test ./...` green in the worktree; note current `internal/worldmap` hash-test seeds for later comparison

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: shared state shapes, recipe table, format gate — every story builds on these. No behavior changes yet.

- [X] T003 Extend `Inventory` struct in `internal/sim/agents.go` per data-model.md (Stone/Water/Planks/RefinedStone/FoodRaw/FoodCooked/Meals ints + `Spears []int`, omitempty; DELETE legacy `Food int`) and fix all compile-touched references
- [X] T004 Add `FuelUntil int64` to `Structure` in `internal/sim/agents.go`; add tuning-constant block per contracts/recipes.md (yields, durations, restore values, fireBurnPerWood, fireFuelCap, spearDurability, restRegenShelter, satiety 900)
- [X] T005 [P] Create `internal/sim/recipes.go`: authoritative recipe table (inputs/outputs/duration/site rule) matching contracts/recipes.md exactly, with a table-driven test asserting the mirror
- [X] T006 [P] Add new payload structs in `internal/sim/agents.go` with canonical field order per contracts/events.md: `CraftedPayload`, `AtePayload` (replaces empty-shape use), `CookedPayload`, `BathedPayload`, `RefueledPayload`, `SpearBrokePayload`, `FireBurnedOutPayload`
- [X] T007 [P] Bump `FormatVersion` 1→2 in `internal/world/world.go`; extend `internal/world/world_test.go`: v1 manifest refused with the unsupported-version error (quickstart §5)
- [X] T008 Reducer scaffolding in `internal/sim/state.go`: register all new event type cases as explicit no-ops-for-now with TODO-per-story markers, so each story fills its own case without merge collisions — then `go test ./...` green

---

## Phase 3: User Story 1 — Stone and water enter the world (P1) 🎯 MVP

**Goal**: outcrops generate on every seed; quarrying and water collection work end to end, event-sourced and deterministic.

**Independent test**: fresh worlds show outcrops (same-seed identical); a directed agent quarries (inventory + permanent depletion) and collects water; contested quarry resolves like contested chop.

- [ ] T009 [US1] Add `Rock` TileKind + elevation-correlated outcrop placement (~6% dry land, after trees before forage, purpose tag `"rock"`) in `internal/worldmap/worldmap.go`; `Passable` excludes Rock; `Buildable` unchanged (research R1)
- [ ] T010 [US1] Extend `internal/worldmap/worldmap_test.go`: outcrops present across seed spread; same-seed identical `Hash()`; water/trees/forage/dens still present; ≥25% buildable floor (SC-001)
- [ ] T011 [US1] Add `Quarried []Point` overlay to `State` in `internal/sim/state.go` and merge into `effectiveKind`/`passable` in `internal/sim/terrain.go` (depleted = passable, not buildable, not quarryable, distinct kind for TUI)
- [ ] T012 [US1] Add `quarry` and `collect_water` to `resolveGoal` in `internal/sim/policy.go` (nearestAdjacentTo Rock-not-quarried / Water) and durations (400/60) to `intentDuration` in `internal/sim/agents.go`
- [ ] T013 [US1] Executor completion in `internal/sim/executor.go`: emit `agent.quarried` / `agent.collected_water` (HarvestPayload) with completion-tick re-validation; reducer cases in `internal/sim/state.go` (+2 Stone + Quarried append / +1 Water) per contracts/events.md
- [ ] T014 [US1] Tests in `internal/sim`: quarry happy path, contested quarry (second agent's work yields nothing), water inexhaustible, replay byte-identity over a quarry+collect run
- [ ] T015 [P] [US1] TUI: Rock glyph + quarried-out rendering in `internal/tui/views.go`
- [ ] T016 [P] [US1] Planner vocabulary: add `quarry, collect_water` to `goalVocabulary` in `internal/mind/prompt.go` with one-line guidance

**Checkpoint**: US1 fully functional — deliverable MVP.

---

## Phase 4: User Story 2 — Fine-grained food and cooking at the fire (P2)

**Goal**: unit-food economy live; fires burn fuel, cook, and get refueled; degraded-mode contract re-proven.

**Independent test**: yields land in new units; eating is most-nutritious-first to satiety; fire burnout/refuel/relight cycle works; planner-less village survives 3 game days, zero crafting/cooking events.

- [ ] T017 [US2] Reducer yield changes in `internal/sim/state.go`: `agent.foraged` → +2 FoodRaw; `agent.hunted` → +8 FoodRaw (bare); delete `eatFoodValue`
- [ ] T018 [US2] Eat rewrite: reflex/planner instant eat in `internal/sim/policy.go` + `internal/sim/executor.go` emits `agent.ate` with `AtePayload{meals, cooked, raw, food_after}` (most-nutritious-first to satiety 900, absolute after-value); reducer applies counts + absolute need
- [ ] T019 [US2] Fire fuel: `build_fire` completion sets `FuelUntil = tick + 2×fireBurnPerWood` (reducer, `internal/sim/state.go`); per-tick fuel sweep in `internal/sim/executor.go` emits `sim.fire_burned_out{x,y}` exactly once on the `tick−1 < FuelUntil ≤ tick` transition; `warmAt` in `internal/sim/terrain.go` requires lit
- [ ] T020 [US2] `refuel_fire` goal (instant on arrival): resolveGoal case in `internal/sim/policy.go`, `agent.refueled{agent,x,y,fuel_until}` absolute + cap now+12h, reducer relights; REFLEX addition per R5 — refuel dying/cold fire when carrying wood (night-cold step + prep step); also REMOVE shelter-building from the reflex prep ladder (shelter is planner-only now) and restate larder-stocking in raw units
- [ ] T021 [US2] `cook` at fire: resolveGoal (nearest lit fire), duration 240, completion re-validates lit-ness, `agent.cooked{agent, station: fire, consumed, produced, kind: food_cooked}` (batch ≤8), reducer case
- [ ] T022 [US2] Tests in `internal/sim`: eat ordering/satiety/absolute payload; burnout emits once + refuel re-arms + cold fire refuses cook (contested pattern); reflex refuels; replay byte-identity over a food+fire run
- [ ] T023 [US2] Degraded-mode regression test: planner-less village of 8 survives ≥3 game days with zero `agent.crafted`/`agent.cooked`/`agent.bathed` events (SC-002) — the doctrine gate for this feature
- [ ] T024 [P] [US2] TUI: lit vs cold fire styling in `internal/tui/views.go`; inventory pane shows food triplet
- [ ] T025 [P] [US2] Planner vocabulary: `cook, refuel_fire` + guidance in `internal/mind/prompt.go`

**Checkpoint**: survival economy rebalanced and proven safe.

---

## Phase 5: User Story 3 — Crafting chain: planks, refined stone, spear (P3)

**Goal**: raw → intermediate → tool chain works anywhere; spear boosts hunts and wears out.

**Independent test**: craft planks → refined stone → spear; hunt with spear (12 yield, 600 ticks, use spent); third hunt breaks it with a memory.

- [ ] T026 [US3] Hand-craft goals `craft_planks, craft_stone, craft_spear` in `internal/sim/policy.go` (target = current tile) + durations; executor completion re-validates inputs via the recipes table and emits `agent.crafted{agent, kind}`; reducer applies recipe deltas (spear appends 3 uses to `Spears`, sorted)
- [ ] T027 [US3] Spear-aware hunting in `internal/sim/executor.go` + `internal/sim/state.go`: carrying a spear ⇒ hunt duration 600 and yield 12; completion spends `Spears[0]`; last use co-emits `agent.spear_broke{agent}` + high-salience memory entry in `internal/sim/memory.go`
- [ ] T028 [US3] Tests in `internal/sim`: recipe re-validation (insufficient inputs ⇒ no event), spend-lowest ordering, break-at-zero with memory, bare-vs-spear hunt parameters, replay byte-identity over a craft+hunt run
- [ ] T029 [P] [US3] Planner vocabulary: `craft_planks, craft_stone, craft_spear` + chain guidance ("planks from wood; spear needs wood + refined stone") in `internal/mind/prompt.go`

**Checkpoint**: two-step chain proven end to end.

---

## Phase 6: User Story 4 — The oven: meals and baths (P4)

**Goal**: the flagship station — stone-chain build, fueled meal batches, baths as water's first consumer.

**Independent test**: build oven from 4 refined stone + 2 planks; cook batch (1 wood + ≤8 raw → meals); bathe (1 water + 1 wood → +150 morale/+300 warmth absolute); no wood ⇒ no effect.

- [ ] T030 [US4] `build_oven` goal (build_shelter pattern, duration 900) in `internal/sim/policy.go`; `agent.built{kind: oven}` reducer case consumes 4 RefinedStone + 2 Planks and adds the structure
- [ ] T031 [US4] Oven cooking: extend `cook` resolution to prefer/accept ovens (station from target structure), duration 360, completion consumes 1 Wood fuel + ≤8 FoodRaw → Meals via `agent.cooked{station: oven, kind: meals}`; fuel-absent ⇒ no-effect resolution
- [ ] T032 [US4] `bathe` goal at oven (duration 240): completion consumes 1 Water + 1 Wood, emits `agent.bathed{agent, morale_after, warmth_after}` (absolute, capped) + positive-tone memory entry in `internal/sim/memory.go`; oven-built high-salience memory too
- [ ] T033 [US4] Tests in `internal/sim`: oven build costs, meal batch, bath effects absolute/capped, no-fuel no-ops for both batch actions, replay byte-identity over an oven run
- [ ] T034 [P] [US4] TUI: oven glyph in `internal/tui/views.go`
- [ ] T035 [P] [US4] Planner vocabulary: `build_oven, bathe` + guidance in `internal/mind/prompt.go`

**Checkpoint**: full economy loop (stone → oven → meals; water → baths) live.

---

## Phase 7: User Story 5 — Shelter joins the plank economy (P5)

**Goal**: shelter re-costed to planks; sleeping there recovers rest faster.

**Independent test**: build consumes 8 planks; sleeping on shelter regenerates rest at 6/min vs 4.

- [ ] T036 [US5] Re-cost `build_shelter` (resolveGoal validation + reducer) to 8 Planks in `internal/sim/policy.go` + `internal/sim/state.go`
- [ ] T037 [US5] Shelter rest bonus: `decayNeeds` in `internal/sim/executor.go` uses `restRegenShelter` (6) when asleep on a shelter tile; test both rates + plank cost in `internal/sim`

**Checkpoint**: all five items live.

---

## Phase 8: User Story 6 — An old world's people survive the new world (P6)

**Goal**: `scriptworld migrate` carries a v1 world's people across the format break;
the land resets. Reference target: `myworld-01` (107k events, tick 257,400).

**Independent test**: copy a v1 world fixture; migrate; people intact, map reborn,
archive present, zero-snapshot replay byte-identical; unclean/second runs refused.

- [ ] T038 [US6] `world.migrated` event: `WorldMigratedPayload{from_format, source_events, source_tick, state}` in `internal/sim/state.go` with wholesale state-replace reducer case (validates name/seed match); registered per contracts/events.md
- [ ] T039 [US6] v1 legacy decode + transform in `internal/sim/migrate.go`: migration-only reader for the v1 state shape (`Inventory.Food int`, no FuelUntil/Quarried); pure transform per research R10 — carry people-state verbatim (tick continuity), reset map-bound state, re-place agents via genesis placement on the v2 map, Wood 1:1, legacy Food × 3 → Meals
- [ ] T040 [US6] `scriptworld migrate <world>` command in `cmd/scriptworld` + `internal/world`: refuse running daemon (pid/sock check); require `LatestValidSnapshot.seq == max(events.seq)` else refuse with start+stop-under-v1 instructions; archive `world.db` → `world.v1.db` (existing archive ⇒ refuse, the already-migrated guard); write fresh db with `world.created` + `world.migrated` + initial snapshot; bump manifest `format_version` to 2; extend the v2 daemon's unsupported-version error to name the command
- [ ] T041 [US6] Migration tests: build a v1 fixture via legacy-shaped JSON (memories, relations, debts, rumors, structures, mid-flight intents, carried food/wood); migrate; assert people carried + map-state reset + conversion math + agents on passable tiles; delete all snapshots and replay from genesis ⇒ byte-identical state (SC-007's determinism half); refusal cases: uncovered tail events, second migration, running daemon
- [ ] T042 [US6] Migrate the real `myworld-01` (after `cp -R` backup): run the command, start the world under v2, verify souls/chronicle/relationships intact and outcrops present; record the run's observations on TASK-50 (SC-007)

**Checkpoint**: the format break has a door; myworld-01 lives on new land.

---

## Phase 9: Polish & Cross-Cutting

**Purpose**: full-surface observability, whole-feature verification, DoD tail.

- [ ] T043 [P] TUI inventory pane full expansion in `internal/tui/views.go` (wood/stone/water/planks/rstone + food triplet + spear count with min uses; SC-006)
- [ ] T044 [P] Whole-feature replay test in `internal/sim`: one scripted run exercising EVERY new event type, byte-identical state hash on replay (SC-004); confirm new types no-op under unknown-type convention
- [ ] T045 Quickstart smoke: run quickstart.md §2 (deterministic, no LLM) and §3 (planner progression toward SC-003) against a fresh world; record observations on TASK-50
- [ ] T046 `go test ./...` + full determinism suite green; open TASK-50's single PR from `.worktrees/task-50`
- [ ] T047 Post-merge DoD tail: `/grounding-wiki:wiki-update` (executor, event-types, worldmap-generation, reflex-policy, sim-state-reducer, tui-client, agent-mind, snapshots, world-save-directory, cli-scriptworld), `spec-bridge:sync`, worktree cleanup

---

## Dependencies & Execution Order

- **Phase 2 blocks everything** (state shapes, recipes, payloads, format gate).
- **US1 (Phase 3)** depends only on Phase 2 → MVP.
- **US2 (Phase 4)** independent of US1 mechanics (food/fire touch no stone) but shares
  reducer files — execute after US1 to avoid churn.
- **US3 (Phase 5)** needs US1 (stone for refined stone) and US2's unit food for spear
  hunt yields.
- **US4 (Phase 6)** needs US3 (refined stone + planks) and US2 (raw food, fuel pattern).
- **US5 (Phase 7)** needs US3's planks only.
- **US6 (Phase 8)** needs the full v2 state shape settled — after US1–US5, so the
  transform targets the final format (a migration into a half-built format would need
  re-migrating). T042 (real myworld-01) ideally runs after T044's whole-feature replay
  test is green.
- **Phase 9** last.

Story completion order: US1 → US2 → US3 → US4 → US5 → US6.

## Parallel Opportunities

- Phase 2: T005, T006, T007 in parallel after T003/T004.
- Within each story: TUI + vocabulary tasks ([P]) parallel to each other and after the
  mechanics land (T015∥T016, T024∥T025, T029, T034∥T035, T043∥T044).
- Cross-story parallelism is intentionally NOT recommended: reducer/policy/executor are
  shared files; sequential stories keep the single branch clean.

## Implementation Strategy

MVP = Phase 2 + Phase 3 (US1): the world gains stone and water, provably deterministic —
demonstrable alone. Then US2 (the balance-sensitive slice, Opus tier, guarded by the
degraded-mode regression T023), then the chain stories US3→US5, the migration door
US6 (Opus tier — determinism-critical, touches state/store/world cross-package), polish,
one PR. The real myworld-01 migration (T042) is the feature's closing ceremony.

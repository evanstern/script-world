# Tasks: Walls, Axes, and Paths

**Input**: Design documents from `/specs/032-walls-axes-paths/`

**Prerequisites**: plan.md, spec.md, research.md (R1–R8), data-model.md, contracts/recipes.md, contracts/events.md, quickstart.md

**Tests**: included — the repo convention is tests alongside code (constitution V; quickstart defines the 7 scenario checks these tasks implement).

**Organization**: grouped by user story (US1 walls P1, US2 axes P2, US3 paths P3); each story is an independently testable increment.

> **Coverage-gate rule**: the boot gate (`ValidateToolCoverage`, internal/sim/toolcheck.go) fails any commit where a registry row lacks its `goalResolvers` arm — a story's registry task and resolver task land together (same commit group) to keep every checkpoint green.

## Phase 1: Setup

**Purpose**: the shared tuning surface every story reads

- [x] T001 Add the spec-032 constants block to internal/sim/agents.go (wallPlankCost/wallStoneCost/wallPlankHP/wallStoneHP/buildWallTicks/demolishChipHP/demolishTicks/repairHPPerUnit/repairTicks/pathStoneCost/buildPathTicks/axeDurability/chopYieldBare/chopYieldAxe/quarryYieldBare/quarryYieldAxe — literals per contracts/recipes.md and research R8; leave chopWood/quarryYield in place until T014 replaces their uses)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: state fields and pure helpers that US1 and US3 both build on

- [x] T002 Add Structure.HP (`json:"hp,omitempty"`) to internal/sim/agents.go and the pure helpers isWall/wallMaxHP/wallAt/pathAt/agentAt to internal/sim/terrain.go (data-model.md helper table); assert pre-032 snapshot bytes are unchanged by the additive field in internal/sim/state_test.go

**Checkpoint**: compiles green, no behavior change — user stories can begin

---

## Phase 3: User Story 1 - Walls shape the world (Priority: P1) 🎯 MVP

**Goal**: buildable plank/stone walls that block pathing, with HP, multi-cycle demolish, and repair (spec FR-001..007; research R1, R2, R5)

**Independent Test**: quickstart scenarios 2–4 — build a wall and watch routes detour; chip a plank wall down in 2 cycles / stone in 6 under one intent; repair a chipped wall; occupancy guard rejects building onto an occupied tile

- [x] T003 [P] [US1] Add build_wall_plank (2 planks) and build_wall_stone (2 refined_stone) recipes plus the wallRepairMaterial helper to internal/sim/recipes.go; extend the contract-mirror assertions in internal/sim/recipes_test.go with the contracts/recipes.md literals
- [x] T004 [P] [US1] Teach passable() to return false on standing-wall tiles in internal/sim/terrain.go; add reroute-around-wall and unreachable-when-enclosed BFS tests in internal/sim/wall_test.go (new file)
- [x] T005 [US1] Add worldToolsBase rows build_wall_plank/build_wall_stone/demolish/repair with PromptGloss lines (walls block movement + HP + repairable; demolish cycles; repair cost) in internal/tool/registry.go — lands with T006 (coverage gate)
- [x] T006 [US1] Add goalResolvers arms in internal/sim/policy.go: wall builds via nearestAdjacentTo over buildSite (stand=Target, build=Res; input check per recipe), demolish targeting the nearest wall via nearestAdjacentTo over isWall, repair targeting the nearest damaged wall (HP < wallMaxHP) with 1 matching material carried
- [x] T007 [US1] Executor work in internal/sim/executor.go: wall-build completion re-validates buildSite(ResX,ResY) && !agentAt(ResX,ResY) and emits agent.built with Res coords + a situated builder memory (shelter salience); demolish cycles emit agent.wall_chipped or (HP ≤ chip) agent.wall_destroyed; repair cycles re-validate wall+damage+material and emit agent.wall_repaired; contested-wall completions resolve via agent.intent_done (contracts/events.md ordering rules)
- [x] T008 [US1] Reducer arms in internal/sim/state.go: agent.built stamps HP=wallMaxHP for wall kinds; agent.wall_chipped subtracts demolishChipHP and resets the actor's Intent.WorkStart=0; agent.wall_destroyed removes the structure and clears the intent; agent.wall_repaired consumes 1 matching material, clamps HP to max, and either resets WorkStart (damaged + material remains) or clears the intent
- [ ] T009 [US1] Scenario tests in internal/sim/wall_test.go: full wall lifecycle (build→chip×N→destroy→tile passable), plank 2 cycles vs stone 6, repair math + at-full-HP no-resolve, occupancy guard, no-agent-ever-on-wall-tile over a long run, replay hash identical (quickstart scenarios 2–4, 7)
- [ ] T010 [P] [US1] TUI glyphs in internal/tui/views.go: wall_plank "▤", wall_stone "▩", dim style when HP < wallMaxHP (cold-fire precedent); extend internal/tui/tui_test.go fixture

**Checkpoint**: walls fully functional and demoable with no axe/path code present

---

## Phase 4: User Story 2 - Axes make harvesting worthwhile (Priority: P2)

**Goal**: craft_axe (1 plank + 1 stone, 10 uses) gating chop AND quarry at 1 bare / 3 with axe, full spear-pattern durability + storage symmetry (spec FR-008..012; research R4)

**Independent Test**: quickstart scenarios 1 and 6 — yield comparison with/without axe, break on 10th use, axes move through piles/chests like spears

- [ ] T011 [P] [US2] Add Inventory.Axes and Pile.Axes ([]int, sorted ascending, omitempty) to internal/sim/agents.go; count len(Axes) in bulk(); add "axes" to canonicalKinds; extend Pile.empty(); snapshot-stability assertions in internal/sim/state_test.go
- [ ] T012 [US2] Add the craft_axe recipe (1 plank + 1 stone → 1 axe) and "axe" cases in craftKindFor/craftGoalFor in internal/sim/recipes.go; add the craft_axe worldToolsBase row + axe gloss and "axes" in itemKinds in internal/tool/registry.go; route craft_axe through the shared craft resolver closure in internal/sim/policy.go (single commit group — coverage gate)
- [ ] T013 [US2] Executor in internal/sim/executor.go: co-emit agent.axe_broke immediately after a chop/quarry completion when pre-event Axes[0]==1 (same batch, spear-broke precedent incl. situated memory)
- [ ] T014 [US2] Reducer in internal/sim/state.go: agent.crafted "axe" appends an axeDurability-use axe (sorted); agent.chopped/agent.quarried switch to chopYieldBare|chopYieldAxe / quarryYieldBare|quarryYieldAxe derived from pre-mutation len(Axes) and spend Axes[0] when carried; add the agent.axe_broke arm; DELETE chopWood/quarryYield from internal/sim/agents.go and fix every stale use the compiler surfaces
- [ ] T015 [US2] Storage plumbing: move axes through drop/pick_up/deposit/withdraw transfer paths in internal/sim/state.go (uses-preserving, sorted, "axes" kind key) and let give_item grant fresh axes in internal/sim/miracles.go
- [ ] T016 [US2] Tests in internal/sim/axe_test.go (new): craft→10 uses, 1-vs-3 yields both verbs, durability spend, break-on-last-use companion ordering, bare-after-break, bulk truncation atop axe yield, storage round-trip; update yield expectations in existing internal/sim/craft_test.go and internal/sim/quarry_test.go (2 → 1/3)

**Checkpoint**: axe economy live; walls (US1) untouched and still green

---

## Phase 5: User Story 3 - Paths speed travel (Priority: P3)

**Goal**: build_path (1 stone) tile improvement; stepping from a path tile moves at exactly 2x via the dual-phase cadence; routing unchanged (spec FR-013..015; research R3)

**Independent Test**: quickstart scenario 5 — paved corridor traversed in half the ticks (±1 step); only steps FROM path tiles accelerate

- [ ] T017 [US3] Add the build_path recipe (1 stone, no HP) to internal/sim/recipes.go + contract-mirror test; add the build_path worldToolsBase row + path gloss to internal/tool/registry.go; add the build_path goalResolvers arm (stand-on-target build, build_fire pattern) to internal/sim/policy.go (single commit group — coverage gate)
- [ ] T018 [US3] Movement cadence in internal/sim/executor.go: replace the single modulo gate with phase := (nextTick+int64(i)*3)%moveEveryTicks; step on phase==0 always, and on phase==2 iff pathAt(s, a.X, a.Y) (research R3); wall/oven-style completion validation for build_path is the existing generic buildSite arm — extend the executor's build-kind switch to include it
- [ ] T019 [P] [US3] TUI path glyph "·" (terrain-level, structures/agents win) in internal/tui/views.go
- [ ] T020 [US3] Tests in internal/sim/path_speed_test.go (new): N-tile paved corridor = half the unpaved ticks (±1), mixed-route per-step acceleration, off-path agents unaffected, replay hash identical

**Checkpoint**: all three stories independently functional

---

## Phase 6: Polish & Cross-Cutting

- [ ] T021 Whole-feature determinism pass in internal/sim/whole_feature_test.go (or a new spec-032 section): one session exercising axe+wall+path end-to-end, replayed to a byte-identical hash; pre-032 snapshot (no hp/axes fields) loads unchanged (quickstart scenario 7)
- [ ] T022 Run quickstart Gate 0 (go build ./... && go vet ./... && go test ./internal/sim/ ./internal/tool/ ./internal/tui/) and the TUI smoke; fix anything surfaced
- [ ] T023 Re-ground: /grounding-wiki:wiki-update over touched notes (docs/wiki/executor.md, reflex-policy.md, sim-state-reducer.md, tool-registry.md), then player-docs freshness check (node .claude/skills/player-docs/scripts/check-freshness.mjs --check) and regenerate if stale

---

## Dependencies & Execution Order

- **Setup (T001)** → **Foundational (T002)** → user stories.
- **US1 (T003–T010)**: T003/T004/T010 parallel; T005+T006 together (gate); T007 after T005/T006; T008 after T007's event names exist; T009 last.
- **US2 (T011–T016)**: independent of US1 (different reducer arms/files-sections); T011 first; T012 commit group; T013→T014 (emitter/reducer pair); T015 after T011; T016 last.
- **US3 (T017–T020)**: independent of US1/US2; T017 commit group; T018 core; T019 parallel; T020 last.
- **Polish (T021–T023)** after all desired stories.
- Stories are sequential on one branch (one TASK, one PR) but US2/US3 have no code dependency on US1 beyond T001/T002.

## Parallel Example: User Story 1

```text
# After T002, launch together:
T003 recipes+mirror-test (internal/sim/recipes.go)
T004 passable+reroute tests (internal/sim/terrain.go, wall_test.go)
T010 TUI glyphs (internal/tui/views.go)
# Then T005+T006 (one commit group), T007, T008, T009.
```

## Implementation Strategy

MVP = Phase 1 + 2 + US1 (walls): demoable world-shaping with no axe/path code. Then US2 (economy rebalance — note bare yields drop to 1 the moment T014 lands, making the axe load-bearing), then US3. Each checkpoint must hold Gate 0 green (build + vet + sim/tool/tui tests + coverage gate). Implementation runs on the spec-implementer agent at the **Opus 4.8 tier** (plan.md Constitution Check V: cross-package, core movement/pathability semantics).

# Tasks: Metatron Miracles

**Input**: Design documents from `/specs/016-metatron-miracles/`

**Prerequisites**: plan.md, spec.md (clarified 2026-07-22), research.md (R1–R8),
data-model.md, contracts/interfaces.md, quickstart.md

**Tests**: INCLUDED — the spec's success criteria are test artifacts (SC-002 replay
byte-identity, SC-003 drift test, SC-005 adversarial gratis strip); this project's
doctrine is tests alongside code.

**Organization**: grouped by user story. Board: TASK-59; tier: Opus 4.8 (doctrine-adjacent
+ cross-package, per plan Constitution Check); one branch `task-59-metatron-miracles` in
`.worktrees/task-59`, one PR.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [ ] T001 Create worktree: `git fetch origin && git pull --ff-only` in root, then `git worktree add .worktrees/task-59 -b task-59-metatron-miracles origin/main`; all subsequent work happens inside `.worktrees/task-59`

---

## Phase 2: Foundational (blocking prerequisites for all stories)

**⚠️ No user story work until this phase completes.**

- [X] T002 Create `internal/sim/miracles.go` with the four payload structs (`TimeSnappedPayload{ToTick,Gratis}`, `ItemGrantedPayload{Agent,Kind,Qty,Gratis}`, `EntityMovedPayload{Class,X,Y,ToX,ToY,Gratis}`, `EntityRemovedPayload{Class,X,Y,Gratis}`), the cost table (`miracleCost`: time_snapped 2, others 1), the shared charge spend/validate helper (checks `MetatronCharges >= cost` unless gratis, decrements unless gratis), and an `applyMiracle(e store.Event) error` dispatcher with stub arms returning "not implemented"
- [X] T003 Route the four `metatron.*` miracle event types from `State.Apply` in `internal/sim/state.go` to `applyMiracle`, and add the four entries to `injectSocialWhitelist` in `internal/sim/loop.go` (contracts/interfaces.md §4)
- [X] T004 [P] Add IPC plumbing in `internal/ipc/protocol.go` (`MiracleArgs` with kind/day/time/villager/item/qty/class/x/y/to_x/to_y/gratis; `MiracleData` with kind/charges/gratis/summary) and a `case "miracle"` arm in `internal/ipc/server.go` that validates args shape and returns "unknown kind" for unimplemented kinds; MUST NOT require `srv.llm`/`srv.metatron` (works on pure-sim worlds)
- [X] T005 [P] Add the `miracle` subcommand skeleton to `cmd/promptworld/main.go` (usage text for all four verbs per contracts/interfaces.md §3, `--force` flag parsing, world resolution, IPC call, exit 0/1 with summary/reason) wired to the T004 IPC command
- [X] T006 [P] Create the shared batch-builder in `internal/metatron/miracle_batch.go`: `BuildMiracleBatch(kind, params, gratis) ([]store.Event, error)` producing the miracle event plus FR-018 `agent.memory_added` events (fixed deterministic templates, salience `SalDream`; recipients per data-model.md perception table); used by BOTH the IPC server arm and the angel path so the two doors cannot drift (research R6); unit-test the builder's event/memory composition in `internal/metatron/metatron_test.go`

**Checkpoint**: all four event types land-able (stubs), whitelist open, both doors reach the builder.

---

## Phase 3: User Story 1 — Rescue a stuck villager: entity move/remove (P1) 🎯 MVP

**Goal**: move villagers/structures/piles and remove structures/piles/terrain via recorded, charge-priced, replay-deterministic events; villagers can never be removed.

**Independent test**: quickstart Scenario A — CLI-move a villager next to another with the world running; verify position, cancelled intent, memory, charge spend, and post-restart recovery.

- [ ] T007 [US1] Implement the `metatron.entity_moved` reducer arm in `internal/sim/miracles.go`: class dispatch (villager/structure/pile), target-presence check at (x,y) (reject absent class, FR-014), destination validation (villager → `passable`; structure → `buildSite`; pile → `passable` + merge onto existing destination pile per one-pile-per-tile doctrine), villager move clears `Intent` and stamps `IdleSince` to landing tick, structures move whole (FuelUntil/Owner/Store), charge spend via the T002 helper
- [ ] T008 [US1] Implement the `metatron.entity_removed` reducer arm in `internal/sim/miracles.go`: reject class `villager`; structure removal spills chest `Store` to a ground pile on the tile before deletion; pile removal deletes contents; terrain removal routes tree→`Cleared` (standard regrow), forage→`Harvested` (standard regrow), rock→`Quarried`; already-overlaid terrain rejected as no-op target
- [ ] T009 [P] [US1] Reducer unit tests in `internal/sim/miracles_test.go`: happy paths per class, impassable/occupied destination rejects, absent-class rejects, villager-remove reject, chest spill contents equality, terrain overlay routing, insufficient-charge reject, no partial application on any reject (state deep-equal to before)
- [ ] T010 [US1] Wire move/remove through both doors: extend `landMiracle` cases in `internal/metatron/turn.go` (name→index resolution, in-fiction refusal reasons per contracts §1) and complete the IPC `move`/`remove` kinds in `internal/ipc/server.go` using `BuildMiracleBatch` + `InjectSocial`; finish the CLI `move`/`remove` verbs in `cmd/promptworld/main.go` (`<class> <x,y> [<x1,y1>]` parsing)
- [ ] T011 [P] [US1] Replay byte-identity test (SC-002 pattern from `chest_test.go`) in `internal/sim/miracles_test.go`: scripted move+remove sequence (incl. a chest spill) replays from log to identical state hash; plus IPC round-trip test for `move` in `internal/ipc/ipc_test.go`

**Checkpoint**: US1 shippable — the Ash incident is solvable with one CLI command (charged).

---

## Phase 4: User Story 2 — Operator force door: gratis miracles (P2)

**Goal**: the player console can land any miracle with cost waived; validation and recording unchanged; the model structurally cannot.

**Independent test**: quickstart Scenario B — with bank at 0, `--force` a grant/move; verify success, untouched bank, `"gratis":true` in the log; over-cap `--force` grant still rejects.

- [ ] T012 [US2] Thread gratis end-to-end for implemented kinds: CLI `--force` → `MiracleArgs.Gratis` → payload; verify the reducer helper (T002) skips spend-and-check only; response `MiracleData.Gratis` reflects it; assert in `internal/ipc/ipc_test.go` that a zero-bank forced move succeeds and bank stays 0
- [ ] T013 [P] [US2] Adversarial structural-strip test (SC-005) in `internal/metatron/metatron_test.go`: crafted model reply JSON containing `"miracle": {..., "gratis": true}` parses via `parseTurn` into a landed CHARGED event (no gratis field exists on the turn contract; assert payload `gratis=false` and a charge was spent)
- [ ] T014 [P] [US2] Validation-survives-gratis tests in `internal/sim/miracles_test.go` (forced invalid move/remove rejected identically to charged) and log-visibility check (recorded event payload carries `"gratis":true`, enumerable after the fact, SC-004)

**Checkpoint**: emergency operations never require stopping the daemon again.

---

## Phase 5: User Story 3 — Time snap with shift semantics (P2)

**Goal**: forward-only clock jump; relative durations frozen, history untouched, phase behavior follows the new clock, no charge minting.

**Independent test**: quickstart Scenario C — snap to day 2 11:30; clock reads new time, fire retains remaining burn, bank unchanged, everyone remembers the lurch; past-target rejected.

- [ ] T015 [US3] Implement `rebaseTicks(s *State, delta int64)` in `internal/sim/miracles.go` per the data-model.md taxonomy: SHIFT the listed relative-duration fields (Intent.WorkStart≠0, IdleSince, LastTalk, LastGive, Hail.Until, Structure.FuelUntil, pile FoodBatch.SpoilAt, Harvest.Regrow, DenUse ready/cooldown, Gru.LastAttack, Meeting.OpenedTick/GatherStart, Debt.Due); KEEP history/identity fields; doctrine comment stating every future tick-anchored field must be classified here
- [ ] T016 [US3] Implement the `metatron.time_snapped` reducer arm in `internal/sim/miracles.go`: reject `to_tick <= Tick` (FR-008), apply `rebaseTicks`, set `Tick = to_tick`, spend 2 charges via the T002 helper; wire day/HH:MM→tick conversion door-side (angel + IPC) via `internal/clock` (extend clock with a parse helper only if none exists)
- [ ] T017 [P] [US3] Taxonomy guard test in `internal/sim/miracles_test.go`: reflective walk over the marshalled state tree that fails when a tick-anchored `int64` field exists in the state structs without a SHIFT/KEEP classification entry (the drift-hazard tripwire from research R3)
- [ ] T018 [P] [US3] Drift test (SC-003) in `internal/sim/miracles_test.go`: whole-day variant — mid-activity state (in-flight intent, lit fire, ripening rot, pending regrow), snap +86400, drive snapped and control worlds through identical scripted ticks, assert event streams/final state identical modulo tick offset; arbitrary-delta variant — per-field remaining-duration preservation assertions; plus no-minting test (snap across ≥2 regen boundaries leaves bank unchanged, FR-010) and paused-world snap test
- [ ] T019 [US3] Wire `snap-time` through both doors: `landMiracle` time_snap case in `internal/metatron/turn.go` (system prompt gains the cost table + miracle grammar per contracts §1), IPC `time_snap` kind completion in `internal/ipc/server.go`, CLI `snap-time <day> <HH:MM>` verb in `cmd/promptworld/main.go`; snap memory for every living villager rides the T006 builder; replay byte-identity extended to a snapped log in `internal/sim/miracles_test.go`

**Checkpoint**: *snap* — it's 11:30 tomorrow, and the drift test proves nothing else moved.

---

## Phase 6: User Story 4 — Item grant (P3)

**Goal**: provision a living villager with known items, reject-never-clamp at the bulk cap.

**Independent test**: quickstart Scenario D-adjacent — grant food to a villager, see inventory +qty and a memory; over-cap grant rejects whole.

- [ ] T020 [US4] Implement the `metatron.item_granted` reducer arm in `internal/sim/miracles.go`: validate agent index/alive, kind against the Inventory key set (wood, stone, water, planks, refined_stone, food_raw, food_cooked, meals, spear), qty > 0, reject whole when `bulk(inv)+grant > bulkCap` (FR-011); spears append fresh full-use entries (sorted ascending); charge spend via T002 helper
- [ ] T021 [US4] Wire `give` through both doors (`landMiracle` case with villager-name resolution; IPC `give_item` kind; CLI `give <villager> <item> <qty>` verb) with the grant memory riding the T006 builder
- [ ] T022 [P] [US4] Tests in `internal/sim/miracles_test.go`: happy grant, over-cap whole-reject (inventory unchanged), unknown-kind/dead-villager/zero-qty rejects, spear-uses shape, replay byte-identity including a grant

**Checkpoint**: all four miracle families live behind both doors.

---

## Phase 7: Polish & Cross-Cutting

- [ ] T023 [P] Run the full quickstart (scenarios A–F) against a throwaway world per `specs/016-metatron-miracles/quickstart.md`; fix anything it surfaces; `go build ./... && go vet ./... && go test ./...` green
- [ ] T024 [P] Reconcile help/usage text in `cmd/promptworld/main.go` (top-level help lists `miracle`), and the metatron system prompt's cost table wording in `internal/metatron/turn.go`
- [ ] T025 Record the tier choice + rubric justification on TASK-59 (`backlog task edit 59 --append-notes ...`), run `spec-bridge:sync` to mirror phase progress, open the PR from `.worktrees/task-59` (one PR for the whole TASK); post-merge lifecycle: `/grounding-wiki:wiki-update` for notes sourcing `internal/sim`, `internal/metatron`, `internal/ipc`, `cmd/promptworld`

---

## Dependencies

```
T001 → T002 → T003 ─┬─ T004 → T005
                    ├─ T006
                    ▼
        US1: T007 → T008 → T009/T010/T011
                    ▼ (first landed kind exists)
        US2: T012 → T013/T014          (needs ≥1 implemented kind from US1)
        US3: T015 → T016 → T017/T018/T019   (independent of US1/US2 after Phase 2)
        US4: T020 → T021 → T022             (independent of US1–US3 after Phase 2)
                    ▼
        Polish: T023/T024 → T025
```

- US3 and US4 are independent of US1/US2 once Phase 2 lands (parallelizable).
- US2 formally depends on US1 only because gratis needs an implemented kind to exercise.

## Parallel Execution Examples

- After T003: T004, T005 (skeleton), T006 in parallel (different files).
- Within US1: T009 ∥ T011 after T007/T008; T010 ∥ T009.
- Within US3: T017 ∥ T018 after T015/T016.
- Cross-story: one implementer lands US3 (pure sim + clock) while another lands US4 —
  both only touch shared files at small, disjoint case-arms.

## Implementation Strategy

**MVP = Phase 1–3 (US1)**: after T011, the motivating incident is solved end-to-end with
a charged CLI move; ship-worthy checkpoint. Then US2 (tiny, high-leverage), then US3
(the deep one — re-base + drift proof), then US4, then polish. Subtasks are internal
breakdown: all commits ride `task-59-metatron-miracles`, merging in TASK-59's single PR.

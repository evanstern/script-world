# Tasks: Nightly Consolidation + Persona Firewall

**Input**: Design documents from `/specs/004-nightly-consolidation/`
**Tests**: Included — every SC is only provable by tests or the recorded quickstart run.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Foundational sim core (blocking)

- [X] T001 State + types in internal/sim/consolidate.go + state.go — Belief type, Agent.Beliefs/Narrative/LastConsolidatedNight(−1 genesis)/ConsolidatedUpTo/LastConsolidateMark, State.NextBeliefID, all five event payload structs, memory (tick,hash) identity helper
- [X] T002 Reducer cases in internal/sim/consolidate.go (wired from state.go Apply) — memory_promoted (salience boost, cap, no-op absent), memory_faded (remove, no-op absent), belief_revised (create id 0 via NextBeliefID / revise in place, clamp), narrative_set, consolidated marker (night+mark bump; accepted also advances ConsolidatedUpTo)
- [X] T003 Injection door extension in internal/sim/loop.go — five consolidation event types added to the whitelisted batch; dry-run/atomicity contract unchanged
- [X] T004 [P] Sim tests in internal/sim/consolidate_test.go — reducer table incl. no-op targets and clamps, once-per-night ledger idempotence across restart-shaped replays, buffer boundary math, replay determinism with consolidation timelines (SC-004)

**Checkpoint**: model-free consolidation substrate complete and proven.

---

## Phase 2: US1 — Sleep consolidates the day (P1)

- [X] T005 [US1] Trigger + worker in internal/mind/consolidate.go — observe agent.slept, guard (night/gap/alive/buffer per contracts/consolidation-events.md), FIFO single-flight worker, skipped_empty marker without a call, transport failure = no marker + deferred log
- [X] T006 [US1] Prompt + parse — consolidation prompt (persona + anchor + buffer with tick/hash + beliefs held + relations summary; oldest-first truncation >60) in internal/mind/consolidate.go; parseConsolidation in internal/mind/parse.go per contracts/consolidation-output.md
- [X] T007 [US1] Batch build + landing — accepted batch (promotes, fades, gist, beliefs, narrative, marker) and rejected/skipped batches through the injection door, all-or-nothing; daemon.log outcome lines (FR-010)
- [X] T008 [P] [US1] Driver tests in internal/mind/consolidate_test.go — scripted mock model: one accepted night lands the full batch; second sleep same night no-ops; transport failure injects nothing and retries next sleep; malformed output → rejected marker only

---

## Phase 3: US3 — The firewall holds (P2, blocking US2's live claim)

- [X] T009 [US3] persona.Anchor + persona.DriftMarkers (8 authored each) in internal/persona/personas.go — one temperament line + contradiction lexicon per villager
- [X] T010 [US3] Validator in internal/mind/validate.go — three layers per contracts/consolidation-output.md (structure, anchor echo, drift lexicon), stable rejection reasons
- [X] T011 [P] [US3] Firewall tests in internal/mind/validate_test.go — fixture drift rejected 100% with zero landings (SC-002); structural violations table; persona.md bytes identical through a full cycle + no post-genesis persona write API (FR-007)

---

## Phase 4: US2 — Souls that grow (P2)

- [X] T012 [US2] Scribe render in internal/scribe/scribe.go — soul.md gains "Who I am becoming" (narrative) and "Beliefs" (statement, confidence, provenance) sections from state
- [X] T013 [P] [US2] Scribe tests — beliefs/narrative render, multi-night growth visible (two synthetic nights of events → narrative changed, beliefs reference two days)

---

## Phase 5: Polish

- [ ] T014 [P] Full `go test ./... -race`; live 3-night run per quickstart Scenario 2 (+ degraded night Scenario 3 if practical); record specs/004-nightly-consolidation/quickstart-results.md
- [ ] T015 Wiki (new consolidation note + re-pins of touched sources), board sync, PR

## Dependencies

P1 → P2 → {P3, P4} → P5. US3 validator (T010) gates what US1's worker lands, so T005–T008
integrate it behind an interface stub until T010 replaces it. One TASK, one PR on
`task-9-consolidation`.

## Implementation strategy

MVP = Phase 1 + Phase 2 (consolidation lands, souls compress). Firewall (P3) ships in the
same PR before any live overnight run; scribe growth (P4) is the player-visible payoff;
P5 proves SC-001/003/005 live and closes the PDLC loop.

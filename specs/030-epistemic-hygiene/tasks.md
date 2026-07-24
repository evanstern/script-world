# Tasks: Epistemic Hygiene for Emergent Lore

**Input**: Design documents from `specs/030-epistemic-hygiene/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: included — determinism/replay obligations and the eval gate demand them (constitution V: tests alongside
code). All implementation executes via `spec-implementer` agents; tier ruling in plan.md (sim/mind doctrine slices
and the eval-gated prompt → Opus 4.8; scribe rendering + docs → Sonnet).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [x] T001 Cut worktree `.worktrees/task-79` (branch `task-79-epistemic-hygiene`) from fresh `origin/main`;
      baseline `go test ./...` green before any change

---

## Phase 2: Foundational (Blocking Prerequisites)

- [x] T002 Memory origin substrate: add `Memory.Origin` + payload field (`internal/sim/agents.go`), make the three
      situated constructors take a required origin parameter and stamp every emission site per
      contracts/events-and-decay.md (`action`/`witness`/`report` in `internal/sim/memory.go` + call sites,
      `gist` in `internal/sim/social.go`, `digest` in `internal/sim/consolidate.go` day-gist, `omen` at the
      Metatron dream/omen delivery site); add the direct-perception classifier (pure function) + per-site stamping
      and classifier table tests; snapshot-byte test proving legacy memories (absent field) marshal unchanged

**Checkpoint**: every landed memory carries an origin; classifier deterministic; `go test ./internal/sim/` green.

---

## Phase 3: User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP

**Goal**: evidence-cited beliefs; "witnessed" survives only on direct-perception evidence; coerce-not-reject.

**Independent Test**: quickstart §1–§2 — scripted reflections through the validator prove both directions;
coercion counter visible on the night's marker.

- [x] T003 [US1] Consolidation contract: prompt gains the evidence-citation instruction + witnessed/told guidance;
      output parse accepts `evidence` ordinals with `MaxBeliefEvidence` (4) best-first pre-trim, in
      `internal/mind/consolidate.go` per contracts/consolidation-contract.md
- [x] T004 [US1] Validator enforcement: resolve evidence like promote/fade refs, apply the coercion table
      (witnessed→kept/told/inferred), add the non-fatal coercion counter to the `agent.consolidated` marker
      payload, in `internal/mind/validate.go`; fixture-table tests proving FR-004 both directions incl.
      no-evidence, unresolvable-ref, and old-shaped (no evidence field) outputs
- [x] T005 [US1] Landing + replay: `agent.belief_revised` payload gains `evidence` (resolved identities) +
      `direct`; reducer stores them; formation stamps `Reinforced = e.Tick` (normative note in
      contracts/events-and-decay.md) in `internal/sim/consolidate.go`; reducer table tests + replay byte-identity
      for a log containing coerced beliefs (SC-003 first half)

**Checkpoint**: the Birch case cannot recur (SC-001); nights land atomically; replay proven.

---

## Phase 4: User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2)

**Goal**: computed decay with the documented curve; floor semantics; reinforcement seam.

**Independent Test**: quickstart §3 — curve table to the tick; floor crossing changes rendering/prompt surfacing;
reinforcement resets; replay holds.

- [x] T006 [US2] Decay arithmetic: `Belief.Reinforced` field, `EffectiveConfidence`, doctrine constants
      (`BeliefHalfLifeDays` 8, `BeliefConfidenceFloor` 20) in `internal/sim/consolidate.go`; revision refreshes
      the stamp iff `direct` (US2-AC3); curve table tests (formation, half-life boundary, floor crossing, legacy
      grandfather `Reinforced == 0`, post-reinforcement reset)
- [x] T007 [US2] Reinforcement seam: `agent.belief_reinforced` event type whitelisted through the injection door
      (`internal/sim/loop.go` whitelist + total reducer arm in `internal/sim/consolidate.go`); tests emitting it
      through the door incl. vanished-target no-op; replay suite extended with a reinforcement event (SC-003
      second half); seam documented in the events contract (already) + doc comment naming the future producer
- [x] T008 [US2] Read sites: scribe Beliefs section renders effective values and the hedged below-floor form
      (`internal/scribe/scribe.go`); consolidation held-beliefs block shows effective values with "(faded)"
      markers (`internal/mind/consolidate.go`); any other belief-surfacing prompt excludes below-floor; render
      tests pinning the hedged form and the exclusion

**Checkpoint**: myths fade from conviction into story; nothing stored mutates; SC-002 proven.

---

## Phase 5: User Story 3 — Gists preserve attribution (Priority: P3)

**Goal**: attribution-preserving outcome prompt, shipped only through the eval gate.

**Independent Test**: quickstart §4 — eval numbers meet the ship bar before convo.go changes; live sample clean.

- [x] T009 [US3] Eval assets: `eval/fixtures/` (≥3 speculation, ≥3 action-discussed-not-done, ≥4 control scenes),
      `eval/old.md` (current outcome prompt verbatim), `eval/new.md` (attribution variant), and
      `scripts/eval-prompt-79.sh` (modeled on `scripts/eval-prompt-73.sh`) per contracts/eval-protocol.md
- [x] T010 [US3] Run the eval (fixtures × variants × N≥3 on the standard local model), judge-score, write
      `eval/decision.md` with the pre-stated tolerance and the numbers; record numbers + verdict on TASK-79
      (AC #3) — THE GATE: T011 may not start unless the bar is met (≥50% reduction, controls in tolerance)
- ~~T011 [US3] Ship the winning prompt in `internal/mind/convo.go`~~ — **CLOSED won't-ship, 2026-07-24**
      (planning-tier ruling from eval/decision.md): gate NOT met. Standard tier gemma4:12b-mlx: 0/18 defects
      before AND after (controls 12/12) — nothing to fix; cogito:3b (the tier world-01 runs, which produced the
      Thornspire defects): 3/18→5/18, no reduction — wording doesn't help the failing tier. convo.go intentionally
      unchanged; the confabulation class is model-tier, not prompt. Operational follow-up filed on the board
      (upgrade world-01 local tier). AC #3's live-sample half moves to T013 (current prompt, standard tier).

**Checkpoint**: gists attribute; the laundering pump is off; numbers on the task.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T012 Full-suite gate: `go test -count=1 ./...` green, `go vet ./...` clean, `gofmt -l` clean on
      branch-touched files; confirm no-format-bump argument (legacy snapshot byte tests from T002/T006 pass)
- [x] T013 Live validation per quickstart §2/§3/§5 in a scratch home (provenance honesty live, decay/floor
      observation or fixture-accelerated equivalent, SC-005 myth-survives sample); record in
      `specs/030-epistemic-hygiene/quickstart-results.md` (012/T045 precedent for anything out of budget)
- [x] T014 Post-merge re-grounding: `/grounding-wiki:wiki-update` (nightly-consolidation, agent-mind,
      social-fabric, sim-state-reducer, event-types, testing-strategy + any planner-prompt notes touched) +
      player-docs freshness/refresh; `spec-bridge:sync`; worktree cleanup

---

## Dependencies & Execution Order

- Setup → Foundational (T002) blocks US1 (classifier) and transitively US2.
- Within US1: T003 → T004 → T005 (contract → enforcement → landing).
- US2 depends on US1's `direct` flag (T005): T006 → T007/T008 (T007 ∥ T008, different files).
- US3 is independent of US1/US2 (prompt-only): T009 may start after Setup; T010 gates T011 hard.
- Polish last; T014 post-merge.

### Parallel Opportunities

- T009 ∥ (T002–T008) — disjoint files (convo/eval vs consolidation/sim).
- T007 ∥ T008 within US2. T012 ∥ T013 at polish.

## Implementation Strategy

MVP = US1 (provenance honesty): smallest slice that kills the live dishonesty. Then US2 (decay turns residue into
myth), US3 (turns off the pump) — US3 can interleave earlier since it's disjoint, but its GATE (T010) is absolute.
One branch, one PR (TASK-79); commit per task; suite green at every checkpoint.

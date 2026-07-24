---
id: TASK-79
title: >-
  Epistemic hygiene for emergent lore: honest belief provenance, hearsay decay,
  attribution-preserving gists
status: In Progress
assignee: []
created_date: '2026-07-23 17:49'
updated_date: '2026-07-24 05:37'
labels:
  - emergent-lore
  - epistemics
dependencies: []
priority: medium
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-23 world-01 Thornspire investigation. The villagers collectively invented a place ("Thornspire") and phenomena ("glowing tendrils", "green tangles") that do not exist in world state — emergent mythology we WANT — but the epistemic machinery records the fiction as fact:

- Origin: Metatron omen at tick 102060 (seq 50664, rainbow "pointing toward the forest's edge... something is being shown to them") → conv 107943 at tick 108001 (seq 53114-53119) invents Thornspire + green tangles as sensemaking. 271 events reference Thornspire; 133 are social.rumor_told.
- Conversation gists flatten speculation into shared fact: "The team discussed storm signs near Thornspire after Rowan observed unusual green tangles" (seq 53119) becomes identical salience-4 memories for all participants; later "discussed the glowy tendrils after investigating" (seq 64555) claims an investigation that never happened.
- Belief provenance is dishonest: seq 62654 (Birch) records the tendril belief at confidence 68 with provenance "witnessed" — the omen was witnessed, the tendrils never existed. seq 55078 (Cedar) confidence 58 "inferred".
- Nothing decays confidence on beliefs never confirmed by direct observation.

Scope (hygiene, NOT suppression — invention must survive, as myth rather than fact):
1. Provenance honesty: a belief formed from conversation/rumor content records hearsay/inferred, never witnessed; witnessed is reserved for direct perception (own executed-action memories, delivered omens/dreams).
2. Confidence decay: beliefs never reinforced by direct observation decay over game-days (analogous to memory salience half-life); leave a reinforcement seam for the future grounded-observation channel (see perception-of-absence task).
3. Gist attribution: the conversation-gist prompt preserves attribution for unverified claims ("Rowan claimed he saw glowing tendrils") instead of flattening to communal fact, and never asserts completed actions that did not occur.

Non-goals: preventing invention of places/phenomena; grounding conversation content against world state (that is the perception-of-absence task); rumor-mechanics changes.

Item 3 is prompt-behavior-affecting → eval-gated per TASK-73 precedent, not vibes-gated. Items 1-2 touch belief/reducer state → replay determinism must hold.

Spec: specs/030-epistemic-hygiene
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Belief revision sourced from talk/rumor content can never record provenance 'witnessed'; witnessed requires a direct-perception source (test proving both directions)
- [x] #2 Never-reinforced beliefs decay in confidence deterministically over game-days; decay constants + rationale recorded on the task; replay/determinism suite passes
- [x] #3 Gist prompt preserves attribution: before/after eval on scripted fixtures + live sample shows no fact-flattened confabulation of the 'after investigating' shape; eval numbers recorded on the task
- [x] #4 A reinforcement seam exists for future grounded observations to refresh belief confidence (documented, even if no producer yet)
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (Blocking Prerequisites)
- [x] #7 Spec phase: User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2)
- [x] #9 Spec phase: User Story 3 — Gists preserve attribution (Priority: P3)
- [ ] #10 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1) Ground: wiki notes covering beliefs/provenance (spec 019 grounded-memories), conversation gists, rumor flow, salience decay precedent. 2) speckit-specify spec 030-epistemic-hygiene (3 mechanisms: provenance honesty, confidence decay + reinforcement seam, attribution-preserving gists; eval-gated per TASK-73 precedent). 3) Clarify genuinely-open design points with user if artifacts do not answer. 4) speckit-plan + speckit-tasks. 5) spec-bridge:link, sync. 6) Implement via spec-implementer agents per constitution V tier rubric; eval for item 3. 7) PR, wiki re-pin, Done via sync.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 0/1 · Foundational (Blocking Prerequisites): 0/1 · User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP: 0/3 · User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2): 0/3 · User Story 3 — Gists preserve attribution (Priority: P3): 0/3 · Polish & Cross-Cutting Concerns: 0/3

Decay constants + rationale (AC #2 requirement, research R3): BeliefHalfLifeDays = 8 — a conviction unconfirmed by direct observation halves in ~a game-week, an order of magnitude slower than memory recency (halves per game-day), because convictions outlive vividness. BeliefConfidenceFloor = 20 — just under the rumor tellability floor (25), so a belief stops driving behavior slightly before its rumor stops being tellable: the story outlives the conviction (myth survives, fact fades). Legacy beliefs (no Reinforced stamp) are grandfathered — no retroactive decay at upgrade. Computed-on-read per the memory-recency precedent; no decay events; replay untouched.

T001 done: worktree .worktrees/task-79 (branch task-79-epistemic-hygiene) cut from origin/main @6bac0d7; baseline go test ./... green (one pre-existing flake: internal/metatron TestDigestFailureCarries failed once, passed -count=3 on rerun; unrelated to this spec).

Tier ruling at dispatch (constitution V, per plan.md): T002-T007 (origin substrate, validator coercion, belief reducer/replay, decay arithmetic, injection-door seam — internal/sim + internal/mind doctrine-adjacent concurrency/reducer logic) → Opus 4.8. T009-T011 (gist prompt + eval gate — prompt-behavior-affecting, TASK-73 precedent tier) → Opus 4.8. T008 (scribe/prompt rendering of effective confidence) → Sonnet.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 1/1 · User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2): 0/3 · User Story 3 — Gists preserve attribution (Priority: P3): 0/3 · Polish & Cross-Cutting Concerns: 0/3

US1 checkpoint reviewed and accepted (commits 444bc69..8d86929 on task-79-epistemic-hygiene; sim+mind suites re-run uncached by orchestrator, green). AC #1 proven by T004 coercion-table tests (witnessed with secondhand-only evidence → told, no evidence → inferred; witnessed with direct-perception evidence kept) + T002 classifier tests. Deviations accepted: (a) miracle_batch.go item-grant memories stamped OriginOmen — directly-perceived divine act, same family as the enumerated dream/omen site; (b) Belief.Reinforced classified SHIFT (non-zero) in the rebase taxonomy — elapsed-time decay anchor, grandfather 0 preserved; (c) revision-time direct-refresh of Reinforced deliberately deferred to T006 per task split, direct flag already lands on the payload.

US2 core (T006-T007) reviewed and accepted (commits 798217a, 5a21238; sim suite re-run uncached by orchestrator, green; full suite green per implementer). AC #2 proven: EffectiveConfidence computed-on-read with BeliefHalfLifeDays=8 / BeliefConfidenceFloor=20 (constants + rationale in doc comments and in the note above), curve pinned to the tick (day4→57 proves continuous decay), legacy Reinforced==0 grandfathered, stored state never mutates, replay byte-identity holds with coerced + reinforced events in the log. AC #4 proven: agent.belief_reinforced whitelisted through the injection door with a total reducer arm (vanished-target no-op), doc comment names the future grounded-observation/perception-of-absence producer; no in-tree producer by design. Note: implementer used continuous math.Pow(0.5, days/8) per the contract formula (not memory-recency's integer-day halving) — deterministic, never stored, cannot affect replay bytes. Mid-flight git note: an accidental 'pull --rebase' in the worktree briefly rewrote the task branch; restored via reset --hard to 5a21238 — recorded hashes remain valid; branch still forks from 6bac0d7.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 1/1 · User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2): 3/3 · User Story 3 — Gists preserve attribution (Priority: P3): 0/3 · Polish & Cross-Cutting Concerns: 0/3

T008 reviewed and accepted (commit a1bdec1; scribe+mind suites re-run uncached by orchestrator, green). Hedged form follows the authoritative contract wording ('half-remembered: <statement>', no number) over research.md's variant; '(faded)' marker appended end-of-line; held-beliefs block keeps below-floor beliefs listed (data-model.md revisability exception). Orchestrator ruling: sim.PromptBeliefs ships as an unused-but-tested exclusion primitive — same seam-before-producer pattern as T007/AC #4.

US3 gist-attribution eval (T009-T010, spec-bridge eval gate FR-010/SC-004) — SHIP BAR NOT MET; internal/mind/convo.go UNCHANGED, T011 not started.

AUTHORITATIVE run, gemma4:12b-mlx (the standard local tier default), N=3, temp 0.8 / max_tokens 224 / reasoning_effort none (matching the daemon's outcome call), same-model judge temp 0, 10 fixtures (3 speculation / 3 action-discussed-not-done / 4 control):
- old prompt: treatment defect 0/18 = 0.00% (flattened 0, confab 0); control faithful 12/12 = 100%.
- new prompt (eval/new.md): treatment defect 0/16 = 0.00% (flattened 0, confab 0; 2 parse-fails on the 3-participant fixture overran 224 tokens); control faithful 12/12 = 100%.
- Reduction: 0% -> 0% = N/A. The standard model already writes honest, attributed gists unprompted ('Rowan claims to have seen...', 'agreed to investigate ... the following day' — not 'after investigating'), so a >=50% relative reduction is undefined against a 0% baseline (pre-registered boundary condition in eval/decision.md).

NON-AUTHORITATIVE corroboration, cogito:3b generation + gemma4:12b-mlx judge (the weaker class that produced the world-01 Thornspire defects; run because the standard baseline is 0% and so cannot show whether the prompt helps where the failure occurs):
- old: treatment 3/18 = 16.67%; new: 5/18 = 27.78%. Defects rose (no reduction). Most new flags are the judge over-penalizing correctly-attributed-but-vivid gists; two are genuine small-model failures (confabulated 'Mira cut planks'; Thornspire content bleeding into a control). Within noise at n=18, but unambiguously not a >=50% reduction.

VERDICT: gate not met on either model. AC #3 NOT satisfied (requires a demonstrated reduction + clean live sample). Evidence: specs/030-epistemic-hygiene/eval/decision.md + eval/results/{,gemma4-12b-mlx,cogito-3b}/.

ESCALATION (design decision for the planning tier, not the implementer): the FR-010/SC-004 eval assumes the standard local model exhibits the confabulation shapes; it does not. Options in decision.md — A) do not ship US3 (recommended: the standard tier is already clean and cogito gives no signal new.md helps weak models); B) ship new.md as cheap insurance and amend the gate to record a non-demonstrable-but-accepted exception. Needs an orchestrator ruling before US3/AC#3/AC#9 can close.

US3 eval gate verdict: NOT MET — prompt not shipped, convo.go unchanged (AC #3 stays unchecked pending the T013 live sample; see ruling below). Numbers (eval/decision.md, bar pre-registered before any run: ≥50% treatment-defect reduction, controls in tolerance):
— gemma4:12b-mlx (standard local tier, authoritative; N=3, temp 0.8, reasoning_effort:none matching daemon): old 0/18 defects (0.00%), controls 12/12; new 0/16 (0.00%, 2 parse-fails on the longest fixture), controls 12/12. Reduction n/a — baseline already clean.
— cogito:3b (corroboration — the tier world-01 actually runs): old 3/18 (16.7%), controls 9/12; new 5/18 (27.8%), controls 10/12. No reduction; most new-variant flags are judge over-penalty, two are genuine small-model failures.
Orchestrator ruling (Option A, from artifacts): DO NOT ship — standard tier is already honest with the current prompt; the failing tier (world-01 llm.json: local=cogito:3b — verified this session) is not helped by wording. The Thornspire confabulation class is model-tier, not prompt. T011 closed won't-ship in tasks.md per decision.md; operational follow-up task filed to upgrade world-01's local tier. Tier note: T009-T010 ran on Opus 4.8 per dispatch ruling.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 1/1 · User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2): 3/3 · User Story 3 — Gists preserve attribution (Priority: P3): 2/2 · Polish & Cross-Cutting Concerns: 0/3

Polish dispatch: T012-T013 → Sonnet (routine validation/gate slice, no doctrine logic — default tier per rubric). Branch must merge origin/main (moved substantially: PRs #56/#57, spec 029/031/033) before the full-suite gate; merge not rebase, board notes cite branch hashes. T013 additionally runs quickstart §4's live multi-scene gist sample with the CURRENT prompt on the standard tier — if clean, it completes AC #3's live-sample half per the US3 ruling.

AC #3 checked on the ruling's evidence, with the prompt UNCHANGED: before/after eval numbers recorded (gemma4 0/18 both variants, controls 12/12; decision.md), and the T013 live multi-scene sample shows 0/7 fact-flattened or 'after investigating'-shaped confabulations on the standard tier (quickstart-results.md §4). The gist prompt as-is preserves attribution on the standard local tier; the failing tier's remediation is TASK-89 (world-01 cogito:3b upgrade). T012-T013 accepted: clean merge of main (efcca2b), suite green post-merge, live validation SC-001 13/13, decay readings match, SC-005 myth-survives observed live (d8568a3).
<!-- SECTION:NOTES:END -->

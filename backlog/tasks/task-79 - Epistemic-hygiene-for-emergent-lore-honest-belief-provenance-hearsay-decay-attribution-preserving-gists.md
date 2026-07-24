---
id: TASK-79
title: >-
  Epistemic hygiene for emergent lore: honest belief provenance, hearsay decay,
  attribution-preserving gists
status: In Progress
assignee: []
created_date: '2026-07-23 17:49'
updated_date: '2026-07-24 03:06'
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
- [ ] #2 Never-reinforced beliefs decay in confidence deterministically over game-days; decay constants + rationale recorded on the task; replay/determinism suite passes
- [ ] #3 Gist prompt preserves attribution: before/after eval on scripted fixtures + live sample shows no fact-flattened confabulation of the 'after investigating' shape; eval numbers recorded on the task
- [ ] #4 A reinforcement seam exists for future grounded observations to refresh belief confidence (documented, even if no producer yet)
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (Blocking Prerequisites)
- [x] #7 Spec phase: User Story 1 — Beliefs carry honest provenance (Priority: P1) 🎯 MVP
- [ ] #8 Spec phase: User Story 2 — Unconfirmed beliefs fade into myth (Priority: P2)
- [ ] #9 Spec phase: User Story 3 — Gists preserve attribution (Priority: P3)
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
<!-- SECTION:NOTES:END -->

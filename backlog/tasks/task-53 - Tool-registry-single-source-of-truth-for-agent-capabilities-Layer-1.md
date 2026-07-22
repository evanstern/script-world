---
id: TASK-53
title: 'Tool registry: single source of truth for agent capabilities (Layer 1)'
status: Done
assignee: []
created_date: '2026-07-22 02:49'
updated_date: '2026-07-22 18:16'
labels:
  - llm
dependencies: []
priority: high
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Design decision (2026-07-21, TASK-52 exploration): formalize everything an agent can do — metatron to villagers — as a Tool: name + param schema + gate + effect + cost, in ONE registry. Core principle: a tool call is a REQUEST; an event is the FACT; the gate decides; the executor grounds work in time and space. The model never asserts outcomes.

This is the standalone formalization layer — NO model-API change, behavior-identical, replay-identical. It de-risks the tool-use loop (TASK-52) and journal tools (TASK-16) that build on it.

Current state being fixed: the action vocabulary is hand-maintained in three duplicate maps plus a prompt string — goalVocabulary (internal/mind/prompt.go:15), validGoals (internal/mind/parse.go:31), planGoals (internal/sim/plan.go:33) — and adding a verb touches ~7 sites in lockstep (also policy.go resolveGoal, agents.go intentDuration, executor.go executeAtTarget, a reducer arm). Each capability (planner goals, say, muse, gist, metatron nudge, governance rephrase) has bespoke prompt contract + parser + injection plumbing. Grounding for spec 014 (2026-07-22) found the maps have ALREADY drifted in shipped code: planGoals is missing the 9 spec-012 verbs (defect TASK-55, violates spec 012 FR-020) — spec 014 FR-012 records its cure as the sole permitted behavioral delta.

Scope:
- Registry defines every tool: name, param schema, gate (precondition vs live state), effect (intent-producing vs immediate event batch vs read-only), cost (duration / charges / size budget).
- Prompt vocabulary, mind-side parse validation, and sim-door validation all DERIVED from the registry — the triple maps die.
- Tool classes: world tools (chop/forage/build/... -> intents, executor-grounded), expressive tools (say/muse/journal writes -> immediate whitelisted events), read tools (search/read -> data back into cognition, no events; enabled later by TASK-52).
- Per-agent ROSTERS: capability = roster membership. Villager roster = world + expressive tools; Metatron roster = converse/nudge_dream/nudge_omen. Future asymmetry (chief proposes laws) becomes a roster edit, not new plumbing.
- muse joins the registry as a roster tool (decision: merged into roster, not a separate scheduled channel). In this layer its scheduling/trigger stays as-is; agents choosing to muse via tool call arrives with the TASK-52 loop.
- Migrate the existing verbs (19 as of spec 014 grounding: original 10 + spec 012's 9) + say + muse + conversation gist + metatron nudge onto registry entries.

Both injection doors (Loop.InjectIntent loop.go:127, Loop.InjectSocial whitelist loop.go:146) and the landing ladder (generation/staleness/guards) are preserved exactly — the registry formalizes them, it does not relax them.

Spec: specs/014-tool-registry
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 One registry defines every tool (name, param schema, gate, effect, cost); prompt vocabulary, mind parse validation, and sim door validation are derived from it — duplicate maps removed
- [x] #2 Existing capabilities (10 verbs, say, muse, gist, metatron nudge) are registry entries; behavior- and replay-identical (existing event logs reproduce identical state)
- [x] #3 Per-agent tool rosters exist; villager and metatron rosters expressed as data; an action outside an agent's roster is rejected at the door
- [ ] #4 Injection doors and landing-ladder validation (generation, staleness, guards) unchanged and covered by tests
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational — the registry package (blocks all stories)
- [x] #7 Spec phase: User Story 1 — One place to define a capability (P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 — Existing capabilities migrate unchanged (P2)
- [x] #9 Spec phase: User Story 3 — Capability is roster membership (P3)
- [x] #10 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 0/4 · Foundational — the registry package (blocks all stories): 0/6 · User Story 1 — One place to define a capability (P1) 🎯 MVP: 0/9 · User Story 2 — Existing capabilities migrate unchanged (P2): 0/5 · User Story 3 — Capability is roster membership (P3): 0/4 · Polish & Cross-Cutting: 0/5

Implementation started 2026-07-22. T001 gate verified: TASK-51 Done (PR #33 merged), spec-013 verbs on main. Worktree .worktrees/task-53 (branch task-53-tool-registry) cut from 6ea47bf.

Tier (T004, Principle V rubric): Opus 4.8 for T005–T016, T020–T026 — cross-package architectural change (new internal/tool + internal/mind + internal/sim + internal/metatron + internal/daemon), door/replay-sensitive (injection doors, landing ladder). Sonnet eligible for T017–T019, T027–T031 per tasks.md; executing Phases 1–3 as one Opus slice.

Grounding delta since spec 014 was planned: TASK-58 (merged today, de1ef19) added plannerReplySchema() in internal/mind/parse.go — a FOURTH derived surface consuming validGoals/validKinds/planStepCap (plus schema_test.go). T012's derivation must feed it; vocabulary is 24 verbs post-spec-013 (T002 re-enumeration covers).

Phases 1–3 complete on task-53-tool-registry (commits 8a2550e, d269c40, 36bb197) by spec-implementer @ Opus 4.8. Orchestrator re-verified: dead-map sweep empty, golden prompt UNCHANGED, TestPlannerReplySchema green, tool+sim+mind suites pass. FR-012 delta confirmed as exactly the 9 spec-012 verbs (spec-013's 5 were already in planGoals).

Ratified by planning tier:
1. converse = Expressive with empty Events; Validate() one-directional (Events ⇒ Expressive). FR-008/FR-010 inconsistency to be reconciled in T030 doc pass.
2. qty not modeled as Param (no numeric ParamKind in fixed contract); validateKindQty remains enforcement. TASK-52 will need a numeric ParamKind — recorded in catalog.
3. T013 single-goal door deferral: registry membership in substance via goalResolvers table + T016 coverage check; explicit roster door check is T025 (Phase 5) to keep rejection strings byte-identical in Phase 3.
4. Duration literal duplication per R7 accepted; Phase 6 must add a hand-equal cross-check test (registry DurationTicks ≡ sim constants) to prevent silent drift.

Phases 4–5 complete (commits 8cb5a86, 998049b) by spec-implementer @ Opus 4.8; orchestrator re-verified roster/whitelist/coverage tests + golden prompt green, worktree clean.

US2 identity proven: full suite green with ZERO replay/determinism test edits; caps (say 300B / gist 200B / muse 200 runes / nudge 400B) registry-sourced at identical values; whitelist pinned diff-identical (17 entries). US3: roster membership enforced at both doors — villager door rejects nudge_*/converse/say/unknown with byte-identical reason; metatron reducer gates form on RosterMetatron, no charge on refusal.

T024 live smoke (throwaway world, PROMPTWORLD_HOME sandboxed, cogito:3b): boot gate passed live; planners landed (538ms reply); multi-step plans landed including one naming collect_water — the FR-012 delta visible live (old planGoals would have rejected it); 20 musings; executor events across the widened vocabulary (quarry, collect_water, craft_planks, pick_up). No conversation reached in the ~6.5 game-hour window (emergent; unit-covered) — noted, not a failure.

Remaining: Phase 6 (T029 ladder audit, T030 doc reconciliation incl. FR-008/FR-010 converse + ratified duration hand-equal cross-check test, T031 quickstart+vet, T032 wiki re-pin, T033 sync/PR/close-out). T029–T031 → Sonnet per tasks.md tier split.

spec-bridge sync: Setup: 0/4 · Foundational — the registry package (blocks all stories): 0/6 · User Story 1 — One place to define a capability (P1) 🎯 MVP: 0/9 · User Story 2 — Existing capabilities migrate unchanged (P2): 0/5 · User Story 3 — Capability is roster membership (P3): 0/4 · Polish & Cross-Cutting: 0/5

PR #36 opened from .worktrees/task-53 (8 commits, phases 1–6 complete incl. wiki re-pin on-branch). Awaiting merge; post-merge: re-run spec-bridge:sync (phase mirrors re-check from merged tasks.md → Done-eligible), close TASK-55, remove worktree.

spec-bridge sync: Setup: 4/4 · Foundational — the registry package (blocks all stories): 6/6 · User Story 1 — One place to define a capability (P1) 🎯 MVP: 9/9 · User Story 2 — Existing capabilities migrate unchanged (P2): 5/5 · User Story 3 — Capability is roster membership (P3): 4/4 · Polish & Cross-Cutting: 5/5 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 4/4 · Foundational — the registry package (blocks all stories): 6/6 · User Story 1 — One place to define a capability (P1) 🎯 MVP: 9/9 · User Story 2 — Existing capabilities migrate unchanged (P2): 5/5 · User Story 3 — Capability is roster membership (P3): 4/4 · Polish & Cross-Cutting: 5/5). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->

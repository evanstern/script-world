---
id: TASK-53
title: 'Tool registry: single source of truth for agent capabilities (Layer 1)'
status: In Progress
assignee: []
created_date: '2026-07-22 02:49'
updated_date: '2026-07-22 05:33'
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
- [ ] #1 One registry defines every tool (name, param schema, gate, effect, cost); prompt vocabulary, mind parse validation, and sim door validation are derived from it — duplicate maps removed
- [ ] #2 Existing capabilities (10 verbs, say, muse, gist, metatron nudge) are registry entries; behavior- and replay-identical (existing event logs reproduce identical state)
- [ ] #3 Per-agent tool rosters exist; villager and metatron rosters expressed as data; an action outside an agent's roster is rejected at the door
- [ ] #4 Injection doors and landing-ladder validation (generation, staleness, guards) unchanged and covered by tests
<!-- AC:END -->

---
id: TASK-16
title: 'Grounded memories: context-rich episodic capture'
status: In Progress
assignee: []
created_date: '2026-07-19 15:56'
updated_date: '2026-07-23 03:44'
labels:
  - memory
  - agent-mind
dependencies:
  - TASK-52
priority: high
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
<!-- SECTION:DESCRIPTION:BEGIN -->
Memories are terse strings baked at emission — executor templates like 'Built a fire.' / 'Talked with %s.' (internal/sim/executor.go, internal/sim/memory.go) and convo gists (internal/mind/convo.go). soul.md is a faithful view over them (internal/scribe/scribe.go); there is NO richer store behind it — the events table payload IS the whole memory. Yet the grounding detail already exists as sibling events that memories never reference: agent.thought carries the planner's reason (sim/loop.go:371), social.conversation_turn carries full dialogue text (mind/convo.go:135), agent.built/foraged/hunted carry X/Y coords. Task: enrich episodic capture so each memory is situated — extend MemoryAddedPayload (internal/sim/agents.go:218) with structured context (place/coords, driving intent reason when one exists, refs to underlying events e.g. conv id) and enrich the deterministic templates with where/why. All detail must be baked at emission and reducer-applied so replay stays model-free and deterministic. This directly upgrades the input TASK-9 consolidation digests and what prompts/soul.md show.

## Agent-authored journal (added 2026-07-21)

Second layer on top of the deterministic episodic capture: give each agent a personal journal — a tiny markdown wiki the agent writes itself. Where the memory stream is system-authored (baked at emission), the journal is agent-authored: the agent decides what is worth noting for later.

- Tools/skills exposed to the agent mind: write_journal_entry and search_journal as the core pair; optionally read_journal (fetch a specific page/entry) and delete_from_journal (remove text) so the agent can curate.
- The only imposed rule is a hard size cap (characters/pages) enforced at write time — no guidance on how or when to use it. The point is to observe what usage behavior emerges.
- Journal mutations must be event-sourced like everything else: each write/delete lands as an event and is reducer-applied, so replay reproduces identical journals with no model calls (same invariant as AC #4).
- Journal content becomes retrievable context for the mind (search results fed into prompts), complementing the situated memories above.
<!-- SECTION:DESCRIPTION:END -->

Spec: specs/019-grounded-memories
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Memory payloads carry structured context — place, cause/intent reason when present, and refs to source events (e.g. conversation id) — and soul.md renders it
- [ ] #2 Deterministic executor memories are situated (where + why), not bare verbs like 'Built a fire.'
- [ ] #3 Conversation memories reference their transcript so what was said is retrievable from the memory, not just a gist
- [ ] #4 Replay from the event log reproduces identical souls; no model calls or out-of-band lookups
- [ ] #5 Each agent has a personal markdown journal with write_journal_entry and search_journal tools exposed to the mind (read_journal / delete_from_journal optional)
- [ ] #6 Journal is size-capped (fixed character/page budget) enforced at write time; no other usage rules imposed on the agent
- [ ] #7 Journal mutations are event-sourced and reducer-applied — replay reproduces identical journals with no model calls
- [ ] #8 Spec phase: Setup
- [ ] #9 Spec phase: Foundational (blocking US1, US2, US4)
- [ ] #10 Spec phase: User Story 1 — Situated deterministic memories (P1)
- [ ] #11 Spec phase: User Story 2 — Conversation memories reference their transcript (P1)
- [ ] #12 Spec phase: User Story 3 — Agent-authored journal (P2)
- [ ] #13 Spec phase: User Story 4 — Faithful replay of memories and journals (P1, integration proof)
- [ ] #14 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit run complete (specs/019-grounded-memories): spec (3 clarifications encoded 2026-07-22: reject-whole-write at cap, all four journal tools in scope, 4000-rune budget), plan, research R1–R11, data-model, contracts (memory-context, journal-tools), quickstart, tasks (23 tasks, 7 phases).
Layer 1: MemoryAddedPayload/Memory gain Where/Why/Conv (omitempty); Intent gains Reason populated by reducer from agent.intent_set; describePlace baked at emission; situated constructor variants + pinned text grammar; convo gist carries Conv keying social.conversation_turn transcript; scribe renders situated lines.
Layer 2: Agent.Journal (reducer-assigned stable ids), journal.entry_written/deleted on injectSocialWhitelist, budget 4000 runes enforced IN the reducer Apply arm so the InjectSocial dry-run rejects at the door (gate decides, not handler); 4 registry tools (2 Expressive + first 2 production Read) on LoopRosterVillager; scribe journal.md view; determinism suite extended (incl. pre-019 fixture).
Execution: one branch task-16-grounded-memories in .worktrees/task-16, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
--------------------------------------------------
2026-07-21: Journal tools decided as tool-driven, not prompt stuffing — search results enter context only when the agent calls search_journal. This requires an agent tool-use loop, which does not exist (llm.Orchestrator is single-shot). Created TASK-52 as the prerequisite; TASK-16 now depends on it.

2026-07-21: prereq chain extended — TASK-53 (tool registry) -> TASK-52 (tool loop) -> TASK-16 journal tools. Journal write tools are expressive-class registry entries; search/read are read-class (loop-dependent). One acting tool per cognition; read tools exempt.

Re-grounding 2026-07-22: line refs drifted — agent.thought emit is now loop.go:531/543 (was 371); conversation-turn emit convo.go:311 with ConversationTurnPayload carrying Text at sim/social.go:138 (was convo.go:135); MemoryAddedPayload now agents.go:246 (was 218); executor memory template literals live in executor.go (memoryEvent helper) — memory.go holds the constructor, not the strings. All mechanisms hold; premise unchanged.

2026-07-23: Spec 019 planning done and linked via spec-bridge (status derived In Progress). Tier decision (constitution V rubric): Opus 4.8 implementer — cross-package (sim/mind/tool/scribe/persona), doctrine-adjacent (injectSocialWhitelist admission, reducer-gate budget enforcement, InjectSocial door semantics, mind orchestration handlers). Rendering/tests ride the same slices; no Sonnet split worth the coordination cost.

2026-07-23: Implementation (Opus 4.8 spec-implementer) complete on task-16-grounded-memories — T002–T020, T022–T023 done across 4 commits (2c70234 Layer 1 + transcript refs, 4ea54a7 journal, 2adb42b replay proof + reconciliation, 2c1594a ticks). All touched packages green incl. extended determinism suite. Gate review: FILED-1 accepted (*Journal pointer — omitempty is a no-op on value structs; only shape satisfying FR-014), FILED-2 accepted (IntentSetPayload gains Reason emitted at planner landing; research R2's line refs were wrong). SC-001 scope gap NOT accepted: ~18 emission sites (gru, theft, near-death/witness-death, cold-night, debts, governance) left unsituated — follow-up slice T008b delegated to same implementer (situate all with Where, no fabricated Why, reconcile data-model §5 + research R2/R4). Pre-existing main breakage found during verification filed as TASK-62 (TestCatalogSweep / cog.tool_call digest drift — not this branch). Remaining: T008b, my full-suite gate, T021 live smoke, PR.

2026-07-23: T008b verified (18 sites situated; bare constructors removed — SC-001 now structural; full-day sweep test). Independent full-suite gate: green except pre-existing TASK-62 red. Live smoke (T021, throwaway world, 8x, ~35 game-min): 12/12 memory events carry where; situated soul lines live; [conv 516] transcript recovered from log alone (SC-002); journal.md views render (all empty — emergent, not a defect); restart survival intact (§6). Findings: (1) soul suffixes duplicate the situated text; (2) 'Built a fire at the fire' — describePlace names the just-built structure; (3) DESIGN: zero why lines — spec 017 removed the per-action reason, so no live path produces a reasoned intent; Why pipeline was live-dead. User clarified 2026-07-23: restore via OPTIONAL bounded reason param on acting world tools + set_plan (muse untouched). All three delegated as T024 to the Opus implementer.
<!-- SECTION:NOTES:END -->

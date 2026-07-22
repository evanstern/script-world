---
id: TASK-58
title: >-
  Hotfix: local planner replies unusable — constrain with JSON-schema structured
  outputs
status: Done
assignee: []
created_date: '2026-07-22 13:53'
updated_date: '2026-07-22 14:21'
labels: []
dependencies: []
priority: high
ordinal: 51000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
All 8 villagers' planner replies rejected in world-01 (daemon.log 09:43:08–09:44:00; cog.outcome events 8/8 unusable). Village running on reflex only.

**Diagnosis (complete, file:line):**
- Planner routes to TierLocal = cogito:3b via Ollama OpenAI-compat endpoint: internal/llm/llm.go:56.
- Request is unconstrained — no response_format, only model/messages/stream/max_tokens: internal/llm/providers.go:57-66.
- Strict parser (working as designed) rejects at the door: internal/mind/parse.go:294-355 (validGoals parse.go:38, planStepCap parse.go:36).
- No retry on parse failure; reply dropped, reflex covers: internal/mind/mind.go:381-386.
- Failure classes observed live AND reproduced 3/3 against the running Ollama with the real system prompt: (1) free-text goals ("chop some wood"), (2) plan over 3-step cap, (3) malformed JSON, (4) wrong types (kind as array, qty as string).
- Fix direction verified live: same prompt with response_format {type: json_schema} carrying the goal enum + plan maxItems 3 parsed clean 3/3.

**Fix:** add optional structured-output schema to llm.Request; openaiCompat.call attaches it as response_format json_schema; mind's planner jobs set a schema generated from validGoals/planStepCap (single source of truth). Cloud tier ignores it (parser remains the gate). Spec-exempt per constitution: surgical fix, file:line diagnosis above, ACs below.

**Tier: Opus 4.8** — rubric lines: cross-package change (internal/mind + internal/llm) and touches internal/llm.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Planner requests to the local tier carry response_format json_schema with the goal enum (from validGoals) and plan maxItems (from planStepCap); non-planner kinds unchanged
- [x] #2 Schema is generated from parse.go's validGoals/planStepCap, not a hand-copied duplicate
- [x] #3 Cloud (Anthropic) provider is unaffected by the new field; parseReply remains the final gate
- [x] #4 go build ./... and go test ./... pass
- [x] #5 Live verification: against running Ollama cogito:3b, constrained planner replies parse via parseReply (0 unusable in a repeated probe)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add optional structured-output field to llm.Request (internal/llm/llm.go); openaiCompat.call attaches it as response_format {type: json_schema} when set (internal/llm/providers.go); Anthropic provider ignores it.
2. In internal/mind, build the planner reply JSON schema programmatically from validGoals + planStepCap (parse.go) and set it on planner Submit calls (mind.go runPlan).
3. Tests: schema generation covers all validGoals + maxItems; provider payload includes response_format only when schema set.
4. Verify: go build/test ./...; live probe against Ollama cogito:3b — constrained replies must parse 3/3 via parseReply.
Implementation delegated to spec-implementer @ Opus 4.8 (rubric: cross-package, internal/llm) in .worktrees/task-58.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented by spec-implementer @ Opus 4.8 (rubric: cross-package internal/mind+internal/llm; touches internal/llm). Commit f6bd31a on task-58-planner-structured-outputs.

Deviation from brief, ratified by planning tier: top-level schema requires ["goal","reason"] instead of ["reason"] — reason-only shape left ~1/3 replies schema-valid but unusable ({"reason"} with no goal/plan); anyOf goal-xor-plan rejected empirically (llama.cpp grammar converter bails on anyOf, applying NO constraint). Requiring goal is safe: parseReply prefers a present plan and discards top-level goal (parse.go:322). Also added maxLength bounds (reason 200, target 80) so prose can't overrun the 256-token budget into truncated JSON.

Verification: go build/test ./... all ok (17 pkgs, incl. e2e); live probe vs Ollama cogito:3b through the real provider + parseReply: 0 unusable / 30. Orchestrator re-ran build + llm/mind tests in the worktree: pass.

Owed post-merge: /grounding-wiki:wiki-update (parse.go, llm.go, providers.go are pinned wiki sources); daemon restart to pick up the fix.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Merged in PR #35 (de1ef19). Local planner replies now sampler-constrained via response_format json_schema generated from validGoals/validKinds/planStepCap; live probe 0 unusable / 30 (was 8/8 live). Wiki re-pinned (agent-mind, llm-orchestrator) to de1ef19. Implemented by spec-implementer @ Opus 4.8 per rubric (cross-package, internal/llm). Remaining operational note: running daemons pick up the fix on restart.
<!-- SECTION:FINAL_SUMMARY:END -->

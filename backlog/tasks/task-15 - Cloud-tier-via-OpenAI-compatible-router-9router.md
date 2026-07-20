---
id: TASK-15
title: Cloud tier via OpenAI-compatible router (9router)
status: Done
assignee: []
created_date: '2026-07-19 15:33'
updated_date: '2026-07-19 16:08'
labels: []
dependencies: []
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Allow the cloud LLM tier to target an OpenAI-compatible router (the operator's local 9router at http://192.168.1.92:20128/v1, model cc/claude-haiku-4-5-20251001) instead of the Anthropic API directly. Adds cloud provider selection, inline api_key for local routers, and stream:false hardening.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 cloud.provider=openai_compat routes cloud calls through the chat-completions caller with Bearer auth
- [x] #2 openai-compatible requests pin stream:false (9router streams by default)
- [x] #3 anthropic remains the default provider; keys still never required in llm.json (api_key optional, local-router use only)
- [x] #4 village03 smoke test: a cloud-tier call answers via 9router
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-15-cloud-router off task-8-social-fabric (village03 runs the task-8 feature set; PR against main opens after PR #7 merges)
2. CloudConfig: provider field (anthropic|openai_compat) + optional api_key; openaiCompat: Bearer auth + stream:false
3. Tests: provider selection, auth header, stream pin
4. Point village03 cloud tier at 9router (pricing 0/0 — subscription-routed), restart, smoke test
5. Wiki: refresh the LLM orchestrator note
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-15-cloud-router (stacked on task-8-social-fabric; PR against main opens after PR #7 merges — do NOT open it stacked). Code commit 2a1608f, wiki re-pin e349924. Live smoke test 2026-07-19: village03 cloud tier answered via 9router (cc/claude-haiku-4-5-20251001, 1251ms, $0.0000 — pricing set 0/0 since the router rides the operator's subscription). Race suite green.

PR opened against main after PR #7 merged.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Cloud tier is provider-selectable: anthropic (default) or openai_compat for LAN routers (9router). Inline api_key for local-router credentials only; stream:false pinned; validated at LoadConfig. Live-verified via village03 through 9router (cc/claude-haiku-4-5-20251001, 1251ms). Merged to main as PR #8.
<!-- SECTION:FINAL_SUMMARY:END -->

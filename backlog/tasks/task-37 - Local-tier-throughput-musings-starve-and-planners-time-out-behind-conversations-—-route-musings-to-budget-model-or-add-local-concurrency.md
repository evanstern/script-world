---
id: TASK-37
title: >-
  Local openai_compat caller: disable hidden thinking (per-tier reasoning_effort
  config) and pass max_tokens through
status: Done
assignee: []
created_date: '2026-07-21 03:54'
updated_date: '2026-07-21 13:02'
labels: []
dependencies: []
ordinal: 31000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Observed in live world ~/worlds/myworld (daemon from .worktrees/task-34, 2026-07-20):

- 14 of 17 cog.outcome events unusable: 13 musings dropped with "tier busy; best-effort call dropped", 1 planner "context deadline exceeded" (queue starvation behind conversation-630).
- conversation-630 (13 pts, predicted 246s) monopolized the single local worker for >5.5 real minutes; convoBusy disables the musing fairness floor (internal/mind/mind.go:501) for its whole duration, so zero musings landed after tick 630.
- Bootstrap calibration (local 20s/pt) is 3-4x optimistic on this deployment: landed musing (1pt) took 72s; planners (3pt) 44-48s. No calibration profile was present ("run scriptworld calibrate").
- Structural: 8 villagers x (3pt planners + 1pt musing cadence) + 13pt conversations exceeds one serialized gemma4:12b-mlx worker.

Candidate directions (evaluate, don't presuppose):
1. Route KindMusing to a budget local model (e.g. cogito:3b) as a second local tier (routing map at internal/llm/llm.go:52).
2. Allow >1 in-flight local call where the host can batch.
3. Revisit convoBusy standing down the musing fairness floor for the entire scene duration.

Blocked on: diagnosis of why conversation-630 ran so long (separate investigation, this session).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 openaiCompat.call sends max_tokens from Request.MaxTokens when > 0
- [x] #2 Per-tier optional reasoning_effort in llm.json: local defaults to "none" when unset; cloud omits when unset; explicit "" omits the field
- [x] #3 Unit tests cover the outbound request body for defaults and overrides on both tiers
- [x] #4 Live verification on myworld: with the fixed binary and a calibration profile, cog.outcome shows musings, planners, and a conversation landing (no tier-busy wall)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add optional per-tier `reasoning_effort` (*string) to llm.json config: local unset → default "none"; cloud unset → omit field; explicit "" → omit.
2. openaiCompat.call: include reasoning_effort when non-empty and max_tokens when Request.MaxTokens > 0.
3. Unit tests on request-body shape (defaults + overrides, both tiers).
4. Live verify on rebuilt myworld: new binary, scriptworld calibrate, restart daemon, confirm cog.outcome landings across musing/planner/conversation.
Structural candidates (budget-model routing, local concurrency, convoBusy floor) stay DEFERRED pending re-measurement after this fix — see notes.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
DIAGNOSIS (2026-07-20 session, live probes against the running world):

Root cause is NOT raw model slowness. Chain:

1. `openaiCompat.call` (internal/llm/providers.go:53-59) sends only model/messages/stream — Request.MaxTokens is silently dropped for the local tier, and no thinking control is sent.
2. gemma4 on Ollama defaults to thinking=ON. Every call free-runs a hidden `reasoning` stream before the ~7-128 tokens of actual content. Probe evidence: native /api/chat with think:false → eval 7 tok in 337ms (~21 tok/s, hardware healthy); same tiny prompt with thinking on → 60-120s per call (matches ollama server.log GIN lines: 1m0s-2m0s per request).
3. Inflated per-call latency (3-6x the 20s/pt bootstrap seed) saturates the single-slot Ollama + single local worker: all 13 musings refused (best-effort admission), planners exceed the 120s worker cap / caller ctx ("context deadline exceeded" for Sage, Rowan), and conversation-630 completed all 4 utterance turns but blew the 10-min convoDeadline on the outcome call → all-or-nothing discarded the whole scene (outcome unusable, actual_wall_ms=600000, staleness 2628).
4. No calibration profile masked it: predicted_wall_ms was fantasy throughout.

Fix knob verified: OpenAI-compat endpoint IGNORES `think:false` but HONORS `reasoning_effort:"none"` → clean content, zero reasoning, sub-second generation. Fix should also pass MaxTokens through as max_tokens.

Hardware exonerated: M1 Max 64GB, 74% free, model 100% GPU.

IMPLEMENTATION (branch task-37-reasoning-effort-max-tokens, commit 8b8e058, via spec-implementer):
- config.go: per-tier ReasoningEffort *string + resolveReasoningEffort helper (local nil→"none", cloud nil→omit, explicit ""→omit).
- providers.go: openaiCompat carries resolved reasoningEffort; call() now sends max_tokens (when >0) and reasoning_effort (when non-empty).
- New providers_test.go: 7 assertions on outbound body shape — all pass; full go test ./... green (e2e included).

LIVE VERIFY (AC #4) in progress, world ~/worlds/task37-verify (user's myworld was deleted mid-session):
- First calibration run CONTAMINATED: 30-64s/sample — traced to two zombie daemons hogging Ollama's single slot (user's orphaned myworld daemon on the old thinking-on binary + a leaked e2e test daemon). e2e daemon stopped via CLI; orphaned daemon (pid 69387) needs a manual kill — sandbox blocks the kill syscall. Re-calibration pending a quiet Ollama.

LIVE VERIFY COMPLETE (world task37-verify, fixed binary, calibrated profile):
- Calibration: 30.9 s/pt (contaminated/thinking-on) → 0.9 s/pt. All cognition classes admissible at 32x (before: planners suppressed above 8x).
- Outcomes over the run: 5/5 musings landed (avg 1.9s; previously 13/13 dropped), 12/12 planners landed (avg 2.8s; previously deadline timeouts), 1 conversation landed in 10.5s (previously discarded at the 600s convoDeadline). 4 planner rejected-guard outcomes are semantic (target moved), not latency.
- Wiki: llm-orchestrator re-verified + re-pinned to 8b8e058; freshness gate green (29/29).
- PR: https://github.com/evanstern/script-world/pull/23 (branch task-37-reasoning-effort-max-tokens, commits 8b8e058 + 5c05bb9).
Remaining: merge PR, then worktree cleanup + Done. Operator note: rebuild the daemon binary used for live worlds (currently launched from projects/.../.worktrees/task-34) from main after merge, and re-create worlds' llm.json or leave absent — local reasoning_effort defaults to "none" automatically.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Root cause of all-unusable thoughts fixed: openai_compat caller now sends per-tier reasoning_effort (local defaults "none"; explicit "" opts out; cloud omits unless set) and passes max_tokens. Live verification: 30.9 → 0.9 s/pt calibrated, 5/5 musings + 12/12 planners landed, conversation landed in 10.5s vs prior 600s-deadline discard, all cognition classes admissible at 32x. Wiki llm-orchestrator re-pinned; merged as PR #23 (0264eb3). Structural follow-ups (budget-model musing routing, local concurrency, convoBusy floor) deferred in notes pending post-fix data.
<!-- SECTION:FINAL_SUMMARY:END -->

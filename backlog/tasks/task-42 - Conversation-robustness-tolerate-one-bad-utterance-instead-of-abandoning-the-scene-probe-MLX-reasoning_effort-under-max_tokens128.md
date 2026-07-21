---
id: TASK-42
title: >-
  Conversation robustness: tolerate one bad utterance AND one bad outcome
  instead of abandoning the scene; probe MLX reasoning_effort under
  max_tokens=128
status: Done
assignee: []
created_date: '2026-07-21 13:47'
updated_date: '2026-07-21 18:55'
labels:
  - robustness
dependencies: []
priority: high
ordinal: 36000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From TASK-39: mind/convo.go:196-199 abandons the entire scene on any utterance error (all-or-nothing, TASK-8-era, never had a retry) — one blank say from a starved model kills a 13-point conversation. Add a single retry or skip-turn tolerance. Separately probe whether the MLX endpoint honors reasoning_effort:none now that TASK-37 enforces max_tokens=128 on utterance calls — a thinking model spending its 128 tokens on hidden CoT returns empty say every time (possible aggravation, endpoint-dependent).

WIDENED 2026-07-21 (Opus investigation of "conversation 48896 outcome failed: bad outcome JSON: invalid character 'H'", myworld-01): the scene has a SECOND all-or-nothing failure site — the outcome call at convo.go:204-210 → parseOutcome (parse.go:101). The local model sometimes emits the gist as an unquoted JSON value starting with a participant's name ({"gist": Hazel and Rowan talked...}); firstJSON finds balanced braces, Unmarshal fails, and the whole completed scene is discarded: every conversation_turn, the social.conversation record, all gist memories, relation deltas, and any staged rumor transfer — after the transcript was fully generated (~75s local compute wasted). Measured loss on myworld-01: 2 of 11 conversations (~18%). Pre-dates parallel-4; cloud tier not in path (KindConversation → TierLocal, llm.go:57); MaxTokens is 224 here so the 128-token starvation hypothesis applies only to the utterance site. This task now covers BOTH sites: one bad utterance and one bad outcome must each be tolerable without losing the scene.

Spec: specs/011-conversation-robustness
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Utterance site (convo.go:196-199): one failed/unusable say is retried once or skipped without abandoning the scene; two consecutive failures may still abandon
- [x] #2 Outcome site (convo.go:204-210): a parse-failed outcome is retried once before giving up; scene state (turns, memories, relation deltas, rumor) is not lost when the retry succeeds
- [x] #3 On any outcome/utterance parse failure the raw model reply is persisted (event payload or log) so failures are inspectable after the fact
- [x] #4 Outcome prompt hardened (gist MUST be a quoted string or equivalent); optionally lenient field extraction in parse.go shared with parseSay/parseConsolidation
- [x] #5 MLX reasoning_effort:none probe under max_tokens=128 completed and findings recorded on this task
- [x] #6 Spec phase: Setup
- [x] #7 Spec phase: Foundational (blocking prerequisites for all stories)
- [x] #8 Spec phase: User Story 1 — A completed scene survives one bad summary reply (P1)
- [x] #9 Spec phase: User Story 2 — A scene survives one bad utterance (P2)
- [x] #10 Spec phase: User Story 3 — Parse failures are inspectable (P3)
- [x] #11 Spec phase: User Story 4 — Fewer malformed summaries (P4)
- [x] #12 Spec phase: User Story 5 — MLX reasoning_effort probe (P5)
- [x] #13 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Opus investigation (2026-07-21, myworld-01) of 'conversation 48896 outcome failed: bad outcome JSON: invalid character H': the OUTCOME call (convo.go:204-210 → parseOutcome, parse.go:101) is a sibling failure site to the utterance abandonment this task targets. Local gemma4:12b-mlx emits the gist as an UNQUOTED JSON value starting with a participant's name ({"gist": Hazel and Rowan talked...}) — firstJSON finds balanced braces, Unmarshal fails, whole scene discarded (all turns, memories, relation deltas, rumor transfer; ~75s of local compute). 2 of 11 conversations (~18%) on this world died at this site; first occurrence PRE-dates the parallel-4 restart, cloud tier not in path (KindConversation → TierLocal, llm.go:57). MaxTokens here is 224, so the 128-token starvation hypothesis doesn't apply to this site. Recommended: (1) one bounded retry of the outcome call on parse failure; (2) persist resp.Text on parse failure (currently unrecoverable — this investigation had to infer the payload from participant-name correlation with conv 26430's 'invalid character F'); (3) prompt nudge 'gist MUST be a quoted string'; optionally lenient field extraction in parse.go (also helps parseSay/parseConsolidation). Scope decision pending: widen this task to cover the outcome site, or file a sibling.

Scope decision resolved by user 2026-07-21: TASK-42 widened to cover the outcome site (was: pending widen-vs-sibling).

Implementation tier: Opus 4.8 (constitution V rubric: internal/mind orchestration; doctrine-adjacent all-or-nothing landing semantics; prior-era site shipped live defects). Spec 011 planned on Fable 5; 20 tasks, MVP = US1 outcome retry.

Implementation complete on branch task-42-conversation-robustness; PR #29 open (https://github.com/evanstern/script-world/pull/29). Opus implementer: 5 commits, 20/20 quickstart-gate tests, full suite green; per-scene utterance budget ruling (FR-002/FR-007) applied after gate review. Awaiting review/merge; post-merge: wiki-update (T019) + spec-bridge sync (T020), then Done.

MLX reasoning_effort probe findings (FR-008/US5, AC #5; run live against localhost:11434 gemma4:12b-mlx, utterance-shaped requests, max_tokens=128, N=10 per config): reasoning_effort IS honored, and the empty-utterance hypothesis is CONFIRMED — unset: 10/10 empty replies (median content 0); low: 10/10 empty; none: 0/10 empty, median 62 chars. A thinking model spends the entire 128-token budget on hidden CoT unless effort is 'none'. The orchestrator's existing local-tier default resolveReasoningEffort(..., "none") (llm.go:186) is therefore both necessary and sufficient — load-bearing, do not remove. Probe script: specs/011-conversation-robustness/probe-mlx-reasoning.sh. (Re-applied to main: original note commit landed on the task branch by mistake and was dropped in the rebase.)

spec-bridge sync: all 8 phases 20/20 tasks done; PR #29 merged (b6f2378); wiki re-pinned (cognition, agent-mind, social-fabric). Derivation: Done-eligible.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Both all-or-nothing failure sites in the conversation scene runner now tolerate one bad reply per site (outcome retry + per-scene utterance retry, retry-not-skip), with lenient unquoted-gist recovery (zero extra calls), hardened outcome prompt, raw failed-reply persistence (bounded) + retried telemetry for measurable recovery rates, and doctrine intact (transport errors never retried, stale-at-landing enforced, happy path byte-identical, golden-tested). MLX probe recorded: reasoning_effort honored; 'none' is load-bearing (unset/low = 100% empty at 128 tokens). Delivered via PR #29, merged as b6f2378; Opus 4.8 implementer, 20/20 quickstart-gate tests, full suite green.
<!-- SECTION:FINAL_SUMMARY:END -->

---
id: TASK-42
title: >-
  Conversation robustness: tolerate one bad utterance AND one bad outcome
  instead of abandoning the scene; probe MLX reasoning_effort under
  max_tokens=128
status: To Do
assignee: []
created_date: '2026-07-21 13:47'
updated_date: '2026-07-21 17:28'
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
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Utterance site (convo.go:196-199): one failed/unusable say is retried once or skipped without abandoning the scene; two consecutive failures may still abandon
- [ ] #2 Outcome site (convo.go:204-210): a parse-failed outcome is retried once before giving up; scene state (turns, memories, relation deltas, rumor) is not lost when the retry succeeds
- [ ] #3 On any outcome/utterance parse failure the raw model reply is persisted (event payload or log) so failures are inspectable after the fact
- [ ] #4 Outcome prompt hardened (gist MUST be a quoted string or equivalent); optionally lenient field extraction in parse.go shared with parseSay/parseConsolidation
- [ ] #5 MLX reasoning_effort:none probe under max_tokens=128 completed and findings recorded on this task
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Opus investigation (2026-07-21, myworld-01) of 'conversation 48896 outcome failed: bad outcome JSON: invalid character H': the OUTCOME call (convo.go:204-210 → parseOutcome, parse.go:101) is a sibling failure site to the utterance abandonment this task targets. Local gemma4:12b-mlx emits the gist as an UNQUOTED JSON value starting with a participant's name ({"gist": Hazel and Rowan talked...}) — firstJSON finds balanced braces, Unmarshal fails, whole scene discarded (all turns, memories, relation deltas, rumor transfer; ~75s of local compute). 2 of 11 conversations (~18%) on this world died at this site; first occurrence PRE-dates the parallel-4 restart, cloud tier not in path (KindConversation → TierLocal, llm.go:57). MaxTokens here is 224, so the 128-token starvation hypothesis doesn't apply to this site. Recommended: (1) one bounded retry of the outcome call on parse failure; (2) persist resp.Text on parse failure (currently unrecoverable — this investigation had to infer the payload from participant-name correlation with conv 26430's 'invalid character F'); (3) prompt nudge 'gist MUST be a quoted string'; optionally lenient field extraction in parse.go (also helps parseSay/parseConsolidation). Scope decision pending: widen this task to cover the outcome site, or file a sibling.

Scope decision resolved by user 2026-07-21: TASK-42 widened to cover the outcome site (was: pending widen-vs-sibling).
<!-- SECTION:NOTES:END -->

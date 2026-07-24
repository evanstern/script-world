---
id: TASK-68
title: 'Curriculum ladder: learning topics gated to angel capabilities'
status: To Do
assignee: []
created_date: '2026-07-23 03:28'
updated_date: '2026-07-24 02:42'
labels:
  - review-2026-07-22
  - teaching-game
dependencies:
  - TASK-64
priority: medium
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (new-ideas item 6), SHAPED by the client's stated progression (2026-07-22): "here's Metatron with base settings, you can learn to prompt him like he's Claude/ChatGPT, he has some basic tools" -> "now you learn to edit his instructions (a CLAUDE.md/SKILL.md topic)" -> "now you pick additional things your angel can do in the world (tools)". A learning-topic gate-to-feature pathway: completing a topic unlocks the next capability tier.

Scope: (a) Curriculum design artifact: the ladder of stages, each stage naming the prompt-engineering concept taught (conversational prompting; instruction-file authoring; capability/tool design; indirect influence and prompt-injection awareness — which the Metatron fiction already teaches natively), the world features it requires, and its pass signal. (b) Stage presets: world templates/configs per stage — stage 1 world grants base tools only; stage 2 enables charter editing; stage 3 opens the tool manifest (all substrate from TASK-64). Seeded scenario worlds as exercises ("survive the first night", "get your law passed") using existing systems (needs, gru, norms/votes, secrets) as lesson material, with the chronicle as the score narrative. (c) Gating mechanism: how a stage unlocks — self-serve manual unlock vs artifact-gated (this repo's educate plugin already models topic lifecycles and progress gates; evaluate reusing its shape for player-facing lessons vs a simpler in-game unlock file). Keep v1 gating simple: a per-world stage field in config that the capability manifest reads.

Depends on TASK-64 (instruction surface + tool gating is the substrate). The horizon decision (TASK-66) informs per-stage speed posture.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Curriculum ladder artifact exists: stages, concept taught per stage, required features, pass signal per stage
- [ ] #2 Per-stage world presets exist and are creatable (new --stage or template worlds)
- [ ] #3 Stage gating mechanism decided and implemented (capability manifest honors the world stage)
- [ ] #4 At least two seeded scenario exercises defined with their score-narrative framing
- [ ] #5 Reviewed with the client against their three-stage progression before implementation
<!-- AC:END -->

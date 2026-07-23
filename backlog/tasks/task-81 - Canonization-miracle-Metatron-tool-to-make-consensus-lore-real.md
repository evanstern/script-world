---
id: TASK-81
title: 'Canonization miracle: Metatron tool to make consensus lore real'
status: To Do
assignee: []
created_date: '2026-07-23 17:50'
labels:
  - emergent-lore
  - metatron
  - design-session
dependencies:
  - TASK-79
priority: medium
ordinal: 74000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The "yes, and" answer to emergent mythology (2026-07-23 world-01 Thornspire investigation): the villagers collectively invented Thornspire at the forest's edge in response to the player's own omen (seq 50664: "a great arc of color... pointing toward the forest's edge... something is being shown to them" — and there is nothing there). Instead of only letting reality debunk myths (TASK-80), give the god the power to answer them: a Metatron miracle that instantiates consensus lore into world state. Converts pantomime into quest — the villagers dream, the player decides which dreams become geography.

Scope questions for the spec:
- Toponymy: the map has no named places today. Introduce named regions as world state (e.g. christen the forest-edge area "Thornspire") so situated memories, describePlace, and the chronicle can use villager-coined names — this alone grounds a huge share of the lore vocabulary.
- Instantiation: what can be made real — existing structure/feature kinds vs. new ones (a grove, standing stones, "tendrils" as a flora feature). Start minimal: name-a-region + place-one-feature may be enough.
- Perception of the change: how villagers learn the myth came true — discovery on arrival (pairs directly with TASK-80's grounded observation channel: the next visit CONFIRMS instead of disconfirming) and/or an omen announcing it.
- Economy: charge cost per Metatron miracle doctrine (metatron.nudged / charge machinery); a canonization is a big act — price it accordingly.
- Doctrine: lands as an event through the normal door, replay-deterministic; charter language for how Metatron counsels the player about when canonization serves the village.
- Player surface: how the player sees candidate lore worth canonizing (the rumor/belief corpus already carries it — 271 Thornspire events; maybe Metatron briefs on dominant myths).

Sequencing per plan of record: after TASK-79; runs in parallel with TASK-80, and its perception story composes with TASK-80's observation channel.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec Kit spec produced and linked via spec-bridge before implementation (constitution rigor)
- [ ] #2 Named regions exist as replay-deterministic world state and flow into situated-memory place descriptions and the chronicle
- [ ] #3 A Metatron miracle canonizes a named region and/or places a real feature, charged per doctrine, landing as events through the normal door
- [ ] #4 Villagers perceive the canonization in-world (arrival discovery and/or omen); demonstrated live on world-01's Thornspire as the acceptance scenario
- [ ] #5 Metatron can brief the player on dominant village myths as canonization candidates
<!-- AC:END -->

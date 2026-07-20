---
id: TASK-11
title: 'Chronicle: event narration and the story feed'
status: In Progress
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-20 03:07'
labels:
  - narrative
  - llm
dependencies:
  - TASK-8
ordinal: 11000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The narrator pipeline: compress sim/social events into a readable chronicle (cloud tier), with per-agent and per-thread filters; the catch-up mechanism for an ambient world; renders in the chronicle pane. Grounding: grounded-assumptions.md (Time & posture, Terminal UI).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A running world produces a live chronicle readable in the TUI
- [ ] #2 Filters by agent and by thread work
- [ ] #3 A returning player can catch up on days away from the chronicle alone
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Grounding: the chronicle is THE catch-up mechanism for an ambient world (grounded-assumptions: Time & posture); narrator rides the cloud tier (llm.KindNarrator already routed, TASK-6); TUI chronicle pane is a raw event feed today (views.go). Model output enters the sim only as recorded injected events (inject_social whitelist) — the chronicle follows the same door.

Design:
1. New event chronicle.entry — ChronicleEntryPayload{day, from_tick, to_tick, text, thread, agents[]}. Reducer appends a bounded ring State.Chronicle (cap 256). The ring rides snapshots/state fetch, so ANY attaching client gets narrated history for free — that ring IS the catch-up. Whitelisted in inject_social; unknown-type no-op keeps old replay code compatible.
2. Narrator driver (internal/mind/narrate.go): absorb() collects notable events as pre-named factual lines (deaths, gru emergence/attacks/sightings, conversations with gist+topics+tones, rumors told, promises broken, builds, sampled musings) with in-world timestamps. On day/night boundaries (sim.night_started closes the day chapter, sim.day_started closes the night chapter) the buffer snapshots into a chapter job for a single-flight cloud worker. Model returns strict JSON: 1-3 entries {text, thread slug, agents by name}; validated (names -> indices vs real roster, length/count caps), injected as ONE atomic batch. Transport failure keeps the buffer for retry next boundary (capped, oldest dropped); unparseable output drops the chapter — a gap, never a stall. No llm.json -> no narrator.
3. Scribe also renders chronicle.md in the world dir from the ring — the offline catch-up artifact.
4. TUI chronicle pane: narrated entries from the replica ring, filters: 'a' cycles agent, 't' cycles thread (observed slugs), 'r' toggles the raw event feed (also the automatic fallback when a world has no narrator). Active filters shown in the pane.
Cost: ~2 chapters/game-day on the cloud tier — noise against the $100 ceiling; spend meter guards anyway.
Tests: reducer ring+payload; collector windowing; parse/validate rejections; atomic injection; TUI filters+fallback; full suite -race. Live proof: real world at speed for 2+ game days, then attach fresh and catch up from the chronicle pane alone.
<!-- SECTION:PLAN:END -->

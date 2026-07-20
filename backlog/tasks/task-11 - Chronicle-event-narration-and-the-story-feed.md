---
id: TASK-11
title: 'Chronicle: event narration and the story feed'
status: Done
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-20 04:37'
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
- [x] #1 A running world produces a live chronicle readable in the TUI
- [x] #2 Filters by agent and by thread work
- [x] #3 A returning player can catch up on days away from the chronicle alone
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation landed on task-11-chronicle (61c8850): chronicle.entry + State.Chronicle ring (snapshot-carried catch-up), narrator driver in internal/mind/narrate.go (chapter per day/night boundary, single-flight cloud worker, carry-on-failure, atomic injection), scribe chronicle.md, TUI narrated pane with a/t/r filters. Full suite green with -race, including a live-loop integration test (musings as notable lines -> boundary -> chronicle.entry in store + ring on replay).

Live proof started: world chronicle-proof (seed 11) in session scratchpad, speed 32x. Local tier deliberately dead (TASK-24: operator's gru-test-01 daemon owns the shared Ollama) -> reflex world; cloud tier live via 9router (cc/claude-haiku-4-5-20251001, narrator smoke call 960ms). Executor drama (fires, gru, deaths) feeds the narrator; first chapter lands at the day-1 night boundary.

Live proof (chronicle-proof, seed 11, 32x, gemma4:12b-mlx local + 9router cloud cc/claude-haiku-4-5): two chapters landed — 'day 1, dawn to nightfall' (3 entries) and 'the night after day 1' (2 entries, incl. the gru's prowl from real gru.* events). Narrator reused thread slugs across chapters (winter-preparation, distant-mysteries) exactly as the filter needs. AC#1: live TUI (tmux-driven real binary) renders the narrated chronicle pane from the replica ring. AC#2: 'a' filter (agent Ash -> only Ash entries), 't' filter (winter-preparation -> only that thread), 'r' raw-feed toggle — all verified against the running world. Ring survives daemon restart (deleted chronicle.md regenerated at startup from recovered state — the snapshot path a returning client rides). AC#3 pending: world runs on to accumulate day 2's chapter, then a cold attach must read multiple days from the chronicle alone.

AC#3 live-proven: day-2 chapter landed at nightfall (3 entries; all three threads — winter-preparation, warmth-and-flattery, distant-mysteries — continued across days; rumor spread about Rowan and Hazel narrated from real social events). Cold attach (fresh tmux TUI, no watched history): chronicle pane shows all 8 entries spanning day 1, the night after day 1, and day 2 — multi-day catch-up from the snapshot-carried ring alone. chronicle.md holds the same story offline.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
The chronicle narrator landed: chronicle.entry events + a snapshot-carried State.Chronicle ring (cap 256) make narrated history part of world state — any attaching client catches up for free, and chronicle.md is the offline artifact. The narrator driver collects notable events as named log lines, closes a chapter at each day/night boundary, and lands validated 1-3-entry chapters (text, stable thread slug, cast) through the atomic inject_social door on the cloud tier (~2 calls/game-day). TUI chronicle pane renders the story with agent/thread filters and a raw-feed fallback. All three ACs live-proven on chronicle-proof (32x, gemma + 9router): chapters at real boundaries, threads continued across days, filters verified against the running world, cold attach read 8 entries spanning 2+ days from the ring alone, ring survived restart. Failure honesty: transport failures carry lines forward, bad output is a gap never a stall, quiet chapters spend nothing. Wiki re-grounded (new chronicle note + 6 re-verified). PR: https://github.com/evanstern/script-world/pull/14
<!-- SECTION:FINAL_SUMMARY:END -->

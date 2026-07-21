---
id: TASK-36
title: >-
  Remove hard-coded 11:30 village meeting: convention must be emergent or
  per-world config
status: Done
assignee: []
created_date: '2026-07-21 02:32'
updated_date: '2026-07-21 03:26'
labels:
  - engine
  - sim
  - governance
dependencies: []
references:
  - internal/sim/governance.go
  - internal/world/world.go
  - internal/mind/prompt.go
priority: high
ordinal: 30000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The daily village meeting is hard-coded into the executor: `internal/sim/governance.go:23` pins `meetingConveneSecond = 11*3600+1800` (11:30) and `governanceEvents` (governance.go:407-433) auto-convenes every living villager once per day at that instant, deriving a meeting place on the spot. Villagers magically "know" when and where to meet with no in-world reason — the knowledge is baked into the engine, not the world.

This must go. The meeting convention (that there IS a meeting, when, and where) should exist only when it has a source:

- **Emergent (primary):** villagers don't know where to meet yet — they wander. The convention gets established in-world (e.g., an agent calls a gathering, a place/time becomes convention once a meeting actually happens) rather than being a clock-driven engine beat.
- **Per-world config:** a world's settings/notes (world.json manifest or per-game notes) may declare a meeting convention explicitly; the lifecycle honors it. Absent both, no meeting convenes and governance stays dormant.

Constraints: the rest of the TASK-13 governance machinery (convene → open → speaking turns → votes → norms) stays intact once a meeting does happen; outcome-bearing logic stays deterministic per the executor doctrine (the model never decides outcomes). Fresh default worlds must show NO 11:30 break-routine behavior.

Related: TASK-13 (norms and votes — introduced the hard-coded lifecycle), TASK-23 (interaction v2 design), TASK-12/27 (Metatron — a plausible channel for seeding a convention per-game).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 No hard-coded convene/open clock time remains in the engine; a fresh default world runs a full day with villagers following needs/wandering and no meeting convenes
- [x] #2 A per-world setting (world manifest or per-game config/notes) can declare a meeting convention (time/place), and the existing convene→open→close lifecycle honors it
- [x] #3 An emergent in-world path can establish the meeting convention without any config (design chosen and implemented; deterministic outcome-bearing logic preserved)
- [x] #4 Existing governance lifecycle tests updated to run via an established convention rather than the hard-coded 11:30; -race suite green
- [x] #5 docs/wiki re-pinned for touched sources (wiki-update run)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Design: the meeting convention becomes event-sourced state, never an engine constant. stepEvents is a pure fn of (state,map,tick), so config/emergence enter via a new event.

1. New event meeting.convention_established {convene_second, open_second, x, y, source: config|emergent} reduced into State (e.g. s.MeetingConvention *MeetingConvention; also sets MeetingPlace). Zero/nil = no convention.
2. governanceEvents: delete meetingConveneSecond/meetingOpenSecond consts; convene/open only when s.MeetingConvention != nil and sod matches it. No convention -> governance dormant, villagers untouched.
3. Per-world config: world.json Manifest gains optional meeting block (convene/open as HH:MM or seconds, optional place). On daemon boot, if manifest declares it and state has no convention, inject the establishment event (source config) at the next tick boundary via the existing injection door. scriptworld new writes NO meeting block — emergent is the default.
4. Emergent path (deterministic, doctrine-safe — model decides nothing): a gathering detector in stepEvents: while no convention exists, if >= minVillagersToMeet awake non-exiled villagers sit within meetingRadius of a fire/shelter continuously for a sustained window during daytime, emit convention_established (source emergent; place = that structure, time = the observed gathering hour). Pure fn of state; one-shot.
5. Surface text: mind/prompt.go (daily noon meeting lines), mind/narrate.go (Noon came and went...), scribe.go (assembles daily at noon) — all render from the convention or say nothing when none exists.
6. Tests: governance/meeting tests establish a convention explicitly instead of relying on 11:30; new tests — (a) fresh default world, full day, zero meeting.* events; (b) manifest-declared convention convenes/opens; (c) emergent gathering establishes convention and next-day lifecycle runs. -race suite green.
7. Wiki-update (re-pin governance/world notes), PR from .worktrees/task-36.

Implementation delegated to spec-implementer (Opus) per constitution Principle V; this session reviews, gates, and closes out.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Process decision (2026-07-21): this pass runs off the board-recorded plan (no Spec Kit dir) per operator call; a follow-up pass, if any, gets the strict speckit-specify → plan → tasks → spec-bridge:link flow.

Implemented on branch task-36-emergent-meeting-convention (3 commits: sim engine / world+daemon config / mind+scribe surface). Full go test -race ./... green.

Design shipped:
- Event meeting.convention_established {convene_second,open_second,x,y,source(config|emergent)} -> State.MeetingConvention *MeetingConvention{ConveneSecond,OpenSecond,Source,EstablishedDay}; one-shot reducer, also seeds MeetingPlace. meeting.place_designated kept for old-save replay compat.
- governanceEvents convenes/opens only when MeetingConvention!=nil and sod==convene/open; no convention => dormant. Violation detectors still run (upgraded saves may carry a law without a convention).
- Emergent: sim.gathering_observed events (NOT meeting.*) track a sustained daytime quorum at one fire/shelter in MeetingState (GatherStart/X/Y), reducer-advanced; 30 game-min sustained -> emergent convention (place=structure, convene=observed half-hour). Deterministic, replay-safe.
- Config: world.json manifest optional meeting{convene:HH:MM, open:HH:MM, x?, y?}; validated on world.Open; scriptworld new writes none. daemon.seedMeetingConvention injects the establish event on boot (idempotent via one-shot guard + log).

AC proof (test names):
- AC#1: sim TestFreshDefaultDayNoMeeting (full default day, zero meeting.* events, no convention).
- AC#2: world TestMeetingConfigSeconds/TestOpenRejectsBadMeeting/TestOpenAcceptsMeeting + daemon TestSeedMeetingConventionConfig/Absent/NoCoords + sim TestMeetingLifecycleFullDay (lifecycle honors an established convention at its times).
- AC#3: sim TestEmergentConventionFromGathering (+ next-day lifecycle) and TestEmergentGatheringResets.
- AC#4: existing lifecycle tests migrated via establishConvention helper; TestGovernanceReplay carries convention+meeting events in the log and reduces to identical state; TestGovernedDeterminism green with -race.

Replay/compat note for orchestrator: pre-TASK-36 saves load MeetingConvention=nil, so they stop auto-convening (intended) but retain any existing norms and their enforcement (violation detectors run regardless of convention). AC#5 (wiki re-pin) intentionally left for the orchestrator.

Wiki re-ground done in-branch: 10 notes reviewed against the diff (governance rewritten around the convention; event-types gains meeting.convention_established + sim.gathering_observed rows; daemon-lifecycle, world-save-directory, sim-state-reducer, executor, agent-mind, chronicle, game-clock updated; snapshots re-pin only) — plan gate empty, freshness gate OK (29 notes). Orchestrator review applied 2 fixes (stale comment event name; shelter-safe emergent narration); full -race suite independently re-run green.

PR: https://github.com/evanstern/script-world/pull/22 (base main, branch task-36-emergent-meeting-convention). All 5 ACs ticked; awaiting merge.

PR #22 merged to main (19e41f9). Worktree .worktrees/task-36 removed, branch deleted, root ff-pulled; wiki freshness gate green on main (29 notes).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Removed the hard-coded 11:30/noon village meeting from the engine. The meeting convention is now event-sourced state (meeting.convention_established → State.MeetingConvention): established either by an optional world.json meeting block seeded on daemon boot (source config) or by a deterministic in-world emergence — a sustained 30-game-minute daytime gathering of ≥2 villagers at one fire/shelter, tracked replay-safely via sim.gathering_observed (source emergent). No convention → governance dormant, villagers follow their needs; scriptworld new writes no meeting block, so emergent is the default. TASK-13 lifecycle (convene→open→turns→votes→close) unchanged once a convention exists; norm enforcement survives conventionless upgraded saves. All surface text (prompts, narrator, charter) renders the convention's real hour. Full -race suite green twice; 10 wiki notes re-verified in-branch, gates green. One task, one branch, one PR: #22.
<!-- SECTION:FINAL_SUMMARY:END -->

---
id: TASK-33
title: >-
  Adaptive time throttling: speed as a ceiling, staleness debt as the governor —
  design session
status: To Do
assignee: []
created_date: '2026-07-20 20:48'
labels:
  - design
dependencies:
  - TASK-32
ordinal: 28000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Split from TASK-32 (user, 2026-07-20). PROBLEM — even with the cognition horizon scoping LLM authority by speed, there are moments where the player wants high speed AND high thought fidelity (a crisis unfolding at 32x). Rather than forcing a manual speed drop, the loop could govern itself. CANDIDATE DESIGN — the speed setting becomes a CEILING, not a promise: the sim tracks aggregate in-flight staleness debt (sum over pending planner/conversation jobs of predicted game-time drift, using TASK-32's calibrated seconds-per-point and telemetry) and sheds a speed notch when debt exceeds a budget, recovering when it drains. A feedback controller over a measurable signal — RimWorld-style adaptive time, but driven by cognition load instead of frame rate. QUESTIONS for the session: shed policy (notch-down vs proportional vs micro-pause at decision-critical moments); hysteresis so speed doesn't oscillate; whether debt is global or per-agent-weighted by salience; interaction with the existing SpeedMax-refused-with-LLM rule (does adaptive throttle subsume it?); how the TUI communicates 'you asked for 32x, running at 16x because 3 minds are in flight'; determinism boundary — throttling changes tick pacing (wall-side, like pause) and must never change tick CONTENT, so replay stays byte-identical. DEPENDS on TASK-32: needs the staleness telemetry, seconds-per-point calibration, and debt definition to exist before a governor can act on them. Output: a spec under specs/ linked via spec-bridge.
<!-- SECTION:DESCRIPTION:END -->

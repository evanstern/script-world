---
name: design-grounding
description: The TASK-1 Socratic grounding session outcomes that the daemon substrate implements — time posture, determinism, persistence, stack
kind: concept
sources:
  - docs/design/grounded-assumptions.md
verified_against: 004a430ca16d3f31d9d303b5b59b176bde0bae5f
---

# Design grounding

`docs/design/grounded-assumptions.md` is the written outcome of the TASK-1 Socratic
grounding session: every README/decision-1 assumption confirmed, revised, or rejected,
with citations into the session log. The daemon substrate implements its "Time &
posture" and "Stack & architecture" sections directly.

## How it works

Decisions the current code embodies:

- **Ambient persistent world**: the sim is a daemon; the terminal UI is an attachable
  client; closing the client is not pause — pause is a player verb. Implemented by
  [[daemon-lifecycle]] and the `pause` command in [[sim-loop]].
- **Default compression 4x**: 1 game minute = 15 real seconds; speeds range real-time
  to as-fast-as-affordable. Implemented in [[game-clock]].
- **Go + SQLite + flat files per run**: per-world save directory holding an append-only
  event log, snapshots, and run-bound flat files; never global. Implemented by
  [[world-save-directory]], [[event-log]], [[snapshots]].
- **Graceful degradation**: when the host can't hold the requested rate, the world
  slows honestly rather than dropping ticks. Implemented as auto-slow in [[sim-loop]].

Decisions the substrate anticipated but did not itself implement — the agent mind,
Metatron, the social fabric, chronicle, and the gru — have all since landed (TASK-5
through TASK-13; see [[agent-mind]], [[metatron]], [[social-fabric]], [[chronicle]],
[[gru]]); the `agents/` directory now holds per-agent persona/soul files at runtime.
Five candidate Spec Kit specs are listed at the end of the assumptions doc;
`specs/001-world-daemon` was #1.

## Connections

[[overview]] shows the shape these decisions produced. Backlog decision-2 (in
`backlog/decisions/`) is the durable record of the v1 shape; the assumptions doc cites
the session log in `topics/promptworld-design/101-rumor-mill-grounding/raw-notes.md`.

## Operational notes

This note invalidates only when the assumptions doc itself changes. Cost/inference
decisions (local tier, $100/month ceiling, planner cadence) are recorded there but have
no code yet — they bind TASK-6 (LLM orchestrator).

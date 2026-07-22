# Implementation Plan: Agent Mind v1

**Branch**: `task-7-agent-mind` | **Date**: 2026-07-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-agent-mind/spec.md`

## Summary

Give the eight villagers minds: authored immutable personas + accreting player-readable
souls (both flat files in the save dir), episodic memories as events reduced into sim
state, a deterministic top-K working-memory selector, and a daemon-side mind driver
that schedules planner calls (30-game-min stagger + wake/idle/nightfall/encounter
triggers) through the TASK-6 orchestrator's local tier, injecting structured goals into
the loop as recorded commands. The TASK-5 reflex demotes to an idle-grace fallback and
remains the permanent degraded mode. Replay never calls a model.

## Technical Context

**Language/Version**: Go 1.22+ (existing module; no new dependencies)

**Primary Dependencies**: internal/sim (executor + reducer), internal/llm
(orchestrator), internal/store, internal/world — all existing.

**Storage**: memories are events (`agent.memory_added`) reduced into `sim.State`
(snapshot-carried); `persona.md`/`soul.md` are flat files under
`<world>/agents/<name>/` — personas written once at genesis (mode 0444), souls
regenerated from replica state by a daemon-side scribe.

**Testing**: `go test` — sim-level determinism/memory/window units; mind integration
against an httptest mock local model (cadence, triggers, parse failures, fallback);
persona immutability checks; full `-race` suite; live smoke against real Ollama.

**Target Platform**: unchanged (darwin/arm64 primary).

**Project Type**: extension of the single `promptworld` binary.

**Performance Goals**: planner traffic ≤ 1 call/agent/30 game-min + triggers — at 4x
≈ 16+ calls/hour for 8 agents, well inside local-tier throughput; window selection
O(memories) per call.

**Constraints**: determinism (SC-005) — model output enters only as recorded events;
reflex fallback must be a pure function of event history (idle-grace measured in
ticks); persona firewall structural (no write path + 0444).

**Scale/Scope**: 8 agents; hundreds of memories/agent over a 30-day run (bounded later
by TASK-9 consolidation).

## Constitution Check

`.specify/memory/constitution.md` remains an unfilled template — no gates; passes
vacuously (as recorded for spec 001).

## Project Structure

### Documentation (this feature)

```text
specs/002-agent-mind/
├── spec.md
├── plan.md
├── research.md          # decisions + rationale
├── data-model.md        # memory/persona/thought shapes, new events
├── contracts/
│   ├── agent-files.md   # agents/<name>/ layout, persona/soul formats
│   └── planner-prompt.md# prompt structure + goal JSON contract
├── quickstart.md        # validation scenarios
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/
├── sim/
│   ├── agents.go        # +Memory, Agent.Memories/IdleSince, salience consts
│   ├── memory.go        # NEW: memory emission rules + SelectMemories (top-K)
│   ├── state.go         # reducer: memory_added, thought, planner intent_set, IdleSince
│   ├── executor.go      # memory event emission; reflex idle-grace
│   ├── policy.go        # resolveGoal shared by reflex + injection
│   └── loop.go          # inject_intent command
├── persona/             # NEW: 8 authored personas; genesis file writer (0444)
│   ├── personas.go
│   └── files.go
├── mind/                # NEW: the driver — replica, scheduler, prompt, parse
│   ├── mind.go
│   ├── prompt.go
│   └── parse.go
├── scribe/              # NEW: always-on soul.md writer (LLM-independent)
│   └── scribe.go
└── daemon/daemon.go     # wire scribe (always) + mind (when orchestrator on)
cmd/promptworld/         # `new` seeds personas; usage text
```

**Structure Decision**: mind (LLM-gated) and scribe (always-on) are separate daemon
components with separate replicas — souls must accrete even in a world with no
llm.json. Memory selection lives in `sim` as a pure function so the mind's prompts
and the tests share one implementation.

## Complexity Tracking

No constitution violations. Scope guards: no LLM conversations (TASK-8), no
consolidation/drift-validator (TASK-9), no learned rerankers (parked), personas
authored in-repo only.

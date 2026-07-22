# Implementation Plan: Social Fabric

**Branch**: `task-8-social-fabric` | **Date**: 2026-07-19 | **Spec**: [spec.md](spec.md)

## Summary

Event-sourced social state in the deterministic core — directed relation edges, an
append-only debt ledger with computed reputation, a rumor registry with per-holder
variants and provenance, authored secrets seeded at genesis — plus deterministic
social acts in the executor (give/repay, due-checks, talk affection). On top, a
conversation driver in the mind: bounded multi-turn local-tier dialogues whose
effects (gist memories, tone-driven edge deltas, one paraphrased rumor) enter the sim
only through a new `inject_social` loop command. Primitive talk remains the complete
model-free behavior.

## Technical Context

**Language/Version**: Go (existing module, no new deps)

**Storage**: all social state in `sim.State` via events (snapshot-carried);
secrets authored in `internal/persona`, seeded as tick-0 events by `new`.

**Testing**: sim units (edges, ledger, reputation, rumor chains, determinism/replay
re-proven), convo driver against scripted mock models, live Ollama smoke
(`gemma4:12b-mlx` — the operator's always-on local model).

**Performance**: conversations ≤3+3 turns + 1 outcome call ≈ 7 local calls, one
conversation at a time; planner traffic unchanged.

**Constraints**: replay model-free (SC-005); all LLM output enters as recorded
events; fallbacks keep model-less worlds fully functional (FR-010).

## Constitution Check

Constitution still an unfilled template — passes vacuously.

## Project Structure

```text
specs/003-social-fabric/   spec, plan, research, data-model, contracts/{social-events,conversation-prompt}.md, quickstart, tasks
internal/
├── sim/
│   ├── social.go          # NEW: types, reducer cases (called from state.go), Reputation, Tellable, edge rules
│   ├── social_test.go     # NEW
│   ├── state.go           # State gains Relations/Debts/Rumors/NextIDs; Apply delegates social.*
│   ├── executor.go        # give/repay in encounter slot; hourly due-check; talk edge deltas
│   ├── agents.go          # Agent.Known []KnownRumor; payload structs
│   └── loop.go            # inject_social command (whitelisted batch)
├── persona/personas.go    # Secrets map; SecretEvents() for genesis
├── mind/convo.go          # NEW: conversation driver (own goroutine, immutable ctx, slot=1)
├── mind/prompt.go         # planner prompt social context
└── scribe/scribe.go       # Bonds section in soul.md
cmd/promptworld/           # new: seed secret events at genesis
```

**Structure Decision**: social mechanics live in `sim` as pure functions + reducer
cases (deterministic, testable); only conversation *content* comes from the mind.
The `inject_social` command whitelists event types rather than trusting arbitrary
batches.

## Complexity Tracking

Scope guards: no confrontations, no norms/votes (TASK-13), no free-form promises,
no rumor denial. Conversation concurrency fixed at 1.

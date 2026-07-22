# Implementation Plan: Agent Tool-Use Loop

**Branch**: `task-52-agent-tool-loop` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/017-agent-tool-loop/spec.md`

## Summary

Replace prompt-stuffed free-text replies with a bounded, provider-native tool-use loop
for agent minds. `internal/llm` gains tool-call *transport* (tool declarations, a
multi-turn transcript, parsed tool calls — one `Submit` stays one metered call); a new
`internal/toolloop` package owns the *loop* (iteration cap, one-landed-acting-call
cardinality, read-tool dispatch, artifact recording); `internal/mind` (villager
planner-class cognition) and `internal/metatron` (turn) both adopt it. Cloud uses
Anthropic-native tool use; local uses OpenAI-compat function calling native-first with a
per-model schema-constrained JSON envelope fallback. Every tool call becomes a
`cog.tool_call` event (reducer no-op) correlated by the existing job identifier, which
also lands additively on `IntentSetPayload` (omitempty, byte-stable). The governor
observes whole-loop wall time per cognition; the meter keeps charging per call. The
scheduled musing channel is deleted — `muse` becomes a roster choice with opportunity
cost. Replay never re-runs any loop: only emitted events are facts.

## Technical Context

**Language/Version**: Go (module per go.mod; toolchain as pinned in repo)

**Primary Dependencies**: anthropic-sdk-go v1.58.0 (native tool use, already vendored);
net/http OpenAI-compatible chat-completions transport (existing `openaiCompat`); no new
external dependencies.

**Storage**: existing event-log store (`internal/store`) — events + snapshots + meta
table; no schema change beyond new event *types* (payloads are JSON, additive).

**Testing**: `go test ./...` — golden/byte-identity replay suite
(`whole_feature_test.go`, `sim_test.go`, per-capability replay tests), table-driven door
tests, new loop-driver unit tests with stub callers, provider wire-shape tests, soak
harness for SC-001/SC-005.

**Target Platform**: the promptworld daemon (darwin/linux), local Ollama-class endpoint
+ Anthropic cloud API.

**Project Type**: single Go project — event-sourced simulation daemon.

**Performance Goals**: local tier throughput remains the sim-speed governor; loop adds
rounds only when the model uses them (single-acting-call cognitions cost one round, as
today). Cap: ≤ `loop_max_rounds` (default 8) provider calls per cognition.

**Constraints**: replay byte-identity for all pre-feature logs (TASK-32 omitempty
pattern); the deterministic tick loop never awaits a tool loop; budget ceiling checked
before every billable call; route verdict stays pure arithmetic.

**Scale/Scope**: touches `internal/llm`, new `internal/toolloop`, `internal/tool`,
`internal/mind`, `internal/metatron`, `internal/sim` (payload field + whitelist entry),
`internal/cognition` (class table, observation seam), calibration command. Conversations,
consolidation, narrator/drama/meeting, reflex, executor, reducer arms: untouched.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.* (v1.1.0)

- **I. Artifact-Grounded Action — PASS.** Spec 017 + this plan + research/data-model/
  contracts/quickstart are the decision trail; clarifications recorded in spec.md;
  board task TASK-52 mirrors via spec-bridge; tier decisions recorded on the task.
- **II. One Task, One PR — PASS.** TASK-52 ↔ branch `task-52-agent-tool-loop` in
  `.worktrees/task-52` ↔ one PR. Spec phases are internal breakdown only. Root stays
  on main.
- **III. Gates Over Assertions — PASS.** spec-bridge:link before implementation;
  bridge gate holds status to artifacts; daemon startup validation extends to the new
  registry surface; replay suite is the determinism gate.
- **IV. Grounding Freshness — PASS (with owed work).** The wiki notes listing touched
  files as sources (`llm-orchestrator.md`, `cognition.md`, `agent-mind.md`,
  `tool-registry.md`, `event-types.md`, `sim-state-reducer.md`, `metatron.md`) MUST be
  re-pinned via `/grounding-wiki:wiki-update` before the task is Done — tracked as an
  explicit task in tasks.md.
- **V. Model-Tiered Workflow — PASS.** Planning (this document) on Fable 5.
  Implementation delegated to `spec-implementer` subagents. Tier per the escalation
  rubric: the loop driver, llm transport, governor observation seam, mind/metatron
  migration, and muse-channel removal are cross-package concurrency/orchestration in
  `internal/llm`/`internal/cognition`/`internal/mind` → **Opus 4.8**. Registry additions
  (`Number` ParamKind, schema derivation) and doc/config plumbing are single-package and
  routine → **Sonnet**, escalating one-way if gates fail. Choices + justification
  recorded on TASK-52.

**Post-Phase-1 re-check (2026-07-22): PASS** — no new violations introduced by the
design; Complexity Tracking stays empty (one new package is the *simpler* alternative to
entangling llm with dispatch, per research R1).

## Project Structure

### Documentation (this feature)

```text
specs/017-agent-tool-loop/
├── spec.md              # Feature specification (+ Clarifications 2026-07-22)
├── plan.md              # This file
├── research.md          # Phase 0 — decisions R1–R15
├── data-model.md        # Phase 1 — entities, payloads, config, verdict taxonomy
├── quickstart.md        # Phase 1 — validation guide
├── checklists/
│   └── requirements.md  # Spec quality gate (passed)
├── contracts/
│   ├── loop-api.md      # Go surfaces: llm transport, toolloop driver, tool additions
│   ├── events.md        # New/changed event payloads + byte-stability contract
│   └── provider-wire.md # Native (Anthropic / OpenAI-compat) + fallback envelope shapes
└── tasks.md             # Phase 2 (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── llm/                 # transport: Request{Tools,Turns,SkipObserve}, Response{ToolCalls,Stop},
│   │                    #   ObserveCognition; providers.go native tool exchange both callers;
│   │                    #   config.go tool_mode + loop_max_rounds
│   ├── llm.go
│   ├── providers.go
│   ├── config.go
│   └── meter.go         # UNCHANGED (per-call spend is already correct)
├── toolloop/            # NEW: loop driver — Run(ctx, LoopJob) → LoopResult
│   ├── loop.go          #   iteration cap, cardinality, dispatch, verdicts
│   ├── record.go        #   cog.tool_call artifact assembly (emission via callback)
│   └── loop_test.go     #   stub-caller unit tests (cap, cardinality, batching, errors)
├── tool/                # ParamKind Number{Min,Max}; Tool.InputSchemaJSON override;
│   │                    #   InputSchema() derivation; set_plan entry; Validate lifts
│   │                    #   Read-roster rejection; qty Params on storage verbs
│   ├── tool.go
│   ├── registry.go
│   └── derive.go
├── mind/                # villager cognition → toolloop; muse scheduling DELETED;
│   │                    #   prompt.go tool-era system prompt; parse.go planner path retired
│   ├── mind.go
│   ├── prompt.go
│   ├── parse.go
│   └── telemetry.go     # emitCog carries cog.tool_call batches
├── metatron/            # Turn() → toolloop with RosterMetatron; parseTurn retired
│   └── turn.go
├── sim/                 # IntentSetPayload.Job (omitempty); injectSocialWhitelist
│   │                    #   += cog.tool_call; reducer no-op arm
│   ├── agents.go
│   ├── loop.go
│   └── state.go
└── cognition/           # musing class removed; ValidateKinds updated; estimator
    └── ...              #   observation seam documented (fed via llm.ObserveCognition)
```

**Structure Decision**: single Go project, one new leaf-adjacent package
(`internal/toolloop`, depends on `internal/llm` + `internal/tool` only; handlers and
event emission are injected by consumers). All other work lands in existing packages.

## Complexity Tracking

No constitution violations — table intentionally empty.

# Implementation Plan: llm.json robustness knobs — in-loop cognition retry + configurable max_tokens

**Branch**: `task-72-llm-robustness-knobs` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/025-llm-robustness-knobs/spec.md`

## Summary

Two operator-facing robustness knobs, one PR (TASK-72). (a) The tool-loop cognition
driver (`internal/toolloop/loop.go run()`) gains ONE in-loop retry when a model call
fails as a transport-level provider error: instead of terminating the whole
planner/metatron thought, the loop re-submits the same transcript once. Recovery is
surfaced on `toolloop.Result` and recorded by the consumers as a non-terminal
`cog.outcome` with the existing `sim.OutcomeRetried` marker (TASK-42's conversation
vocabulary — no new event type, no digest-catalog drift). Estimator and breaker
doctrine are preserved structurally: internal Submits already ride `SkipObserve`, the
whole-Run wall-time observation already fires successes-only via a single deferred
exit path, and each Submit strikes the breaker exactly as an independent call does
today. (b) `llm.Config` gains a `max_tokens` object with three per-kind budgets —
`planner` (default 512), `metatron_turn` (default 1024), `consolidation` (default
1024) — normalized by the established warn-not-error clamp convention
(`Rounds()`/`Workers()` pattern: absent/0 = default, 1–4096 pass, out-of-range clamps
with a boot warning, never a boot failure), plumbed through `daemon.go` boot into
`mind.New`/`metatron.New` constructor params the same way `loopRounds` is today.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib; existing provider SDK plumbing untouched (retry sits above the provider seam; budgets ride the existing `llm.Request.MaxTokens` wire)

**Storage**: `llm.json` in the world save directory (existing operator config file); event log (existing `cog.outcome` events via the InjectSocial door)

**Testing**: `go test -race ./...`; toolloop unit tests against the scripted `submitter` stub (established pattern in `internal/toolloop/*_test.go`); config table tests mirroring `TestRoundsNormalization` (`internal/llm/llm_test.go:1080`)

**Target Platform**: the promptworld daemon (darwin/linux dev machines)

**Project Type**: single Go module, multi-package internal architecture

**Performance Goals**: none new — a retry adds at most one provider call's wall time to an already-failed cognition, bounded by the caller's existing timeouts (`callTimeout`, `turnTimeout`)

**Constraints**: estimator discipline (successes-only, whole-Run wall time, `SkipObserve` on internal Submits) unchanged; breaker semantics (busy-is-not-down) unchanged; no new event types (digest catalog `TestCatalogSweep` tripwire); worlds never fail to boot over a tuning knob; byte-for-byte default behavior for configs without the new fields

**Scale/Scope**: ~5 packages touched (`internal/llm`, `internal/toolloop`, `internal/mind`, `internal/metatron`, `internal/daemon`), plus tests and wiki re-pins; no persistence-format or IPC changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Artifact-Grounded Action | PASS | Spec/plan/tasks under `specs/025-llm-robustness-knobs/`; board task TASK-72 to be linked via `spec-bridge:link` before implementation; scope ambiguities were resolved from the TASK-72 artifact and recorded in spec Assumptions |
| II. One Task, One PR | PASS | One branch `task-72-llm-robustness-knobs` in `.worktrees/task-72`, one PR; both knobs are internal breakdown of the one TASK |
| III. Gates Over Assertions | PASS | spec-bridge gate mirrors phase criteria; status advances only with artifacts |
| IV. Grounding Freshness | PASS (with obligation) | Touched files are pinned sources of `tool-loop.md`, `llm-orchestrator.md`, `agent-mind.md`, `metatron.md`, `nightly-consolidation.md`, `daemon-lifecycle.md` (exact set computed by wiki-update); PR is not done until re-pinned — mirrored as AC #5 on TASK-72 |
| V. Model-Tiered Workflow | PASS | Planning on Fable 5 (this plan). Implementation delegated to `spec-implementer` at **Opus 4.8**: cross-package change touching `internal/llm`/`internal/toolloop` orchestration and estimator/breaker doctrine — squarely inside the senior-tier rubric (concurrency/governor logic, doctrine-adjacent behavior). Tier choice + justification to be recorded on TASK-72 |

**Post-Phase-1 re-check**: PASS — design adds no new projects, no new event types, no
new abstractions beyond three config fields, one `Result` extension, and one retry
branch; no Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/025-llm-robustness-knobs/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: design decisions + grounding
├── data-model.md        # Phase 1: config fields, Result surface, normalization rules
├── quickstart.md        # Phase 1: validation scenarios
├── contracts/
│   ├── llm-json.md      # Phase 1: llm.json max_tokens contract
│   └── loop-retry.md    # Phase 1: retry semantics contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (complete)
└── tasks.md             # Phase 2 output (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── llm/
│   ├── config.go        # + MaxTokens struct (planner/metatron_turn/consolidation),
│   │                    #   normalizers mirroring Rounds()/Workers(); bounds 1..4096
│   └── llm_test.go      # + normalization table tests (TestRoundsNormalization pattern)
├── toolloop/
│   ├── loop.go          # + one retry on transport provider_error in run()'s Submit
│   │                    #   error branch; + Result.Retried / Result.RetryReason
│   └── *_test.go        # + fail-once / fail-twice / admission-not-retried /
│                        #   estimator-and-round-cap invariance scripted-stub tests
├── mind/
│   ├── mind.go          # loopMaxTokens const → constructor-injected planner budget
│   ├── consolidate.go   # hardcoded 1024 → injected consolidation budget
│   ├── telemetry.go     # (reuse) cogOutcomeEvent with sim.OutcomeRetried on recovery
│   └── mind_test.go     # + retry-visibility + budget plumbing tests
├── metatron/
│   ├── turn.go          # turnMaxTokens const → constructor-injected turn budget;
│   │                    #   surface res.Retried as cog.outcome through InjectSocial
│   ├── metatron.go      # New() signature gains the turn budget (as loopRounds did)
│   └── *_test.go        # + retry-visibility + budget plumbing tests
└── daemon/
    └── daemon.go        # resolve budgets + print clamp warnings (existing
                         # "daemon: %s" channel); pass into mind.New / metatron.New

cmd/promptworld/
└── calibrate.go         # follows any mind.New/metatron.New signature change (mechanical)

docs/wiki/               # re-pin after merge: tool-loop, llm-orchestrator, agent-mind,
                         # metatron, nightly-consolidation, daemon-lifecycle (as computed)
```

**Structure Decision**: existing single-module layout; no new packages. The knobs
follow the exact plumbing route `loop_max_rounds` (TASK-52) carved: config normalizer
in `internal/llm/config.go` → daemon boot resolution + warning print → constructor
params on `mind.New`/`metatron.New`.

## Complexity Tracking

No constitution violations — table intentionally empty.

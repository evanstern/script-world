# Implementation Plan: Multi-Provider Routing — Registry and Ordered Chains

**Branch**: `task-35-provider-routing` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/024-provider-routing/spec.md` (doctrine: decision-5, parent decision-4)

## Summary

Generalize `internal/llm`'s fixed two-tier (local/cloud) machinery into N declared
providers with ordered per-kind routing chains. Everything that exists per-tier today —
bounded queue + priority lane, worker slots, circuit breaker, seconds-per-point estimator
— becomes a per-provider instance with byte-identical semantics. Admission walks the
kind's chain (skip only on circuit-open / wallet-empty / queue-full), conversation scenes
pin their provider at scene start, one global wallet gains per-provider attribution, and
optional advisory flock leases bound cross-world endpoint concurrency (TASK-24). Legacy
`llm.json` loads unchanged as a derived two-provider registry — the P1 regression gate.

## Technical Context

**Language/Version**: Go 1.24 (existing module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib (`net/http`, `sync`, `golang.org/x/sys/unix` NOT needed —
`syscall.Flock` suffices); `anthropic-sdk-go` (existing, cloud transport); no new deps

**Storage**: existing world store meta table (spend keys); new advisory lock files under
`~/.promptworld/endpoint-leases/` (not world state)

**Testing**: `go test -race ./...`; httptest mock providers (existing pattern in
`internal/llm`); two-orchestrator in-process lease contention tests; e2e daemon boot with
legacy + v2 configs

**Target Platform**: macOS/Linux daemon (flock available on both; lease feature is
POSIX-advisory by design)

**Project Type**: single Go module — engine package (`internal/llm`) + consumers
(`internal/mind`, `internal/toolloop`, `internal/metatron`, `internal/ipc`,
`internal/tui`, `cmd/promptworld`)

**Performance Goals**: routing decision O(chain length) with no allocation on the happy
path; admission stays fail-fast (no new blocking before dispatch except opt-in lease
acquisition, which happens in the worker, bounded by the existing 2-min call cap)

**Constraints**: zero behavior change for legacy configs (routing, errors, metering,
status content); determinism/replay untouched (all routing outside the sim loop);
warn-not-error clamp doctrine for all tuning knobs; config is boot-time-only

**Scale/Scope**: registries of 2–8 providers; chains of 1–4; one lease pool per distinct
normalized endpoint; ~3,800+ calls/day local traffic unchanged

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0.*

- **I. Artifact-Grounded Action** — PASS: decision-5 + this spec/plan on the board via
  spec-bridge (TASK-35); tier choices recorded on the task.
- **II. One Task, One PR** — PASS: all slices land as commits on
  `task-35-provider-routing` (worktree `.worktrees/task-35`), one PR; spec phases are
  internal breakdown.
- **III. Gates Over Assertions** — PASS: spec-bridge gate drives TASK-35's status from
  tasks.md checkboxes; nothing here bypasses it.
- **IV. Grounding Freshness** — PLANNED: merge touches sources of `llm-orchestrator`,
  `cognition`, `tui-client`, `ipc-protocol`, `agent-mind`, `cli-promptworld` wiki notes →
  `/grounding-wiki:wiki-update` after merge is part of Done.
- **V. Model-Tiered Workflow** — PASS with recorded choices: core orchestrator/registry/
  chain/lease/meter/estimator work (US1–US5) is concurrency + scheduling logic in
  `internal/llm` and cross-package seam changes → **Opus 4.8** on `spec-implementer`.
  US6 status/TUI rendering + doc reconciliation is view code → **Sonnet** (default tier).
  Justifications go on TASK-35 per the rubric.

**Post-Phase-1 re-check**: PASS — design adds no new projects, no new dependencies, no
gate bypasses; complexity table below is empty.

## Project Structure

### Documentation (this feature)

```text
specs/024-provider-routing/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0 — settled unknowns
├── data-model.md        # Phase 1 — config/entity shapes and invariants
├── quickstart.md        # Phase 1 — runnable validation scenarios
├── contracts/
│   ├── llm-config.md    # llm.json v2 (registry + routes) + legacy derivation
│   ├── status.md        # per-provider status protocol shape
│   └── endpoint-lease.md# lease directory layout + acquisition protocol
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/llm/
├── config.go        # ProviderConfig, RouteConfig, registry validation, legacy derivation
├── llm.go           # tier → provider struct; routing map → chains; Submit chain-walk;
│                    # Request.Provider pin; Response.Provider + skip records
├── providers.go     # caller construction per transport (openai_compat | anthropic)
├── meter.go         # global ceiling + per-provider attribution keys
├── health.go        # unchanged breaker logic (per-provider instantiation)
├── lease.go         # NEW: advisory flock endpoint lease pool + contended flag
└── *_test.go        # legacy-equivalence, chain-walk, pin, lease, attribution tests

internal/cognition/
└── calibration.go   # SeedFor keyed by provider name + pricing-class bootstrap fallback

internal/mind/
├── mind.go          # orchestrator seam interface additions (resolve/estimate)
├── convo.go         # scene provider pin (resolve at scene start, set on every turn)
└── telemetry.go     # TierFor/SecondsPerPoint reads → kind-granular estimate seam

internal/toolloop/loop.go   # ObserveCognition provider attribution passthrough
internal/ipc/server.go      # status folding (per-provider table)
internal/ipc/protocol.go    # status wire shape (if typed there)
internal/tui/…              # provider table rendering (metatron/status pane)
cmd/promptworld/
├── calibrate.go     # per-provider calibration profile keys
└── llm.go (subcmd)  # one-shot proof path names provider
```

**Structure Decision**: evolve `internal/llm` in place — the `tier` struct is renamed and
multiplied, not wrapped. No new packages except nothing; `lease.go` is a file, not a
package. Consumers keep the same `Submit` seam; new optional fields only.

## Slice / delegation map (constitution V)

| Slice | Content | Tier |
|-------|---------|------|
| US1 (P1) | registry+routes config, validation, legacy derivation, provider structs, chain-head dispatch, per-provider machinery instantiation, equivalence tests | Opus 4.8 |
| US2 (P2) | per-provider estimators + calibration seeding, mind estimate seam, ObserveCognition attribution | Opus 4.8 |
| US3 (P3) | chain-walk fallback + skip records, no_fallback, Request.Provider pin, convo scene pinning | Opus 4.8 |
| US4 (P4) | meter attribution (persisted per-provider keys), admission wallet checks per priced candidate | Opus 4.8 |
| US5 (P5) | flock lease pool, contended flag, cross-orchestrator tests | Opus 4.8 |
| US6 (P6) | status wire shape, TUI provider table, CLI output naming, doc strings | Sonnet |

## Complexity Tracking

*No constitution violations — table intentionally empty.*

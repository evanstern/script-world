# Implementation Plan: Metatron Agency — Standing Orders, Omens & Visions

**Branch**: `task-27-metatron-agency` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/029-metatron-agency/spec.md`

## Summary

Evolve Metatron from a single-turn console responder into a long-running agent.
Five slices, all riding the shipped spec 014/017/021 substrate unchanged: (1) the
nudge taxonomy migrates from `nudge_dream`/`nudge_omen` to `send_vision` (one
villager, anytime) + `send_omen` (night-only, one/group/all) with replay-compatible
grandfathering of historical dream events; (2) `monitor_and_act` places event-sourced
standing orders (`metatron.order_placed/triggered/cancelled/expired` on `State`,
player cap 3, TTL in game days) whose structural predicates are evaluated for free
in Metatron's absorb path; (3) a matched order executes as a system-authored turn
through the existing single-flight `Turn` path and tool loop, landing recorded
injections and queuing a moment; (4) fuzzy conditions add a rate-capped confirm on
a new cheap-routed `KindMetatronWatch`; (5) charge-free meta tools
(`pause`/`start`/`adjust_speed`) wrap `sim.Loop.Do` behind a small loop-control
seam. Budget/tier honesty extends verbatim to triggered turns.

## Technical Context

**Language/Version**: Go 1.24 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib only at the feature seams; internal packages
`internal/tool` (registry), `internal/toolloop` (bounded loop driver),
`internal/metatron` (angel component), `internal/sim` (state/reducer/loop doors),
`internal/llm` (orchestrator, kind routing), `internal/clock`, `internal/store`

**Storage**: append-only event log + snapshots (existing `internal/store`); standing
orders are event-sourced `State` fields — no new storage engine

**Testing**: `go test ./...`; existing suites extended: `metatron_test.go` sentinel
audit, reducer replay/determinism suite, `toolloop` driver tests, registry
`Validate`/coverage boot gates

**Target Platform**: the promptworld daemon (macOS/Linux), IPC/CLI/TUI clients
unchanged except additive status fields

**Project Type**: single Go daemon + CLI/TUI (existing layout)

**Performance Goals**: zero model calls per non-matching observed event (SC-001);
≤1 placement-time compile burden folded into the turn call itself; ≤48
confirm calls/order/game-day worst case (SC-008)

**Constraints**: replay determinism (triggers never execute during replay; all
durable effects are recorded events); existing worlds and `llm.json` files keep
booting (route backfill for the new kind, snapshot upgrade for new state fields);
the structural firewall and whitelist are extended, never relaxed

**Scale/Scope**: ≤3 player orders + system deferral orders per world; 8 villagers;
~6 new tools, 4 new event types, 1 new call kind

## Constitution Check

*GATE: v1.1.0. Checked before Phase 0; re-checked after Phase 1 design.*

- **I. Artifact-Grounded Action** — PASS: this plan derives from TASK-27's recorded
  design decisions and spec.md; every decision lands in research.md/data-model.md;
  progress mirrors to the board via spec-bridge.
- **II. One Task, One PR** — PASS: all work on `task-27-metatron-agency` in
  `.worktrees/task-27`; one PR closes TASK-27.
- **III. Gates Over Assertions** — PASS: boot gates (`tool.Validate`,
  `sim.ValidateToolCoverage`) extend to the new tools; the sentinel audit test is
  the firewall gate; spec-bridge gates board status.
- **IV. Grounding Freshness** — PASS (deferred obligation): touched wiki notes
  (metatron, tool-registry, tool-loop, llm-orchestrator, event-types, sim-loop,
  executor) re-pin via `wiki-update` before merge; player-docs freshness check after.
- **V. Model-Tiered Workflow** — PASS: this plan is planning-tier work;
  implementation delegates to `spec-implementer` at the **Opus 4.8** tier
  (rubric: cross-package — `internal/llm` routing + `internal/toolloop` validator
  generalization + `internal/sim` reducer/state + `internal/metatron`; doctrine-
  adjacent — firewall, replay determinism, charge economy, budget honesty).

**Post-Phase-1 re-check**: PASS — no new packages beyond the existing layout, no
new storage, no door relaxation; Complexity Tracking stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/029-metatron-agency/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── tools.md         # New/changed registry tool entries + rosters
│   ├── events.md        # New event types + payloads + whitelist delta
│   └── routing.md       # KindMetatronWatch routing + config backfill
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/tool/            # registry.go (retire nudge_dream/nudge_omen; add
                          #   send_vision, send_omen, monitor_and_act, cancel_order,
                          #   pause, start, adjust_speed), roster.go, derive.go,
                          #   validate.go (authored-override validator dispatch)
internal/toolloop/        # loop.go validateArgs: generalize authored InputSchemaJSON
                          #   validation beyond set_plan (schema-lite walker)
internal/metatron/        # metatron.go (order mirror, trigger queue, loop-control
                          #   seam), turn.go (landOmen/landVision, systemTurn),
                          #   toolcalls.go (new handlers), orders.go (predicate
                          #   match, watch confirm, trigger worker), digest.go
                          #   (moment lines for deferral/expiry/degradation)
internal/sim/             # metatron.go (order reducer arms, omen night gate,
                          #   vision/omen forms), state.go (State.MetatronOrders),
                          #   loop.go (whitelist delta), executor.go (order expiry
                          #   emission), toolcheck.go (coverage over new tools)
internal/llm/             # llm.go (KindMetatronWatch), config.go (default route,
                          #   backfill for missing new-kind route)
internal/ipc /internal/tui/cli  # additive: status orders field rendering (minimal)
```

**Structure Decision**: existing single-module layout; no new packages except a
possible `internal/metatron/orders.go` file within the existing package.

## Complexity Tracking

*No constitution violations — table intentionally empty.*

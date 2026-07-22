# Implementation Plan: Villagers Tab — per-villager inspection

**Branch**: `015-villagers-tab` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/015-villagers-tab/spec.md`

## Summary

Rename the TUI's fourth dock tab from "souls" to "villagers" and upgrade it
from a flat roster into a per-villager inspector: `j/k` selects a villager,
`⏎` opens a detail view (identity/vitals, itemized inventory, objective,
beliefs/narrative, memories), `esc` returns to the roster. The one data gap —
showing the most recent objective while the villager is idle — is closed with
two reducer-maintained fields on `sim.Agent` (`LastGoal`, `LastGoalTick`,
both `omitempty`) written on `agent.intent_set` and never cleared, so the
value flows through snapshots and log shipping to every observer
(research.md R1). Everything else renders from data the client replica
already holds.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: charmbracelet/bubbletea v1.3.10 (TUI runtime),
charmbracelet/lipgloss v1.1.0 (styles); no new dependencies.

**Storage**: none new — reads the client-side event-sourced replica
(`sim.State`); the two new agent fields persist via existing snapshots
(modernc.org/sqlite store), no schema change.

**Testing**: `go test ./...` — table-driven unit tests in `internal/tui`
(grammar/focus/render suites) and `internal/sim` (reducer + replay
determinism + old-snapshot decode).

**Target Platform**: terminal client (macOS/Linux), narrow-dock and
widescreen/solo layouts at the TUI's existing minimum sizes.

**Project Type**: single Go module; feature touches `internal/sim` (2 fields
+ 1 reducer line) and `internal/tui` (views, keys), plus design docs.

**Performance Goals**: render within the existing frame budget; detail view
is pure formatting over in-memory state — no I/O per frame.

**Constraints**: byte-stable canonical state for pre-feature agents
(`omitempty`); old snapshots must decode (no format-version bump — additive
fields only); never overflow pane budgets; no key collisions with global
keymap.

**Scale/Scope**: 8 villagers today; roster/detail must clamp, not optimize,
for other counts. ~2 files of production code + tests + 4 design-doc edits.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Artifact-Grounded Action** — PASS: spec/plan/research/tasks under
  `specs/015-villagers-tab/`; board task linked via spec-bridge before
  implementation; tier decision recorded on the task.
- **II. One Task, One PR** — PASS: one TASK, one branch (`task-<N>-villagers-tab`
  in `.worktrees/`), one PR; spec phases are internal breakdown.
- **III. Gates Over Assertions** — PASS: spec-bridge gate governs board
  status; no derived state hand-edits.
- **IV. Grounding Freshness** — PASS (planned): `docs/wiki/tui-client.md`
  and `docs/wiki/sim-state-reducer.md` list touched files as sources —
  wiki-update is an explicit exit criterion after merge.
- **V. Model-Tiered Workflow** — PASS: planning done here on the planning
  tier; implementation delegated to `spec-implementer`. Tier call: the slice
  is dominated by single-package view/rendering code; the `internal/sim`
  touch is two additive fields + one reducer assignment with prescribed
  tests, not concurrency/governor logic. **Sonnet tier**, with the rubric
  justification recorded on the board task; escalate per rubric only if
  gates fail.

**Post-design re-check**: PASS — design adds no projects, no dependencies,
no new events, no format bump; complexity table empty.

## Project Structure

### Documentation (this feature)

```text
specs/015-villagers-tab/
├── plan.md              # This file
├── research.md          # Phase 0 — R1..R7 decisions
├── data-model.md        # Phase 1 — Agent field additions, TUI model state
├── quickstart.md        # Phase 1 — validation guide
├── contracts/
│   └── state-and-keys.md  # Phase 1 — state-shape + key-grammar contract
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — not created by plan)
```

### Source Code (repository root)

```text
internal/
├── sim/
│   ├── agents.go        # + Agent.LastGoal, Agent.LastGoalTick (omitempty)
│   ├── state.go         # Apply "agent.intent_set": also set LastGoal/Tick
│   └── *_test.go        # reducer, replay-determinism, old-snapshot decode
└── tui/
    ├── tui.go           # pane rename, villSelected/villDetail state, keys
    ├── views.go         # villagersBody: roster+cursor, detail renderer
    ├── focus_test.go    # key grammar / esc release chain
    ├── render_test.go   # budgets, no-overflow, sections
    └── tui_test.go      # selection clamp on reconnect / nil replica

docs/design/tui/
├── panels/dock.md       # Tab: souls → Tab: villagers (+ selection spec)
├── patterns/keymap.md   # `4` label, villagers j/k/⏎/esc additions, footer
├── pages/solo-views.md  # tab naming where referenced
└── pages/home.md        # tab naming where referenced
```

**Structure Decision**: existing single-module layout; no new packages. The
feature is a TUI change plus a minimal additive reducer change, mirrored into
the load-bearing design docs in the same PR.

## Complexity Tracking

No constitution violations — table intentionally empty.

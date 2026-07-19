---
name: tui-client
description: The Bubble Tea full-screen client — four panes over a live world replica maintained by log shipping (state snapshot + event subscription through the shared reducer)
kind: component
sources:
  - internal/tui/tui.go
  - internal/tui/views.go
verified_against: aff0448e78ebec0f7724fc4c8ab02d4961e37236
---

# TUI client

`internal/tui` is the attachable full-screen client (`scriptworld ui <dir>`), built on
Bubble Tea + Lipgloss. Its core idea: the map renders from a **live replica** of
`sim.State` that the client maintains by log shipping — fetch the state snapshot, then
apply every pushed event through the exact `Apply` reducer the daemon runs. The TUI is
a read replica of the world.

## How it works

`Model` holds the world handle, an `ipc.Client`, the replica, the latest polled
`StatusData`, and a chronicle ring (`chronicleCap = 500` events). All protocol calls
run inside `tea.Cmd`s so the UI never blocks on the socket.

Connection (`connect`): dial → `FetchState` (state JSON + the `last_seq` it reflects)
→ unmarshal into a fresh `sim.NewState(seed)` → `Subscribe(since: last_seq)` — the
replica starts gapless by construction. `listen` delivers one push per invocation and
`Update` re-arms it. `applyEvent` skips seqs already folded into the snapshot, applies
the rest to the replica, bumps its tick, and appends to the chronicle ring.

Resilience: any error becomes `disconnectedMsg` → the header shows the failure and a
2-second retry loop re-dials; a `dropped` push (subscriber overflow) tears the client
down and reconnects from a fresh state snapshot, because the replica may have missed
events. A 1-second poll refreshes the clock/status line (quiet ticks produce no
events, so the replica's tick alone would lag).

Panes (`pane` enum; keys 1–4, tab/shift+tab cycle): **map** (default — a camera
window over the generated terrain from `Model.gameMap` (regenerated locally via
`world.Map()`, [[worldmap-generation]]): water/trees/forage/dens glyphs with the
replica's agents on top (by initial, lowercase asleep, † dead) plus built fires ▲
and shelters ⌂; the camera follows the living agents' centroid, arrow keys pan, `c`
recenters), **chronicle** (raw event feed until TASK-11 narrates it), **metatron**
(stub until TASK-12; already shows [[llm-orchestrator]] tier
health, queues, and monthly spend when the world has one), **souls** (live agent bodies: status, current goal, five-cell
needs gauges, inventory, and each agent's newest memory line; the full soul.md
files live on disk per [[agent-mind]]). Time controls: space toggles
pause/resume based on last-known status; `[`/`]` step through `speedSteps`
(1x → 4x → 8x → 16x → max); `q` detaches — the world keeps running.

## Connections

[[ipc-client]] is the transport; [[ipc-protocol]]'s `state` command exists for this
replica pattern; [[sim-state-reducer]] supplies the shared `Apply`; [[event-types]]
is what the chronicle shows; [[cli-scriptworld]] mounts it as the `ui` subcommand.

## Operational notes

Rendering requires no daemon round trips — map updates come from pushed events, so the
UI stays smooth at max speed (the chronicle simply scrolls fast). Unit tests cover pane
navigation, replica application, ring capping, and quit behavior; an expect-driven PTY
smoke test drives the real binary. When real systems land, panes graduate from stubs
without changing the replica machinery.

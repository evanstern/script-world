---
name: tui-client
description: The Bubble Tea full-screen client — four panes over a live world replica maintained by log shipping (state snapshot + event subscription through the shared reducer)
kind: component
sources:
  - internal/tui/tui.go
  - internal/tui/views.go
verified_against: 65898835d02ec199456eb656ad9187aca3346fbf
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

Resilience: errors become `disconnectedMsg` → the header shows the failure and a
2-second retry loop re-dials; a `dropped` push (subscriber overflow) tears the client
down and reconnects from a fresh state snapshot, because the replica may have missed
events. One exception is fatal (TASK-19): `ipc.ErrReplyTooLarge` (a reply over the
protocol's 64 MiB ceiling — reconnecting cannot shrink the state) quits instead of
retrying, rendering the reason in the final view and exposing it via
`Model.FatalErr()`, which `cmdUI` turns into a non-zero exit. A 1-second poll refreshes the clock/status line (quiet ticks produce no
events, so the replica's tick alone would lag).

Panes (`pane` enum; keys 1–4, tab/shift+tab cycle): **map** (default — a camera
window over the generated terrain from `Model.gameMap` (regenerated locally via
`world.Map()`, [[worldmap-generation]]): water/trees/forage/dens glyphs with the
replica's agents on top (by initial, lowercase asleep, † dead) plus built fires ▲,
shelters ⌂, and the [[gru]] as a red G while it is abroad; the camera follows the
living agents' centroid, arrow keys pan, `c`
recenters), **chronicle** (TASK-11: the narrated story from the replica's
snapshot-carried `State.Chronicle` ring ([[chronicle]]) — day-stamped entries with
thread slugs and cast; `a` cycles an agent filter, `t` cycles a thread filter
across slugs seen in the ring, `r` toggles the raw event feed, which is also the
automatic fallback while a world has no narrated entries), **metatron**
(TASK-12: the console — session transcript + input line over `metatron_chat`;
while active the console owns the keyboard (every printable key types, Enter
sends with an in-flight spinner, Esc returns to the map), the header shows the
⚡ charge bank (from status) and charter provenance (fetched on pane entry),
and tier health + spend remain at the foot — [[metatron]]), **souls** (live agent bodies: status, current goal, five-cell
needs gauges, inventory, and each agent's newest memory line; the full soul.md
files live on disk per [[agent-mind]]). Time controls: space toggles
pause/resume based on last-known status; `[`/`]` step through `speedSteps`
(1x → 4x → 8x → 16x → 32x — max is deliberately off the watchable ladder,
TASK-20); `q` detaches — the world keeps running.

## Connections

[[ipc-client]] is the transport; [[ipc-protocol]]'s `state` command exists for this
replica pattern; [[sim-state-reducer]] supplies the shared `Apply`; [[chronicle]]
fills the story pane and [[event-types]] the raw feed; [[cli-scriptworld]] mounts
it as the `ui` subcommand.

## Operational notes

Rendering requires no daemon round trips — map updates come from pushed events, so the
UI stays smooth at max speed (the chronicle simply scrolls fast). Unit tests cover pane
navigation, replica application, ring capping, and quit behavior; an expect-driven PTY
smoke test drives the real binary. When real systems land, panes graduate from stubs
without changing the replica machinery.

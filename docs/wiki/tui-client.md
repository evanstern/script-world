---
name: tui-client
description: The Bubble Tea full-screen client — a widescreen map+dock composite (with minibuffer and narrow single-pane fallback) over a live world replica maintained by log shipping (state snapshot + event subscription through the shared reducer)
kind: component
sources:
  - internal/tui/tui.go
  - internal/tui/views.go
  - internal/tui/layout.go
  - internal/tui/grammar.go
verified_against: 3911e4ca0bf6dc76ce6960a09db2ffed3ed0e9f4
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

Layout (TASK-34; design reference in `docs/design/tui/`): at ≥112 columns the
client renders the **widescreen composite** — the map on the left and a tabbed
**dock** on the right in a 50/50 split (`computeColumns` in layout.go; the map's
viewport derives from the column budget via `mapViewportTiles`), a one-line
**Metatron minibuffer** above the footer, and per-mode footer hints. Below 112
columns it falls back to the original single-pane UI (header + tab bar + one
active pane), unchanged. `View` output is exactly terminal-height in every mode
(every panel body is clipped to its row budget — `clipContent`), and resizes
re-clamp pan/selection state (`clampGeometry`).

Regions: the **map** is a camera window over the generated terrain from
`Model.gameMap` (regenerated locally via `world.Map()`,
[[worldmap-generation]]): water/trees/forage/dens glyphs with the replica's
agents on top (by initial, lowercase asleep, † dead) plus built fires ▲,
shelters ⌂, and the [[gru]] as a red G while it is abroad; the camera follows
the living agents' centroid, arrow keys pan, `c` recenters. The **dock** hosts
three tabs — keys `2`/`3`/`4` select, the same key again zooms the tab solo,
`1`/`esc` return to the composite: **chronicle** (default; see below),
**metatron** (the angel transcript — replies stream here, or badge the tab
`metatron •` when it isn't visible; charge bank and charter provenance as
before — [[metatron]]), and **souls** (live agent bodies: status, current
goal, needs gauges, inventory, newest memory line; full soul.md files on disk
per [[agent-mind]]).

The **chronicle** renders the narrated story from the replica's
snapshot-carried `State.Chronicle` ring ([[chronicle]]) or the raw feed (`r`
toggles; raw is the automatic fallback with no narrated entries; `a`/`t`
cycle agent/thread filters). Raw lines follow the grammar in grammar.go
(pure functions): agent indices resolve to names, speech events
(`social.conversation_turn`, `social.rumor_told`) render bright as
`{"Speaker"→"Listener"} "utterance"`, scene summaries and default events dim,
`clock.*` yellow. Pausing puts the visible chronicle into **inspect mode**:
`j`/`k`/`g`/`G` select, `⏎` expands the stored event verbatim —
pretty-printed with `// name` annotations beside integer agent indices.

Input follows the **focus contract** (`docs/design/tui/patterns/focus-contract.md`):
viewing never captures typing; `m` focuses the minibuffer (amber border, inline
`esc release · ⏎ send` hint), `esc` always releases, and no keypress is a
silent no-op — the old rule where the metatron pane owned every key while
active is gone. Time controls (minibuffer unfocused): space toggles
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
navigation, replica application, ring capping, quit behavior, the widescreen layout
math (layout.go), chronicle grammar per event class (grammar.go), focus-contract key
routing in both layouts, exact-height rendering invariants across sizes and dense
content, and resize round-trips with live selection; an expect-driven PTY smoke test
drives the real binary. When real systems land, dock tabs graduate from stubs without
changing the replica machinery.

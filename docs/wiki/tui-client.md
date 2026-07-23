---
name: tui-client
description: The Bubble Tea full-screen client — a widescreen map+dock composite (with minibuffer and narrow single-pane fallback) over a live world replica maintained by log shipping (state snapshot + event subscription through the shared reducer)
kind: component
sources:
  - internal/tui/tui.go
  - internal/tui/views.go
  - internal/tui/layout.go
  - internal/tui/grammar.go
  - internal/tui/digest.go
verified_against: 8c44bf21ad22c0f1bad07ae7f2a08072a0cb5544
---

# TUI client

`internal/tui` is the attachable full-screen client (`promptworld ui <dir>`), built on
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
[[worldmap-generation]]): water ~, wood ♠, forage ", rock outcrops ^, and dens
ᴥ glyphs, plus dynamic overlay state read off the replica (never part of the
static tile) — a quarried-out rock outcrop renders as a faint `,` ahead of the
static terrain check — with the replica's agents on top (by initial,
lowercase asleep, † dead) plus built structures: fires render lit ▲ while the
current tick is before the structure's `FuelUntil` and fall back to a faint,
hollow cold glyph △ once fuel runs out, shelters ⌂, ovens ▣, chests ☐ (spec
013 US3), and the [[gru]] as a red G while it is abroad; ground piles (spec
013 US2, `Model.replica.Piles`) render as a dedicated overlay `%`, layered
like structures rather than folded into them so a coincidental tile overlap
loses neither glyph's priority silently; the camera follows the living
agents' centroid, arrow keys pan, `c` recenters.

Inspection (spec 013 T021/T026, SC-006): the map legend — its one designated
inspection surface, content grows the line rather than adding a second row —
appends, for whatever's currently in view, a stockpile-zone summary per pile
cluster and an owner+contents+fullness entry per chest. Piles in view are
grouped into **stockpile zones** by 4-neighbor Manhattan adjacency
(`pileZones`, a render-side-only flood fill — no zone state, matching
spec.md's "an observability grouping of adjacent piles, not a state entity");
each zone renders as `pile(x,y) contents` (single pile) or
`zone[n](x0,y0)-(x1,y1) contents` (multi-pile, bounding box + count), where
contents (`summarizePileContents`) is non-food resource counts plus a spear
count plus a `food Nr/Nc/Nm` batch total when any food is held. Each visible
chest renders as `chest(x,y) [Owner] contents n/48` (`describeChest`, owner
resolved through the same `agentName` helper the chronicle grammar uses,
contents via `summarizeInventoryContents`, capacity `sim.ChestCap`) — a
chest's `Store` is a plain counts inventory rather than dated batches,
because chests preserve food indefinitely (no rot deadlines to track).

The **dock** hosts three tabs — keys `2`/`3`/`4` select, the same key again
zooms the tab solo, `1`/`esc` return to the composite: **chronicle** (default;
see below), **metatron** (the angel transcript — replies stream here, or
badge the tab `metatron •` when it isn't visible; the pane header shows the charge
bank plus the spec-021 instruction/capability provenance summary — charter
default/custom, skill-file count when non-zero, and the granted-tool summary from
`Status.GrantedTools`, quiet for a full-grant default world — [[metatron]]), and **villagers** (renamed from
"souls", spec 015/TASK-56 — now a two-view inspector rather than a flat
roster). The villagers **roster** shows per agent: a selection cursor,
status, current goal, needs gauges, a leading `bulk n/24` derived-load
reading (spec 013 T015, SC-006; `sim.Bulk`/`sim.BulkCap` — the same function
the reducer/executor clamp gathers and crafts against, so the number never
drifts from what an action will actually do), then the full carried-inventory
line — wood/stone/water/planks/refined-stone counts, the food triplet
raw/cooked/meals, and (when carried) a spear count with the most-worn spear's
remaining uses. While the villagers tab is visible, `j`/`k`/`g`/`G` move the
cursor and `⏎` opens the selected villager's **detail view**
(`villagerDetailBody`): identity/vitals, an objective line (active
`Intent.Goal` marked current; else the reducer-stamped `Agent.LastGoal` +
tick marked `last:`; else "no objective yet" — [[sim-state-reducer]]),
itemized inventory, beliefs/narrative when consolidation has produced them,
and episodic memories most-recent-first, each section truncating bottom-up
inside the pane budget. `esc` closes the detail back to the roster ahead of
the solo-release chain; selection state survives tab switches and is clamped
on reconnect. Full soul.md persona files stay on disk per [[agent-mind]].

The **chronicle** renders the narrated story from the replica's
snapshot-carried `State.Chronicle` ring ([[chronicle]]) or the raw feed (`r`
toggles; raw is the automatic fallback with no narrated entries; `a`/`t`
cycle agent/thread filters). Raw lines follow the **digest grammar** (spec
018/TASK-60; grammar.go + digest.go, pure functions emitting styled segments
— never ANSI): every cataloged event type has a `digestRegistry` entry
turning its payload into a readable per-type summary, so a feed line reads
`TICK HH:MM type summary` — natural phrases for narrative families
(`Ash foraged at (14,9)`, speech privileged as `Ash→Rowan "utterance"`),
compact `key=value` fields for the telemetry families (`cog.*`, `clock.*`,
`daemon.*`). Columns align at solo width (tick right-aligned, type padded);
the narrow dock drops the tick and shortens the type to its last segment.
Families carry color-role tints, key tokens (names, speech, amounts, causes)
carry emphasis, and four high-salience types (`agent.died`, `gru.attacked`,
`social.chest_taken`, `norm.violated`) render whole-line alert. The four
[[metatron-miracles]] types render in the metatron family voice, with a
trailing emphasized `(forced)` annotation (`gratisMark`) whenever the
payload's gratis flag waived the charge — an operator force is never
indistinguishable from a charge-priced miracle in the feed. Unregistered
future types fall back to the compact resolved-name JSON of the old grammar
(the agent-index field table — `agentIndexFields`/`agentIndexFieldRe`,
covering `agent`, `a`, `b`, `from`, `to`, `speaker`, `listener`, `subject`,
`owner`, `taker` — still drives that fallback and the inspector). A sweep
test (`digest_test.go`) fails if any type cataloged in [[event-types]] lacks
a digest. Pausing puts the visible chronicle into **inspect mode**:
`j`/`k`/`g`/`G` select, and the selected event's full detail shows
automatically in an always-on **detail pane** at the panel bottom — seq,
tick, type, the stored payload verbatim, pretty-printed with `// name`
annotations beside integer agent indices; `J`/`K` scroll oversized payloads
within the pane's row budget, and `⏎` is a reserved no-op documented as the
attachment point for future jump-off actions
(`docs/design/tui/patterns/chronicle-grammar.md`, `panels/chronicle.md`).

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
fills the story pane and [[event-types]] the raw feed; [[cli-promptworld]] mounts
it as the `ui` subcommand.

## Operational notes

Rendering requires no daemon round trips — map updates come from pushed events, so the
UI stays smooth at max speed (the chronicle simply scrolls fast). Unit tests cover pane
navigation, replica application, ring capping, quit behavior, the widescreen layout
math (layout.go), the digest grammar (per-family digests + the catalog sweep in
digest_test.go, plain/segment equivalence under wrap), focus-contract key
routing in both layouts, exact-height rendering invariants across sizes and dense
content, and resize round-trips with live selection; an expect-driven PTY smoke test
drives the real binary. When real systems land, dock tabs graduate from stubs without
changing the replica machinery.

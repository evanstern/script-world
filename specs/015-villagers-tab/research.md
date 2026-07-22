# Research: Villagers Tab — per-villager inspection

**Feature**: specs/015-villagers-tab | **Date**: 2026-07-22

All unknowns from Technical Context resolved. Facts verified against the
working tree at planning time (post spec-012/013 era, main).

## R1 — How to remember the most recent objective after it clears

**Decision**: Add reducer-maintained fields to `sim.Agent`:
`LastGoal string` + `LastGoalTick int64` (both `omitempty`), written by
`State.Apply` on `agent.intent_set` (internal/sim/state.go:313) and **never
cleared**. Display rule: `Intent != nil` → active objective; else `LastGoal`
→ past objective ("finished/interrupted"); else "no objective yet".

**Rationale**:
- `State.Apply` is the single reducer shared by the daemon AND the TUI's
  client-side replica (log shipping, internal/tui/tui.go:69 "world replica,
  event-sourced client-side"). One change serves both, and the value rides
  snapshots — a freshly attached observer sees pre-attach history (FR-006).
- Direct precedent: `Agent.IdleSince` is "reducer-maintained so the reflex
  grace is a pure function of event history" (agents.go:71-73);
  `Agent.Generation` (TASK-32) was added with `omitempty` precisely so
  "pre-TASK-32 snapshots stay byte-stable" (agents.go:76-79).
- Setting on `intent_set` (not clearing on `intent_done`/`gru.attacked`/
  hail interrupts) means every clear path is covered for free — there are at
  least three intent-clearing sites in Apply and hooking them all buys
  nothing: "most recent objective" is exactly "last goal set".
- No snapshot format bump: spec 012 bumped format-version 2 because a field
  was *removed* (Inventory.Food); *adding* `omitempty` fields keeps old
  snapshots decoding cleanly (field zero-valued → "no objective yet", which
  FR-006/US2-AC3 explicitly allow) and keeps pre-feature agents byte-stable.
- Replay-determinism tests (e.g. TestReplayDeterminismWithQuarryAndWater,
  TestReplayDeterminismWithHails) compare same-version replays — a new
  deterministic reducer field passes them by construction.

**Alternatives considered**:
- *TUI-side tracking* (map agent→last intent fed by `agent.intent_set`
  events after attach): rejected — misses all history before the snapshot
  the client attached from, so a fresh observer sees "idle" for villagers
  with rich history; duplicates reducer logic client-side against the
  project's "pure function of event history" doctrine.
- *Reconstructing from the event log on demand* (scan chronicle backlog):
  rejected — the TUI's event backlog is bounded/sheddable and the chronicle
  already undercounts history live (TASK-46); unreliable source.
- *Storing a full `LastIntent *Intent` copy*: rejected as over-broad — the
  detail view needs the goal name (and optionally when), not target/res
  coordinates of a finished intent. Two scalar fields keep canonical bytes
  minimal.

## R2 — Selection interaction grammar

**Decision**: When the villagers tab is the visible dock tab (or solo'd),
`j`/`k` move the roster selection, `⏎` opens the selected villager's detail
view, `esc` closes the detail view back to the roster; `esc` on the roster
falls through to the existing solo-release behavior. `g`/`G` jump
first/last, mirroring chronicle inspect.

**Rationale**: this is the chronicle-inspect grammar verbatim
(docs/design/tui/patterns/keymap.md "Mode: inspect": `j/k` select, `⏎`
expand/collapse, esc/space semantics), the project's established selection
idiom. No key collisions: `j/k` and `⏎` only bind per-visible-tab, and the
dock shows one tab at a time; map pan keeps the arrow keys by design ("map
pan keeps the arrow keys; inspect deliberately uses j/k"). The
"esc always releases" focus-contract rule (focus-contract.md rule 3) orders
the release chain: detail → solo → home.

**Alternatives considered**: number-key direct selection (1-8) — rejected,
collides with global pane keys 1-4; arrow keys — rejected, reserved for map
pan globally.

## R3 — Where the tab rename lands

**Decision**: Rename all user-visible "souls"/"SOUL READER" strings to
"villagers"/"VILLAGERS": `paneNames` entry (tui.go:47), tab row, footer
hints (views.go:119), headers (views.go:954, 1007), and the tab-row title in
the widescreen dock. Internal identifiers (`paneSouls`, `soulsBody`) rename
to `paneVillagers`/`villagersBody` for coherence — cheap, package-private.
Design docs that specify the tab (dock.md "Tab: souls", keymap.md `4`,
solo-views.md, home.md where mentioned) are updated in the same PR; persona
`soul.md` files and event/sim vocabulary are untouched (spec assumption).

**Rationale**: SC-003 requires no user-visible "soul"; design docs under
docs/design/tui are the load-bearing spec the tests cite (focus_test.go
references them by name) and must not drift from the shipped grammar.

## R4 — Layout budgets for roster + detail

**Decision**: The tab keeps one renderer with two internal views:
- **Roster** (default): today's per-villager summary lines plus a selection
  cursor glyph on the selected row; same width breakpoints as today
  (wide ≥ 40 cols keeps goal/position/needs/carry; narrow drops to
  name+status+health) and the same "drop trailing agents, never overflow"
  height rule (views.go:995-1005).
- **Detail** (after ⏎): sections in fixed priority order — identity/vitals,
  objective (active or last), inventory, beliefs/narrative, memories
  (most-recent-first). Height budget spends top-down so memories truncate
  first (spec edge case); width < 40 condenses each section the way the
  narrow roster does today.

**Rationale**: follows the dock.md souls mandate ("wrap/condense columns;
drop the least important column first when narrow") and the chronicle's
"shed content, never overflow" rule the current soulsBody already cites.

## R5 — Selection state lifecycle

**Decision**: Model fields `villSelected int` (roster cursor, default 0) and
`villDetail bool` (detail open). Clamp `villSelected` against
`len(m.replica.Agents)` on every render/keypress (replica can arrive late or
be replaced on reconnect — connectedMsg swaps m.replica wholesale,
tui.go:280). Keys are no-ops while `m.replica == nil`. State survives tab
switches (dock.md: "Each tab keeps its own state ... across switches").

**Rationale**: mirrors chronSelected handling (clamped at tui.go:719-722,
827-828) and satisfies FR-011 / the out-of-range edge case.

## R6 — Live updates

**Decision**: nothing special needed — the detail view renders from
`m.replica.Agents[villSelected]` each frame, and the replica is updated by
the same event-application path that feeds the map/chronicle. FR-010/SC-005
fall out of the existing render loop.

## R7 — Testing approach

**Decision**: table-driven unit tests in internal/tui following the existing
suites: grammar/focus tests for j/k/⏎/esc and the esc release chain
(focus_test.go patterns), render tests for roster cursor, detail sections,
narrow/short budgets and no-overflow (render_test.go patterns), plus sim
tests for the new reducer fields: intent_set sets LastGoal, intent_done
preserves it, gru.attacked preserves it, replay determinism, and old-snapshot
(field-absent) decode. Wiki notes touched (tui-client.md, sim-state-reducer.md,
snapshots.md if it lists agents.go) get re-pinned post-merge per Principle IV.

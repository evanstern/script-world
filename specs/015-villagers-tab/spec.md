# Feature Specification: Villagers Tab — per-villager inspection

**Feature Branch**: `015-villagers-tab`

**Created**: 2026-07-22

**Status**: Draft

**Input**: User description: "Replace the TUI's 'souls' dock tab with a 'villagers' tab. The new tab lets the user select an individual villager from the roster and see a detail view for that villager: (1) their full carried inventory, (2) their soul/memories — episodic memories, and the consolidated beliefs/narrative where present, (3) their last / most recent objective (intent goal such as moving, chopping wood, talking) — including when the agent is currently idle, i.e. the most recent intent should remain visible after it completes."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Select a villager and inspect them (Priority: P1)

An observer watching a running world opens the villagers tab (formerly "souls"),
sees the roster of villagers, moves a selection cursor through it with the
keyboard, and picks one villager. The tab then shows that villager's detail
view: identity and status (awake/asleep/dead, position, needs), their full
carried inventory itemized by kind, and their current or most recent objective
(e.g. "chopping wood", "walking to (x,y)", "talking"). Leaving the detail view
returns to the roster.

**Why this priority**: this is the feature — turning a flat read-only roster
into an interactive per-villager inspector. Without selection and a detail
view, nothing else in this spec is reachable.

**Independent Test**: launch the TUI against a running world, open the
villagers tab, move the selection to a villager, open their detail view, and
verify inventory and objective match what the chronicle/map show for that
villager. Delivers value standalone: observers can finally answer "what is
Ash carrying and doing?" without reading the raw event log.

**Acceptance Scenarios**:

1. **Given** the TUI is connected to a running world, **When** the observer
   opens the fourth dock tab, **Then** it is labeled as the villagers view
   (no "soul"/"souls" labeling) and lists every villager with a visible
   selection cursor.
2. **Given** the roster is shown, **When** the observer moves the selection
   with the keyboard and confirms, **Then** the tab shows the selected
   villager's detail view: name, status, needs, full itemized inventory, and
   their objective.
3. **Given** a villager's detail view is open, **When** the observer presses
   the dismiss/back key, **Then** the roster returns with the selection where
   it was left.
4. **Given** a villager's detail view is open, **When** the world advances
   (new events arrive), **Then** the detail view updates live to reflect the
   villager's current state without the observer re-selecting.

---

### User Story 2 - See the most recent objective, even when idle (Priority: P2)

An observer inspects a villager who currently has no active objective (they
finished chopping and are idle). The detail view still shows the most recent
objective the villager pursued (e.g. "chopped wood, finished") rather than
just "idle", so the observer knows what the villager last did.

**Why this priority**: villagers spend much of their time idle between
objectives; a detail view that only shows live intents would read "idle" most
of the time and fail the core question "what has this villager been up to?".

**Independent Test**: watch a villager complete an objective, then open their
detail view while they are idle and verify the completed objective is still
named. Deliverable on top of US1's detail view.

**Acceptance Scenarios**:

1. **Given** a villager is actively pursuing an objective, **When** their
   detail view is shown, **Then** the objective is presented as current/active.
2. **Given** a villager completed or abandoned an objective and is now idle,
   **When** their detail view is shown, **Then** the most recently pursued
   objective is still displayed and marked as past (not presented as active).
3. **Given** a villager has had no objective at all since the observer
   connected and none is recoverable from history, **When** their detail view
   is shown, **Then** the objective area states that plainly (e.g. "no
   objective yet") rather than showing stale or fabricated data.

---

### User Story 3 - Read a villager's soul: memories, beliefs, narrative (Priority: P3)

From a villager's detail view the observer can read that villager's inner
life: their episodic memories (most recent first), and — when nightly
consolidation has produced them — their beliefs and self-narrative.

**Why this priority**: the soul content is the depth layer. It rounds out the
inspector but the tab is already useful (US1+US2) without it, and the data
volume makes it the slice most sensitive to layout budgeting.

**Independent Test**: select a villager known to have memories (any villager
after some world time), verify recent memories are listed; after a night has
passed, verify beliefs/narrative appear for a consolidated villager.

**Acceptance Scenarios**:

1. **Given** a selected villager has episodic memories, **When** their detail
   view is shown, **Then** their memories are listed most-recent-first, and as
   many as fit the available space are shown (never overflowing the pane).
2. **Given** a selected villager has consolidated beliefs and/or a narrative,
   **When** their detail view is shown, **Then** those are visible alongside
   the memories.
3. **Given** a selected villager has no memories yet, **When** their detail
   view is shown, **Then** the memories area states that plainly.

---

### Edge Cases

- Dead villager selected: detail view still renders (status "dead"), showing
  their final inventory, last objective, and memories — the morgue view.
- World not yet connected / no state: the tab shows the existing
  "waiting for world state…" placeholder; selection is inert until state
  arrives.
- Very narrow dock width: the roster stays navigable (name + status at
  minimum) and the detail view condenses/drops sections by importance rather
  than overflowing — same shed-content-never-overflow rule the tab follows
  today.
- Very short pane height: detail sections truncate from the bottom (memories
  first) rather than pushing identity/objective off-screen.
- Selection out of range after reconnect or roster change: selection clamps
  to a valid villager instead of crashing or pointing at nothing.
- Observer keys a confirm with no roster loaded: no-op, no crash.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The fourth dock tab MUST be renamed from "souls" to a
  villagers-named view everywhere the name is user-visible (tab bar, footer
  hints, headers), while keeping its existing tab-select key and
  solo/zoom behavior.
- **FR-002**: The villagers tab MUST present the full roster of villagers
  with a keyboard-movable selection cursor, and MUST support confirming a
  selection to open that villager's detail view and dismissing the detail
  view to return to the roster with selection preserved.
- **FR-003**: The detail view MUST show the selected villager's identity and
  vitals: name, awake/asleep/dead status, position, and needs.
- **FR-004**: The detail view MUST show the selected villager's complete
  carried inventory, itemized by kind with counts, including tools and their
  wear where applicable; empty kinds may be omitted, and an entirely empty
  pack MUST be stated plainly.
- **FR-005**: The detail view MUST show the villager's objective: the active
  objective when one exists, otherwise the most recently pursued objective
  clearly marked as past. When no objective is recoverable, the view MUST say
  so rather than show nothing.
- **FR-006**: The most-recent-objective information MUST survive objective
  completion/abandonment for at least the lifetime of the observer's session,
  and MUST be reconstructed from world history where available so a freshly
  attached observer sees past objectives too.
- **FR-007**: The detail view MUST show the villager's episodic memories,
  most recent first, bounded to the available space.
- **FR-008**: The detail view MUST show consolidated beliefs and narrative
  when the villager has them.
- **FR-009**: All villagers-tab views MUST obey the TUI's existing layout
  discipline: render within the given width/height budget, condense or drop
  the least important content first, and never overflow the pane, in both
  narrow-dock and widescreen/solo layouts.
- **FR-010**: The detail view MUST update live as world events arrive, with
  no re-selection required.
- **FR-011**: Selection state MUST remain valid across reconnects and world
  restarts (clamped to the roster), and interactions MUST be no-ops rather
  than failures when no world state is loaded.

### Key Entities

- **Villager (agent)**: a named inhabitant of the world with status
  (awake/asleep/dead), position, needs, carried inventory, an optional active
  objective, episodic memories, and optional consolidated beliefs/narrative.
- **Objective (intent)**: a goal a villager pursues (move, chop wood, talk,
  etc.) with a target; exists while active, and — for this feature — its most
  recent instance remains observable after it ends.
- **Memory / Belief / Narrative**: the villager's inner-life records —
  timestamped episodic memories, and the consolidated beliefs and
  self-narrative produced overnight.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: From the villagers tab, an observer can answer "what is this
  villager carrying and what are they doing?" for any specific villager in
  under 10 seconds using only the keyboard.
- **SC-002**: For an idle villager who has completed at least one objective
  since world start, the detail view names that most recent objective in 100%
  of cases — "idle with no history shown" never occurs for such a villager.
- **SC-003**: The word "soul" no longer appears in any user-visible surface
  of the tab; the tab is discoverable as the villagers view in the tab bar
  and footer hints.
- **SC-004**: No villagers-tab view ever renders outside its pane bounds at
  any terminal size the TUI already supports (the existing minimum sizes),
  verified across narrow-dock and widescreen/solo layouts.
- **SC-005**: The detail view reflects a state change in the selected
  villager (inventory delta, new memory, objective change) on the next
  render after the event arrives, with no user action.

## Assumptions

- The observer's client already holds all required villager data locally
  (inventory, needs, memories, beliefs, narrative, active objective) via the
  event-sourced replica; no new server/daemon queries are needed to render
  the detail view. The only gap is remembering the most recent objective
  after it ends — how that is tracked (client-side from objective events vs
  a reducer-maintained field) is a plan-phase design decision.
- "Villagers" is the chosen user-facing noun for agents in this tab; internal
  code/event names (agent, soul.md persona files) are out of scope and keep
  their names.
- The roster summary (all villagers at a glance) remains available as the
  tab's default view; the detail view is entered per-villager rather than
  replacing the roster outright.
- Existing global keys (tab switching, solo/zoom, pause, quit) keep working
  from within the villagers tab; the new selection keys follow the TUI's
  existing interaction grammar (same movement/confirm/dismiss conventions as
  the chronicle's selection, per the design docs).
- Scrolling within long memory lists is nice-to-have, not required: bounded
  most-recent-first display satisfies this spec.
- 8 villagers is the roster size today; the design should not hard-fail at
  other counts but need not optimize for large rosters.

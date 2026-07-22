# Panel: dock

The right-hand tab container in the widescreen composite. One tab visible at a
time; the dock is the designated home for every future display or control panel.

## Structure

```
┌─ chronicle │ metatron │ villagers ─┐   ← tab row doubles as the panel title
├────────────────────────────────────┤
│                                    │
│  active tab content                │
│                                    │
└────────────────────────────────────┘
```

- Tab row: active tab bright, inactive dim. A tab with unseen content shows a badge
  dot: `metatron •`.
- Keys `2` chronicle · `3` metatron · `4` villagers select tabs; same key again zooms
  solo ([../pages/solo-views.md](../pages/solo-views.md)).
- Each tab keeps its own state (scroll, filters, input history) across switches.
- Adding a future tab = new title in the row + a content renderer; no new layout.

## Tab: chronicle (default)

The feed panel, specified in [chronicle.md](chronicle.md). Default tab on launch.

## Tab: metatron

The angel conversation transcript — history only; input happens in the minibuffer
([minibuffer.md](minibuffer.md)).

```
┌─ chronicle │ METATRON │ villagers ┐
├───────────────────────────────────┤
│ you   why is Rowan hoarding     │
│       wood?                     │
│ angel Rowan's memory holds      │
│       three nights of Ash       │
│       letting the fire die.     │
│       Trust toward Ash: −2.     │
│ you   what does ash want        │
│ angel ⋮ thinking…               │
└─────────────────────────────────┘
```

- Rows alternate `you` (dim label) / `angel` (accent label); text wraps to tab width.
- While a question is in flight the transcript shows a `⋮ thinking…` row.
- **Reply arrival:** if the metatron tab is visible, the reply streams in place. If
  not, the tab row badges (`metatron •`) — the dock never steals the selected tab.
- Scrollback: newest at bottom, and when the tab is selected it opens scrolled to
  bottom.

## Tab: villagers (TASK-56)

A per-villager inspector: a roster with a keyboard-movable selection cursor, and a
detail view opened per-villager. One renderer, two internal views
(`villDetail` in the model), rendering width-aware exactly like every other dock
tab (wrap/condense columns; drop the least important column first when narrow).

### Roster (default)

Today's per-villager summary line plus a selection cursor glyph (`▌`) on the
selected row. Same width breakpoint as before: wide (≥40 cols) keeps
name/status/goal/position, the needs bars, and the full carried-inventory line;
narrow drops to name + status + health. Rows beyond the height budget are
dropped from the bottom (never a partial row).

```
┌─ chronicle │ metatron │ VILLAGERS ┐
├───────────────────────────────────┤
│▌ Ash    awake · chop · (12,9)     │
│    health █████ food ███░░ ...    │
│    carry 2w 0st ... · spear 1(2)  │
│  Rowan  asleep · idle · (4,3)     │
│    health ████░ food ████░ ...    │
│    carry 0w 0st ...               │
└───────────────────────────────────┘
```

- `j`/`k` move the cursor, `g`/`G` jump first/last, `⏎` opens the selected
  villager's detail view (contracts/state-and-keys.md).
- Selection is clamped to the roster and survives tab switches, reconnects, and
  world restarts (out-of-range clamps rather than crashing or pointing at
  nothing).

### Detail (after `⏎`)

Sections render in a fixed priority order and truncate from the **bottom** when
height runs short — memories are shed first, identity/objective/inventory are
never pushed off-screen:

1. **identity/vitals** — name, awake/asleep/dead status (dead villagers still
   render — the morgue view), position, needs bars.
2. **objective** — the active objective (`Intent.Goal` + target, marked
   *current*) when one exists; otherwise the most recently pursued objective
   (`LastGoal` + the tick it was set, marked *last*) if any intent was ever set;
   otherwise "no objective yet". The most-recent-objective value is
   reducer-maintained (`sim.Agent.LastGoal`/`LastGoalTick`, set on
   `agent.intent_set`, never cleared) so it survives objective completion and a
   freshly attached observer sees history via the snapshot, not just live events.
3. **inventory** — every carried kind itemized with counts, spear wear included;
   an entirely empty pack says so plainly rather than rendering nothing.
4. **beliefs/narrative** — consolidated beliefs and self-narrative, shown only
   when nightly consolidation has produced them; silently omitted otherwise.
5. **memories** — episodic memories, most recent first, bounded to whatever
   height remains after the sections above; "no memories yet" when empty.

```
┌─ chronicle │ metatron │ VILLAGERS ┐
├───────────────────────────────────┤
│ ASH                                │
│                                    │
│ Ash · awake · (12,9)               │
│ health █████ food ███░░ rest ████░ │
│                                    │
│ objective: chop → (13,9) (current) │
│                                    │
│ inventory:                         │
│   wood 2                           │
│   spears 1 (uses left: 2)          │
│                                    │
│ memories                           │
│ 08:41 (4★) chopped a fine oak      │
│ 07:02 (2★) shared bread with Rowan │
└───────────────────────────────────┘
```

- `esc` closes the detail view back to the roster, with the roster's selection
  preserved — before the existing solo-release chain (`esc` again releases solo,
  same "esc always releases" ordering as everywhere else,
  [../patterns/focus-contract.md](../patterns/focus-contract.md) rule 3).
- The detail view updates live as world events arrive — it renders straight from
  the replica each frame, so no re-selection is ever required.

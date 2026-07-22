# Panel: chronicle

The event feed. Same component everywhere it appears — dock tab, solo view,
narrow-fallback pane — differing only in width. Line formatting is governed by
[../patterns/chronicle-grammar.md](../patterns/chronicle-grammar.md); this doc
covers the panel's modes.

## Mode 1 — running (clock unpaused)

```
┌─ chronicle │ metatron │ villagers ┐
├─────────────────────────────────┤
│  8801 08:09  agent.foraged      │
│    Ash foraged at (14,9)        │
│  8846 08:11  social.conv_turn   │
│    Ash→Rowan "the fire's low    │
│    again"                       │
│  8850 08:11  social.conv_turn   │
│    Rowan→Ash "I stacked wood    │
│    at dawn"                     │
│  8852 08:12  social.conversation│
│    "argued about firewood" · 6…│
└─────────────────────────────────┘
```

- Auto-follows the tail: new events append at the bottom and the view sticks to
  newest. No manual scrolling while running — pausing is the way to read closely
  (the daemon halts, so nothing scrolls away; this is deliberate).
- Narrow widths (dock) drop the tick column, show the type's short name
  (last `.` segment), and wrap an event to ≤ 3 lines then truncate with `…`;
  solo width keeps the tick column and one line per event, truncating with
  `…`. Full verbatim payload is always in the always-on detail pane below
  (Mode 2) — no extra keypress required.
- `r` toggles raw feed ↔ narrated view (existing narrator entries), `a` / `t`
  filter by agent / thread — existing behaviors, preserved in both modes.

## Mode 2 — inspect (clock paused)

Entered automatically whenever the clock is paused and the chronicle is visible.
Title row switches to show the inspect keys. **TASK-60 (spec
018-chronicle-digest)**: the ⏎-triggered inline expansion this doc used to
describe is gone — the detail pane is now always on, no extra keypress
needed, and stays a fixed-height bottom split so the list above it never
reflows as content changes size (the old expansion's biggest complaint).

Implementation note (TASK-34, extended TASK-60): inspect selects over the
**raw event feed** — selection needs `seq`/`tick`/`type`/`payload`, a shape
narrated chronicle entries (prose, no per-event payload) don't carry. `r`
still records the raw↔narrated preference while paused (it takes effect the
moment the clock resumes), but the inspect view itself always renders
raw-formatted (digest-grammar) rows; this is why every inspect mockup on
this page and in solo-views.md shows digest grammar, never prose.

```
┌─ CHRONICLE · paused — j/k select · J/K scroll detail ─┐
│   8801 08:09  agent.foraged   Ash foraged at (14,9)   │
│▌  8846 08:11  social.conversation_turn  Ash→Rowan "…" │
│   8850 08:12  agent.died      Birch died: exposure    │
├─ DETAIL · seq 1202 ───────────────────────────────────┤
│ {                                                     │
│   "seq": 1202, "tick": 8846,                          │
│   "type": "social.conversation_turn",                 │
│   "payload": { … "speaker": 1,   // Rowan … }         │
│ }                                                     │
│ … (+12 more — J to scroll)        [future: actions]   │
└───────────────────────────────────────────────────────┘
```

- `j`/`k` move the selection (background-highlighted row), `g`/`G` jump to
  first/last. The detail pane always shows the currently-selected event —
  no `⏎` needed.
- The detail pane occupies the bottom `paneRows = min(rows/2, 14)` rows of
  the panel body, separated from the list by a `DETAIL · seq N` rule line;
  the list above keeps whatever remains, floored at 5 rows so it's never
  crowded out entirely by a tall pane.
- The pane shows the **verbatim stored event** pretty-printed — `seq`,
  `tick`, `type`, raw `payload` — with resolved agent names as `// name`
  annotations beside integer indices (see grammar doc). Never rewrite the
  payload itself. Content taller than the pane scrolls with `J`/`K`
  (shift-j/k); a footer (`… (+N more — J to scroll)`) marks how much more
  there is. Selecting a different event resets the scroll back to the top.
- Oversized payloads (`world.migrated`, which embeds the full `sim.State`)
  are windowed the same way as any other payload — only the visible slice
  is ever processed, so an enormous embedded state can't blow the panel's
  row budget; every line is still reachable by scrolling.
- **Extension point**: the pane's bottom-right `[future: actions]` slot and
  the reserved `⏎` key are the documented attachment surface for future
  jump-off actions (e.g. "jump to the referenced event"). `⏎` is a no-op
  today — deliberately, so a future feature has an honest, already-wired
  place to land instead of an undocumented key. The code hook is
  `detailActions(e store.Event) []detailAction` (`internal/tui/tui.go`),
  returning `nil` until something implements it.
- On resume: clear the selection and the detail pane's scroll, snap back to
  tail-follow, return to running mode.
- Selection is remembered while paused even if the user switches tabs and
  returns; the detail scroll resets whenever the selection moves.

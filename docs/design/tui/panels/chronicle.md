# Panel: chronicle

The event feed. Same component everywhere it appears — dock tab, solo view,
narrow-fallback pane — differing only in width. Line formatting is governed by
[../patterns/chronicle-grammar.md](../patterns/chronicle-grammar.md); this doc
covers the panel's modes.

## Mode 1 — running (clock unpaused)

```
┌─ chronicle │ metatron │ villagers ┐
├─────────────────────────────────┤
│ #1198 08:09 agent.talked        │
│   {"a":"Sable","b":"Birch"}     │
│ #1201 08:11 turn                │
│   {"Ash"→"Rowan"} "the fire's   │
│   low again"                    │
│ #1202 08:11 turn                │
│   {"Rowan"→"Ash"} "I stacked    │
│   wood at dawn"                 │
│ #1204 08:12 convo {"gist":…}    │
└─────────────────────────────────┘
```

- Auto-follows the tail: new events append at the bottom and the view sticks to
  newest. No manual scrolling while running — pausing is the way to read closely
  (the daemon halts, so nothing scrolls away; this is deliberate).
- Narrow widths (dock) wrap an event to ≤ 3 lines then truncate with `…`; solo
  width keeps one line per event. Full payload is always in the inspector.
- `r` toggles raw feed ↔ narrated view (existing narrator entries), `a` / `t`
  filter by agent / thread — existing behaviors, preserved in both modes.

## Mode 2 — inspect (clock paused)

Entered automatically whenever the clock is paused and the chronicle is visible.
Title row switches to show the inspect keys.

Implementation note (TASK-34): inspect selects and expands over the **raw event
feed** — selection/expansion need `seq`/`tick`/`type`/`payload`, a shape narrated
chronicle entries (prose, no per-event payload) don't carry. `r` still records the
raw↔narrated preference while paused (it takes effect the moment the clock resumes),
but the inspect view itself always renders raw-formatted rows; this is why every
inspect mockup on this page and in solo-views.md shows raw grammar, never prose.

```
┌─ CHRONICLE · paused — j/k select · ⏎ expand ─┐
├──────────────────────────────────────────────┤
│ #1201 08:11 turn {"Ash"→"Rowan"} "the fire…" │
│▌#1202 08:11 turn  ◂ expanded                 │
│▌  {                                          │
│▌    "seq": 1202, "tick": 8846,               │
│▌    "type": "social.conversation_turn",      │
│▌    "payload": {                             │
│▌      "conv": 102,                           │
│▌      "speaker": 1,   // Rowan               │
│▌      "listener": 0,  // Ash                 │
│▌      "text": "I stacked wood at dawn"       │
│▌    }                                        │
│▌  }                                          │
│ #1203 08:12 rumor_told {"Birch"→"Sable"} "…" │
└──────────────────────────────────────────────┘
```

- `j`/`k` move the selection (background-highlighted row), `g`/`G` jump to
  first/last, `⏎` expands/collapses the selected event inline.
- Expansion shows the **verbatim stored event** pretty-printed — `seq`, `tick`,
  `type`, raw `payload` — with resolved agent names as `// name` annotations beside
  integer indices (see grammar doc). Never rewrite the payload itself.
- One event expanded at a time; expanding another collapses the first.
- On resume: collapse everything, snap back to tail-follow, return to running mode.
- Selection is remembered while paused even if the user switches tabs and returns.

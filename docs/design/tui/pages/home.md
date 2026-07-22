# Page: home (widescreen composite)

The default view whenever the terminal is wide enough (see
[../patterns/layout.md](../patterns/layout.md) for the breakpoint). Replaces the
current "one pane at a time" model as the resting state of the app.

## Mockup

```
 script-world · attached · day 4 · 08:12 · 1×                4 villagers awake
 ┌─ MAP · following centroid ────────────────┐ ┌─ chronicle │ metatron │ villagers ┐
 │ ~ ~ ~ ~ " " ♠ ♠ ♠ ♠ ♠ " " . . . . ▲ . .   │ │ #1198 08:09 agent.talked      │
 │ ~ ~ ~ " " ♠ ♠ A ♠ ♠ " " . . ⌂ ⌂ . . . .   │ │   {"a":"Sable","b":"Birch"}   │
 │ ~ ~ " ♠ ♠ ♠ R ♠ " " . . . ⌂ . B . . .     │ │ #1201 08:11 turn              │
 │ ~ ~ . " " ♠ ♠ " . . . . . . . . . . .     │ │   {"Ash"→"Rowan"} "the fire's │
 │ ~ . . ᴥ . " " . . . . S . . . " " . .     │ │   low again"                  │
 │ ~ . . . . . . . . . . . . " " ♠ ♠ . .     │ │ #1202 08:11 turn              │
 │ ~ ~ . . . . " " . . . . . ♠ ♠ ♠ ♠ . .     │ │   {"Rowan"→"Ash"} "I stacked  │
 │                                           │ │   wood at dawn"               │
 │ ~ water ♠ wood " forage ᴥ den ▲ fire ⌂ sh │ │ #1204 08:12 convo {"gist":…}  │
 └───────────────────────────────────────────┘ └───────────────────────────────┘
 ┌─ METATRON ────────────────────────────────────────────────────────────────┐
 │ ⏎ m — speak with the angel…                                               │
 └───────────────────────────────────────────────────────────────────────────┘
  2 chronicle 3 metatron 4 villagers (again: solo) · m ask · space pause · q quit
```

## Composition

| Region | Spec | Notes |
|---|---|---|
| header | 1 row | existing header content; `PAUSED` indicator in yellow when paused |
| map | left, all remaining columns | [../panels/map.md](../panels/map.md) |
| dock | right, fixed width | [../panels/dock.md](../panels/dock.md); chronicle tab is the default on launch |
| minibuffer | full width, above footer | [../panels/minibuffer.md](../panels/minibuffer.md) |
| footer | 1 row of key hints | hints change with mode — see [../patterns/keymap.md](../patterns/keymap.md) |

## Behavior

- This page is always "underneath": solo views and the narrow fallback are the only
  things that replace it, and `1` / `esc` (when nothing is focused) always return here.
- Map and dock update live simultaneously — no region ever freezes because another
  has attention.
- Pausing the clock changes chronicle-tab behavior (inspect mode) but never the layout.
- Arrow keys pan the map from this page as long as the minibuffer is not focused,
  regardless of which dock tab is selected.

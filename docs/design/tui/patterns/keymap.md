# Pattern: keymap

Every key, every mode. Three modes total; a key means one thing per mode. The
footer always shows the current mode's primary hints.

## Mode: global (minibuffer unfocused — the normal state)

| Key | Action |
|---|---|
| `1` | home composite (from solo: return home; on home: map is already primary) |
| `2` / `3` / `4` | select dock tab chronicle / metatron / villagers; **same key again** → solo zoom; again → back home |
| `m` | focus the minibuffer |
| `space` | pause / resume the clock |
| `[` / `]` | speed down / up |
| `←↑↓→` | pan the map |
| `c` | recenter camera (resume following) |
| `r` | chronicle: toggle raw ↔ narrated |
| `a` / `t` | chronicle: filter by agent / thread |
| `q` | quit |
| `ctrl+c` | quit (from **any** mode) |

## Mode: minibuffer focused (after `m`)

| Key | Action |
|---|---|
| printable keys, `space` | append to buffer (visibly) |
| `backspace` | delete |
| `↑` / `↓` | input history |
| `⏎` | send (empty buffer: release focus instead) |
| `esc` | release focus |
| `ctrl+c` | quit |

No other key does anything silently — see
[focus-contract.md](focus-contract.md) rule 4.

## Mode: inspect (clock paused + chronicle visible; layered on global)

**TASK-60 (spec 018-chronicle-digest)**: the detail pane is always on (no
`⏎` needed to see it — panels/chronicle.md "Mode 2"); `⏎` is reserved for a
future jump-off actions bar (contract §5 "Extension point") rather than
toggling expansion, and `J`/`K` scroll the pane when its content overflows.

| Key | Action |
|---|---|
| `j` / `k` | select next / previous event (also resets detail pane scroll) |
| `g` / `G` | jump to first / last (also resets detail pane scroll) |
| `J` / `K` | scroll the detail pane down / up |
| `⏎` | reserved — no-op today (future jump-off actions) |
| `space` | resume (exits inspect, clears selection and detail scroll) |

All global keys stay live in inspect mode; `j/k/g/G/J/K` are additions, not
replacements. (Map pan keeps the arrow keys; inspect deliberately uses `j/k`
so the two never collide. `J`/`K` mirror `j`/`k` one layer up — selecting a
row vs. scrolling what it shows.)

## Mode: villagers (the villagers tab is the thing visible; layered on global, TASK-56)

Scoped to whichever tab is on screen — the dock shows one tab at a time, so this
never collides with inspect mode's `j/k/J/K` (chronicle) or the map's arrow-pan.
Unlike inspect mode, this does **not** require the clock to be paused.

| Key | Roster view | Detail view |
|---|---|---|
| `j` / `k` | select next / previous villager (clamped) | no-op |
| `g` / `G` | jump to first / last villager | no-op |
| `⏎` | open detail for the selected villager | no-op |
| `esc` | (falls through to global — releases solo/home) | close detail → back to roster |

`esc` follows "esc always releases" ordering
([focus-contract.md](focus-contract.md) rule 3): minibuffer → villager detail →
solo → home — each press of `esc` releases exactly one layer. With no world state
loaded (or an empty roster) `j/k/g/G/⏎` are strict no-ops.

## Footer hints per mode

```
global            2 chronicle 3 metatron 4 villagers (again: solo) · m ask · space pause · q quit
minibuffer        esc release · ⏎ send · ↑↓ history
inspect           j/k select · J/K scroll detail · space resume · m ask
villagers roster  j/k select · ⏎ inspect · space pause · q quit
villagers detail  esc back · space pause · q quit
```

## Migration notes

- `tab`/`shift+tab` pane cycling may remain as aliases for dock-tab cycling; not
  load-bearing.
- Today's "keys 1–4 swap the whole screen" behavior survives only in the narrow
  fallback ([../pages/solo-views.md](../pages/solo-views.md)).

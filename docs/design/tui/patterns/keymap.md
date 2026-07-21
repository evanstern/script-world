# Pattern: keymap

Every key, every mode. Three modes total; a key means one thing per mode. The
footer always shows the current mode's primary hints.

## Mode: global (minibuffer unfocused — the normal state)

| Key | Action |
|---|---|
| `1` | home composite (from solo: return home; on home: map is already primary) |
| `2` / `3` / `4` | select dock tab chronicle / metatron / souls; **same key again** → solo zoom; again → back home |
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

| Key | Action |
|---|---|
| `j` / `k` | select next / previous event |
| `g` / `G` | jump to first / last |
| `⏎` | expand / collapse selected event |
| `space` | resume (exits inspect, collapses expansion) |

All global keys stay live in inspect mode; `j/k/g/G/⏎` are additions, not
replacements. (Map pan keeps the arrow keys; inspect deliberately uses `j/k` so the
two never collide.)

## Footer hints per mode

```
global      2 chronicle 3 metatron 4 souls (again: solo) · m ask · space pause · q quit
minibuffer  esc release · ⏎ send · ↑↓ history
inspect     j/k select · ⏎ expand · space resume · m ask
```

## Migration notes

- `tab`/`shift+tab` pane cycling may remain as aliases for dock-tab cycling; not
  load-bearing.
- Today's "keys 1–4 swap the whole screen" behavior survives only in the narrow
  fallback ([../pages/solo-views.md](../pages/solo-views.md)).

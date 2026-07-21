# Panel: map

The terrain camera viewport. **Rendering is unchanged** from the current
`mapView` (`internal/tui/views.go`) — glyph grid, 2 terminal columns per tile,
camera following the wanderer centroid, night dimming. This doc only respecifies
its sizing and its place in the composite.

## Mockup (in-composite)

```
┌─ MAP · following centroid ────────────────┐
│ ~ ~ ~ ~ " " ♠ ♠ ♠ ♠ ♠ " " . . . . ▲ . .   │
│ ~ ~ ~ " " ♠ ♠ A ♠ ♠ " " . . ⌂ ⌂ . . . .   │
│ ~ ~ " ♠ ♠ ♠ R ♠ " " . . . ⌂ . B . . .     │
│ ~ . . ᴥ . " " . . . . S . . . " " . .     │
│                                           │
│ ~ water ♠ wood " forage ᴥ den ▲ fire ⌂ ⌂  │
└───────────────────────────────────────────┘
```

## Sizing

- Composite mode: map gets **all columns left of the dock** (see
  [../patterns/layout.md](../patterns/layout.md)); viewport tiles =
  `(mapCols - borders) / 2` wide, `bodyRows - legend` tall — same formula family as
  today's `vw/vh` computation, with the new width input.
- Solo/fallback mode: full terminal width, as today.
- Viewport stays a camera window clamped to world size — never letterboxed, never
  scaled.

## Behavior

- Title row states the camera mode: `following centroid` or `panned (c to recenter)`.
- Arrow keys pan whenever the minibuffer is unfocused — from home, regardless of
  selected dock tab. `c` recenters and resumes following.
- Legend stays pinned as the panel's last row; drop the legend before shrinking the
  viewport when rows get scarce.
- Agents render as name-initial glyphs (existing behavior). Future: the souls tab or
  an inspect selection may highlight an agent on the map — out of scope for TASK-34,
  don't preclude it.

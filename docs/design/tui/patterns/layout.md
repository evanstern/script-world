# Pattern: layout

Breakpoints, width math, and style tokens for the widescreen composite. Inputs are
`tea.WindowSizeMsg` width/height, as today.

## Breakpoint

```
width ≥ 112 cols  →  widescreen composite (pages/home.md)
width <  112 cols →  narrow fallback: today's single-pane UI, unchanged
```

112 is the narrowest width where an even 50/50 split (see "Column budget" below)
still leaves both the map and the dock genuinely usable — below it, halving the
terminal would starve both regions rather than either one, so the narrow single-pane
fallback takes over instead of shrinking further. (Implementation note, TASK-34: two
earlier drafts of this section described a fixed-44-col dock with the map holding a
73-col target as the terminal narrowed toward the breakpoint; a planning decision
during implementation replaced that with a straight 50/50 split — see "Column budget".
The breakpoint value itself (112) did not need to change: at 112 the split gives
map=56 / dock=55, both still comfortably usable.)
Resizing across the breakpoint swaps layouts live without losing state. Height has no
breakpoint: rows get scarce → panels shed their lowest-priority rows (map legend
first).

## Column budget (widescreen)

```
totalCols
├─ gutter: 1 col
├─ dock:   (totalCols − gutter) / 2            (floors down on an odd split)
└─ map:    totalCols − gutter − dock           (takes the odd leftover column;
                                                 viewport tiles = (mapCols − 4) / 2 —
                                                 2 cols border + 2 cols padding)
```

Map and dock split the terminal 50/50 — a planning decision (TASK-34) superseding an
earlier fixed-44-col dock. The map takes the extra column when `totalCols − gutter` is
odd, so it is never smaller than the dock.

## Row budget (widescreen)

```
totalRows
├─ header:     1
├─ body:       remainder (map panel ∥ dock panel — both full body height)
├─ minibuffer: 3 (bordered single line)
└─ footer:     1
```

Solo views: the solo panel takes the whole body; minibuffer + footer persist.

## Style tokens

One named Lipgloss style per role, defined once beside the existing styles in
`views.go` — panels refer to roles, never to raw colors:

| Token | Role | Today's anchor |
|---|---|---|
| `panelBorder` | dormant panel/dock borders | existing `styleBox` rounded border |
| `panelFocus` | focused minibuffer border + title | yellow, same hue as `PAUSED` |
| `tabActive` / `tabInactive` / `tabBadge` | dock tab row | badge dot = `metatron •` |
| `feedDim` | seq, time, default payloads | existing faint/dim style |
| `feedType` | event type column | — |
| `feedName` | `{"A"→"B"}` speaker pairs | — |
| `feedSpeech` | quoted utterances (brightest text on screen) | — |
| `feedClock` | `clock.*` events | yellow |
| `feedSelect` | inspect-mode selected row | background highlight |

Map glyph colors are unchanged (existing terrain/agent styles, night dimming).

## Composition notes

- Panels are composed with `lipgloss.JoinHorizontal/JoinVertical` over
  independently rendered strings — same technique `View()` uses today, two columns
  instead of one.
- Every panel is handed its exact `(width, height)` and must render to it; no panel
  measures the terminal itself. This is the contract that makes dock-tab vs. solo
  "same component, two widths" work.
- Implementation note (TASK-34, B1): rendering to *exactly* the handed height is a
  hard requirement, not an aspiration — Bubble Tea scrolls a taller-than-terminal
  `View()` up, which pushes the header off the top of the screen. Two lipgloss facts
  make this easy to violate by accident: `Style.Height()` only *pads* short content,
  it never truncates tall content, so one overlong content line silently grows a
  panel instead of erroring; and a style's own `Padding(0,1)` eats 2 columns out of
  whatever `Width()` was set to, before any text renders, so the truly safe content
  width is `Width − 2`, not `Width`. Every panel body in `views.go` clips each
  content line to that true width before handing it to a bordered/padded box
  (`clipContent`) rather than relying on lipgloss's own wrapping to stay in bounds.

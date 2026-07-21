# Pattern: layout

Breakpoints, width math, and style tokens for the widescreen composite. Inputs are
`tea.WindowSizeMsg` width/height, as today.

## Breakpoint

```
width ≥ 112 cols  →  widescreen composite (pages/home.md)
width <  112 cols →  narrow fallback: today's single-pane UI, unchanged
```

112 is the point where the dock has shrunk to its 36-col floor (see "Column budget"
below) against a map holding its 73-col target — below that, the composite has
nothing left to give up and the narrow fallback takes over. (Implementation note,
TASK-34: an earlier draft of this line quoted a 64-col map + a fixed 44-col dock at
the breakpoint; that arithmetic assumed the dock stays at its max width down to 112,
which contradicts "shrink dock before map" below. The Column budget section is
authoritative — the dock is what gives up columns between 118 and 112, not the map.)
Resizing across the breakpoint swaps layouts live without losing state. Height has no
breakpoint: rows get scarce → panels shed their lowest-priority rows (map legend
first).

## Column budget (widescreen)

```
totalCols
├─ dock:      44 cols fixed  (min 36 — shrink dock before map below 118 cols)
├─ gutter:    1 col
└─ map:       remainder      (viewport tiles = (mapCols − borders) / 2)
```

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

# Panel: dock

The right-hand tab container in the widescreen composite. One tab visible at a
time; the dock is the designated home for every future display or control panel.

## Structure

```
┌─ chronicle │ metatron │ souls ─┐   ← tab row doubles as the panel title
├────────────────────────────────┤
│                                │
│  active tab content            │
│                                │
└────────────────────────────────┘
```

- Tab row: active tab bright, inactive dim. A tab with unseen content shows a badge
  dot: `metatron •`.
- Keys `2` chronicle · `3` metatron · `4` souls select tabs; same key again zooms
  solo ([../pages/solo-views.md](../pages/solo-views.md)).
- Each tab keeps its own state (scroll, filters, input history) across switches.
- Adding a future tab = new title in the row + a content renderer; no new layout.

## Tab: chronicle (default)

The feed panel, specified in [chronicle.md](chronicle.md). Default tab on launch.

## Tab: metatron

The angel conversation transcript — history only; input happens in the minibuffer
([minibuffer.md](minibuffer.md)).

```
┌─ chronicle │ METATRON │ souls ──┐
├─────────────────────────────────┤
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

## Tab: souls

The existing souls view content, re-homed at dock width — **content unchanged**,
rendering made width-aware (wrap/condense columns; drop the least important column
first when narrow).

```
┌─ chronicle │ metatron │ SOULS ──┐
├─────────────────────────────────┤
│ Ash    ⌂ shelter   mood −1      │
│ Rowan  ♠ woods     mood +1      │
│ Birch  " foraging  mood  0      │
│ Sable  ᴥ den       mood  0      │
└─────────────────────────────────┘
```

(Rows above are representative — keep whatever fields the current souls pane shows.)

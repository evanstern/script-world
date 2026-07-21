# Panel: minibuffer (Metatron input)

The **only text input in the app**. One bordered line above the footer, present on
every widescreen page (home and solo views). Replaces typing directly into the
Metatron pane. Governed by the focus contract
([../patterns/focus-contract.md](../patterns/focus-contract.md)); transcript and
replies live in the dock's metatron tab ([dock.md](dock.md)).

## States

### 1 · Dormant (default)

```
┌─ METATRON ────────────────────────────────────────────────┐
│ ⏎ m — speak with the angel…                               │
└───────────────────────────────────────────────────────────┘
```

Dim border, dim placeholder that names the focus key. Zero keyboard ownership —
every global key works.

### 2 · Focused (`m`)

```
┌─ METATRON ────────────────────────────────────────────────┐  ← amber border
│ why did rowan lie about the wood▌       esc release · ⏎ send
└───────────────────────────────────────────────────────────┘
```

Amber border + live cursor + the exit hint rendered **inside the panel chrome**,
right-aligned. The focused state documents its own escape, every time it is drawn.

### 3 · Busy (question sent)

```
┌─ METATRON ────────────────────────────────────────────────┐
│ ⋮ the angel is answering…                    esc to background
└───────────────────────────────────────────────────────────┘
```

- Focus is released automatically on send; `esc` (or any navigation) just proceeds —
  busy never blocks the UI.
- When the reply arrives: if the dock is on the metatron tab it streams there;
  otherwise the tab badges `metatron •` and the minibuffer flashes one dim line
  (`answer arrived — 3 to read`) before returning to dormant.

## Rules

- Input history: `↑`/`↓` while focused cycle previous questions (session-scoped).
- Multi-line input is out of scope; long questions wrap within the single logical line.
- `⏎` on an empty buffer releases focus (no-op send).
- The minibuffer is chromeless-adjacent to the footer: footer hints while focused
  shrink to the minibuffer-mode keys only (see
  [../patterns/keymap.md](../patterns/keymap.md)).
- IPC send/receive is the existing Metatron console protocol
  (`specs/005-metatron/contracts/console-protocol.md`) — transport unchanged, only
  the surface moves.

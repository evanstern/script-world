# TUI design reference — widescreen (TASK-34)

Decision record and index for the script-world TUI redesign. This directory is the
source of truth for implementing the widescreen layout: an implementer should be able
to build every screen from these files without re-deriving intent.

## Decision

**Chosen direction: B + C hybrid** (from the TASK-34 spike mockups).

- **B — tabbed dock:** the right side of the widescreen composite is a single dock
  with tabs (chronicle · metatron · souls). One tab visible at a time; the dock is
  the extension point for future displays/controls.
- **C — Metatron minibuffer:** Metatron's *input* leaves the pane system entirely and
  becomes a one-line minibuffer above the footer — the only text input in the app.
  Angel replies land in the dock's **metatron** tab (this resolves C's open question
  about where replies live).
- Option A (stacked right rail) was considered and rejected: its three-way rail split
  starves the chronicle of rows and its docked one-line Metatron loses history.

Visual mockups from the spike: https://claude.ai/code/artifact/dfb04194-b379-4733-a586-9882b5e0746e
(exploratory; where it disagrees with these files, these files win).

## Region anatomy

Every widescreen frame is composed of exactly five regions:

```
┌─ header ─ script-world · day 4 · 08:12 · 1× · [PAUSED] ──────────────────┐
│ ┌─ MAP ──────────────────────────────┐ ┌─ DOCK ───────────────────────┐  │
│ │                                    │ │  chronicle │ metatron │ souls│  │
│ │  camera viewport over terrain      │ ├──────────────────────────────┤  │
│ │  (existing renderer, resized)      │ │                              │  │
│ │                                    │ │  active tab content          │  │
│ │                                    │ │                              │  │
│ └────────────────────────────────────┘ └──────────────────────────────┘  │
│ ┌─ METATRON minibuffer (dormant: 1 dim line · focused: amber border) ─┐  │
│ └──────────────────────────────────────────────────────────────────────┘ │
└─ footer ─ key hints ──────────────────────────────────────────────────────┘
```

## Files

Pages (what fills the terminal):

- [pages/home.md](pages/home.md) — the widescreen composite, the app's default view
- [pages/solo-views.md](pages/solo-views.md) — full-width zoom of any dock tab; narrow-terminal fallback

Panels (regions inside a page):

- [panels/map.md](panels/map.md) — terrain camera viewport
- [panels/dock.md](panels/dock.md) — tab container; souls + metatron tab content
- [panels/chronicle.md](panels/chronicle.md) — the feed panel: running scroll + paused inspect
- [panels/minibuffer.md](panels/minibuffer.md) — the Metatron input line and its states

Patterns (cross-cutting rules, apply everywhere):

- [patterns/focus-contract.md](patterns/focus-contract.md) — who owns the keyboard, when
- [patterns/chronicle-grammar.md](patterns/chronicle-grammar.md) — event line format + JSON inspector
- [patterns/keymap.md](patterns/keymap.md) — every key, every mode
- [patterns/layout.md](patterns/layout.md) — breakpoints, width math, style tokens

## Ground rules for the implementer

1. These docs specify *behavior and composition*, not Go structure. Reuse the existing
   renderers (`internal/tui/views.go`) wherever a panel says "content unchanged".
2. The narrow-terminal fallback preserves today's single-pane UI — never delete it.
3. Mockup content (agent names, event payloads) is representative; bind to the real
   event types in `internal/sim` and names from the replica. Where a doc names an
   event type it exists in the codebase today.
4. Any deviation forced by implementation reality gets recorded back into these files
   in the same PR — the reference must stay true after the build.

# Page: solo views + narrow fallback

Two ways the home composite gets replaced: deliberately (solo zoom) and by
necessity (terminal too narrow).

## Solo zoom

Pressing a dock tab's key **twice** zooms that tab to full width (first press selects
the tab in the dock; second press, while it is already selected, goes solo). The map
is solo-able the same way with `1`.

```
state machine (per key k ∈ {2,3,4}):
  home, tab≠k   --k-->  home, tab=k
  home, tab=k   --k-->  solo(k)
  solo(k)       --k-->  home, tab=k          (same key toggles back)
  solo(k)       --1 or esc-->  home, tab=k
```

### Mockup — solo chronicle (`2` `2`)

```
 script-world · attached · day 4 · 08:12 · 1×                          PAUSED
 ┌─ CHRONICLE · raw · paused — j/k select · ⏎ expand · r narrated ──────────┐
 │ #1198 08:09 agent.talked              {"a":"Sable","b":"Birch"}          │
 │ #1201 08:11 social.conversation_turn  {"Ash"→"Rowan"} "the fire's low ag…│
 │▌#1202 08:11 social.conversation_turn  ◂ expanded                        ▐│
 │▌  {                                                                     ▐│
 │▌    "seq": 1202, "tick": 8846,                                          ▐│
 │▌    "type": "social.conversation_turn",                                 ▐│
 │▌    "payload": {                                                        ▐│
 │▌      "conv": 102,                                                      ▐│
 │▌      "speaker": 1,     // Rowan                                        ▐│
 │▌      "listener": 0,    // Ash                                          ▐│
 │▌      "text": "I stacked wood at dawn, ask Birch"                       ▐│
 │▌    }                                                                   ▐│
 │▌  }                                                                     ▐│
 │ #1203 08:12 social.rumor_told         {"Birch"→"Sable"} "ash lets the f…"│
 └───────────────────────────────────────────────────────────────────────────┘
 ┌─ METATRON ────────────────────────────────────────────────────────────────┐
 │ ⏎ m — speak with the angel…                                               │
 └────────────────────────────────────────────────────────────────────────────┘
  2 back to map · space resume · q quit
```

### Solo rules

- Solo renders the **same component** as the dock tab, just wider — one
  implementation, two widths ([../panels/chronicle.md](../panels/chronicle.md),
  [../panels/dock.md](../panels/dock.md)). No solo-only features.
- The minibuffer and footer persist in every solo view; the map's live state keeps
  updating underneath and is intact on return.
- Tab state (scroll position, filters, expanded event) survives the round trip
  home → solo → home.

## Narrow fallback

Below the widescreen breakpoint ([../patterns/layout.md](../patterns/layout.md)),
the app renders **today's single-pane UI unchanged**: header + tab bar + one active
pane + footer, keys `1–4` swap panes exactly as the current `internal/tui` does.

```
 script-world · day 4 · 08:12 · 1×
 [ map ] chronicle  metatron  souls
 ┌───────────────────────────────────┐
 │ ~ ~ " ♠ ♠ A ♠ " . . ⌂ . B . .     │
 │ ~ . . ᴥ . " . . . S . . " " .     │
 └───────────────────────────────────┘
  1-4 panes · space pause · q quit
```

- The two Metatron fixes still apply in fallback mode: the focus contract
  ([../patterns/focus-contract.md](../patterns/focus-contract.md)) governs the
  metatron pane, and the chronicle grammar
  ([../patterns/chronicle-grammar.md](../patterns/chronicle-grammar.md)) formats the
  feed. Layout is the only thing that degrades.
- Crossing the breakpoint (resize) swaps layouts live; no state is lost.

# Contract: state shape & key grammar — Villagers Tab

**Feature**: specs/015-villagers-tab | **Date**: 2026-07-22

## Event contract

**No new events. No changed payloads.** The feature consumes the existing
`agent.intent_set` (IntentSetPayload) and the existing snapshot/log-shipping
protocol unchanged. Anything that replays the event log (daemon, TUI replica,
offline tools) picks up `LastGoal`/`LastGoalTick` purely from the reducer.

## Canonical state contract (`sim.Agent` JSON)

```jsonc
{
  "name": "Ash",
  // ... existing fields unchanged ...
  "last_goal": "chop_wood",   // NEW, omitempty — most recent intent_set goal
  "last_goal_tick": 18422     // NEW, omitempty — tick of that intent_set
}
```

Guarantees:
1. Absent for agents that never had an intent (byte-stable canonical state
   for pre-feature snapshots and fresh worlds).
2. Old snapshots decode without error; zero values mean "no objective yet".
3. Written only by `State.Apply` on `agent.intent_set`; never cleared.
4. No snapshot format-version bump.

## Key grammar contract (villagers tab visible — dock tab 4 or its solo)

| Key | Roster view | Detail view |
|---|---|---|
| `j` / `k` | select next / previous villager (clamped) | no-op |
| `g` / `G` | jump to first / last villager | no-op |
| `⏎` | open detail for selected villager | no-op |
| `esc` | (falls through) release solo → home | close detail → roster |
| `4` | zoom solo / back home (unchanged) | unchanged |
| `2`/`3`/`1`, `tab` | switch tabs (state preserved) | switch tabs (state preserved) |
| globals (`m`, `space`, `[`, `]`, `q`, arrows) | unchanged | unchanged |

Rules:
- Keys bind only while the villagers tab is the visible dock tab (or solo'd)
  — no collision with chronicle inspect's `j/k` (one tab visible at a time)
  or map arrow-pan.
- "esc always releases" ordering (focus-contract.md rule 3): minibuffer →
  **villager detail** → solo → home.
- With `replica == nil` or an empty roster: `j/k/g/G/⏎` are strict no-ops.
- Footer hint (global mode) renames the tab: `… 4 villagers (again: solo) …`;
  while the villagers tab is visible the hint advertises
  `j/k select · ⏎ inspect` (and `esc back` in detail).

## Rendering contract (budget discipline)

- Both views render inside the given (width, height) budget; never overflow
  (existing shed-content rule).
- Roster: wide (≥40 cols) keeps today's columns + a cursor glyph on the
  selected row; narrow keeps name + status + health + cursor.
- Detail section order (truncate from the bottom): identity/vitals →
  objective → inventory → beliefs/narrative → memories (most recent first).
- Header/tab strings contain "villagers", never "soul(s)" (SC-003).

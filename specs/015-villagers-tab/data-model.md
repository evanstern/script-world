# Data Model: Villagers Tab

**Feature**: specs/015-villagers-tab | **Date**: 2026-07-22

## Modified entity: `sim.Agent` (internal/sim/agents.go)

Two additive, reducer-maintained fields (research.md R1):

| Field | Type | JSON | Semantics |
|---|---|---|---|
| `LastGoal` | `string` | `last_goal,omitempty` | Goal of the most recent `agent.intent_set` for this agent ("chop_wood", "talk_to", "move", …). Never cleared — empty means "no objective ever set". |
| `LastGoalTick` | `int64` | `last_goal_tick,omitempty` | Tick of that `agent.intent_set` event. 0 with an empty `LastGoal` means "never". |

**Write rule (single writer)**: `State.Apply`, case `"agent.intent_set"`
(internal/sim/state.go:313) — alongside setting `a.Intent`, also set
`a.LastGoal = p.Goal`, `a.LastGoalTick = e.Tick`. No other writer; no event
clears it (`agent.intent_done`, `gru.attacked`, hail interrupts all leave it
untouched by construction).

**Validation / invariants**:
- `Intent != nil` ⇒ `LastGoal == Intent.Goal` (intent_set writes both).
- `omitempty` keeps canonical bytes for pre-feature/never-intent agents
  byte-identical (precedent: `Generation`, `Plan`, `Hail`).
- Old snapshots (field absent) decode to zero values ⇒ UI shows "no
  objective yet". No snapshot format-version bump (additive only).
- Deterministic: pure function of the event log ⇒ replay-determinism suites
  pass by construction.

## Derived display state: objective (no storage)

| Condition | Detail view shows |
|---|---|
| `Intent != nil` | active objective: goal (+ target), marked current |
| `Intent == nil && LastGoal != ""` | past objective: `LastGoal` + tick, marked past ("last:") |
| `Intent == nil && LastGoal == ""` | "no objective yet" |

## New TUI model state (internal/tui/tui.go, client-only, never persisted)

| Field | Type | Semantics |
|---|---|---|
| `villSelected` | `int` | Roster cursor, default 0. Clamped to `[0, len(replica.Agents))` at every use; replica nil ⇒ keys no-op. |
| `villDetail` | `bool` | Detail view open for `villSelected`. `esc` closes; roster selection preserved. |

**Lifecycle / transitions**:

```
roster (villDetail=false)
  j/k/g/G → move villSelected (clamped)
  ⏎       → villDetail=true            (no-op if replica nil/empty)
detail (villDetail=true)
  esc     → villDetail=false           (before solo-release in the esc chain)
  j/k     → (optional, out of scope: no-op)
tab switches, resize → state preserved (dock.md per-tab state rule)
reconnect (connectedMsg replaces replica) → clamp villSelected; keep villDetail
```

Rename only (no semantic change): `paneSouls` → `paneVillagers`,
`soulsView/soulsBody` → `villagersView/villagersBody`, `paneNames[3]` and all
user-visible strings "souls"/"SOUL READER" → "villagers"/"VILLAGERS".

## Unchanged entities read by the detail view

`Agent.Name/X/Y/Needs/Asleep/Dead`, `Agent.Inv` (all kinds incl. `Spears`
wear), `Agent.Intent`, `Agent.Memories` (rendered most-recent-first via
existing order + `FormatMemory`), `Agent.Beliefs`, `Agent.Narrative`.

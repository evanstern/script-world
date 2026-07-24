# Event Contract Additions: spec 032 (walls, axes, paths)

New event types and reuse of existing ones. All payloads are canonical-JSON like every
existing event; yields/HP deltas are constants or derived from pre-event state — never
carried in payloads unless noted (the "outcome payload only when arithmetic could drift"
doctrine).

## Reused events

- **`agent.built`** (`BuiltPayload{Agent, Kind, X, Y}`) — now also carries kinds
  `wall_plank`, `wall_stone`, `path`. Reducer: spends `recipeFor("build_"+Kind).Inputs`
  (existing generic arm); NEW: stamps `HP = wallMaxHP(Kind)` for wall kinds. For wall
  kinds, (X, Y) is the intent's Res tile (adjacent-stand build); executor completion
  re-validates `buildSite(ResX,ResY) && !agentAt(ResX,ResY)`.
- **`agent.crafted`** (`CraftedPayload{Agent, Kind}`) — new kind `"axe"`. Reducer appends
  a fresh `axeDurability`-use axe to `Inv.Axes` (sorted ascending), spends recipe inputs.
- **`agent.chopped` / `agent.quarried`** (`HarvestPayload`, unchanged shape) — reducer
  yield becomes bare/axe pair derived from pre-mutation `len(Inv.Axes)`; carrying an axe
  decrements `Axes[0]` in the same application (spear-hunt clone).
- **`metatron.entity_removed`** — walls and paths are removable like any structure
  (no change; walls disappearing make their tile passable by construction).

## New events

| Type | Payload | Emitter rule | Reducer rule |
|---|---|---|---|
| `agent.axe_broke` | `{agent}` | co-emitted by executor immediately after a chop/quarry completion when pre-event `Axes[0] == 1` (same batch) | remove `Axes[0]` |
| `agent.wall_chipped` | `{agent, x, y}` | demolish work-cycle completion when wall HP − demolishChipHP ≥ 1 | wall at (x,y): `HP -= demolishChipHP`; reset acting agent's `Intent.WorkStart = 0` (cycle continues) |
| `agent.wall_destroyed` | `{agent, x, y}` | demolish work-cycle completion when wall HP − demolishChipHP ≤ 0 | remove wall structure at (x,y); clear intent; tile passable again |
| `agent.wall_repaired` | `{agent, x, y}` | repair work-cycle completion; wall present, damaged, 1 matching material carried (all re-validated) | `HP = min(max, HP + repairHPPerUnit)`; consume 1 matching material; if still damaged AND material remains → `WorkStart = 0`, else clear intent |

## Ordering & batch rules

- `agent.axe_broke` rides the SAME event batch as its harvest event, immediately after it —
  apply order: harvest decrements `Axes[0]` to 0, then axe_broke removes it (spear
  precedent, `agent.spear_broke`).
- Demolish/repair cycles emit exactly one event per completed work cycle; the multi-cycle
  loop is driven by the reducer's `WorkStart = 0` reset re-arming the executor's existing
  `agent.work_started` → duration gate. No new scheduling events.
- Contested-wall: demolish/repair completions re-validate the wall still stands at
  (ResX, ResY); a vanished wall resolves via `agent.intent_done` only (existing pattern).
- Builder memory: wall builds emit a situated builder memory (shelter salience tier);
  paths and chips/repairs emit none (spam avoidance — forage/chop precedent).

## Determinism

All new arithmetic is integer, constant-driven, reducer-applied, and derived from
pre-event state; replay reproduces byte-identical canonical state. Movement change
(dual-phase cadence, research R3) is tick-pure and stateless: `phase == 0` always steps,
`phase == 2` steps iff the agent's current tile holds a path.

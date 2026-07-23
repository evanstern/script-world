# Contract: Extended Metatron Status (IPC)

`metatron.Status` — the model-free peek returned by the existing `MetatronStatus` IPC
verb (`ipc/client.go` / `server.go`). Extended, not versioned: old fields keep exact
meaning; new fields are additive and `omitempty` where sensible, so existing clients are
unaffected (encoding/json ignores unknown fields).

## Shape

```json
{
  "charges": 3,
  "charter_default": false,
  "soul_tail": "...",
  "skills": ["10-weather.md", "20-diplomacy.md"],
  "granted_tools": ["nudge_dream", "nudge_omen", "work_miracle(move,give_item)"],
  "manifest_default": false
}
```

| Field | Meaning |
|---|---|
| `charges` | unchanged |
| `charter_default` | unchanged — on-disk charter == shipped default |
| `soul_tail` | unchanged |
| `skills` | effective skill filenames, composition order (post-eligibility, ≤8). Empty/omitted ⇒ none |
| `granted_tools` | granted roster in registry order; `work_miracle` suffixed with `(kind,…)` only when kinds are restricted |
| `manifest_default` | true ⇒ no `capabilities.json` (full default grant) |

Read discipline: computed fresh per call from disk (same per-read rule as the turn).

## TUI rendering (the ONLY TUI delta in this feature)

Console header line (today: `default charter` / `custom charter`) becomes e.g.:

```
custom charter · 2 skills · tools: dream, omen, miracles(move,give_item)
```

- charter part unchanged in meaning; `N skills` omitted when zero;
- `tools:` lists granted set in short form (`dream`, `omen`, `miracles`), with kind
  restriction shown when present; `tools: none` for a conversation-only world;
- when `manifest_default` is true, the tools part may be omitted (full grant is the
  unremarkable default) — keeps the header quiet for stock worlds.

Region constraint: implementation touches `consoleStatusMsg` handling and the header
render only — digest, villager detail, and transcript regions belong to TASK-63's
concurrent branch.

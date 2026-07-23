# Contract: Capability Manifest (`capabilities.json`)

Per-world declaration of which acting tools the world grants the angel. This is the
substrate TASK-68's stage presets will write.

## Shape

```json
{
  "tools": ["nudge_dream", "nudge_omen", "work_miracle"],
  "miracle_kinds": ["move", "remove", "give_item", "time_snap"]
}
```

- `tools` — subset of the metatron loop roster (`nudge_dream`, `nudge_omen`,
  `work_miracle`). Order irrelevant; declared roster order stays registry order.
- `miracle_kinds` — optional; subset of `move`, `remove`, `give_item`, `time_snap`.
  Omitted/null ⇒ all kinds (when `work_miracle` is granted). Ignored when
  `work_miracle` is not granted.

## Semantics

| Situation | Effective grant | Notice? |
|---|---|---|
| No file | full roster, all kinds (today's behavior, byte-compatible) | no |
| Valid subset | exactly that subset | no |
| `"tools": []` | nothing grantable; conversation still works | no |
| Malformed JSON / wrong types | full roster (charter-style permissive fallback) | yes |
| Unknown tool/kind name | that name ignored; valid remainder applies | yes |
| Edited between turns | new grants in effect next turn (per-read) | no |

## Guarantees (the three layers)

1. **Structurally absent**: an ungranted tool is not declared to the model — it is
   missing from the tool schemas sent with the turn, and a restricted `work_miracle`
   declares only the granted kinds in its `kind` enum.
2. **Absent from instructions**: the derived tool guidance describes only granted
   tools/kinds; costs shown come from the single authoritative cost table.
3. **Refused at the door**: a call naming an ungranted tool/kind (however induced) is
   rejected before landing — handler absence + grant check in `landNudge`/`landMiracle`,
   with the sim reducer dry-run as unchanged final authority.

Never gateable: conversation (the angel's text reply). Charge ACCRUAL is untouched by
the manifest; grants gate spending paths only.

## Consumers

- `internal/metatron` `Turn()`/`Status()` — reads per-turn/per-peek.
- TASK-68 stage presets — will write this file per stage (no code change per stage,
  SC-006).

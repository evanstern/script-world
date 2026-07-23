# Contract: llm.json `max_tokens` knobs (TASK-72)

The operator-facing surface. `llm.json` lives in the world save directory; these
fields are optional and additive — every pre-existing file remains valid and
byte-for-byte behavior-compatible.

## Schema (addition to the existing file)

```json
{
  "monthly_budget_usd": 100,
  "local":  { "...": "unchanged" },
  "cloud":  { "...": "unchanged" },
  "loop_max_rounds": 8,

  "max_tokens": {
    "planner": 768,
    "metatron_turn": 2048,
    "consolidation": 1500
  }
}
```

All three keys optional; the `max_tokens` object itself optional.

## Semantics

| Key | Governs | Default (absent/0) | Range | Out of range |
|-----|---------|--------------------|-------|--------------|
| `planner` | per-round budget of every villager planner tool-loop call | 512 | 1–4096 | clamp + boot warning |
| `metatron_turn` | per-round budget of every metatron console-turn tool-loop call | 1024 | 1–4096 | clamp + boot warning |
| `consolidation` | budget of the nightly consolidation call | 1024 | 1–4096 | clamp + boot warning |

- **Never a boot failure.** Negative → default with warning; > 4096 → 4096 with
  warning; warnings print on the daemon's boot channel
  (`daemon: llm.json max_tokens.<key> …`), one line per offending key,
  accumulating with any `parallel` / `loop_max_rounds` / `tool_mode` warnings.
- **Explicit 0 = unset** (default, silent) — same convention as `loop_max_rounds`.
- **Not governed** (unchanged hardcodes): conversation utterance (128) and outcome
  (224), meeting (72), narrator (800), metatron **digest** (400). The key is named
  `metatron_turn`, not `metatron`, precisely because the digest budget shares the
  metatron call kind but is out of scope.

## Compatibility

- A file without `max_tokens` (every existing world) → all three defaults, zero
  warnings, wire-identical requests to today.
- `promptworld new` (`WriteDefault`) does NOT emit the `max_tokens` object — the
  default config stays minimal and the knob stays opt-in.

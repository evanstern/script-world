# Contract: Tool Catalog Deltas (spec 029)

The registry (`internal/tool/registry.go`) is the single source of truth; this
contract pins the declared surface. Guidance prose derives via
`MetatronToolGuidance`; schemas derive via `InputSchema` unless authored.

## Retired

- `nudge_dream`, `nudge_omen` — entries removed from the catalog and both rosters.
  `tool.Lookup` misses; the cap literal readers (`sim.NudgeTextMax`,
  `metatron.nudgeTextMax`) re-point at `send_vision`. Historical `metatron.nudged`
  events with `form: "dream"` remain replayable (reducer grandfathering — see
  events.md).

## Added — influence

### send_vision
- Params: `target` (AgentName, required), `text` (Text, required, ≤400 bytes),
  descriptions on both.
- Gate: Charge. Effect: Expressive. Events: `metatron.nudged`,
  `agent.memory_added`. Cost: 1 charge.
- Door semantics: exactly one living target; any time of day; memory prefix
  `"You saw a vision: "`.

### send_omen
- Params: `targets` (Text, required — comma-separated living villager names, or
  `everyone`), `text` (Text, required, ≤400 bytes).
- Gate: Charge. Effect: Expressive. Events: `metatron.nudged`,
  `agent.memory_added`. Cost: 1 charge (one charge regardless of target count).
- Door semantics: night (`State.Night`) required to LAND; a daytime call places a
  system-origin deferral order instead (never a hard refusal for daytime alone);
  memory prefix `"You witnessed an omen: "`.

## Added — standing orders

### monitor_and_act (authored InputSchemaJSON — arrays)
```json
{
  "type": "object",
  "properties": {
    "condition":   {"type": "string", "maxLength": 300},
    "action":      {"type": "string", "maxLength": 400},
    "event_types": {"type": "array", "minItems": 1, "maxItems": 4,
                    "items": {"type": "string", "enum": ["<observable-event enum, see below>"]}},
    "agent":       {"type": "string"},
    "keywords":    {"type": "array", "maxItems": 6, "items": {"type": "string", "maxLength": 40}},
    "confirm":     {"type": "boolean"},
    "ttl_days":    {"type": "integer", "minimum": 1, "maximum": 7}
  },
  "required": ["condition", "action", "event_types"]
}
```
- Gate: None. Effect: Expressive. Events: `metatron.order_placed`. Cost: free.
- The `event_types` enum is the curated observable vocabulary (declared beside the
  entry): `agent.slept`, `agent.woke`, `agent.died`, `agent.memory_added`,
  `agent.intent_set`, `social.conversation`, `social.promise_broken`,
  `social.rumor_told`, `gru.attacked`, `norm.violated`,
  `sim.night_started`, `sim.day_started`. (Pinned against real emitted types at
  T003: the draft's `meeting.norm_enacted` is emitted by no code — `norm.violated`
  is the real norms-family observable and replaced it.)
- Door semantics: player-origin cap 3 active; ttl default 3 game days.
- **Driver change**: `toolloop.validateArgs` generalizes authored-override
  validation from the hardcoded `validateSetPlan` to a schema-lite walker
  (required keys, string/integer/boolean scalars, string arrays with enum/
  maxLength/minItems/maxItems, integer bounds). `set_plan` must validate
  identically through the walker (existing driver tests are the equivalence pin).

### cancel_order
- Params: `id` (Text, required).
- Gate: None. Effect: Expressive. Events: `metatron.order_cancelled`. Cost: free.

## Added — meta (loop control)

All three: Gate None, Effect Expressive with EMPTY Events (the `converse`
precedent — acting cardinality applies; nothing injected), Cost zero. Handlers
call the `LoopControl` seam (`sim.Loop.Do`); the clock's own events
(`clock.paused`/`clock.resumed`) remain the record.

- `pause` — no params.
- `start` — `speed` (Enum over clock speeds, optional; absent = resume current).
  The loop's `resume` command ignores its speed argument (found at T018), so a
  supplied speed is honored as `set_speed` THEN `resume` — two loop commands,
  one tool call (planning-tier ruling on the T018 finding; lands with polish).
- `adjust_speed` — `speed` (Enum, required).

## Rosters

- `LoopRosterMetatron()` = `send_omen`, `send_vision`, `monitor_and_act`,
  `cancel_order`, `work_miracle`, `pause`, `start`, `adjust_speed` (registry
  order; `converse` stays deliberately absent).
- `RosterMetatron` (door name set) mirrors the same names.
- Villager rosters unchanged.

## Capability manifest (spec 021 interaction)

Every new tool is individually grantable/withholdable in `capabilities.json`;
missing manifest = full roster (unchanged); legacy manifests naming retired
`nudge_*` tools grant nothing for those names (unknown-name rule, unchanged) —
the status surface shows the effective roster.

## Fixed frame addition

One sentence appended to the non-negotiables block (compile-time constant):
meta tools and standing orders may be used only when the player asks or a
standing order the player placed authorizes it — never on the angel's own
initiative.

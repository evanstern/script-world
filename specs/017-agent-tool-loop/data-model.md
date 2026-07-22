# Data Model: Agent Tool-Use Loop (spec 017)

Entities, payloads, and configuration this feature introduces or extends. Wire shapes
live in [contracts/provider-wire.md](contracts/provider-wire.md); Go signatures in
[contracts/loop-api.md](contracts/loop-api.md); event byte-stability rules in
[contracts/events.md](contracts/events.md).

## 1. Tool declaration (transport-level)

What a cognition presents to the model, derived from the spec-014 registry.

| Field | Type | Source |
|---|---|---|
| Name | string | `tool.Tool.Name` |
| Description | string | `tool.Tool.PromptGloss` (or authored text where gloss is shared/empty) |
| InputSchema | JSON Schema object | `tool.InputSchema(t)` — derived from `Params`, or the `InputSchemaJSON` override (set_plan) |

Derivation rules (`tool.InputSchema`):
- `AgentName` → `{"type":"string"}` (semantic validation stays gate-side)
- `Text` → `{"type":"string"}` + `maxLength` from `MaxRunes`/`MaxBytes` when set
- `Enum` → `{"type":"string","enum":[…]}`
- `Number` (new) → `{"type":"integer"}` + `minimum`/`maximum` when `Min`/`Max` set
- `Required` params → schema `required` array; `additionalProperties: false`

## 2. Registry additions (`internal/tool`)

- **`ParamKind Number`** with `Param.Min, Param.Max int` (0,0 = unbounded). Pays the
  spec-014 debt: `qty` becomes a declared optional `Number` param (Min 1) on `drop`,
  `pick_up`, `deposit`, `withdraw`.
- **`Tool.InputSchemaJSON json.RawMessage`** — optional authored-schema override;
  only `set_plan` uses it in this feature.
- **`set_plan` catalog entry** — Effect World, Gate Resolvable, villager roster,
  loop-only (excluded from the legacy prose surfaces `VocabularyLine`/`WorldGoals`/
  `PlanStepGoals`, which stay byte-stable). Schema: `steps` array, 1–planStepCap items
  of `{goal: enum(PlanStepGoals), kind?: enum(validKinds), qty?: integer ≥ 1}`.
- **Roster semantics**: `Validate` now admits Read-effect tools on rosters (zero
  production Read entries ship in this task; test fixtures exercise the path).
- **New derived surface**: `LoopRoster(kind)` — the ordered `[]Tool` a cognition
  declares (villager: world verbs + `set_plan` + `muse`; metatron as-built (T020):
  `nudge_dream`, `nudge_omen`, `work_miracle` — `converse` is deliberately NOT a
  declared tool: the model's final text IS the converse channel, so speaking is the
  loop's natural termination rather than a callable). `say`/`gist` stay scene-gated
  and are NOT in the villager loop roster this task (scenes remain driver-run).
- **`work_miracle` (T019b as-built, post-#38)**: Effect **Expressive** (lands through
  InjectSocial via the shared miracle builder, same door/family as nudges; Expressive
  is also what lets it declare its Events for coverage pinning), Gate Charge, flat
  `Params` (kind enum + scalar per-kind params, gratis structurally absent) rather
  than an authored InputSchemaJSON — the driver's validateArgs routes authored
  schemas to the set_plan structural validator, a latent constraint to generalize
  when a third authored-schema tool appears.

## 3. Transcript (transport-level, ephemeral)

Never persisted, never replayed.

- **Turn**: `{Role: user|assistant, Blocks: []Block}`
- **Block**: one of `Text{s}`, `ToolUse{ID, Name, Args}`, `ToolResult{ForID, Content,
  IsError}`
- **ToolCall** (parsed from a Response): `{ID, Name, Args json.RawMessage}`
- **StopReason**: `end_turn | tool_use | max_tokens | other`

## 4. Loop instance (`internal/toolloop`)

| Field | Type | Notes |
|---|---|---|
| Job | string | existing job identifier `<class>-<agent>-<snapshotTick>` |
| Kind | llm.Kind | routing key (planner, metatron) |
| Roster | []tool.Tool | declared tools |
| Handlers | map[string]Handler | injected by consumer |
| MaxRounds | int | from config `loop_max_rounds`, clamped 1–16, default 8 |
| System, Seed prompt | string | initial turns |

**Handler** = `func(ctx, call ToolCall) Outcome` where **Outcome** is
`{Verdict, ResultForModel string, Err error}` — mutating handlers wrap the inject doors
and translate the door's accept/reject into a verdict; read handlers return data.

**LoopResult**: `{Final string, Landed *ToolCall, Rounds int, Calls []CallRecord,
Usage []llm.Response, TotalMillis int64, Term Termination}` with **Termination** ∈
`landed | model_done | cap_exhausted | admission_refused | provider_error | ctx_done`.

State machine per round: submit → (Stop==tool_use?) → dispatch each call in order
(read → feed result; acting → door → landed? terminate after recording trailing
rejections : feed rejection) → next round. Terminal on: landed acting call, Stop ==
end_turn, round cap, admission/provider error, context cancel.

## 5. Verdict taxonomy (`cog.tool_call.verdict`)

| Verdict | Meaning | Consumes action? |
|---|---|---|
| `landed` | acting call admitted by its door; grounding events emitted | yes (ends loop) |
| `rejected_gate` | door refused (stale/guard/scene/charge) | no |
| `rejected_cardinality` | acting call after one already landed this cognition | no (loop ends) |
| `rejected_unknown` | tool name not in declared roster/registry | no |
| `rejected_malformed` | args fail schema/param validation | no |
| `read_ok` / `read_error` | read-effect dispatch outcome | no |
| `unlanded` | loop terminated (cap/error) before this call could ground | no |

## 6. Event payload changes (see contracts/events.md for byte rules)

- **NEW `cog.tool_call`** (reducer no-op; whitelist addition):
  `{job, ordinal, tool, args?, verdict, reason?, tier, snapshot_tick}` — `args` is the
  raw call arguments JSON, capped (truncated + flagged) at 2 KiB; `reason` omitempty.
- **CHANGED `agent.intent_set`** (`IntentSetPayload`, agents.go:624): new field
  `job string json:"job,omitempty"` — set only on planner-loop landings (from
  `InjectArgs.JobID`); reflex/executor emissions omit it. All other fields untouched.
- **UNCHANGED but load-bearing**: `agent.plan_set` already carries `Job`;
  `cog.thought`/`cog.outcome` already carry `Job` — correlation chain closes with the
  two additions above.
- **REMOVED emissions**: none. (Musing's `agent.thought` events still occur — now caused
  by the `muse` tool handler instead of the scheduled channel.)

## 7. Configuration (`llm.json`)

| Field | Type | Default | Semantics |
|---|---|---|---|
| `loop_max_rounds` | int | 8 | hard iteration cap; clamp 1–16 with warning (never boot-fails) |
| `local.tool_mode` | `"native"` \| `"json"` | `"native"` | per-model strategy; `"json"` engages the fallback envelope |
| `cloud.tool_mode` | same | `"native"` | honored only by the `openai_compat` cloud provider; Anthropic is always native |

## 8. Governor observation unit (`internal/cognition` seam)

- Estimator observation for loop cognitions = **whole-loop wall millis**, reported once
  per cognition via `Orchestrator.ObserveCognition(kind, totalMillis)`; loop-internal
  Submits set `Request.SkipObserve` and feed nothing individually.
- Spend metering unchanged: per billable call, `Allow()` pre-call / `Add()` post-call.
- `musing` DecisionClass removed; `kindToClass` and `ValidateKinds` updated; planner
  class points (3) now denominate whole-loop time — calibration command exercises a
  representative loop.

## 9. Retired data

- `KindMusing` (llm), `musing` DecisionClass (cognition), mind muse queue/scheduling
  fields, `plannerReplySchema` as planner contract, metatron `turnReply`/`parseTurn`.
- No stored data migrates: event logs are append-only and old payloads replay
  byte-identically (additive omitempty only).

# Phase 0 Research: Agent Tool-Use Loop (spec 017)

All decisions below resolve the plan's unknowns. Code references are to main at planning
time (post spec-014 merge, PR #36).

## R1 — Where the loop lives: new `internal/toolloop` package; `internal/llm` stays transport

**Decision**: split transport from orchestration. `internal/llm` gains *tool-call
transport*: `Request` carries tool declarations and a multi-turn transcript; `Response`
carries parsed tool calls. One `Submit` remains exactly one provider call (one admission
check, one metered call, one circuit-breaker sample). The *loop driver* is a new package
`internal/toolloop` that owns iteration, cardinality, cap enforcement, dispatch, and
artifact recording — consumed by both `internal/mind` (villager cognition) and
`internal/metatron` (turn).

**Rationale**: llm.go's worker/queue/meter/health machinery is per-call and must stay
that way (FR-011: budget checked before each billable call — free if the loop just calls
`Submit` N times). Handlers need sim-side context (inject doors), which llm must not
import; a separate driver package with injected handler funcs keeps llm a leaf transport
and lets mind and metatron share one loop implementation.

**Alternatives considered**: (a) loop inside `Orchestrator` (llm) — rejected: llm would
need handler callbacks reaching into mind/sim, inverting the dependency direction and
entangling metering with dispatch. (b) loop private to `internal/mind` — rejected:
metatron adopts the loop in this task (clarification 2026-07-22) and lives in its own
package.

## R2 — Cloud native tool use: Anthropic Messages `tools` via the existing SDK

**Decision**: `anthropicCaller` (providers.go:132) declares tools with the SDK's native
`tools` parameter and exchanges `tool_use` / `tool_result` content blocks across turns.
anthropic-sdk-go v1.58.0 (already in go.mod:12) fully supports this. System-prompt
caching (`CacheControlEphemeral`) is kept; tool declarations are stable per cognition
kind, so they join the cached prefix.

**Rationale**: board AC#3 names native cloud tool use; the SDK is already vendored and
the Messages API tool contract is the provider's first-class path.

**Alternatives considered**: structured-output emulation on cloud — rejected: strictly
worse than the native path where the native path exists (weaker model steering, no
parallel-call semantics, no stop-reason signal).

## R3 — Local tier: native-first function calling, per-model fallback (clarification)

**Decision**: `openaiCompat` (providers.go:26) gains the OpenAI-compatible
`tools`/`tool_calls` function-calling exchange. A new per-model config field
`local.tool_mode` (`"native"` default, `"json"` fallback; same shape as the existing
per-model `reasoning_effort` knob, config.go:36) selects the strategy. `tool_mode:
"json"` engages the fallback convention (R4). The same field exists on `cloud` for
`openai_compat` cloud routers.

**Rationale**: clarification session 2026-07-22 chose native-first with per-model
config. gemma-class models behind Ollama/llama.cpp vary in function-calling quality; the
project already documented llama.cpp grammar quirks empirically (parse.go:122 notes), so
the escape hatch must be per-model, not per-tier.

**Alternatives considered**: JSON-everywhere on local (simpler, one path) — rejected by
clarification; native-only (no fallback) — rejected: risks the primary planner tier.

## R4 — Fallback convention: schema-constrained single-call envelope

**Decision**: the documented fallback (FR-010) is a provider-agnostic structured-output
convention: tools are described in the system prompt (derived gloss + parameter shapes);
each round constrains the reply with `response_format: json_schema` (already plumbed —
`Request.ResponseSchema`, llm.go:88) to the envelope
`{"tool": "<name enum>", "args": {…}, "done": bool}`; tool results are fed back as a
plain user-role turn. One envelope = one tool call round, so FR-003–FR-008 hold
identically (the loop driver cannot tell which wire shape produced a call).

**Rationale**: this is today's proven local path — the planner already ships
`plannerReplySchema` through `response_format` and it is the empirically hardened route
(llama.cpp grammar notes). The envelope generalizes it from "goal reply" to "tool call".

**Alternatives considered**: free-text `<tool_call>` tag parsing (Hermes-style) —
rejected: strictly less reliable than grammar-constrained output on the same backends;
prompt-only JSON without schema constraint — rejected for the same reason.

## R5 — llm.Request/Response surface: transcript + tool declarations, additive

**Decision**: additive fields, zero behavior change when absent:
- `Request.Tools []ToolDecl` — `{Name, Description, InputSchema json.RawMessage}`.
- `Request.Turns []Turn` — ordered transcript (`role` + content blocks: text,
  `tool_use`, `tool_result`); when empty, today's `Prompt` single-message path is used
  byte-identically.
- `Response.ToolCalls []ToolCall` — `{ID, Name, Args json.RawMessage}`; empty for plain
  text replies. `Response.Stop StopReason` (`end_turn` | `tool_use` | other) so the
  driver distinguishes "model finished" from "model wants results".
Existing single-shot callers (conversation, consolidation, narrator, drama, meeting)
pass no tools/turns and are untouched.

**Rationale**: keeps one `Submit` = one metered, one admitted, one observed call —
FR-011 falls out of the shape instead of needing bookkeeping.

**Alternatives considered**: a separate `SubmitLoop` orchestrator API — rejected per R1.

## R6 — Tool-call artifact: new `cog.tool_call` event, reducer no-op, through existing doors

**Decision**: every tool call the loop sees becomes one `cog.tool_call` event:
`{Job, Ordinal, Tool, Args (raw, capped), Verdict, Reason?, Tier, SnapshotTick}`.
Verdicts: `landed`, `rejected_gate`, `rejected_cardinality`, `rejected_unknown`,
`rejected_malformed`, `read_ok`, `read_error`, `unlanded` (loop terminated before
grounding). The event type is added to `injectSocialWhitelist` (loop.go:152) as a
reducer no-op — exactly the existing `cog.*` telemetry pattern (state.go:321). Mind
lands them through its telemetry door (`emitCog`, telemetry.go:159); metatron through
its existing sim-side landing path.

**Rationale**: board AC#5 requires rejected/never-grounded calls to be recorded
artifacts. Reducer-no-op telemetry through the whitelisted social door is the
established, replay-safe channel for exactly this class of record.

**Alternatives considered**: logging to a side file — rejected: violates Principle I
(the event log is the queryable paper trail) and AC#5's "queryable from the event log".

## R7 — Correlation: `Job` (+ per-call `Ordinal`) threads request → verdict → grounding

**Decision**: reuse the existing job identifier (`"<class>-<agent>-<snapshotTick>"`,
minted at telemetry.go:41, already threaded as `InjectArgs.JobID`, loop.go:58, and
carried by `cog.thought`/`cog.outcome`/`agent.plan_set`). Add `Job string
json:"job,omitempty"` to `IntentSetPayload` (agents.go:624), populated from
`in.JobID` at the planner-landing emission site. Reflex- and executor-authored
intent_set events have no job and omit the field — old and new logs stay byte-stable
(TASK-32 omitempty pattern, cognition.go:78-82). `cog.tool_call.Ordinal` numbers calls
within a job.

**Rationale**: the identifier already exists end-to-end except for the one payload gap
the board note names; an additive omitempty field is the proven byte-stability pattern.

**Alternatives considered**: new correlation UUID per call — rejected: a second
identifier system where one already threads through would recreate the inference problem
it cures.

## R8 — Cardinality: driver-enforced, one *landed* acting call ends the loop

**Decision**: `internal/toolloop` enforces spec FR-004: Read-effect calls dispatch
freely within the cap; the first acting (World/Expressive) call that *lands* terminates
the loop as its final answer. A rejected acting call (gate/malformed/off-roster) is
recorded, fed back as the call's tool result, and does not consume the action. If one
model response batches several calls, calls after the landed acting call are rejected
`rejected_cardinality` and recorded; the loop then ends.

**Rationale**: spec User Story 1 scenario 4 and the recorded design decision ("one
acting tool per cognition"); ending on landing avoids a wasted trailing model round.

## R9 — Governor & metering: per-call spend, whole-loop observation

**Decision**: metering is untouched — each `Submit` already checks `meter.Allow()`
pre-call and records actual cost post-call (llm.go:385), so an exhausted budget refuses
the *next* loop iteration pre-spend (spec Story 5 scenario 3). Estimation changes:
`Request` gains `SkipObserve bool`; the loop driver sets it on every internal call and
reports one whole-loop observation per cognition via a new orchestrator method
(`ObserveCognition(kind, totalMillis)`) feeding the same per-tier estimator
(estimate.go). `route.go` verdict arithmetic is untouched (pure function over recorded
observations); the estimator simply converges on whole-loop seconds-per-point.
Calibration (`promptworld calibrate`) is updated to exercise a representative loop so
seeded sec/pt matches the new unit.

**Rationale**: spec FR-011 — the cognition is the governor's unit; per-call EWMA
feeding (llm.go:370) would count one cognition as N fractional observations and skew
sec-per-point low, mis-arming the suppression gate.

**Alternatives considered**: scaling class Points by expected call count — rejected:
Points are declared doctrine (registry.go:37), not a runtime-tuned value, and expected
call count is model-dependent.

## R10 — Muse: scheduled channel deleted; muse is a roster choice (clarification)

**Decision**: delete the cadence-fired musing path — `Mind.muse` scheduling
(mind.go:497), the muse queue/worker, `KindMusing` routing, and the `musing`
DecisionClass (registry.go). The `muse` registry tool stays and is offered in the
villager loop roster; its handler lands `agent.thought` through the social door exactly
as today's musing landing does. `BestEffort` admission stays in llm (musing was its only
user, but the mechanism is doctrine for any future drop-when-busy kind and its removal
would churn admission tests for no benefit). The cognition completeness gate
(`ValidateKinds`, registry.go:101) is updated with the kind's removal.

**Rationale**: clarification session 2026-07-22 chose "remove now"; the recorded design
decision is that musing carries opportunity cost — one cognition spent musing is not
spent acting.

**Alternatives considered**: parallel channels during transition — rejected by
clarification.

## R11 — Plans: a `set_plan` acting tool with an authored input schema

**Decision**: today's planner may reply with a multi-step plan (`reply.Plan` →
`injectPlan`, mind.go:437 → `agent.plan_set`). In the loop, that capability becomes an
explicit acting tool `set_plan` (World effect, villager roster): input schema is an
authored JSON Schema (steps: 1–planStepCap items of `{goal ∈ PlanStepGoals, kind?,
qty?}`), grounding through the existing `InjectArgs.Plan` path unchanged. To carry it,
the registry `Tool` gains an optional `InputSchemaJSON json.RawMessage` override; tools
without it get their schema derived from `Params` (R12). `set_plan` is loop-only
vocabulary: it is excluded from the legacy derived prose surfaces (`VocabularyLine`,
`WorldGoals`) which remain byte-stable for any remaining consumer and for the 014 golden
test until those surfaces retire with the free-text path.

**Rationale**: plans are load-bearing behavior (SC-005 parity); the registry's
`Param` model (scalar kinds) cannot express a steps array, and hand-authoring one
schema is far cheaper than inventing a composite ParamKind for a single tool.

**Alternatives considered**: dropping plan capability — rejected (behavior regression);
composite/array ParamKind — rejected (heavy generalization for one consumer); per-step
tool calls accumulating a plan — rejected (violates one-acting-call cardinality and
multiplies rounds).

## R12 — Registry additions: `ParamKind Number`, schema derivation, Read entries admitted

**Decision**: `internal/tool` gains: (a) `Number` ParamKind with optional `Min`/`Max`
(the spec-014 debt — storage verbs' `qty` becomes a declared Param on `drop`, `pick_up`,
`deposit`, `withdraw`); (b) a derived surface `InputSchema(t Tool) json.RawMessage`
producing a JSON Schema object from `Params` (honoring `InputSchemaJSON` override, R11);
(c) `Validate` admits Read-effect entries on rosters (014 rejected them; the loop is the
consumer that lifts the restriction). No production Read tool ships in this task —
TASK-16 brings the journal tools; loop read-dispatch is proven with test-fixture read
tools (SC-001's mid-loop read is exercised in the soak harness roster).

**Rationale**: the debt is recorded in specs/014-tool-registry/contracts/tool-catalog.md;
schema derivation is the single source of truth for both native wire shapes and the
fallback envelope.

## R13 — Metatron turn adoption (clarification: in scope)

**Decision**: `Metatron.Turn` (turn.go:68) migrates from single `Submit` + `parseTurn`
to the shared loop with `RosterMetatron` (`converse`, `nudge_dream`, `nudge_omen`).
Nudge calls dispatch through the existing landing path (`landNudge`, turn.go:139 —
charge economy stays reducer-enforced, R7 of spec 014); `converse` text remains the
final answer channel (transcript-only, no world events). Metatron runs on the cloud
tier, so this is also the native-Anthropic proving ground. Its `cog.tool_call`
artifacts land through the same whitelisted door as its existing telemetry.

**Rationale**: clarification session 2026-07-22 chose villager + metatron; metatron's
tools are already registered with the charge gate, and the cloud tier needs a real
consumer to prove AC#3's cloud leg.

**Amendment (2026-07-22, post PR #38 / spec 016 merge)**: the metatron turn now also
works miracles (`turnReply.Miracle`, landed via `landMiracle` through the shared
operator-console builder). Spec 016 already enforces "at most one mediated act per
turn" (nudge wins over miracle), which maps exactly onto this spec's
one-landed-acting-call cardinality — no doctrine conflict. The loop migration
therefore adds a `work_miracle` registry entry (charge-gated, authored
`InputSchemaJSON` over the miracle parameter surface — kind/day/time/villager/item/
qty/class/x/y/to_x/to_y, mirroring spec 016's turn contract with gratis structurally
absent) to `LoopRosterMetatron`, with its handler wrapping `landMiracle` unchanged.
Tracked as T019b (registry entry, Sonnet) and folded into T020's handler scope.

## R14 — Iteration cap and budgets

**Decision**: hard iteration cap default **8 provider rounds** per cognition,
operator-configurable as `llm.json: loop_max_rounds` (clamped 1–16, warn-not-error like
`local.parallel`, config.go:58). Read-call count is bounded by the round cap; no
separate read budget in this task. Cap exhaustion → `cog.outcome` failure family +
`unlanded` verdicts on any unresolved calls; zero world mutation (nothing landed means
nothing emitted through the doors).

**Rationale**: spec assumption "operator-configurable with safe defaults"; 8 rounds
covers read-then-act patterns (TASK-16's search-then-write) with headroom while bounding
adversarial loops; clamp-not-reject matches the project's config doctrine (worlds never
fail to boot over a tuning knob).

## R15 — What retires, what stays (blast-radius ledger)

- **Retires**: mind's free-text planner parse for villager cognition (`parseReply`
  consumption path), `plannerReplySchema` as the planner's reply contract (its
  hardening lessons move into the R4 envelope), scheduled musing (R10), `parseTurn`
  (metatron), `KindMusing` + `musing` class.
- **Stays untouched**: conversation scenes (`convo.go` — `parseSay`/`parseOutcome`),
  nightly consolidation, narrator/drama/meeting kinds, reflex policy (`decideIntent`),
  the landing ladder and whitelist semantics (one whitelist *addition*: `cog.tool_call`),
  executor, reducer arms, snapshots, calibration file ownership.
- **Wiki notes owed re-pin after merge** (Principle IV): `llm-orchestrator.md`,
  `cognition.md`, `agent-mind.md`, `tool-registry.md`, `event-types.md`,
  `sim-state-reducer.md` (whitelist), `metatron.md`.

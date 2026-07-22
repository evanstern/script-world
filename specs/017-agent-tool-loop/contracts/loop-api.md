# Contract: Loop API surface (spec 017)

Go surfaces this feature adds or changes. Signatures are the contract; bodies are
implementation. Dependency rule: `internal/tool` stays a leaf; `internal/toolloop`
imports only `internal/llm` + `internal/tool` (+ stdlib); handlers and event emission
are injected by consumers (`internal/mind`, `internal/metatron`).

## Package `internal/llm` — transport additions (all additive)

```go
// ToolDecl is one declared tool on a Request.
type ToolDecl struct {
    Name        string
    Description string
    InputSchema json.RawMessage // JSON Schema object
}

// Turn/Block model the multi-turn transcript. A Request with nil Turns
// behaves byte-identically to today's single-message path.
type Turn struct {
    Role   Role // RoleUser | RoleAssistant
    Blocks []Block
}
type Block struct {
    Text       string          // one of the three is set
    ToolUse    *ToolUseBlock   // assistant-side call echo
    ToolResult *ToolResultBlock
}
type ToolUseBlock struct{ ID, Name string; Args json.RawMessage }
type ToolResultBlock struct{ ForID, Content string; IsError bool }

// Request gains (existing fields unchanged):
//   Tools       []ToolDecl      // nil = no tools parameter sent (today's behavior)
//   Turns       []Turn          // nil = single Prompt message (today's behavior)
//   SkipObserve bool            // loop-internal call: worker feeds no estimator sample

// Response gains:
//   ToolCalls []ToolCall        // parsed calls; nil for plain text
//   Stop      StopReason        // end_turn | tool_use | max_tokens | other
type ToolCall struct{ ID, Name string; Args json.RawMessage }

// ObserveCognition reports one whole-cognition wall-time observation to the
// tier estimator (the loop's replacement for per-call worker feeding).
func (o *Orchestrator) ObserveCognition(kind Kind, totalMillis int64)
```

Provider obligations (`providers.go`) — see contracts/provider-wire.md for wire shapes:
- `anthropicCaller`: maps Tools/Turns to native Messages `tools` + `tool_use`/
  `tool_result` blocks; always native; system prompt caching preserved.
- `openaiCompat`: `tool_mode:"native"` → `tools`/`tool_calls` function calling;
  `tool_mode:"json"` → fallback envelope via `response_format` (never both).
- A caller MUST return `ToolCalls` in the model's emission order and MUST NOT retry
  internally (one Submit = one attempt, unchanged).

Config (`config.go`): `Config.LoopMaxRounds int` (`loop_max_rounds`, clamp 1–16 via a
`Rounds()` normalizer mirroring `Workers()`, default 8), `LocalConfig.ToolMode string`
and `CloudConfig.ToolMode string` (`"" == "native"`; unknown value → warn + native).

Unchanged by contract: `Submit` admission ladder (budget → circuit → best-effort →
queue), one worker per cloud tier, `meter.Allow`/`meter.Add` per call, `workerCallCap`,
health striking, `BestEffort` semantics, every existing Kind's routing.

## Package `internal/toolloop` (NEW)

```go
// Handler executes one tool call. Mutating handlers wrap an inject door and
// translate its accept/reject; read handlers return data and emit nothing.
type Handler func(ctx context.Context, call llm.ToolCall) Outcome

type Outcome struct {
    Verdict        Verdict // see data-model §5
    ResultForModel string  // fed back as the call's tool_result
    Err            error   // infrastructure failure (≠ rejection)
}

type Job struct {
    JobID     string          // existing cognition job identifier
    Kind      llm.Kind
    System    string
    Seed      string          // initial user turn
    Roster    []tool.Tool     // declared tools (schema via tool.InputSchema)
    Handlers  map[string]Handler
    MaxRounds int
    MaxTokens int64
    Record    func(CallRecord) // artifact sink — consumer lands cog.tool_call
}

type CallRecord struct {
    JobID    string
    Ordinal  int
    Tool     string
    Args     json.RawMessage // capped copy
    Verdict  Verdict
    Reason   string
    Tier     string
}

type Result struct {
    Final       string        // model's final text (converse channel / closing prose)
    Landed      *llm.ToolCall // nil when Term != TermLanded
    Rounds      int
    TotalMillis int64
    Term        Termination   // landed | model_done | cap_exhausted |
                              // admission_refused | provider_error | ctx_done
}

// Run drives the bounded loop: submit → dispatch → feed back → repeat.
// Guarantees: terminates within MaxRounds; at most one landed acting call;
// every model tool call produces exactly one Record; Read-effect tools never
// consume the action; SkipObserve set on every internal Submit; the estimator
// is fed at most once per Run, and ONLY for completed cognitions (landed /
// model_done / cap_exhausted — terminations that measure completed model
// work). Refused or errored loops (admission_refused / provider_error /
// ctx_done) feed NOTHING — the successes-only doctrine the worker path
// documents ("a fast failure is not a latency observation of completed
// thought"). [Amended 2026-07-22, T025 FILED-1: the original "exactly one
// report on every return path" fed 0ms refusals into the EWMA, collapsing
// the estimate toward zero under sustained refusal.]
func Run(ctx context.Context, orch *llm.Orchestrator, j Job) (Result, error)
```

Enforcement owned by `Run` (not handlers): roster membership (`rejected_unknown`),
schema/param validation before dispatch (`rejected_malformed`), cardinality
(`rejected_cardinality`), cap, and verdict recording. Handlers own only their door's
verdict (`landed` / `rejected_gate` / `read_ok` / `read_error`).

## Package `internal/tool` — additions

```go
const Number ParamKind = … // after Enum
// Param gains: Min, Max int  (Number bounds; 0,0 = unbounded)
// Tool gains:  InputSchemaJSON json.RawMessage // authored override (set_plan only)

// InputSchema derives the JSON Schema for a tool from Params, honoring the
// InputSchemaJSON override. Deterministic output (stable key order).
func InputSchema(t Tool) json.RawMessage

// LoopRoster returns the ordered declared-tool list for a cognition kind.
func LoopRosterVillager() []Tool // world verbs + set_plan + muse
func LoopRosterMetatron() []Tool // as-built (T020): nudge_dream, nudge_omen,
                                 // work_miracle — converse is deliberately NOT
                                 // declared; the model's final text IS the
                                 // converse channel (model_done terminates)
```

Catalog delta: `set_plan` entry (World, Resolvable, authored schema); `qty` Number
param on drop/pick_up/deposit/withdraw. `VocabularyLine`/`PromptGlossBlock`/
`WorldGoals`/`PlanStepGoals` remain byte-stable (set_plan excluded); the 014 golden
prompt test is retired only together with the free-text planner prompt it pinned
(same commit, noted in the test).

`Validate` change: rosters MAY reference Read-effect tools (014's rejection lifted);
Number params MUST have Min ≤ Max when both set; InputSchemaJSON MUST be a valid JSON
object when present.

## Consumption points

| Consumer | Replaces | Call |
|---|---|---|
| `internal/mind` planner path (`runPlan`) | `Submit`+`parseReply`+`plannerReplySchema` | `toolloop.Run` with villager roster; handlers wrap `InjectIntent` (world verbs, set_plan) and the social door (muse) |
| `internal/mind` muse scheduling | scheduled `KindMusing` channel | DELETED — muse is a handler |
| `internal/metatron.Turn` | `Submit`+`parseTurn` | `toolloop.Run` with metatron roster; nudge handlers wrap `landNudge`; `Final` = converse text |
| `internal/sim` | — | `IntentSetPayload.Job` (omitempty); whitelist += `cog.tool_call`; reducer no-op arm |
| `internal/cognition` | `musing` class | class table minus musing; `ValidateKinds` updated |
| `promptworld calibrate` | single-shot probe for planner | representative loop probe (whole-loop sec/pt) |

## Test contract

1. **Loop driver unit tests** (stub caller, no network): cap exhaustion, one-landed-call
   cardinality, batched-calls trailing rejection, read-then-act, rejection feedback +
   retry within cap, admission-refused mid-loop, every path records exactly one
   CallRecord per model call and exactly one ObserveCognition report.
2. **Provider wire tests**: both callers translate Tools/Turns/ToolCalls per
   provider-wire.md (recorded-fixture round-trips); `tool_mode:"json"` produces the
   envelope schema and parses it back to ToolCalls.
3. **Replay byte-identity**: existing suite passes unmodified on pre-feature fixture
   logs; new-run replay reproduces byte-identical state with a nil model (loop never
   invoked during replay).
4. **Payload stability**: `IntentSetPayload` without Job marshals byte-identically to
   pre-feature; with Job, field order is canonical and stable.
5. **Correlation query test**: from a run fixture, resolve intent_set→job→cog.tool_call
   chain and rejected-call artifacts purely by identifier (SC-003).
6. **Governor**: estimator fed whole-loop observations converges (no per-call samples
   when SkipObserve set); budget-exhausted mid-loop refuses pre-spend and terminates
   with `admission_refused` (SC-004).
7. **Startup validation**: registry fixtures with bad Number bounds / bad override
   schema / Read-roster entries (now legal) behave per Validate contract.

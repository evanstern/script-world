// Package toolloop is the bounded tool-use loop driver (spec 017, TASK-52):
// submit → dispatch → feed back → repeat, until an acting call lands, the model
// finishes, the round cap trips, or the transport fails.
//
// Doctrine (preserved verbatim from the TASK-52 design decisions): a tool call
// is a REQUEST; an event is the FACT; the gate decides; the executor grounds
// work in time and space. The driver enforces bounds and RECORDS requests — it
// never mutates world state. Every durable effect flows through a handler that
// wraps an existing landing door (InjectIntent, the social whitelist), so the
// loop cannot manufacture a fact the gates would not admit. Reads return data
// and ground nothing. Speaking / musing / thinking are tools too: game-state
// integrity applies to expression, not only world mutation.
//
// The driver is transport-agnostic and sim-agnostic. It imports only
// internal/llm (the wire) and internal/tool (the schema/roster source);
// handlers, artifact recording, and event emission are injected by the
// consumer (internal/mind, internal/metatron), keeping this package a shared
// leaf below both (research R1).
package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// Handler executes one tool call. A mutating handler wraps an inject door and
// translates its accept/reject into a Verdict; a read handler returns data and
// emits nothing. The driver owns everything a handler does not: roster
// membership, argument validation, cardinality, the cap, and recording.
type Handler func(ctx context.Context, call llm.ToolCall) Outcome

// Outcome is one handler's report. Verdict is the door's disposition (landed /
// rejected_gate for acting handlers; read_ok / read_error for read handlers).
// ResultForModel is fed back as the call's tool_result (a read's data, a gate's
// rejection explanation). Err is an INFRASTRUCTURE failure — not a rejection —
// and terminates the loop with provider_error.
type Outcome struct {
	Verdict        Verdict
	ResultForModel string
	Err            error
}

// Job is one bounded loop run's inputs. JobID is the existing cognition job
// identifier ("<class>-<agent>-<snapshotTick>"); it threads every CallRecord.
type Job struct {
	JobID     string
	Kind      llm.Kind
	System    string
	Seed      string // initial user turn
	Roster    []tool.Tool
	Handlers  map[string]Handler
	MaxRounds int
	MaxTokens int64
	Record    func(CallRecord) // artifact sink; the consumer buffers/lands records
}

// Termination is how a Run ended (data-model.md §4). landed / model_done /
// cap_exhausted return a nil error; admission_refused / provider_error /
// ctx_done return the underlying transport (or handler) error alongside.
type Termination string

const (
	TermLanded           Termination = "landed"
	TermModelDone        Termination = "model_done"
	TermCapExhausted     Termination = "cap_exhausted"
	TermAdmissionRefused Termination = "admission_refused"
	TermProviderError    Termination = "provider_error"
	TermCtxDone          Termination = "ctx_done"
)

// Result is the loop's outcome. Landed is non-nil iff Term == TermLanded.
// Final is the model's last-round text (the closing prose / converse channel).
// TotalMillis is the whole-Run wall time — the governor's observation unit.
type Result struct {
	Final       string
	Landed      *llm.ToolCall
	Rounds      int
	TotalMillis int64
	Term        Termination
}

// submitter is the transport seam Run drives. *llm.Orchestrator satisfies it;
// Run takes the concrete orchestrator (the contract surface) and delegates to
// run over this interface, so the driver's control flow is unit-testable
// against a scripted stub without a network or a real orchestrator.
type submitter interface {
	Submit(ctx context.Context, req llm.Request) (llm.Response, error)
	ObserveCognition(kind llm.Kind, totalMillis int64)
}

// Run drives the bounded loop. Guarantees (contracts/loop-api.md): it
// terminates within MaxRounds provider rounds; at most one acting call lands;
// every model tool call yields exactly one CallRecord via j.Record (ordinals
// 1-based, dense, emission-ordered); read-effect tools never consume the
// action; SkipObserve rides every internal Submit; and the governor estimator
// is fed the whole-Run wall time on completed terminations only (successes-only:
// landed / model_done / cap_exhausted), never on the failure family
// (admission_refused / provider_error / ctx_done) — mirroring the worker path's
// "a fast failure is not a latency observation of completed thought".
func Run(ctx context.Context, orch *llm.Orchestrator, j Job) (Result, error) {
	return run(ctx, orch, j)
}

func run(ctx context.Context, s submitter, j Job) (res Result, err error) {
	start := time.Now()
	// The whole-loop governor observation (data-model.md §8): SUCCESSES-ONLY,
	// mirroring the worker path's doctrine (internal/llm/llm.go — "a fast
	// failure is not a latency observation of completed thought"). res.TotalMillis
	// is always recorded (it is part of the Result), but the estimator is fed
	// ONLY on a completed termination — landed / model_done / cap_exhausted, each
	// of which measured completed model work (cap_exhausted did N full provider
	// rounds). The failure family — admission_refused / provider_error / ctx_done —
	// did no completed thought and feeds NOTHING, so a refused/errored loop cannot
	// skew the EWMA toward zero. Per-round Submits set SkipObserve so no fractional
	// samples reach the estimator either. This is a single exit-path mechanism, so
	// it still fires at most once (never double-fires).
	defer func() {
		res.TotalMillis = time.Since(start).Milliseconds()
		switch res.Term {
		case TermLanded, TermModelDone, TermCapExhausted:
			s.ObserveCognition(j.Kind, res.TotalMillis)
		}
	}()

	// MaxRounds <= 0 is treated as 1 defensively; config (llm.Config.Rounds())
	// normalizes the operator value upstream, so this only guards a direct
	// caller that forgot to set it.
	maxRounds := j.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}

	roster := make(map[string]tool.Tool, len(j.Roster))
	for _, t := range j.Roster {
		roster[t.Name] = t
	}
	tools := make([]llm.ToolDecl, 0, len(j.Roster))
	for _, t := range j.Roster {
		tools = append(tools, llm.ToolDecl{
			Name:        t.Name,
			Description: t.PromptGloss,
			InputSchema: tool.InputSchema(t),
		})
	}

	ordinal := 0
	record := func(name string, args json.RawMessage, v Verdict, reason, tier string) {
		ordinal++
		if j.Record != nil {
			j.Record(CallRecord{
				JobID:   j.JobID,
				Ordinal: ordinal,
				Tool:    name,
				Args:    capArgs(args),
				Verdict: v,
				Reason:  reason,
				Tier:    tier,
			})
		}
	}

	// The transcript accumulates across rounds: it opens with the seed user
	// turn, and each round appends exactly ONE assistant turn (the echoed
	// response) then one user turn (that round's tool results). The
	// one-assistant-turn-per-round invariant is load-bearing: the openaiCompat
	// json fallback synthesizes call IDs as "env-<round>" from the count of
	// assistant turns in the transcript, so any deviation would collide IDs.
	transcript := []llm.Turn{{Role: llm.RoleUser, Blocks: []llm.Block{{Text: j.Seed}}}}
	var landed *llm.ToolCall
	rounds := 0

	for {
		if cerr := ctx.Err(); cerr != nil {
			res.Rounds = rounds
			res.Term = TermCtxDone
			return res, cerr
		}

		resp, serr := s.Submit(ctx, llm.Request{
			Kind:        j.Kind,
			System:      j.System,
			Tools:       tools,
			Turns:       transcript,
			MaxTokens:   j.MaxTokens,
			SkipObserve: true,
		})
		if serr != nil {
			res.Rounds = rounds
			res.Term = terminationForSubmitErr(serr)
			return res, serr
		}
		rounds++
		res.Rounds = rounds
		res.Final = resp.Text
		tier := string(resp.Tier)

		// Echo the model's turn (text + tool_use blocks) — exactly one
		// assistant turn per round.
		transcript = append(transcript, assistantEcho(resp))

		calls := resp.ToolCalls
		if len(calls) == 0 {
			// The model produced no actionable call. Run reports model_done
			// honestly; the CONSUMER decides how to record the failure
			// outcome (FR-015).
			res.Term = TermModelDone
			return res, nil
		}

		atCap := rounds >= maxRounds
		var results []llm.Block

		for i := 0; i < len(calls); i++ {
			call := calls[i]

			// Cardinality: once an acting call has landed, EVERY remaining call
			// this response is rejected (reads included) — the cognition's one
			// action is spent (FR-004, R8).
			if landed != nil {
				const reason = "an acting tool already landed this cognition"
				record(call.Name, call.Args, VerdictRejectedCardinality, reason, tier)
				results = append(results, resultBlock(call.ID, "rejected: "+reason, true))
				continue
			}

			// Driver-side validation, in order: roster membership, then
			// argument schema, then handler presence.
			t, ok := roster[call.Name]
			if !ok {
				reason := fmt.Sprintf("tool %q is not on this cognition's roster", call.Name)
				record(call.Name, call.Args, VerdictRejectedUnknown, reason, tier)
				results = append(results, resultBlock(call.ID, reason, true))
				continue
			}
			if reason := validateArgs(t, call.Args); reason != "" {
				record(call.Name, call.Args, VerdictRejectedMalformed, reason, tier)
				results = append(results, resultBlock(call.ID, reason, true))
				continue
			}
			h, ok := j.Handlers[call.Name]
			if !ok {
				reason := fmt.Sprintf("tool %q has no handler", call.Name)
				record(call.Name, call.Args, VerdictRejectedUnknown, reason, tier)
				results = append(results, resultBlock(call.ID, reason, true))
				continue
			}

			// Read-effect: return data, ground nothing, exempt from
			// cardinality. On the final round a read cannot inform any future
			// action, so it is recorded unlanded rather than dispatched (the
			// loop is out of rounds to use its result).
			if !isActing(t) {
				if atCap {
					record(call.Name, call.Args, VerdictUnlanded,
						"round cap reached before the read result could inform an action", tier)
					continue
				}
				out := h(ctx, call)
				if out.Err != nil {
					recordInfraFailure(record, call, calls[i+1:], out.Err, tier)
					res.Term = TermProviderError
					return res, out.Err
				}
				reason := ""
				if out.Verdict == VerdictReadError {
					reason = out.ResultForModel
				}
				record(call.Name, call.Args, out.Verdict, reason, tier)
				results = append(results, resultBlock(call.ID, out.ResultForModel, out.Verdict == VerdictReadError))
				continue
			}

			// Acting (World / Expressive): dispatched on every round including
			// the final one — an acting call can LAND as the terminal answer
			// without needing a follow-up round.
			out := h(ctx, call)
			if out.Err != nil {
				recordInfraFailure(record, call, calls[i+1:], out.Err, tier)
				res.Term = TermProviderError
				return res, out.Err
			}
			if out.Verdict == VerdictLanded {
				record(call.Name, call.Args, VerdictLanded, "", tier)
				c := call
				landed = &c
				res.Landed = &c
				// Feed-back is built for completeness but never sent: the loop
				// ends this round. Remaining calls fall to the cardinality arm.
				results = append(results, resultBlock(call.ID, out.ResultForModel, false))
				continue
			}
			// rejected_gate: the door refused; feed the reason back so the
			// model can try a different action within the remaining cap.
			record(call.Name, call.Args, out.Verdict, out.ResultForModel, tier)
			results = append(results, resultBlock(call.ID, out.ResultForModel, true))
		}

		if landed != nil {
			res.Term = TermLanded
			return res, nil
		}
		if atCap {
			res.Term = TermCapExhausted
			return res, nil
		}
		transcript = append(transcript, llm.Turn{Role: llm.RoleUser, Blocks: results})
	}
}

// terminationForSubmitErr maps a Submit failure onto a Termination. Context
// cancellation is its own family; the admission-ladder sentinels
// (budget/queue/circuit/best-effort) collapse to admission_refused; anything
// else is a provider_error.
func terminationForSubmitErr(err error) Termination {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return TermCtxDone
	case errors.Is(err, llm.ErrBudgetExhausted),
		errors.Is(err, llm.ErrQueueFull),
		errors.Is(err, llm.ErrTierBusy),
		errors.Is(err, llm.ErrTierDown):
		return TermAdmissionRefused
	default:
		return TermProviderError
	}
}

// recordInfraFailure records a handler infrastructure failure: the failing call
// as unlanded (carrying the error), and every trailing call in the same batch
// as unlanded too — the loop terminates before they could be dispatched, but
// every model tool call must still yield exactly one record.
func recordInfraFailure(record func(string, json.RawMessage, Verdict, string, string), call llm.ToolCall, trailing []llm.ToolCall, err error, tier string) {
	record(call.Name, call.Args, VerdictUnlanded, err.Error(), tier)
	for _, t := range trailing {
		record(t.Name, t.Args, VerdictUnlanded, "loop terminated (provider error) before dispatch", tier)
	}
}

// assistantEcho renders the model's response as one assistant transcript turn:
// its prose first (when any), then one tool_use block per emitted call, keeping
// the call IDs so a following tool_result can key back to them.
func assistantEcho(resp llm.Response) llm.Turn {
	var blocks []llm.Block
	if resp.Text != "" {
		blocks = append(blocks, llm.Block{Text: resp.Text})
	}
	for _, c := range resp.ToolCalls {
		blocks = append(blocks, llm.Block{ToolUse: &llm.ToolUseBlock{ID: c.ID, Name: c.Name, Args: c.Args}})
	}
	return llm.Turn{Role: llm.RoleAssistant, Blocks: blocks}
}

// isActing reports whether a tool consumes the cognition's one action (World or
// Expressive effect). Read-effect tools do not.
func isActing(t tool.Tool) bool {
	return t.Effect == tool.World || t.Effect == tool.Expressive
}

// validateArgs runs the driver-side, JSON-Schema-level argument check that
// precedes dispatch (rejected_malformed). It is deliberately minimal — the
// landing door re-validates everything semantic against live state; this only
// catches shapes the model can repair (missing required args, wrong scalar
// type, enum membership, number bounds, text caps). A tool with an authored
// InputSchemaJSON override (set_plan) gets structural validation instead
// (validateSetPlan). Returns "" when the arguments pass.
func validateArgs(t tool.Tool, raw json.RawMessage) string {
	args := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "arguments must be a JSON object"
		}
	}
	if len(t.InputSchemaJSON) > 0 {
		return validateSetPlan(args)
	}
	for _, p := range t.Params {
		rawv, present := args[p.Name]
		if !present {
			if p.Required {
				return fmt.Sprintf("missing required argument %q", p.Name)
			}
			continue
		}
		switch p.Kind {
		case tool.AgentName:
			if _, ok := jsonString(rawv); !ok {
				return fmt.Sprintf("argument %q must be a string", p.Name)
			}
		case tool.Text:
			s, ok := jsonString(rawv)
			if !ok {
				return fmt.Sprintf("argument %q must be a string", p.Name)
			}
			if p.MaxRunes > 0 && utf8.RuneCountInString(s) > p.MaxRunes {
				return fmt.Sprintf("argument %q exceeds its %d-rune cap", p.Name, p.MaxRunes)
			}
			if p.MaxBytes > 0 && len(s) > p.MaxBytes {
				return fmt.Sprintf("argument %q exceeds its %d-byte cap", p.Name, p.MaxBytes)
			}
		case tool.Enum:
			s, ok := jsonString(rawv)
			if !ok {
				return fmt.Sprintf("argument %q must be a string", p.Name)
			}
			if !contains(p.Enum, s) {
				return fmt.Sprintf("argument %q must be one of %v", p.Name, p.Enum)
			}
		case tool.Number:
			n, ok := jsonInt(rawv)
			if !ok {
				return fmt.Sprintf("argument %q must be an integer", p.Name)
			}
			if p.Min != 0 && n < int64(p.Min) {
				return fmt.Sprintf("argument %q must be >= %d", p.Name, p.Min)
			}
			if p.Max != 0 && n > int64(p.Max) {
				return fmt.Sprintf("argument %q must be <= %d", p.Name, p.Max)
			}
		}
	}
	return ""
}

// validateSetPlan is the minimal structural check for set_plan's authored
// schema (spec 017 R11): a "steps" array of 1..PlanStepCap objects, each with a
// "goal" from the legacy world-goal vocabulary and an optional "kind" (item
// vocabulary) and "qty" (integer >= 1). The inject door re-validates the plan
// against live state; this only rejects a shape the model can repair.
func validateSetPlan(args map[string]json.RawMessage) string {
	stepsRaw, ok := args["steps"]
	if !ok {
		return `missing required argument "steps"`
	}
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal(stepsRaw, &steps); err != nil {
		return `"steps" must be an array of step objects`
	}
	if len(steps) < 1 {
		return `"steps" must contain at least one step`
	}
	if len(steps) > tool.PlanStepCap {
		return fmt.Sprintf(`"steps" must contain at most %d steps`, tool.PlanStepCap)
	}
	goals := tool.WorldGoals()
	kinds := tool.ItemKinds()
	for i, step := range steps {
		goalRaw, ok := step["goal"]
		if !ok {
			return fmt.Sprintf("step %d is missing a goal", i+1)
		}
		goal, ok := jsonString(goalRaw)
		if !ok || !goals[goal] {
			return fmt.Sprintf("step %d has an unknown goal", i+1)
		}
		if kindRaw, ok := step["kind"]; ok {
			k, ok := jsonString(kindRaw)
			if !ok || !contains(kinds, k) {
				return fmt.Sprintf("step %d has an unknown item kind", i+1)
			}
		}
		if qtyRaw, ok := step["qty"]; ok {
			n, ok := jsonInt(qtyRaw)
			if !ok || n < 1 {
				return fmt.Sprintf("step %d qty must be an integer >= 1", i+1)
			}
		}
	}
	return ""
}

func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func jsonInt(raw json.RawMessage) (int64, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	if f != math.Trunc(f) {
		return 0, false
	}
	return int64(f), true
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

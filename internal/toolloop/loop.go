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
	// Provider optionally pins every round's Submit to one EXPLICIT declared
	// provider (spec 024 R3), riding straight through to llm.Request.Provider and
	// overriding the run-level resolve below. `promptworld calibrate` (T020) sets
	// it so a reference loop sample measures the NAMED provider. Empty (the
	// default, and every live mind/metatron caller) is NOT unpinned: Run resolves
	// the kind's provider ONCE at run start (ResolveProvider) and pins the whole
	// run to it (spec 024 R9 / FR-008 extension), so a multi-round cognition — and
	// the spec-025 in-loop retry — never changes providers mid-transcript.
	Provider string
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
	// Retried / RetryReason record the run's ONE in-loop transport retry (spec
	// 025 FR-001/FR-004, contracts/loop-retry.md): Retried is set when a
	// transport-level provider error was re-submitted once (recovered OR
	// twice-failed), and RetryReason carries the FIRST failure's error text.
	// RetryReason is non-empty iff Retried is true. The consumer surfaces this
	// as a non-terminal cog.outcome so a recovery is never silent (SC-003).
	Retried     bool
	RetryReason string
}

// submitter is the transport seam Run drives. *llm.Orchestrator satisfies it;
// Run takes the concrete orchestrator (the contract surface) and delegates to
// run over this interface, so the driver's control flow is unit-testable
// against a scripted stub without a network or a real orchestrator.
// ResolveProvider is the run-level pin seam (spec 024 R9 / FR-008 extension): a
// dry chain-walk naming the provider the kind currently resolves to, called once
// at run start so every round — including the spec-025 retry — targets one
// provider (research R9, the cognition-run analog of scene pinning).
type submitter interface {
	Submit(ctx context.Context, req llm.Request) (llm.Response, error)
	ObserveCognition(kind llm.Kind, provider string, totalMillis int64)
	ResolveProvider(kind llm.Kind) (string, error)
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
//
// Transport retry (spec 025 FR-001..FR-006, contracts/loop-retry.md): a run
// retries a failed Submit EXACTLY ONCE, and only when the failure classifies as
// a transport-level provider error (terminationForSubmitErr → provider_error).
// The retry re-submits the identical transcript (a failed Submit appended
// nothing) and consumes no round (rounds count model responses). On a second
// transport failure — or on the first, if the run's retry is already spent — the
// run terminates provider_error with the latest error, exactly as a single
// failure terminates without this feature. Admission refusals and context
// cancellation never retry (the governor spoke; busy-is-not-down is preserved),
// and the tool-handler infrastructure-failure sites below are NOT transport
// failures (the model call succeeded) and are never retried. A retry recovered
// in the success family produces exactly one ObserveCognition, a twice-failed
// run zero — the estimator/breaker invariance is structural, not new mechanism.
// Result.Retried / Result.RetryReason report the retry for the consumer to surface.
func Run(ctx context.Context, orch *llm.Orchestrator, j Job) (Result, error) {
	return run(ctx, orch, j)
}

func run(ctx context.Context, s submitter, j Job) (res Result, err error) {
	start := time.Now()
	// Run-level provider pin (spec 024 R9 / FR-008 extension, spec 025
	// composition): a multi-round cognition resolves its provider ONCE at run
	// start and stamps it on EVERY round — including the spec-025 in-loop
	// transport retry — so a thought never changes models mid-transcript. This is
	// the cognition-run analog of a conversation scene's pin: with per-call
	// chain-walking, the breaker strike from the very failure being retried is
	// itself a walk trigger, so a retry (or any later round) could land on a
	// different provider than the transcript's earlier rounds, mixing native vs
	// JSON tool-call conventions and mis-attributing the whole-run observation.
	// An explicit Job.Provider (calibrate's reference pin) is honored as-is and
	// never re-resolved. A ResolveProvider miss (unknown kind — a boot-validated
	// impossibility in production; a stub that omits the seam in tests) leaves the
	// pin empty, falling back to today's per-kind routing; a genuinely down pinned
	// provider fails the run per spec 025 semantics (the retry re-hits the SAME
	// provider) and the NEXT cognition's resolve walks the chain to a fallback.
	pin := j.Provider
	if pin == "" {
		if resolved, rerr := s.ResolveProvider(j.Kind); rerr == nil {
			pin = resolved
		}
	}
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
	// it still fires at most once (never double-fires). Attribution rides the run
	// PIN, exact by construction: every round Submits with Provider: pin, so a
	// completed run was served entirely by the pinned provider — no need to read
	// back a per-round serving name that could only ever equal the pin. When the
	// pin is empty (an unresolvable kind — a failure path that never reaches this
	// success-only observe), ObserveCognition falls back to the chain head.
	defer func() {
		res.TotalMillis = time.Since(start).Milliseconds()
		switch res.Term {
		case TermLanded, TermModelDone, TermCapExhausted:
			s.ObserveCognition(j.Kind, pin, res.TotalMillis)
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
			Provider:    pin,
			SkipObserve: true,
		})
		if serr != nil {
			// Transport retry (spec 025 FR-001/FR-002, contracts/loop-retry.md):
			// ONE re-submit per run on a transport-level provider error only.
			// terminationForSubmitErr already excludes admission refusals
			// (TermAdmissionRefused — the governor spoke, retrying would fight
			// busy-is-not-down) and context cancellation/deadline (TermCtxDone).
			// A failed Submit appended nothing, so the transcript is byte-identical
			// on re-submission, and no round is consumed (rounds++ is below, after
			// a SUCCESSFUL Submit). RetryReason keeps the FIRST failure's text; the
			// second failure (if any) propagates from the terminate branch as today.
			if !res.Retried && terminationForSubmitErr(serr) == TermProviderError {
				res.Retried = true
				res.RetryReason = serr.Error()
				continue
			}
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
// InputSchemaJSON override (set_plan, monitor_and_act, …) is validated against
// that schema by the schema-lite walker (validateAuthored, spec 029 R5).
// Returns "" when the arguments pass.
func validateArgs(t tool.Tool, raw json.RawMessage) string {
	args := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "arguments must be a JSON object"
		}
	}
	if len(t.InputSchemaJSON) > 0 {
		return validateAuthored(t.InputSchemaJSON, raw)
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

// validateAuthored validates raw arguments against a tool's authored
// InputSchemaJSON with a schema-lite walker (spec 029 R5 / T002), generalizing
// the retired set_plan-only structural check. It understands the JSON-Schema
// subset the registry actually authors and RECURSES through it — object
// `required` + `properties`, array `minItems`/`maxItems`/`items`, string
// `enum`/`maxLength`, and integer `minimum`/`maximum` — so set_plan (a "steps"
// array of step objects) validates identically to the old validateSetPlan and
// monitor_and_act (string arrays with enum/bounds, spec 029) rides the same
// code. Like the Params path it is deliberately structural: it rejects only the
// shapes the model can repair, and the landing door re-validates everything
// semantic against live state. Unknown keywords and additionalProperties are
// ignored — the old dispatch enforced neither, so this never rejects more than
// it did. Returns "" on success.
func validateAuthored(schemaJSON, raw json.RawMessage) string {
	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		// An authored schema that is not a JSON object is a registry bug caught
		// by tool.Validate at boot, never a model-repairable argument error.
		return ""
	}
	return walkSchema("", schema, raw)
}

// walkSchema validates one JSON value (raw) against one schema-lite node,
// recursing into object properties and array items. `path` names the value in
// the returned reason (e.g. `steps[2].goal`), empty at the top level. A
// malformed or type-less schema node passes (nothing to enforce) rather than
// panicking — the "structural only" contract.
func walkSchema(path string, schema map[string]any, raw json.RawMessage) string {
	typ, _ := schema["type"].(string)
	switch typ {
	case "object":
		fields := map[string]json.RawMessage{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &fields); err != nil {
				return fmt.Sprintf("%s must be an object", labelOr(path, "arguments"))
			}
		}
		for _, req := range schemaStrings(schema["required"]) {
			if _, ok := fields[req]; !ok {
				return fmt.Sprintf("missing required argument %q", childPath(path, req))
			}
		}
		// Presence is `required`'s job; each present property is validated
		// against its own sub-schema. Keys absent from `properties`
		// (additionalProperties) are ignored, as the old dispatch was.
		props, _ := schema["properties"].(map[string]any)
		for name, sub := range props {
			child, ok := fields[name]
			if !ok {
				continue
			}
			subSchema, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			if reason := walkSchema(childPath(path, name), subSchema, child); reason != "" {
				return reason
			}
		}
	case "array":
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Sprintf("%s must be an array", labelOr(path, "arguments"))
		}
		if min, ok := schemaInt(schema["minItems"]); ok && len(items) < min {
			return fmt.Sprintf("%s must contain at least %d item(s)", labelOr(path, "arguments"), min)
		}
		if max, ok := schemaInt(schema["maxItems"]); ok && len(items) > max {
			return fmt.Sprintf("%s must contain at most %d item(s)", labelOr(path, "arguments"), max)
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for i, elem := range items {
				if reason := walkSchema(fmt.Sprintf("%s[%d]", path, i), itemSchema, elem); reason != "" {
					return reason
				}
			}
		}
	case "string":
		s, ok := jsonString(raw)
		if !ok {
			return fmt.Sprintf("%s must be a string", labelOr(path, "argument"))
		}
		if max, ok := schemaInt(schema["maxLength"]); ok && utf8.RuneCountInString(s) > max {
			return fmt.Sprintf("%s exceeds its %d-character cap", labelOr(path, "argument"), max)
		}
		if enum := schemaStrings(schema["enum"]); len(enum) > 0 && !contains(enum, s) {
			return fmt.Sprintf("%s must be one of %v", labelOr(path, "argument"), enum)
		}
	case "integer":
		n, ok := jsonInt(raw)
		if !ok {
			return fmt.Sprintf("%s must be an integer", labelOr(path, "argument"))
		}
		if min, ok := schemaInt(schema["minimum"]); ok && n < int64(min) {
			return fmt.Sprintf("%s must be >= %d", labelOr(path, "argument"), min)
		}
		if max, ok := schemaInt(schema["maximum"]); ok && n > int64(max) {
			return fmt.Sprintf("%s must be <= %d", labelOr(path, "argument"), max)
		}
	case "boolean":
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return fmt.Sprintf("%s must be a boolean", labelOr(path, "argument"))
		}
	}
	return ""
}

// childPath joins an object path with a property name for a readable reason;
// the top-level path is empty, so a top-level property reads as its bare name.
func childPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// labelOr returns path, or fallback when path is empty (the top-level value has
// no name of its own).
func labelOr(path, fallback string) string {
	if path == "" {
		return fallback
	}
	return path
}

// schemaStrings coerces a schema keyword (`required`, `enum`) that json.Unmarshal
// left as []any into a []string, dropping any non-string member.
func schemaStrings(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// schemaInt coerces a numeric schema keyword (`minItems`, `maximum`,
// `maxLength`, …), which json.Unmarshal decodes as float64, into an int; ok is
// false when the keyword is absent or non-numeric (nothing to enforce).
func schemaInt(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int(f), true
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

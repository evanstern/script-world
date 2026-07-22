package metatron

// The metatron console turn's tool-use loop wiring (spec 017 T020): the loop
// handlers that wrap landNudge / landMiracle, and the cog.tool_call telemetry
// landing (the T018 pattern, mirrored from internal/mind/telemetry.go through
// metatron's own social door).
//
// Doctrine (spec 017): a tool call is a REQUEST; the door decides. The handlers
// never mutate the world — they call landNudge / landMiracle, which land through
// the InjectSocial door (charge economy stays reducer-enforced), and translate
// the door's accept/reject into a Verdict. A door refusal becomes a
// rejected_gate the model may correct within the loop's round cap — a behavior
// UPGRADE over the pre-loop single-shot refusal (a mistyped villager name can be
// retried instead of ending the turn). converse is NOT a handler: the model's
// final text (toolloop.Result.Final) is the converse channel.

import (
	"context"
	"encoding/json"
	"log"
	"sort"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
)

// turnDispatch carries one console turn's loop state: the charge/alive snapshot
// read at turn start (as the pre-loop path read them), the buffered CallRecords
// (the T018 telemetry sink), and the result pointer a landed handler writes its
// Nudge/Miracle report onto. Only one act ever lands (the driver's cardinality),
// so at most one of result.Nudge / result.Miracle is ever set.
type turnDispatch struct {
	mt      *Metatron
	charges int
	alive   map[int]bool
	result  *TurnResult

	records []toolloop.CallRecord
}

// record is the Job.Record sink — one CallRecord per model tool call.
func (d *turnDispatch) record(r toolloop.CallRecord) {
	d.records = append(d.records, r)
}

// turnHandlers builds the handler map the tool-use loop dispatches against for
// one console turn. nudge_dream / nudge_omen wrap landNudge (the tool name fixes
// the form); work_miracle wraps landMiracle. converse is absent by design — it
// is the final-text channel, and it is not on the declared loop roster
// (tool.LoopRosterMetatron), so the model never calls it as a tool.
func (mt *Metatron) turnHandlers(d *turnDispatch) map[string]toolloop.Handler {
	return map[string]toolloop.Handler{
		"nudge_dream":  mt.handleNudge(d, "dream"),
		"nudge_omen":   mt.handleNudge(d, "omen"),
		"work_miracle": mt.handleMiracle(d),
	}
}

// handleNudge wraps landNudge for one form. Door accept → landed (the report is
// written onto the shared result); door/validation refusal → rejected_gate
// carrying the human-readable reason, fed back so the model may correct a bad
// target within the round cap.
func (mt *Metatron) handleNudge(d *turnDispatch, form string) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		// target is dream-only; the driver already enforced its presence for
		// nudge_dream (required AgentName param) and its absence is harmless for
		// omen (landNudge ignores it).
		target := argString(call.Args, "target")
		text := argString(call.Args, "text")
		if nudge, why := mt.landNudge(form, target, text, d.charges, d.alive); nudge != nil {
			d.result.Nudge = nudge
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the " + form + " reached its mark"}
		} else {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// handleMiracle wraps landMiracle. Same accept/reject translation as handleNudge.
func (mt *Metatron) handleMiracle(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		if miracle, why := mt.landMiracle(parseMiracleArgs(call.Args), d.charges); miracle != nil {
			d.result.Miracle = miracle
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the miracle is worked: " + miracle.Summary}
		} else {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// refusal guarantees a non-empty rejection reason for the model's feedback and
// the cog.tool_call record (landNudge/landMiracle always give one, but a blank
// would otherwise be silently fed back and backfilled).
func refusal(why string) string {
	if why == "" {
		return "the act could not be completed"
	}
	return why
}

// parseMiracleArgs decodes a work_miracle tool call's arguments into the flat
// miracleArgs the landMiracle body reads. The driver's validateArgs already
// gated the scalar types (schema-level), so this is a lenient reader; the door's
// dry-run is the semantic authority.
func parseMiracleArgs(raw json.RawMessage) miracleArgs {
	var a miracleArgs
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &a)
	}
	return a
}

// argString reads a string-valued argument from a tool call's raw JSON object;
// "" when absent or the wrong shape (the driver's schema validation already
// gates required/typed args, so this is a lenient reader — mirrors mind's).
func argString(raw json.RawMessage, key string) string {
	m := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}

// --- cog.tool_call telemetry (spec 017 FR-007, T018) ---

// verdictRequiresReason reports whether a verdict's cog.tool_call MUST carry a
// non-empty reason (contracts/events.md): every rejection and every read error
// is the queryable explanation AC#5 promises. Mirrors mind's own predicate.
func verdictRequiresReason(v toolloop.Verdict) bool {
	switch v {
	case toolloop.VerdictRejectedGate, toolloop.VerdictRejectedCardinality,
		toolloop.VerdictRejectedUnknown, toolloop.VerdictRejectedMalformed,
		toolloop.VerdictReadError:
		return true
	default:
		return false
	}
}

// toolCallEvent converts one buffered CallRecord into its cog.tool_call event
// via the shared sim-side constructor (the same authority mind uses at T018).
// snapshotTick is the turn's world tick. The reason invariant is enforced here:
// an empty reason on a verdict that requires one is backfilled with the verdict
// name and logged as the driver-contract violation it would be.
func (mt *Metatron) toolCallEvent(r toolloop.CallRecord, snapshotTick int64) store.Event {
	reason := r.Reason
	if reason == "" && verdictRequiresReason(r.Verdict) {
		reason = string(r.Verdict)
		log.Printf("metatron: cog.tool_call %s ordinal %d verdict %s missing reason; backfilled", r.JobID, r.Ordinal, r.Verdict)
	}
	b, _ := json.Marshal(sim.NewCogToolCallPayload(
		r.JobID, r.Ordinal, r.Tool, r.Args,
		string(r.Verdict), reason, r.Tier, snapshotTick,
	))
	return store.Event{Type: "cog.tool_call", Payload: b}
}

// emitToolCalls lands one console turn's buffered CallRecords as cog.tool_call
// events (spec 017 FR-007, T018), one per record, on EVERY termination path so a
// rejected / never-grounded call is recorded even when nothing landed. The
// records ride ONE dedicated all-or-nothing batch through the same InjectSocial
// door the nudge/miracle grounding events used — a separate batch, so it neither
// reorders nor entangles with them. Events go out in ordinal order. An empty
// buffer (a converse-only turn) emits nothing — no empty batch.
func (mt *Metatron) emitToolCalls(records []toolloop.CallRecord, snapshotTick int64) {
	if mt.social == nil || len(records) == 0 {
		return
	}
	ordered := append([]toolloop.CallRecord(nil), records...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Ordinal < ordered[j].Ordinal })
	events := make([]store.Event, 0, len(ordered))
	for _, r := range ordered {
		events = append(events, mt.toolCallEvent(r, snapshotTick))
	}
	if err := mt.social.InjectSocial(events); err != nil {
		// The world outlives its observability — a rejected telemetry batch is
		// logged, never fatal (mirrors mind's emitCog).
		log.Printf("metatron: cog.tool_call telemetry rejected: %v", err)
	}
}

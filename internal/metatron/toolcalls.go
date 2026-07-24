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

	"github.com/evanstern/promptworld/internal/clock"
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
	night   bool // mirrored State.Night at turn start — the omen gate (spec 029 T005)
	tick    int64
	result  *TurnResult
	grant   grantSet // this world's capability grant (spec 021 US2): gates handlers + land

	records []toolloop.CallRecord
}

// record is the Job.Record sink — one CallRecord per model tool call.
func (d *turnDispatch) record(r toolloop.CallRecord) {
	d.records = append(d.records, r)
}

// turnHandlers builds the handler map the tool-use loop dispatches against for
// one turn — console OR system-authored (spec 029 T012). send_vision / send_omen
// wrap the influence landers; monitor_and_act / cancel_order wrap the standing-
// order door (spec 029 T009); work_miracle wraps landMiracle. converse is absent
// by design — it is the final-text channel, and it is not on the declared loop
// roster (tool.LoopRosterMetatron), so the model never calls it as a tool.
//
// Capability gating (spec 021 US2, door layer / R5.3): a handler is installed
// ONLY for a tool the world grants. An ungranted tool therefore has no handler
// and the loop rejects any call to it as rejected_unknown — structural absence
// at the door, matching the structural absence in the declaration and the prose.
//
// The meta tools pause / start / adjust_speed wrap the LoopControl seam (spec 029
// US5, T018): each is grant-gated like every other tool (structural absence when
// ungranted), and drives mt.loop.Do — the SAME clock control the IPC server uses.
// They inject no world event and spend no charge (Effect Expressive, EMPTY Events)
// but consume the turn's one act.
func (mt *Metatron) turnHandlers(d *turnDispatch) map[string]toolloop.Handler {
	h := make(map[string]toolloop.Handler, 9)
	if d.grant.allows("send_vision") {
		h["send_vision"] = mt.handleVision(d)
	}
	if d.grant.allows("send_omen") {
		h["send_omen"] = mt.handleOmen(d)
	}
	if d.grant.allows("monitor_and_act") {
		h["monitor_and_act"] = mt.handleMonitor(d)
	}
	if d.grant.allows("cancel_order") {
		h["cancel_order"] = mt.handleCancelOrder(d)
	}
	if d.grant.allows("work_miracle") {
		h["work_miracle"] = mt.handleMiracle(d)
	}
	if d.grant.allows("pause") {
		h["pause"] = mt.handlePause(d)
	}
	if d.grant.allows("start") {
		h["start"] = mt.handleStart(d)
	}
	if d.grant.allows("adjust_speed") {
		h["adjust_speed"] = mt.handleAdjustSpeed(d)
	}
	return h
}

// handleVision wraps landVision (spec 029 T006). Door accept → landed (the report
// is written onto the shared result); door/validation refusal → rejected_gate
// carrying the human-readable reason, fed back so the model may correct a bad
// target within the round cap.
func (mt *Metatron) handleVision(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		target := argString(call.Args, "target")
		text := argString(call.Args, "text")
		if nudge, why := mt.landVision(target, text, d.charges, d.alive, d.grant); nudge != nil {
			d.result.Nudge = nudge
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the vision reached its mark"}
		} else {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// handleOmen wraps landOmen (spec 029 T006/T016): the `targets` arg is a
// comma-list or "everyone" (R3); the mirrored night flag (d.night) chooses the
// path. A NIGHT omen lands at once (nudge); a DAY omen defers to nightfall as a
// system-origin standing order (order) — the ResultForModel promises nightfall so
// the model's reply sets the player's expectation. A bad name / ungranted tool /
// rejected deferral is fed back as a rejected_gate the model may repair.
func (mt *Metatron) handleOmen(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		targets := argString(call.Args, "targets")
		text := argString(call.Args, "text")
		nudge, order, why := mt.landOmen(targets, text, d.charges, d.night, d.tick, d.alive, d.grant)
		switch {
		case nudge != nil:
			d.result.Nudge = nudge
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the omen will reach them"}
		case order != nil:
			d.result.Order = &OrderReport{ID: order.ID, Condition: order.Condition}
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded,
				ResultForModel: "an omen belongs to the night — I have set it to reach them the moment darkness falls (" + order.ID + ")"}
		default:
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// handleMonitor wraps placeOrder (spec 029 T009): a monitor_and_act call places a
// player-origin standing order through the InjectSocial door. A landed placement
// writes the OrderReport onto the shared result and ends the turn (one act, the
// Expressive cardinality); a door rejection (cap reached, ttl out of range,
// unknown watched villager, uncompilable condition) is fed back as a rejected_gate
// the model may repair within the round cap. The driver's schema already rejected
// an empty event_types as rejected_malformed; a semantically uncompilable
// condition (e.g. unknown agent) is this gate's refusal-with-counsel (research R5).
func (mt *Metatron) handleMonitor(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		if order, why := mt.placeOrder("player", parseOrderArgs(call.Args), d.tick, d.grant); order != nil {
			d.result.Order = &OrderReport{ID: order.ID, Condition: order.Condition}
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the watch is set (" + order.ID + ")"}
		} else {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// handleCancelOrder wraps cancelOrder (spec 029 T009): cancel_order lands
// metatron.order_cancelled for the named id. The reducer resolves the
// cancel/expiry/trigger race — an unknown or already-lapsed id refuses with
// counsel. A landed cancel records the id and ends the turn.
func (mt *Metatron) handleCancelOrder(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		id := argString(call.Args, "id")
		if why := mt.cancelOrder(id, d.grant); why == "" {
			d.result.Cancelled = append(d.result.Cancelled, id)
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the watch is released"}
		} else {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// handleMiracle wraps landMiracle. Same accept/reject translation as handleVision.
func (mt *Metatron) handleMiracle(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		if miracle, why := mt.landMiracle(parseMiracleArgs(call.Args), d.charges, d.grant); miracle != nil {
			d.result.Miracle = miracle
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the miracle is worked: " + miracle.Summary}
		} else {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
	}
}

// handlePause / handleStart / handleAdjustSpeed drive the LoopControl seam (spec
// 029 US5, T018/R10). Effect Expressive with EMPTY Events: they inject NOTHING
// (the loop's own clock.paused / clock.resumed / clock.speed_set stay the record),
// spend no charge, and consume the turn's one act. The mapping is R10's:
// pause→Do("pause"), start→Do("resume", speed-or-empty), adjust_speed→
// Do("set_speed", speed). A LoopControl error maps to an in-fiction rejected_gate.
func (mt *Metatron) handlePause(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, _ llm.ToolCall) toolloop.Outcome {
		if why := mt.controlLoop(d, "pause", "", "the world holds still — I have paused it"); why != "" {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
		return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the world holds still — I have paused it"}
	}
}

// handleStart resumes the clock (spec 029 R10): start→Do("resume", speed). The
// loop's resume command changes only the paused state — a supplied speed is inert
// through resume (see the implementer finding); the pace is set with adjust_speed.
func (mt *Metatron) handleStart(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		speed := clock.Speed(argString(call.Args, "speed"))
		if why := mt.controlLoop(d, "resume", speed, "the world moves again"); why != "" {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
		return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the world moves again"}
	}
}

// handleAdjustSpeed sets the clock pace (spec 029 R10): adjust_speed→
// Do("set_speed", speed). The `speed` arg is a required Enum over clockSpeeds, so
// the driver already gated membership; ParseSpeed is the door-side re-check.
func (mt *Metatron) handleAdjustSpeed(d *turnDispatch) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		raw := argString(call.Args, "speed")
		speed, err := clock.ParseSpeed(raw)
		if err != nil {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal("that is not a pace I can set (" + raw + ")")}
		}
		if why := mt.controlLoop(d, "set_speed", speed, "the world now moves at "+raw); why != "" {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: refusal(why)}
		}
		return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "the world now moves at " + raw}
	}
}

// controlLoop calls the LoopControl seam and, on success, sets the human-readable
// Clock line on the shared result (so the turn records the meta act even when the
// model adds no prose — the "nothing landed" fallback keys on result.Clock too).
// Returns "" on success or an in-fiction refusal the handler feeds back as a
// rejected_gate. A nil seam (a world wired without loop control) refuses in
// fiction rather than panicking — defense-in-depth behind handler absence.
func (mt *Metatron) controlLoop(d *turnDispatch, name string, speed clock.Speed, clockLine string) string {
	if mt.loop == nil {
		return "I cannot touch the flow of time in this world"
	}
	if _, err := mt.loop.Do(name, speed); err != nil {
		return "the world would not heed me (" + err.Error() + ")"
	}
	d.result.Clock = clockLine
	return ""
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

// emitRetried surfaces the console turn's one in-loop transport retry (spec 025
// FR-004/SC-003): when the loop consumed its retry — recovered OR twice-failed —
// it lands a NON-TERMINAL cog.outcome carrying sim.OutcomeRetried and the first
// failure's reason, through the same InjectSocial door the cog.tool_call batch
// rides. No new event type (cog.outcome/retried is the TASK-42 vocabulary — the
// digest catalog stays green), and a silent retry is a contract violation: this
// makes the recovery countable from the trail alone. jobID/snapshotTick match
// the turn's cog.tool_call correlation key so the marker joins the same chain.
func (mt *Metatron) emitRetried(jobID string, snapshotTick int64, reason string) {
	if mt.social == nil {
		return
	}
	b, _ := json.Marshal(sim.CogOutcomePayload{
		Job:          jobID,
		Outcome:      sim.OutcomeRetried,
		SnapshotTick: snapshotTick,
		Reason:       reason,
	})
	if err := mt.social.InjectSocial([]store.Event{{Type: "cog.outcome", Payload: b}}); err != nil {
		// The world outlives its observability — logged, never fatal.
		log.Printf("metatron: retry telemetry rejected: %v", err)
	}
}

package toolloop

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/evanstern/promptworld/internal/llm"
)

// Verdict is the disposition the driver records for one model tool call
// (data-model.md §5). The DRIVER owns rejected_unknown / rejected_malformed /
// rejected_cardinality / unlanded; a handler's Outcome owns landed /
// rejected_gate / read_ok / read_error. Every model tool call ends with
// exactly one of these.
type Verdict string

const (
	// VerdictLanded — an acting call was admitted by its door; grounding events
	// were emitted. Consumes the cognition's one action and ends the loop.
	VerdictLanded Verdict = "landed"
	// VerdictRejectedGate — the door refused the acting call (stale / guard /
	// scene / charge). Does not consume the action; the loop may continue.
	VerdictRejectedGate Verdict = "rejected_gate"
	// VerdictRejectedCardinality — an acting call arrived after one had already
	// landed this cognition. Recorded, never dispatched; the loop ends.
	VerdictRejectedCardinality Verdict = "rejected_cardinality"
	// VerdictRejectedUnknown — the call named a tool that is not on this
	// cognition's roster (or carries no handler).
	VerdictRejectedUnknown Verdict = "rejected_unknown"
	// VerdictRejectedMalformed — the call's arguments failed driver-side schema
	// / param validation (a reason the model can repair).
	VerdictRejectedMalformed Verdict = "rejected_malformed"
	// VerdictReadOK / VerdictReadError — a read-effect dispatch's outcome.
	VerdictReadOK    Verdict = "read_ok"
	VerdictReadError Verdict = "read_error"
	// VerdictUnlanded — the loop terminated (cap reached / infrastructure
	// failure) before this call could ground. Grounds nothing.
	VerdictUnlanded Verdict = "unlanded"
)

// CallRecord is the first-class artifact for one model tool call (FR-007). The
// consumer's Record sink lands it as a cog.tool_call event; the {JobID, Ordinal}
// pair is the correlation key (ordinals are 1-based, dense per job, in
// model-emission order across every round). Args is a capped copy (see capArgs).
type CallRecord struct {
	JobID   string
	Ordinal int
	Tool    string
	Args    json.RawMessage // capped copy of the call's arguments
	Verdict Verdict
	Reason  string
	Tier    string
}

// maxArgsBytes caps a recorded arguments copy. It mirrors the 2 KiB cap the
// cog.tool_call payload enforces (contracts/events.md); capping here keeps the
// CallRecord self-consistent, and the sim payload re-applies the identical
// marker (spec 017 T016), so the two never disagree.
const maxArgsBytes = 2048

// capArgs returns a bounded copy of a call's raw arguments. Arguments within the
// cap are copied verbatim (the record must not alias the transcript's buffer);
// anything larger collapses to a valid JSON string field
// {"_truncated":true,"prefix":"…"} carrying a UTF-8-clean prefix, matching the
// events.md truncation contract.
func capArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) <= maxArgsBytes {
		out := make(json.RawMessage, len(raw))
		copy(out, raw)
		return out
	}
	prefix := string(raw[:maxArgsBytes])
	// A byte-boundary cut can split a multi-byte rune; drop the dangling
	// partial so the prefix is valid UTF-8 (json.Marshal would otherwise
	// substitute U+FFFD).
	for len(prefix) > 0 && !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	b, err := json.Marshal(map[string]any{"_truncated": true, "prefix": prefix})
	if err != nil {
		// Marshaling a bool and a string cannot fail.
		panic("toolloop: capArgs marshal: " + err.Error())
	}
	return b
}

// resultBlock builds a user-turn tool_result block feeding one call's outcome
// back to the model, keyed to the call by ID.
func resultBlock(id, content string, isErr bool) llm.Block {
	return llm.Block{ToolResult: &llm.ToolResultBlock{ForID: id, Content: content, IsError: isErr}}
}

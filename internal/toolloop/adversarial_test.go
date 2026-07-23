package toolloop

// Adversarial verification pass (spec 017 T025): tests that try to BREAK the
// loop's core invariants — bounded termination under hostile model behavior,
// cardinality (exactly one landed acting call; consumed only on LANDED),
// dense per-call records on every path, the cap off-by-one, args capping at the
// boundary, and byte-deterministic wire declarations (prompt-cache stability).
//
// Each test states the invariant it attacks; a passing test is the evidence the
// invariant holds against that attack. New attacks the T024-era suite did not
// cover are added here rather than in loop_test.go so the adversarial pass is
// legible as one artifact.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// respStop scripts a response with an explicit Stop reason (the resp() helper in
// loop_test.go derives Stop from len(calls); several attacks below need a Stop
// that DISAGREES with the call count — the whole point of the max-tokens attack).
func respStop(stop llm.StopReason, text string, calls ...llm.ToolCall) func(llm.Request) (llm.Response, error) {
	return func(llm.Request) (llm.Response, error) {
		return llm.Response{Tier: llm.TierLocal, Text: text, ToolCalls: calls, Stop: stop}, nil
	}
}

// --- Invariant 2/4: cardinality consumes only on LANDED ---

// TestGateRejectThenLandSameBatch: two acting calls in ONE response, the first
// gate-rejected, the second landable. The rejected call must NOT consume the
// cognition's action, so the second must get a fair dispatch and LAND in the
// same round (data-model §5: rejected_gate "does not consume the action"). This
// is the same-round mirror of TestRejectedGateRetryWithinCap (which retries in a
// later round) — it pins that cardinality keys on LANDED, not on "an acting call
// was seen".
func TestGateRejectThenLandSameBatch(t *testing.T) {
	forage := lookup(t, "forage")
	chop := lookup(t, "chop")
	h := drive(t, 8, []tool.Tool{forage, chop},
		map[string]Handler{"forage": gateHandler("no berries in range"), "chop": landHandler("chopping")},
		resp("", call("a1", "forage", "{}"), call("a2", "chop", "{}")),
	)
	if h.err != nil || h.res.Term != TermLanded {
		t.Fatalf("term = %q err = %v, want landed", h.res.Term, h.err)
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1 (both calls dispatched in one round)", h.res.Rounds)
	}
	if h.res.Landed == nil || h.res.Landed.Name != "chop" {
		t.Fatalf("landed = %+v, want chop (the second call, after the first was rejected)", h.res.Landed)
	}
	// The gate rejection is recorded rejected_gate (not cardinality) and the
	// second call is recorded landed — NOT rejected_cardinality.
	if !eqVerdicts(h.verdicts(), VerdictRejectedGate, VerdictLanded) {
		t.Fatalf("verdicts = %v, want [rejected_gate landed] — a rejected acting call must not consume the action", h.verdicts())
	}
	h.assertObservedOnce(t)
}

// TestTwoLandableCallsOnlyFirstLands: two calls both wired to land. Exactly one
// may land; the second must be rejected_cardinality (never a double-land). This
// is the core cardinality attack — a model that emits two "winning" acts.
func TestTwoLandableCallsOnlyFirstLands(t *testing.T) {
	forage := lookup(t, "forage")
	chop := lookup(t, "chop")
	landed := 0
	countingLand := func(context.Context, llm.ToolCall) Outcome {
		landed++
		return Outcome{Verdict: VerdictLanded, ResultForModel: "ok"}
	}
	h := drive(t, 8, []tool.Tool{forage, chop},
		map[string]Handler{"forage": countingLand, "chop": countingLand},
		resp("", call("a1", "forage", "{}"), call("a2", "chop", "{}")),
	)
	if h.res.Term != TermLanded || h.res.Landed == nil || h.res.Landed.Name != "forage" {
		t.Fatalf("landed = %+v term = %q, want forage/landed", h.res.Landed, h.res.Term)
	}
	if landed != 1 {
		t.Fatalf("landing handler fired %d times, want 1 — the second acting call must NOT reach a door", landed)
	}
	if !eqVerdicts(h.verdicts(), VerdictLanded, VerdictRejectedCardinality) {
		t.Fatalf("verdicts = %v, want [landed rejected_cardinality]", h.verdicts())
	}
}

// TestHandlerLandsButErrors: a handler that reports BOTH Verdict==landed AND a
// non-nil Err. Err is an infrastructure failure and must win: nothing lands, the
// loop terminates provider_error, and Landed stays nil (a fact was never
// admitted, so the cognition must not claim one).
func TestHandlerLandsButErrors(t *testing.T) {
	forage := lookup(t, "forage")
	boom := errors.New("door acked then the process died")
	landButErr := func(context.Context, llm.ToolCall) Outcome {
		return Outcome{Verdict: VerdictLanded, ResultForModel: "ok", Err: boom}
	}
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landButErr},
		resp("", call("a1", "forage", "{}")),
	)
	if h.res.Term != TermProviderError || !errors.Is(h.err, boom) {
		t.Fatalf("term = %q err = %v, want provider_error / boom (Err must win over Verdict)", h.res.Term, h.err)
	}
	if h.res.Landed != nil {
		t.Errorf("landed = %+v, want nil — an errored handler must not count as a landing", h.res.Landed)
	}
	if !eqVerdicts(h.verdicts(), VerdictUnlanded) {
		t.Errorf("verdicts = %v, want [unlanded]", h.verdicts())
	}
	// provider_error is a failure termination — successes-only feed (T025b).
	h.assertNotObserved(t)
}

// --- Invariant 4: cap off-by-one and mixed final-round batch ---

// TestMaxRoundsOneActingLands: with MaxRounds=1, an acting call in round 1 (the
// first and only round, which is also the final round) MUST still land — acting
// calls are dispatched on every round including the last.
func TestMaxRoundsOneActingLands(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 1, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		resp("", call("a1", "forage", "{}")),
	)
	if h.res.Term != TermLanded || h.res.Landed == nil || h.res.Landed.Name != "forage" {
		t.Fatalf("term = %q landed = %+v, want landed/forage", h.res.Term, h.res.Landed)
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1", h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictLanded) {
		t.Errorf("verdicts = %v, want [landed]", h.verdicts())
	}
}

// TestMaxRoundsOneMixedFinalBatchReadThenAct: MaxRounds=1, one response with a
// read THEN an acting call. The final-round read grounds nothing (recorded
// unlanded — its result could inform no future round), but the acting call in
// the same batch must still LAND. This is the mixed-batch case the final-round-
// read asymmetry must not mis-handle.
func TestMaxRoundsOneMixedFinalBatchReadThenAct(t *testing.T) {
	forage := lookup(t, "forage")
	readFired := false
	h := drive(t, 1, []tool.Tool{readTool, forage},
		map[string]Handler{
			"peek": func(context.Context, llm.ToolCall) Outcome {
				readFired = true
				return Outcome{Verdict: VerdictReadOK, ResultForModel: "d"}
			},
			"forage": landHandler("ok"),
		},
		resp("", call("r1", "peek", "{}"), call("a1", "forage", "{}")),
	)
	if h.res.Term != TermLanded || h.res.Landed == nil || h.res.Landed.Name != "forage" {
		t.Fatalf("term = %q landed = %+v, want landed/forage", h.res.Term, h.res.Landed)
	}
	if readFired {
		t.Errorf("the final-round read must NOT be dispatched (its result can inform no future round)")
	}
	if !eqVerdicts(h.verdicts(), VerdictUnlanded, VerdictLanded) {
		t.Fatalf("verdicts = %v, want [unlanded landed] — read recorded unlanded, act still lands", h.verdicts())
	}
}

// TestMaxRoundsOneMixedFinalBatchActThenRead: the act comes FIRST and lands, so
// the trailing read is rejected_cardinality (the action is spent), NOT unlanded.
func TestMaxRoundsOneMixedFinalBatchActThenRead(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 1, []tool.Tool{readTool, forage},
		map[string]Handler{"peek": readHandler("d"), "forage": landHandler("ok")},
		resp("", call("a1", "forage", "{}"), call("r1", "peek", "{}")),
	)
	if h.res.Term != TermLanded {
		t.Fatalf("term = %q, want landed", h.res.Term)
	}
	if !eqVerdicts(h.verdicts(), VerdictLanded, VerdictRejectedCardinality) {
		t.Fatalf("verdicts = %v, want [landed rejected_cardinality]", h.verdicts())
	}
}

// --- Invariant 1: bounded termination under hostile stop reasons ---

// TestStopMaxTokensMidCallMalformedTerminates: the model is truncated mid-tool-
// call (StopMaxTokens) leaving unparseable arguments. The loop must not hang or
// mis-dispatch: it records rejected_malformed, feeds the error back, and stays
// bounded — here a later clean round lands. The loop is Stop-agnostic by design,
// so an adversarial Stop value cannot change termination.
func TestStopMaxTokensMidCallMalformedTerminates(t *testing.T) {
	forage := lookup(t, "forage")
	drop := lookup(t, "drop")
	h := drive(t, 8, []tool.Tool{forage, drop},
		map[string]Handler{"forage": landHandler("ok"), "drop": landHandler("ok")},
		// StopMaxTokens with a call whose args JSON was cut off mid-object.
		respStop(llm.StopMaxTokens, "", call("m1", "drop", `{"qty":`)),
		resp("", call("a1", "forage", "{}")),
	)
	if h.err != nil || h.res.Term != TermLanded {
		t.Fatalf("term = %q err = %v, want landed (max-tokens truncation must stay bounded)", h.res.Term, h.err)
	}
	if h.res.Rounds != 2 {
		t.Errorf("rounds = %d, want 2", h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictRejectedMalformed, VerdictLanded) {
		t.Fatalf("verdicts = %v, want [rejected_malformed landed]", h.verdicts())
	}
}

// TestToolUseStopWithZeroCallsTerminates: an adversarial/malformed provider says
// Stop==tool_use but returns ZERO calls. The loop must treat "no actionable
// call" as model_done and terminate — never spin waiting for calls that will
// never come.
func TestToolUseStopWithZeroCallsTerminates(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		respStop(llm.StopToolUse, ""), // tool_use stop, but no calls
	)
	if h.err != nil || h.res.Term != TermModelDone {
		t.Fatalf("term = %q err = %v, want model_done", h.res.Term, h.err)
	}
	if h.res.Rounds != 1 || len(h.recs) != 0 {
		t.Errorf("rounds = %d recs = %d, want 1/0", h.res.Rounds, len(h.recs))
	}
	h.assertObservedOnce(t)
}

// TestGiantBatchAllUnknownTerminatesOneRound: a single response with a huge
// batch of off-roster calls. Every call is recorded (dense ordinals), the round
// is bounded, and the loop still terminates at the cap — no unbounded growth
// beyond the response's own size.
func TestGiantBatchAllUnknownTerminatesOneRound(t *testing.T) {
	const n = 5000
	calls := make([]llm.ToolCall, n)
	for i := range calls {
		calls[i] = call("g", "nonesuch", "{}")
	}
	h := drive(t, 1, []tool.Tool{lookup(t, "forage")},
		map[string]Handler{"forage": landHandler("ok")},
		resp("", calls...),
	)
	if h.res.Term != TermCapExhausted {
		t.Fatalf("term = %q, want cap_exhausted", h.res.Term)
	}
	if len(h.recs) != n {
		t.Fatalf("records = %d, want %d (one per model call)", len(h.recs), n)
	}
	for i, r := range h.recs {
		if r.Ordinal != i+1 {
			t.Fatalf("record %d ordinal = %d, want %d (dense, 1-based)", i, r.Ordinal, i+1)
			break
		}
		if r.Verdict != VerdictRejectedUnknown {
			t.Fatalf("record %d verdict = %q, want rejected_unknown", i, r.Verdict)
			break
		}
	}
	h.assertObservedOnce(t)
}

// TestRepeatedIdenticalReadsExhaustCap: a model that emits the identical read
// call forever must still be bounded by the cap.
func TestRepeatedIdenticalReadsExhaustCap(t *testing.T) {
	scripts := make([]func(llm.Request) (llm.Response, error), 16)
	for i := range scripts {
		scripts[i] = resp("", call("r", "peek", `{"same":"args"}`))
	}
	h := drive(t, 16, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("nothing changes")},
		scripts...,
	)
	if h.res.Term != TermCapExhausted || h.res.Rounds != 16 {
		t.Fatalf("term = %q rounds = %d, want cap_exhausted / 16", h.res.Term, h.res.Rounds)
	}
	if h.orch.calls != 16 {
		t.Errorf("Submit called %d times, want exactly 16 (bounded by the cap)", h.orch.calls)
	}
}

// --- Invariant 3: args capping at the boundary, record does not alias ---

// TestCapArgsExactBoundary: input of exactly maxArgsBytes is copied verbatim
// (the <= boundary); one byte over truncates to the marker.
func TestCapArgsExactBoundary(t *testing.T) {
	// Build a valid JSON object whose encoded length is exactly maxArgsBytes.
	fill := maxArgsBytes - len(`{"k":""}`)
	exact := json.RawMessage(`{"k":"` + strings.Repeat("a", fill) + `"}`)
	if len(exact) != maxArgsBytes {
		t.Fatalf("test setup: exact len = %d, want %d", len(exact), maxArgsBytes)
	}
	out := capArgs(exact)
	if string(out) != string(exact) {
		t.Errorf("exactly-2048 args must be copied verbatim, got a %d-byte transform", len(out))
	}
	over := json.RawMessage(`{"k":"` + strings.Repeat("a", fill+1) + `"}`)
	capped := capArgs(over)
	var marker struct {
		Truncated bool `json:"_truncated"`
	}
	if err := json.Unmarshal(capped, &marker); err != nil || !marker.Truncated {
		t.Errorf("one byte over the cap must truncate to the marker, got %q err=%v", capped, err)
	}
}

// TestCapArgsMultiByteRuneBoundary: when the 2048-byte cut lands in the middle of
// a multi-byte rune, the truncated prefix must remain valid UTF-8 (json.Marshal
// would otherwise substitute U+FFFD, silently corrupting the record).
func TestCapArgsMultiByteRuneBoundary(t *testing.T) {
	// "世" is 3 bytes (E4 B8 96). Pad so the 2048-byte boundary falls inside one.
	// Prefix up to byte 2047, then a run of multi-byte runes crossing 2048.
	head := `{"k":"` + strings.Repeat("a", maxArgsBytes-len(`{"k":"`)-1)
	raw := json.RawMessage(head + strings.Repeat("世", 20) + `"}`)
	if len(raw) <= maxArgsBytes {
		t.Fatalf("test setup: raw must exceed the cap, got %d", len(raw))
	}
	capped := capArgs(raw)
	if !utf8.Valid(capped) {
		t.Fatalf("capped args are not valid UTF-8: %q", capped)
	}
	var marker struct {
		Truncated bool   `json:"_truncated"`
		Prefix    string `json:"prefix"`
	}
	if err := json.Unmarshal(capped, &marker); err != nil || !marker.Truncated {
		t.Fatalf("truncation marker malformed: %q err=%v", capped, err)
	}
	if !utf8.ValidString(marker.Prefix) {
		t.Errorf("truncated prefix is not valid UTF-8: %q", marker.Prefix)
	}
	// The prefix must not end in a dangling partial rune (no trailing U+FFFD).
	if strings.ContainsRune(marker.Prefix, utf8.RuneError) {
		t.Errorf("prefix contains U+FFFD — a multi-byte rune was split, not dropped")
	}
}

// TestRecordArgsDoNotAliasResponseBuffer: the CallRecord's Args must be an
// independent copy — mutating the response's arg buffer after the loop must not
// change what was recorded. A record that aliased the transcript buffer could be
// silently rewritten before the consumer lands it as a cog.tool_call.
func TestRecordArgsDoNotAliasResponseBuffer(t *testing.T) {
	forage := lookup(t, "forage")
	shared := json.RawMessage(`{"k":"v"}`)
	scriptMutating := func(llm.Request) (llm.Response, error) {
		return llm.Response{Tier: llm.TierLocal, Stop: llm.StopToolUse,
			ToolCalls: []llm.ToolCall{{ID: "a1", Name: "forage", Args: shared}}}, nil
	}
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		scriptMutating,
	)
	if len(h.recs) != 1 {
		t.Fatalf("records = %d, want 1", len(h.recs))
	}
	before := string(h.recs[0].Args)
	// Corrupt the shared buffer the response handed in.
	for i := range shared {
		shared[i] = 'Z'
	}
	if string(h.recs[0].Args) != before {
		t.Errorf("record Args aliased the response buffer: was %q, now %q", before, string(h.recs[0].Args))
	}
}

// --- Invariant 7 (KNOWN FLAGGED): deterministic wire declarations ---

// declsFor mirrors exactly how run() builds the []llm.ToolDecl it sends on the
// wire from a roster, so the determinism this test pins is the determinism the
// provider actually sees (and the Anthropic prompt-cache prefix depends on).
func declsFor(roster []tool.Tool) []llm.ToolDecl {
	tools := make([]llm.ToolDecl, 0, len(roster))
	for _, tl := range roster {
		tools = append(tools, llm.ToolDecl{
			Name:        tl.Name,
			Description: tl.PromptGloss,
			InputSchema: tool.InputSchema(tl),
		})
	}
	return tools
}

func marshalDecls(t *testing.T, decls []llm.ToolDecl) string {
	t.Helper()
	b, err := json.Marshal(decls)
	if err != nil {
		t.Fatalf("marshal decls: %v", err)
	}
	return string(b)
}

// TestWireDeclarationsDeterministic: two independent constructions of each
// production roster's wire declarations must be BYTE-IDENTICAL. Any Go-map
// iteration reaching declaration order or a tool's schema would make the wire
// bytes vary per cognition — busting the prompt-cache prefix (cost) and making
// runs non-reproducible (the T023-flagged risk). This pins the invariant at the
// exact surface run() feeds the transport.
func TestWireDeclarationsDeterministic(t *testing.T) {
	rosters := map[string]func() []tool.Tool{
		"villager": tool.LoopRosterVillager,
		"metatron": tool.LoopRosterMetatron,
	}
	for name, build := range rosters {
		t.Run(name, func(t *testing.T) {
			var first string
			for i := 0; i < 32; i++ {
				got := marshalDecls(t, declsFor(build()))
				if i == 0 {
					first = got
					continue
				}
				if got != first {
					t.Fatalf("construction %d differs from the first — non-deterministic wire declarations:\n%s\nvs\n%s", i, first, got)
				}
			}
			// Sanity: the roster is non-empty and every decl carries a schema.
			for _, d := range declsFor(build()) {
				if d.Name == "" || len(d.InputSchema) == 0 {
					t.Errorf("declaration missing name/schema: %+v", d)
				}
			}
		})
	}
}

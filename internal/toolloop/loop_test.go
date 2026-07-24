package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// Test seam (documented choice): Run's contract signature takes the concrete
// *llm.Orchestrator, but the driver's control flow is exercised through the
// unexported run() over the minimal `submitter` interface (Submit +
// ObserveCognition), which *llm.Orchestrator satisfies. A scripted stub lets
// each test drive per-round responses, transport errors, and cancellation
// deterministically with no network, and count ObserveCognition reports
// exactly — the clearer tests the loop-api §Test-contract item 1 asks for. The
// env-<round> ID synthesis these transcripts feed is pinned end-to-end in
// internal/llm/openai_tool_test.go; here we pin the loop's own obligation that
// the synthesis depends on: exactly one assistant turn appended per round.

// stubOrch is a scripted submitter: one func per provider round returning that
// round's response (or a transport error). It captures every request (to assert
// SkipObserve and transcript growth) and every ObserveCognition report.
type stubOrch struct {
	t         *testing.T
	scripts   []func(req llm.Request) (llm.Response, error)
	calls     int
	reqs      []llm.Request
	observes  []int64
	observedP []string // the serving provider each ObserveCognition carried
	// resolve is the provider name Run's run-level pin seam (ResolveProvider)
	// returns (spec 024 R9). Empty (the default of every pre-T022 test) leaves the
	// run unpinned — Submit's Provider stays whatever Job.Provider carried, so the
	// existing loop/retry tests behave exactly as before. A pin test sets it so the
	// stub resolves a concrete provider once at run start.
	resolve      string
	resolveCalls int // how many times Run asked the pin seam (must be ≤1 per run)
}

func (s *stubOrch) Submit(ctx context.Context, req llm.Request) (llm.Response, error) {
	if !req.SkipObserve {
		s.t.Errorf("round %d: Submit request must set SkipObserve", s.calls+1)
	}
	s.reqs = append(s.reqs, req)
	if s.calls >= len(s.scripts) {
		s.t.Fatalf("unexpected Submit #%d (only %d rounds scripted)", s.calls+1, len(s.scripts))
	}
	f := s.scripts[s.calls]
	s.calls++
	return f(req)
}

func (s *stubOrch) ObserveCognition(kind llm.Kind, provider string, totalMillis int64) {
	s.observes = append(s.observes, totalMillis)
	s.observedP = append(s.observedP, provider)
}

// ResolveProvider is the run-level pin seam: the loop calls it once at run start
// (when Job.Provider is empty) to name the run's provider. Returns the scripted
// resolve name; an empty name leaves the run unpinned (pre-T022 behavior).
func (s *stubOrch) ResolveProvider(kind llm.Kind) (string, error) {
	s.resolveCalls++
	return s.resolve, nil
}

// --- helpers ---

func call(id, name, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: name, Args: json.RawMessage(args)}
}

// resp scripts a response: tool_use stop when calls are present, else end_turn.
func resp(text string, calls ...llm.ToolCall) func(llm.Request) (llm.Response, error) {
	return func(llm.Request) (llm.Response, error) {
		r := llm.Response{Tier: llm.TierLocal, Text: text, ToolCalls: calls}
		if len(calls) > 0 {
			r.Stop = llm.StopToolUse
		} else {
			r.Stop = llm.StopEndTurn
		}
		return r, nil
	}
}

func fail(err error) func(llm.Request) (llm.Response, error) {
	return func(llm.Request) (llm.Response, error) { return llm.Response{}, err }
}

// respFrom scripts a response served by a NAMED provider (spec 024): the loop
// captures resp.Provider so the whole-loop observation lands on the estimator
// that actually did the work, not blindly on the kind's chain head.
func respFrom(provider, text string, calls ...llm.ToolCall) func(llm.Request) (llm.Response, error) {
	return func(req llm.Request) (llm.Response, error) {
		r, _ := resp(text, calls...)(req)
		r.Provider = provider
		return r, nil
	}
}

// readTool is the read-effect fixture: the registry ships zero Read entries
// (they arrive with TASK-16), so the loop's read-dispatch path is proven with a
// fixture tool, exactly as research R12 anticipates.
var readTool = tool.Tool{Name: "peek", Effect: tool.Read}

func lookup(t *testing.T, name string) tool.Tool {
	t.Helper()
	tl, ok := tool.Lookup(name)
	if !ok {
		t.Fatalf("registry missing %q", name)
	}
	return tl
}

// landHandler / readHandler / gateHandler / errHandler are the four handler
// shapes the driver dispatches against.
func landHandler(msg string) Handler {
	return func(context.Context, llm.ToolCall) Outcome {
		return Outcome{Verdict: VerdictLanded, ResultForModel: msg}
	}
}
func readHandler(data string) Handler {
	return func(context.Context, llm.ToolCall) Outcome {
		return Outcome{Verdict: VerdictReadOK, ResultForModel: data}
	}
}
func gateHandler(reason string) Handler {
	return func(context.Context, llm.ToolCall) Outcome {
		return Outcome{Verdict: VerdictRejectedGate, ResultForModel: reason}
	}
}
func errHandler(err error) Handler {
	return func(context.Context, llm.ToolCall) Outcome {
		return Outcome{Err: err}
	}
}

type harness struct {
	orch *stubOrch
	recs []CallRecord
	res  Result
	err  error
}

func drive(t *testing.T, maxRounds int, roster []tool.Tool, handlers map[string]Handler, scripts ...func(llm.Request) (llm.Response, error)) *harness {
	t.Helper()
	return driveCtx(t, context.Background(), maxRounds, roster, handlers, scripts...)
}

func driveCtx(t *testing.T, ctx context.Context, maxRounds int, roster []tool.Tool, handlers map[string]Handler, scripts ...func(llm.Request) (llm.Response, error)) *harness {
	t.Helper()
	h := &harness{orch: &stubOrch{t: t, scripts: scripts}}
	job := Job{
		JobID:     "planner-ada-412800",
		Kind:      llm.KindPlanner,
		System:    "you are ada",
		Seed:      "what next?",
		Roster:    roster,
		Handlers:  handlers,
		MaxRounds: maxRounds,
		Record:    func(r CallRecord) { h.recs = append(h.recs, r) },
	}
	h.res, h.err = run(ctx, h.orch, job)
	return h
}

// assertObservedOnce pins the successes-only feed (T025b): a COMPLETED
// termination (landed / model_done / cap_exhausted) feeds the estimator exactly
// once. Failure terminations feed nothing — see assertNotObserved.
func (h *harness) assertObservedOnce(t *testing.T) {
	t.Helper()
	if len(h.orch.observes) != 1 {
		t.Errorf("ObserveCognition called %d times, want exactly 1 (completed termination)", len(h.orch.observes))
	}
}

// assertNotObserved pins the other half of the successes-only rule: a failure
// termination (admission_refused / provider_error / ctx_done) did no completed
// model work and must NOT feed the estimator, so it cannot skew the EWMA.
func (h *harness) assertNotObserved(t *testing.T) {
	t.Helper()
	if len(h.orch.observes) != 0 {
		t.Errorf("ObserveCognition called %d times on a failure path, want 0 (successes-only feed)", len(h.orch.observes))
	}
}

func (h *harness) verdicts() []Verdict {
	out := make([]Verdict, len(h.recs))
	for i, r := range h.recs {
		out[i] = r.Verdict
	}
	return out
}

func eqVerdicts(got []Verdict, want ...Verdict) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func countAssistant(turns []llm.Turn) int {
	n := 0
	for _, tn := range turns {
		if tn.Role == llm.RoleAssistant {
			n++
		}
	}
	return n
}

// --- tests ---

// (1) An immediate acting call lands.
func TestImmediateActingLands(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("foraging")},
		resp("", call("c1", "forage", "{}")),
	)
	if h.err != nil {
		t.Fatalf("err = %v", h.err)
	}
	if h.res.Term != TermLanded {
		t.Fatalf("term = %q, want landed", h.res.Term)
	}
	if h.res.Landed == nil || h.res.Landed.Name != "forage" {
		t.Fatalf("landed = %+v, want forage", h.res.Landed)
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1", h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictLanded) {
		t.Errorf("verdicts = %v, want [landed]", h.verdicts())
	}
	if h.recs[0].Ordinal != 1 || h.recs[0].Tier != "local" || h.recs[0].JobID != "planner-ada-412800" {
		t.Errorf("record meta = %+v", h.recs[0])
	}
	h.assertObservedOnce(t)
}

// (2) A read informs an action: read then act, results fed back.
func TestReadThenAct(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 8, []tool.Tool{readTool, forage},
		map[string]Handler{"peek": readHandler("berries nearby"), "forage": landHandler("ok")},
		resp("", call("r1", "peek", "{}")),
		resp("", call("a1", "forage", "{}")),
	)
	if h.err != nil || h.res.Term != TermLanded {
		t.Fatalf("term = %q err = %v, want landed", h.res.Term, h.err)
	}
	if h.res.Rounds != 2 {
		t.Errorf("rounds = %d, want 2", h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictReadOK, VerdictLanded) {
		t.Fatalf("verdicts = %v, want [read_ok landed]", h.verdicts())
	}
	// Round 2's request must carry round 1 echoed as an assistant tool_use and
	// the read result fed back as a user tool_result keyed to the same ID.
	r2 := h.orch.reqs[1]
	if countAssistant(r2.Turns) != 1 {
		t.Fatalf("round 2 request has %d assistant turns, want 1", countAssistant(r2.Turns))
	}
	var sawUse, sawResult bool
	for _, tn := range r2.Turns {
		for _, b := range tn.Blocks {
			if b.ToolUse != nil && b.ToolUse.ID == "r1" && b.ToolUse.Name == "peek" {
				sawUse = true
			}
			if b.ToolResult != nil && b.ToolResult.ForID == "r1" && b.ToolResult.Content == "berries nearby" && !b.ToolResult.IsError {
				sawResult = true
			}
		}
	}
	if !sawUse || !sawResult {
		t.Errorf("round 2 transcript missing use/result: use=%v result=%v turns=%+v", sawUse, sawResult, r2.Turns)
	}
	h.assertObservedOnce(t)
}

// (3) A never-acting model exhausts the cap; the final round's read grounds
// nothing and is recorded unlanded.
func TestCapExhaustionNeverActing(t *testing.T) {
	h := drive(t, 3, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("nothing useful")},
		resp("", call("r1", "peek", "{}")),
		resp("", call("r2", "peek", "{}")),
		resp("", call("r3", "peek", "{}")),
	)
	if h.err != nil || h.res.Term != TermCapExhausted {
		t.Fatalf("term = %q err = %v, want cap_exhausted", h.res.Term, h.err)
	}
	if h.res.Rounds != 3 {
		t.Errorf("rounds = %d, want 3", h.res.Rounds)
	}
	if h.res.Landed != nil {
		t.Errorf("landed = %+v, want nil", h.res.Landed)
	}
	if !eqVerdicts(h.verdicts(), VerdictReadOK, VerdictReadOK, VerdictUnlanded) {
		t.Errorf("verdicts = %v, want [read_ok read_ok unlanded]", h.verdicts())
	}
	// dense 1-based ordinals across rounds.
	for i, r := range h.recs {
		if r.Ordinal != i+1 {
			t.Errorf("record %d ordinal = %d, want %d", i, r.Ordinal, i+1)
		}
	}
	if h.recs[2].Reason == "" {
		t.Errorf("unlanded record must carry a reason")
	}
	h.assertObservedOnce(t)
}

// (4) A batched response: read + acting + trailing acting → the read grounds,
// the first acting call lands, the trailing acting call is rejected on
// cardinality, and the loop ends that round.
func TestBatchedTrailingCardinality(t *testing.T) {
	forage := lookup(t, "forage")
	chop := lookup(t, "chop")
	h := drive(t, 8, []tool.Tool{readTool, forage, chop},
		map[string]Handler{"peek": readHandler("scan"), "forage": landHandler("ok"), "chop": landHandler("ok")},
		resp("", call("r1", "peek", "{}"), call("a1", "forage", "{}"), call("a2", "chop", "{}")),
	)
	if h.err != nil || h.res.Term != TermLanded {
		t.Fatalf("term = %q err = %v, want landed", h.res.Term, h.err)
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1", h.res.Rounds)
	}
	if h.res.Landed == nil || h.res.Landed.Name != "forage" {
		t.Fatalf("landed = %+v, want forage", h.res.Landed)
	}
	if !eqVerdicts(h.verdicts(), VerdictReadOK, VerdictLanded, VerdictRejectedCardinality) {
		t.Fatalf("verdicts = %v, want [read_ok landed rejected_cardinality]", h.verdicts())
	}
	if h.recs[2].Tool != "chop" {
		t.Errorf("trailing rejected call = %q, want chop", h.recs[2].Tool)
	}
	if h.orch.calls != 1 {
		t.Errorf("Submit called %d times, want 1 (loop ends on landing)", h.orch.calls)
	}
	h.assertObservedOnce(t)
}

// (5) A gate-rejected acting call is fed back; the model retries a different
// action within the cap and lands.
func TestRejectedGateRetryWithinCap(t *testing.T) {
	forage := lookup(t, "forage")
	chop := lookup(t, "chop")
	h := drive(t, 8, []tool.Tool{forage, chop},
		map[string]Handler{"forage": gateHandler("no berries in range"), "chop": landHandler("chopping")},
		resp("", call("a1", "forage", "{}")),
		resp("", call("a2", "chop", "{}")),
	)
	if h.err != nil || h.res.Term != TermLanded {
		t.Fatalf("term = %q err = %v, want landed", h.res.Term, h.err)
	}
	if h.res.Rounds != 2 {
		t.Errorf("rounds = %d, want 2", h.res.Rounds)
	}
	if h.res.Landed == nil || h.res.Landed.Name != "chop" {
		t.Fatalf("landed = %+v, want chop", h.res.Landed)
	}
	if !eqVerdicts(h.verdicts(), VerdictRejectedGate, VerdictLanded) {
		t.Fatalf("verdicts = %v, want [rejected_gate landed]", h.verdicts())
	}
	if h.recs[0].Reason != "no berries in range" {
		t.Errorf("gate rejection reason = %q, want the door's message", h.recs[0].Reason)
	}
	// Round 2 must feed the gate rejection back as an error result.
	r2 := h.orch.reqs[1]
	var fedBack bool
	for _, tn := range r2.Turns {
		for _, b := range tn.Blocks {
			if b.ToolResult != nil && b.ToolResult.ForID == "a1" && b.ToolResult.IsError {
				fedBack = true
			}
		}
	}
	if !fedBack {
		t.Errorf("round 2 did not feed the gate rejection back as an error result")
	}
	h.assertObservedOnce(t)
}

// (6) Unknown and malformed calls are recorded and fed back; the loop continues
// within the cap and eventually lands.
func TestUnknownAndMalformedFeedback(t *testing.T) {
	forage := lookup(t, "forage")
	drop := lookup(t, "drop") // qty is a Number param with Min 1
	h := drive(t, 8, []tool.Tool{forage, drop},
		map[string]Handler{"forage": landHandler("ok"), "drop": landHandler("ok")},
		resp("", call("u1", "fly", "{}")),         // off-roster
		resp("", call("m1", "drop", `{"qty":0}`)), // qty below Min 1
		resp("", call("a1", "forage", "{}")),
	)
	if h.err != nil || h.res.Term != TermLanded {
		t.Fatalf("term = %q err = %v, want landed", h.res.Term, h.err)
	}
	if h.res.Rounds != 3 {
		t.Errorf("rounds = %d, want 3", h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictRejectedUnknown, VerdictRejectedMalformed, VerdictLanded) {
		t.Fatalf("verdicts = %v, want [rejected_unknown rejected_malformed landed]", h.verdicts())
	}
	if h.recs[0].Reason == "" || h.recs[1].Reason == "" {
		t.Errorf("rejection records must carry a repairable reason: %+v", h.recs[:2])
	}
	// The unknown call was never dispatched, so drop's handler never fired for
	// the malformed round either — both were driver-side rejections fed back.
	r2 := h.orch.reqs[1]
	if countAssistant(r2.Turns) != 1 {
		t.Errorf("round 2 request assistant turns = %d, want 1", countAssistant(r2.Turns))
	}
	h.assertObservedOnce(t)
}

// (7) Budget exhausts mid-loop: round 2's Submit is refused before any spend.
func TestAdmissionRefusedMidLoop(t *testing.T) {
	h := drive(t, 8, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("data")},
		resp("", call("r1", "peek", "{}")),
		fail(llm.ErrBudgetExhausted),
	)
	if h.res.Term != TermAdmissionRefused {
		t.Fatalf("term = %q, want admission_refused", h.res.Term)
	}
	if !errors.Is(h.err, llm.ErrBudgetExhausted) {
		t.Errorf("err = %v, want ErrBudgetExhausted", h.err)
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1 (round 2 never completed)", h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictReadOK) {
		t.Errorf("verdicts = %v, want [read_ok]", h.verdicts())
	}
	h.assertNotObserved(t)
}

// (8a) A generic Submit failure terminates provider_error. Since spec 025 a
// transport provider error is retried ONCE first, so termination needs TWO
// consecutive failures — the retry is exercised in retry_test.go; here the pin
// is that a spent-retry provider error still terminates as before (rounds 0, no
// records, the error propagated, nothing observed).
func TestProviderErrorFromSubmit(t *testing.T) {
	boom := errors.New("chat-completions HTTP 500")
	h := drive(t, 8, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("x")},
		fail(boom),
		fail(boom),
	)
	if h.res.Term != TermProviderError {
		t.Fatalf("term = %q, want provider_error", h.res.Term)
	}
	if !errors.Is(h.err, boom) {
		t.Errorf("err = %v, want boom", h.err)
	}
	if h.res.Rounds != 0 || len(h.recs) != 0 {
		t.Errorf("rounds = %d recs = %d, want 0/0", h.res.Rounds, len(h.recs))
	}
	h.assertNotObserved(t)
}

// (8b) A handler infrastructure failure (not a rejection) terminates
// provider_error after recording the call — and any batch siblings — unlanded.
func TestProviderErrorFromHandler(t *testing.T) {
	forage := lookup(t, "forage")
	chop := lookup(t, "chop")
	down := errors.New("inject door unavailable")
	h := drive(t, 8, []tool.Tool{forage, chop},
		map[string]Handler{"forage": errHandler(down), "chop": landHandler("ok")},
		resp("", call("a1", "forage", "{}"), call("a2", "chop", "{}")),
	)
	if h.res.Term != TermProviderError || !errors.Is(h.err, down) {
		t.Fatalf("term = %q err = %v, want provider_error / door error", h.res.Term, h.err)
	}
	if h.res.Landed != nil {
		t.Errorf("landed = %+v, want nil", h.res.Landed)
	}
	// The failing call and its never-dispatched sibling are both unlanded.
	if !eqVerdicts(h.verdicts(), VerdictUnlanded, VerdictUnlanded) {
		t.Fatalf("verdicts = %v, want [unlanded unlanded]", h.verdicts())
	}
	if h.recs[0].Reason != down.Error() {
		t.Errorf("failing call reason = %q, want the infra error", h.recs[0].Reason)
	}
	h.assertNotObserved(t)
}

// (9) Context cancellation between rounds terminates ctx_done.
func TestContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// The read handler cancels the context; the loop's top-of-round check then
	// short-circuits before round 2's Submit.
	cancelOnRead := func(context.Context, llm.ToolCall) Outcome {
		cancel()
		return Outcome{Verdict: VerdictReadOK, ResultForModel: "data"}
	}
	h := driveCtx(t, ctx, 8, []tool.Tool{readTool},
		map[string]Handler{"peek": cancelOnRead},
		resp("", call("r1", "peek", "{}")),
		resp("", call("a1", "forage", "{}")), // must never be reached
	)
	if h.res.Term != TermCtxDone {
		t.Fatalf("term = %q, want ctx_done", h.res.Term)
	}
	if !errors.Is(h.err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", h.err)
	}
	if h.orch.calls != 1 {
		t.Errorf("Submit called %d times, want 1 (round 2 short-circuited)", h.orch.calls)
	}
	if !eqVerdicts(h.verdicts(), VerdictReadOK) {
		t.Errorf("verdicts = %v, want [read_ok]", h.verdicts())
	}
	h.assertNotObserved(t)
}

// (10) Transcript invariant: after N rounds the transcript holds exactly N
// assistant turns — the growth the openaiCompat env-<round> ID synthesis
// depends on. Round k's request (built before round k's echo) must carry
// exactly k-1 assistant turns.
func TestTranscriptGrowthOneAssistantPerRound(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 5, []tool.Tool{readTool, forage},
		map[string]Handler{"peek": readHandler("d"), "forage": landHandler("ok")},
		resp("", call("r1", "peek", "{}")),
		resp("", call("r2", "peek", "{}")),
		resp("", call("r3", "peek", "{}")),
		resp("", call("r4", "peek", "{}")),
		resp("", call("a1", "forage", "{}")),
	)
	if h.res.Term != TermLanded || h.res.Rounds != 5 {
		t.Fatalf("term = %q rounds = %d, want landed / 5", h.res.Term, h.res.Rounds)
	}
	if len(h.orch.reqs) != 5 {
		t.Fatalf("captured %d requests, want 5", len(h.orch.reqs))
	}
	for i, r := range h.orch.reqs {
		// Request i (0-based) is round i+1; it carries i assistant turns.
		if got := countAssistant(r.Turns); got != i {
			t.Errorf("round %d request has %d assistant turns, want %d", i+1, got, i)
		}
	}
	h.assertObservedOnce(t)
}

// (12) The model finishes with prose and no tool call → model_done, Final text.
func TestModelDone(t *testing.T) {
	forage := lookup(t, "forage")
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		resp("I have nothing to do right now."),
	)
	if h.err != nil || h.res.Term != TermModelDone {
		t.Fatalf("term = %q err = %v, want model_done", h.res.Term, h.err)
	}
	if h.res.Final != "I have nothing to do right now." {
		t.Errorf("final = %q", h.res.Final)
	}
	if h.res.Landed != nil || len(h.recs) != 0 {
		t.Errorf("landed = %+v recs = %d, want nil / 0", h.res.Landed, len(h.recs))
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1", h.res.Rounds)
	}
	h.assertObservedOnce(t)
}

// MaxRounds <= 0 is treated as 1 (defensive; config normalizes upstream).
func TestMaxRoundsDefensiveClamp(t *testing.T) {
	h := drive(t, 0, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("x")},
		resp("", call("r1", "peek", "{}")),
	)
	if h.res.Term != TermCapExhausted || h.res.Rounds != 1 {
		t.Fatalf("term = %q rounds = %d, want cap_exhausted / 1", h.res.Term, h.res.Rounds)
	}
	if !eqVerdicts(h.verdicts(), VerdictUnlanded) {
		t.Errorf("verdicts = %v, want [unlanded] (round 1 is also the final round)", h.verdicts())
	}
	h.assertObservedOnce(t)
}

// validateArgs unit table: driver-side schema/param validation covering scalar
// kinds, caps, enum membership, number bounds, and set_plan's authored schema.
func TestValidateArgs(t *testing.T) {
	drop := lookup(t, "drop")        // kind Enum (itemKinds), qty Number Min 1
	talk := lookup(t, "talk_to")     // target AgentName, required
	muse := lookup(t, "muse")        // text, MaxRunes 200
	setPlan := lookup(t, "set_plan") // authored steps schema

	cases := []struct {
		name    string
		tl      tool.Tool
		args    string
		wantErr bool
	}{
		{"drop ok kind+qty", drop, `{"kind":"wood","qty":3}`, false},
		{"drop ok empty", drop, `{}`, false},
		{"drop bad kind", drop, `{"kind":"gold"}`, true},
		{"drop qty below min", drop, `{"qty":0}`, true},
		{"drop qty non-integer", drop, `{"qty":1.5}`, true},
		{"talk_to ok", talk, `{"target":"bram"}`, false},
		{"talk_to missing required", talk, `{}`, true},
		{"talk_to wrong type", talk, `{"target":7}`, true},
		{"muse within cap", muse, `{"text":"a quiet thought"}`, false},
		{"muse over rune cap", muse, `{"text":"` + strings.Repeat("x", 201) + `"}`, true},
		{"set_plan ok", setPlan, `{"steps":[{"goal":"chop"},{"goal":"build_fire","qty":2}]}`, false},
		{"set_plan bad goal", setPlan, `{"steps":[{"goal":"teleport"}]}`, true},
		{"set_plan empty steps", setPlan, `{"steps":[]}`, true},
		{"set_plan over cap", setPlan, `{"steps":[{"goal":"chop"},{"goal":"chop"},{"goal":"chop"},{"goal":"chop"}]}`, true},
		{"set_plan missing steps", setPlan, `{}`, true},
		{"set_plan bad qty", setPlan, `{"steps":[{"goal":"chop","qty":0}]}`, true},
		{"set_plan bad kind", setPlan, `{"steps":[{"goal":"chop","kind":"gold"}]}`, true},
		{"not an object", drop, `["nope"]`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateArgs(c.tl, json.RawMessage(c.args))
			if (got != "") != c.wantErr {
				t.Errorf("validateArgs(%s) = %q, wantErr=%v", c.args, got, c.wantErr)
			}
		})
	}
}

// TestValidateAuthoredWalker (spec 029 T002): the generalized schema-lite walker
// validates a monitor_and_act-shaped authored schema — nested string arrays with
// an enum + item bounds, a bounded integer, and a boolean — the shape the retired
// set_plan-only validator could not express. The schema is inlined (the registry
// entry lands in T003) so this pins the driver seam on its own. Reasons are not
// asserted (the driver contract records the verdict, not the wording); only
// accept/reject, exactly as TestValidateArgs does.
func TestValidateAuthoredWalker(t *testing.T) {
	// The monitor_and_act schema from specs/029-metatron-agency/contracts/tools.md.
	schema := `{
		"type": "object",
		"properties": {
			"condition":   {"type": "string", "maxLength": 300},
			"action":      {"type": "string", "maxLength": 400},
			"event_types": {"type": "array", "minItems": 1, "maxItems": 4,
			                "items": {"type": "string", "enum": ["agent.slept","agent.woke","agent.died","sim.night_started"]}},
			"agent":       {"type": "string"},
			"keywords":    {"type": "array", "maxItems": 6, "items": {"type": "string", "maxLength": 40}},
			"confirm":     {"type": "boolean"},
			"ttl_days":    {"type": "integer", "minimum": 1, "maximum": 7}
		},
		"required": ["condition", "action", "event_types"]
	}`
	tl := tool.Tool{Name: "monitor_and_act", InputSchemaJSON: json.RawMessage(schema)}

	cases := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{"minimal ok", `{"condition":"when Rowan sleeps","action":"comfort her","event_types":["agent.slept"]}`, false},
		{"full ok", `{"condition":"c","action":"a","event_types":["agent.slept","agent.woke"],"agent":"rowan","keywords":["harvest","well"],"confirm":true,"ttl_days":5}`, false},
		{"missing required event_types", `{"condition":"c","action":"a"}`, true},
		{"missing required condition", `{"action":"a","event_types":["agent.slept"]}`, true},
		{"bad enum member", `{"condition":"c","action":"a","event_types":["agent.flew"]}`, true},
		{"event_types empty (minItems)", `{"condition":"c","action":"a","event_types":[]}`, true},
		{"event_types over maxItems", `{"condition":"c","action":"a","event_types":["agent.slept","agent.woke","agent.died","sim.night_started","agent.slept"]}`, true},
		{"wrong scalar type (condition int)", `{"condition":123,"action":"a","event_types":["agent.slept"]}`, true},
		{"confirm not boolean", `{"condition":"c","action":"a","event_types":["agent.slept"],"confirm":"yes"}`, true},
		{"ttl over maximum", `{"condition":"c","action":"a","event_types":["agent.slept"],"ttl_days":8}`, true},
		{"ttl below minimum", `{"condition":"c","action":"a","event_types":["agent.slept"],"ttl_days":0}`, true},
		{"keywords over maxItems", `{"condition":"c","action":"a","event_types":["agent.slept"],"keywords":["1","2","3","4","5","6","7"]}`, true},
		{"keyword over item maxLength", `{"condition":"c","action":"a","event_types":["agent.slept"],"keywords":["` + strings.Repeat("x", 41) + `"]}`, true},
		{"event_types not an array", `{"condition":"c","action":"a","event_types":"agent.slept"}`, true},
		{"not an object", `["nope"]`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateArgs(tl, json.RawMessage(c.args))
			if (got != "") != c.wantErr {
				t.Errorf("validateArgs(%s) = %q, wantErr=%v", c.args, got, c.wantErr)
			}
		})
	}
}

// capArgs truncates oversized arguments to the valid-JSON marker and copies
// small payloads verbatim without aliasing.
func TestCapArgs(t *testing.T) {
	small := json.RawMessage(`{"a":1}`)
	out := capArgs(small)
	if string(out) != `{"a":1}` {
		t.Errorf("small copy = %q", out)
	}
	// mutate the source; the copy must not change.
	small[2] = 'Z'
	if string(out) != `{"a":1}` {
		t.Errorf("capArgs aliased the source buffer: %q", out)
	}
	big := json.RawMessage(`{"blob":"` + strings.Repeat("y", 4000) + `"}`)
	capped := capArgs(big)
	if len(capped) >= len(big) {
		t.Errorf("oversized args not truncated: %d bytes", len(capped))
	}
	var marker struct {
		Truncated bool   `json:"_truncated"`
		Prefix    string `json:"prefix"`
	}
	if err := json.Unmarshal(capped, &marker); err != nil || !marker.Truncated || marker.Prefix == "" {
		t.Errorf("truncation marker malformed: %q err=%v", capped, err)
	}
	if capArgs(nil) != nil {
		t.Errorf("capArgs(nil) must stay nil (omitempty)")
	}
}

// TestObserveCarriesPinnedProvider (spec 024 T009 → T022): the whole-loop
// observation carries the run's PIN — the provider ResolveProvider named once at
// run start — not a per-round serving name read back off the response. Under
// run-level pinning the two coincide by construction (every round Submits with
// Provider: pin), so feeding the pin is the exact, honest attribution. A read →
// act loop whose kind resolves to "slow" observes exactly once, naming "slow".
func TestObserveCarriesPinnedProvider(t *testing.T) {
	forage := lookup(t, "forage")
	h := &harness{orch: &stubOrch{t: t, resolve: "slow", scripts: []func(llm.Request) (llm.Response, error){
		respFrom("slow", "", call("r1", "peek", "{}")),
		respFrom("slow", "", call("a1", "forage", "{}")),
	}}}
	job := Job{
		JobID: "planner-ada-412800", Kind: llm.KindPlanner, System: "you are ada", Seed: "what next?",
		Roster:    []tool.Tool{readTool, forage},
		Handlers:  map[string]Handler{"peek": readHandler("berries nearby"), "forage": landHandler("ok")},
		MaxRounds: 4,
	}
	h.res, h.err = run(context.Background(), h.orch, job)
	if h.res.Term != TermLanded {
		t.Fatalf("term = %q, want landed", h.res.Term)
	}
	if len(h.orch.observedP) != 1 {
		t.Fatalf("observed %d times, want exactly 1", len(h.orch.observedP))
	}
	if h.orch.observedP[0] != "slow" {
		t.Errorf("whole-loop observation named provider %q, want the pinned provider slow", h.orch.observedP[0])
	}
	// The pin was stamped on every round's Submit.
	for i, r := range h.orch.reqs {
		if r.Provider != "slow" {
			t.Errorf("round %d Submit.Provider = %q, want the pin slow", i+1, r.Provider)
		}
	}
}

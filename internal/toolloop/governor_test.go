package toolloop

// T023 (spec 017 US5 / SC-004 / quickstart §6): the governor stays truthful when
// a cognition becomes N model calls. Two properties are proven against a REAL
// llm.Orchestrator (stub httptest servers, no network), the layer where metering
// and estimator feeding actually live:
//
//   - Whole-loop observation: the loop feeds the tier estimator EXACTLY ONE
//     whole-cognition sample (via ObserveCognition), never per-round fractions
//     (every internal Submit rides SkipObserve). A non-loop Submit still feeds
//     per-call — the regression guard that the opt-out is scoped to the loop.
//   - Budget-exhausted mid-loop: with the monthly ceiling consumed by an earlier
//     round, the next round is refused BEFORE any HTTP (the stub sees no second
//     request); the loop terminates admission_refused and recorded spend equals
//     the sum of the completed rounds' costs exactly.
//
// Route-verdict purity (FR-011's "routing verdict MUST remain pure arithmetic
// over recorded observations") is unchanged by this feature — cognition.Route
// reads no estimator and no clock; it is a pure function of (class, ticks/sec,
// sec/pt). That purity is pinned by cognition/route_test.go (TestRouteIsPure,
// TestRouteSuppressionCarriesArithmetic) and the loop change touches neither
// route.go nor its inputs, so no new purity test is owed here.

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// preSeededMeter reports a spend already over budget for every plausible month
// key, so the FIRST cloud Submit is refused with ErrBudgetExhausted before any
// HTTP — a loop that does zero completed model work.
type preSeededMeter struct{ m map[string]string }

func newExhaustedMeter() *preSeededMeter {
	now := time.Now().UTC()
	m := map[string]string{}
	for _, mo := range []time.Time{now.AddDate(0, -1, 0), now, now.AddDate(0, 1, 0)} {
		m["llm_spend_"+mo.Format("2006-01")] = "9999"
	}
	return &preSeededMeter{m: m}
}

func (s *preSeededMeter) GetMeta(key string) (string, error) { return s.m[key], nil }
func (s *preSeededMeter) SetMeta(key, value string) error    { s.m[key] = value; return nil }

// TestRefusedLoopDoesNotFeedEstimatorSC004b (spec 017 T025b, FILED-1): the
// estimator's feed is SUCCESSES-ONLY, mirroring the worker path's doctrine
// (internal/llm/llm.go: "a fast failure is not a latency observation of
// completed thought"). A loop refused before any HTTP (budget exhausted →
// admission_refused, zero rounds, zero completed thought) must feed NOTHING, so
// the tier estimate stays at its bootstrap. Before the fix, the loop's defer
// fed ObserveCognition on every return path, dragging cloud sec/pt 10.0 → 8.0
// on a cognition that did no work — a governor-truthfulness skew that collapses
// the estimate toward zero under sustained refusal.
func TestRefusedLoopDoesNotFeedEstimatorSC004b(t *testing.T) {
	cfg := llm.Config{
		MonthlyBudgetUSD: 100,
		Local:            llm.LocalConfig{Endpoint: "http://unused", Model: "unused"},
		Cloud:            llm.CloudConfig{Provider: llm.ProviderOpenAICompat, Endpoint: "http://unused", Model: "unused"},
	}
	orch, err := llm.New(cfg, newExhaustedMeter())
	if err != nil {
		t.Fatal(err)
	}
	defer orch.Close()

	before := orch.SecondsPerPoint(llm.TierCloud)
	if math.Abs(before-cognition.BootstrapCloudSecPerPt) > 1e-9 {
		t.Fatalf("pre-run cloud estimate = %g, want bootstrap %g", before, cognition.BootstrapCloudSecPerPt)
	}

	// A metatron cognition routes to cloud; the budget is exhausted, so the
	// first Submit is refused before any HTTP — the loop did zero work.
	res, rerr := Run(context.Background(), orch, Job{
		JobID:     "metatron-0-1",
		Kind:      llm.KindMetatron,
		System:    "s",
		Seed:      "hi",
		Roster:    []tool.Tool{lookup(t, "forage")},
		Handlers:  map[string]Handler{"forage": landHandler("ok")},
		MaxRounds: 8,
	})
	if res.Term != TermAdmissionRefused || rerr == nil {
		t.Fatalf("term = %q err = %v, want admission_refused / budget error", res.Term, rerr)
	}
	if res.Rounds != 0 {
		t.Fatalf("rounds = %d, want 0 (refused before any completed round)", res.Rounds)
	}

	after := orch.SecondsPerPoint(llm.TierCloud)
	if math.Abs(after-before) > 1e-9 {
		t.Errorf("a refused loop (zero completed work) moved cloud sec/pt %g → %g; "+
			"the estimator feed must be successes-only (landed/model_done/cap_exhausted)", before, after)
	}
}

// TestWholeLoopFeedsEstimatorOnceSC004 drives a real orchestrator through a
// two-round loop (read → act) and reads the tier's live seconds-per-point to
// prove the observation count. The estimator seeds at the local bootstrap
// (20 s/pt); one EWMA step toward a ~0 s/pt observation (the stub answers
// instantly) lands at 0.8·20 = 16.0. The three possible outcomes are cleanly
// separable:
//
//	0 samples (loop never observed)        → 20.0   (unchanged)
//	1 sample  (ONE whole-loop observation) → 16.0   ← the correct behavior
//	2 samples (per-round fractions leaked)  → 12.8   (0.8·16.0)
//
// So asserting ≈16.0 after the loop proves both that a whole-loop observation
// landed AND that no per-round sample did (SkipObserve held on every internal
// Submit). A following NON-loop Submit must feed per-call, dropping the estimate
// to ≈12.8 — the regression that the loop's opt-out did not disable per-call
// feeding for ordinary single-shot cognition.
func TestWholeLoopFeedsEstimatorOnceSC004(t *testing.T) {
	forage := lookup(t, "forage")

	var reqs int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqs, 1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // loop round 1: a read-tool call, fed back
			json.NewEncoder(w).Encode(toolCallResp("peek"))
		case 2: // loop round 2: the acting call lands
			json.NewEncoder(w).Encode(toolCallResp("forage"))
		default: // the non-loop regression Submit: plain content
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "ok"}, "finish_reason": "stop"}},
				"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 2},
			})
		}
	}))
	defer srv.Close()

	orch := newEquivOrch(t, srv.URL, llm.ToolModeNative)

	if got := orch.SecondsPerPoint(llm.TierLocal); math.Abs(got-cognition.BootstrapLocalSecPerPt) > 1e-9 {
		t.Fatalf("pre-run estimate = %g, want the bootstrap %g", got, cognition.BootstrapLocalSecPerPt)
	}

	job := Job{
		JobID:     "planner-ada-412800",
		Kind:      llm.KindPlanner,
		System:    "you are ada",
		Seed:      "what next?",
		Roster:    []tool.Tool{readTool, forage},
		Handlers:  map[string]Handler{"peek": readHandler("berries nearby"), "forage": landHandler("foraging")},
		MaxRounds: 4,
	}
	res, err := Run(context.Background(), orch, job)
	if err != nil || res.Term != TermLanded {
		t.Fatalf("loop: term=%q err=%v, want landed/nil", res.Term, err)
	}
	if res.Rounds != 2 {
		t.Fatalf("loop ran %d rounds, want 2 (a whole-loop observation must cover both)", res.Rounds)
	}

	afterLoop := orch.SecondsPerPoint(llm.TierLocal)
	if math.Abs(afterLoop-16.0) > 0.1 {
		t.Errorf("estimate after loop = %g, want ~16.0 (exactly ONE whole-loop observation; "+
			"20.0 = none, 12.8 = two per-round fractions leaked)", afterLoop)
	}

	// Regression: a NON-loop single-shot Submit still feeds a per-call sample.
	if _, err := orch.Submit(context.Background(), llm.Request{Kind: llm.KindPlanner, Prompt: "hi"}); err != nil {
		t.Fatalf("non-loop Submit: %v", err)
	}
	afterSingle := orch.SecondsPerPoint(llm.TierLocal)
	if math.Abs(afterSingle-12.8) > 0.1 {
		t.Errorf("estimate after a non-loop Submit = %g, want ~12.8 (0.8·16.0) — the per-call "+
			"feed path must stay live for single-shot cognition", afterSingle)
	}
}

// TestBudgetExhaustedMidLoopSC004 proves the monthly ceiling refuses a loop's
// next round BEFORE any spend, and that recorded spend is exactly the sum of the
// rounds that completed. The cloud tier (metatron's route) is billed per call;
// round 1's cost consumes the tiny budget, so round 2's Submit is refused at the
// admission ladder's budget gate — no HTTP, no partial charge — and the loop
// terminates admission_refused (SC-004; quickstart §6).
func TestBudgetExhaustedMidLoopSC004(t *testing.T) {
	var reqs int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqs, 1)
		w.Header().Set("Content-Type", "application/json")
		// Round 1: a read-tool call (loop continues), billed at 1000 prompt
		// tokens → $1.00 at the pricing below, which exhausts the $0.50 budget.
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content":    "",
					"tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "peek", "arguments": "{}"}}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 0},
		})
	}))
	defer srv.Close()

	cfg := llm.Config{
		MonthlyBudgetUSD: 0.50,
		Local:            llm.LocalConfig{Endpoint: "http://unused", Model: "unused"},
		Cloud: llm.CloudConfig{
			Provider:        llm.ProviderOpenAICompat,
			Endpoint:        srv.URL,
			Model:           "mock-cloud",
			InputUSDPerMTok: 1000, // 1000 prompt tokens ⇒ $1.00 per round
		},
	}
	orch, err := llm.New(cfg, newMemMeter())
	if err != nil {
		t.Fatal(err)
	}
	defer orch.Close()

	job := Job{
		JobID:     "metatron-0-412800",
		Kind:      llm.KindMetatron, // routes to the cloud tier
		System:    "you are the gatekeeper",
		Seed:      "assess the village",
		Roster:    []tool.Tool{readTool},
		Handlers:  map[string]Handler{"peek": readHandler("all is quiet")},
		MaxRounds: 8,
	}
	res, err := Run(context.Background(), orch, job)

	if res.Term != TermAdmissionRefused {
		t.Errorf("termination = %q, want admission_refused", res.Term)
	}
	if err == nil {
		t.Fatal("expected the budget-exhausted error, got nil")
	}

	if got := atomic.LoadInt32(&reqs); got != 1 {
		t.Errorf("stub saw %d requests, want exactly 1 (round 2 must be refused BEFORE any HTTP)", got)
	}

	// Recorded spend equals the sum of completed rounds' costs — here round 1's
	// $1.00 exactly, and not a fraction more from the refused round.
	spent := orch.StatusSnapshot().Spent
	if math.Abs(spent-1.0) > 1e-9 {
		t.Errorf("recorded spend = %g, want exactly 1.0 (round 1 only)", spent)
	}
}

// toolCallResp is a one-tool-call OpenAI chat-completions response with no
// arguments, billed at a nominal token count.
func toolCallResp(name string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"content":    "",
				"tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": name, "arguments": "{}"}}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2},
	}
}

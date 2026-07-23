package toolloop

import (
	"context"
	"errors"
	"testing"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// Run-level provider pinning (spec 024 T022, research R9 / FR-008 extension): a
// multi-round cognition — and the spec-025 in-loop transport retry — resolves its
// provider ONCE at run start and stamps it on every round, so a thought never
// changes models mid-transcript. These tests drive the same scripted stubOrch as
// loop_test.go: the stub's resolve names what the run's pin seam returns, and the
// captured reqs let each round's Request.Provider be asserted directly.

// runPinned drives one loop run with a scripted pin: resolve is what
// ResolveProvider returns (the walk's answer at run start), explicitPin is a
// caller-set Job.Provider (calibrate's path) that overrides resolution. It
// returns the harness so tests can read res, reqs, observedP, and resolveCalls.
func runPinned(t *testing.T, resolve, explicitPin string, maxRounds int, roster []tool.Tool, handlers map[string]Handler, scripts ...func(llm.Request) (llm.Response, error)) *harness {
	t.Helper()
	h := &harness{orch: &stubOrch{t: t, resolve: resolve, scripts: scripts}}
	job := Job{
		JobID:     "planner-ada-412800",
		Kind:      llm.KindPlanner,
		System:    "you are ada",
		Seed:      "what next?",
		Roster:    roster,
		Handlers:  handlers,
		MaxRounds: maxRounds,
		Provider:  explicitPin,
		Record:    func(r CallRecord) { h.recs = append(h.recs, r) },
	}
	h.res, h.err = run(context.Background(), h.orch, job)
	return h
}

// assertAllRoundsPinned checks every captured Submit named the same provider —
// the mid-transcript continuity guarantee (no silent re-route).
func assertAllRoundsPinned(t *testing.T, h *harness, want string) {
	t.Helper()
	if len(h.orch.reqs) == 0 {
		t.Fatalf("no Submits captured")
	}
	for i, r := range h.orch.reqs {
		if r.Provider != want {
			t.Errorf("round %d Submit.Provider = %q, want the pin %q", i+1, r.Provider, want)
		}
	}
}

// (1) The spec-025 retry re-submits to the SAME pinned provider, even though a
// fresh chain-walk after the failure's breaker strike would prefer an admissible
// fallback. The pin resolves to "slow"; the first Submit fails with a transport
// error (retry-eligible), the retry lands again on "slow" and also fails, so the
// run terminates provider_error per 025 semantics — it never silently serves the
// fallback. The loop asked the pin seam exactly once (no re-resolve on retry).
func TestRetryStaysOnPinnedProviderNeverFallsBack(t *testing.T) {
	forage := lookup(t, "forage")
	boom := errors.New("connection reset by peer") // transport → TermProviderError → retry-eligible
	h := runPinned(t, "slow", "", 4, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		fail(boom), // round 1: transport failure on the pinned provider
		fail(boom), // the retry: same pin, fails again → provider_error
	)
	if h.res.Term != TermProviderError {
		t.Fatalf("term = %q, want provider_error (twice-failed pin, no fallback)", h.res.Term)
	}
	if !errors.Is(h.err, boom) {
		t.Errorf("err = %v, want the transport error propagated", h.err)
	}
	if !h.res.Retried {
		t.Errorf("Retried = false, want true (the one in-loop retry was consumed)")
	}
	if h.orch.calls != 2 {
		t.Errorf("Submit called %d times, want exactly 2 (original + one retry, no third)", h.orch.calls)
	}
	// The retry targeted the SAME provider — never the would-be fallback.
	assertAllRoundsPinned(t, h, "slow")
	if h.orch.resolveCalls != 1 {
		t.Errorf("ResolveProvider called %d times, want exactly 1 (resolve once, never re-walk on retry)", h.orch.resolveCalls)
	}
	// A failed run feeds the estimator nothing (successes-only).
	if len(h.orch.observes) != 0 {
		t.Errorf("failed run observed %d times, want 0", len(h.orch.observes))
	}
}

// (2) A multi-round run never changes provider mid-transcript. The pin resolves
// to "A"; a read → read → act loop runs three rounds, and every round's
// Request.Provider is identical. The pin seam is consulted once for the whole run.
func TestMultiRoundNeverChangesProviderMidTranscript(t *testing.T) {
	forage := lookup(t, "forage")
	h := runPinned(t, "A", "", 4, []tool.Tool{readTool, forage},
		map[string]Handler{"peek": readHandler("berries nearby"), "forage": landHandler("ok")},
		respFrom("A", "", call("r1", "peek", "{}")),
		respFrom("A", "", call("r2", "peek", "{}")),
		respFrom("A", "", call("a1", "forage", "{}")),
	)
	if h.res.Term != TermLanded {
		t.Fatalf("term = %q, want landed", h.res.Term)
	}
	if h.res.Rounds != 3 {
		t.Errorf("rounds = %d, want 3", h.res.Rounds)
	}
	assertAllRoundsPinned(t, h, "A")
	if h.orch.resolveCalls != 1 {
		t.Errorf("ResolveProvider called %d times, want exactly 1 for the whole run", h.orch.resolveCalls)
	}
	// Attribution rides the pin.
	if len(h.orch.observedP) != 1 || h.orch.observedP[0] != "A" {
		t.Errorf("observation = %v, want exactly one naming the pin A", h.orch.observedP)
	}
}

// (3) After a run whose pinned provider was down fails, the NEXT run's resolution
// walks the chain to the fallback. The loop re-resolves per run (never caches the
// pin across runs): run 1 pins "A" and fails; the operator's walk now yields "B"
// (A's breaker opened), so run 2 pins and is served by "B".
func TestNextRunResolvesToFallbackAfterPinnedFailure(t *testing.T) {
	forage := lookup(t, "forage")
	boom := errors.New("connection reset by peer")

	// Run 1: pinned to A, fails twice (transport), terminates provider_error.
	run1 := runPinned(t, "A", "", 4, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		fail(boom), fail(boom),
	)
	if run1.res.Term != TermProviderError {
		t.Fatalf("run1 term = %q, want provider_error", run1.res.Term)
	}
	assertAllRoundsPinned(t, run1, "A")

	// Run 2: a FRESH run. A is now open, so the walk resolves to B — the loop
	// re-resolves (no cross-run cache) and B serves the whole run.
	run2 := runPinned(t, "B", "", 4, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		respFrom("B", "", call("a1", "forage", "{}")),
	)
	if run2.res.Term != TermLanded {
		t.Fatalf("run2 term = %q, want landed (fallback served)", run2.res.Term)
	}
	assertAllRoundsPinned(t, run2, "B")
	if run2.orch.resolveCalls != 1 {
		t.Errorf("run2 ResolveProvider called %d times, want exactly 1 (re-resolved fresh)", run2.orch.resolveCalls)
	}
	if len(run2.orch.observedP) != 1 || run2.orch.observedP[0] != "B" {
		t.Errorf("run2 observation = %v, want exactly one naming the fallback B", run2.orch.observedP)
	}
}

// (4) An explicit Job.Provider pin (calibrate's reference sample) is honored
// end-to-end and NEVER re-resolved: the loop skips the pin seam entirely, stamps
// the caller's provider on every round including a retry, and attributes to it.
func TestExplicitJobProviderPinHonoredEndToEnd(t *testing.T) {
	forage := lookup(t, "forage")
	boom := errors.New("connection reset by peer")
	// resolve is set to a DIFFERENT provider to prove the explicit pin wins and
	// the seam is never consulted.
	h := runPinned(t, "would-resolve-here", "calib-target", 4, []tool.Tool{readTool, forage},
		map[string]Handler{"peek": readHandler("berries nearby"), "forage": landHandler("ok")},
		fail(boom), // transport blip → one retry, still on the explicit pin
		respFrom("calib-target", "", call("r1", "peek", "{}")),
		respFrom("calib-target", "", call("a1", "forage", "{}")),
	)
	if h.res.Term != TermLanded {
		t.Fatalf("term = %q, want landed", h.res.Term)
	}
	if !h.res.Retried {
		t.Errorf("Retried = false, want true (one transport retry consumed)")
	}
	if h.orch.resolveCalls != 0 {
		t.Errorf("ResolveProvider called %d times, want 0 (explicit pin never re-resolves)", h.orch.resolveCalls)
	}
	// Every Submit — the failed one, its retry, and both completed rounds —
	// targeted the explicit pin.
	assertAllRoundsPinned(t, h, "calib-target")
	if len(h.orch.observedP) != 1 || h.orch.observedP[0] != "calib-target" {
		t.Errorf("observation = %v, want exactly one naming the explicit pin", h.orch.observedP)
	}
}

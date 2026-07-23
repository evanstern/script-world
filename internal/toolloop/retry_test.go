package toolloop

// Transport-retry tests (spec 025 US1, FR-001..FR-006; contracts/loop-retry.md).
// They drive the same scripted `submitter` stub as loop_test.go — one func per
// provider round — so a fail-once / fail-twice / admission-refused / ctx-done /
// handler-infra script exercises the retry branch deterministically with no
// network, and ObserveCognition counts pin the estimator invariance.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// (1) fail-once: the first Submit fails with a transport error; the run retries
// the identical transcript once and the second Submit lands — the recovery the
// feature exists for (SC-001). Retried is set, RetryReason is the FIRST failure,
// the re-submitted request is byte-identical, and the recovered run feeds the
// estimator exactly once (contract invariant 2).
func TestRetryFailOnceRecovers(t *testing.T) {
	boom := errors.New("chat-completions HTTP 502")
	forage := lookup(t, "forage")
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("foraging")},
		fail(boom),
		resp("", call("c1", "forage", "{}")),
	)
	if h.err != nil {
		t.Fatalf("err = %v, want nil (recovered)", h.err)
	}
	if h.res.Term != TermLanded {
		t.Fatalf("term = %q, want landed (recovered in the success family)", h.res.Term)
	}
	if !h.res.Retried || h.res.RetryReason != boom.Error() {
		t.Errorf("retried=%v reason=%q, want true / %q", h.res.Retried, h.res.RetryReason, boom.Error())
	}
	if h.orch.calls != 2 {
		t.Errorf("Submit called %d times, want 2 (one failed, one retry)", h.orch.calls)
	}
	if h.res.Rounds != 1 {
		t.Errorf("rounds = %d, want 1 (the failed attempt consumed no round)", h.res.Rounds)
	}
	// Transcript integrity: the failed Submit appended nothing, so the retried
	// request is byte-identical to the first (contract invariant 5).
	if !reflect.DeepEqual(h.orch.reqs[0], h.orch.reqs[1]) {
		t.Errorf("retried request differs from the first:\n first=%+v\n retry=%+v", h.orch.reqs[0], h.orch.reqs[1])
	}
	h.assertObservedOnce(t)
}

// (2) fail-twice: the retry also fails; the run terminates provider_error with
// the SECOND error propagated, no third attempt (SC-002). Retried stays set (the
// retry WAS consumed — countable from the trail) and the estimator is never fed.
func TestRetryFailTwiceTerminates(t *testing.T) {
	first := errors.New("boom-1")
	second := errors.New("boom-2")
	h := drive(t, 8, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("x")},
		fail(first),
		fail(second),
	)
	if h.res.Term != TermProviderError {
		t.Fatalf("term = %q, want provider_error", h.res.Term)
	}
	if !errors.Is(h.err, second) {
		t.Errorf("err = %v, want the SECOND error %v (latest propagated)", h.err, second)
	}
	if h.orch.calls != 2 {
		t.Errorf("Submit called %d times, want exactly 2 (no third attempt)", h.orch.calls)
	}
	if !h.res.Retried || h.res.RetryReason != first.Error() {
		t.Errorf("retried=%v reason=%q, want true / %q (the first failure)", h.res.Retried, h.res.RetryReason, first.Error())
	}
	h.assertNotObserved(t)
}

// (3) admission refusals never retry — the governor spoke; retrying would
// violate busy-is-not-down (FR-002). Each ladder sentinel terminates
// admission_refused on the first failure with no retry.
func TestRetryAdmissionNeverRetried(t *testing.T) {
	for _, se := range []error{llm.ErrTierBusy, llm.ErrQueueFull, llm.ErrBudgetExhausted, llm.ErrTierDown} {
		h := drive(t, 8, []tool.Tool{readTool},
			map[string]Handler{"peek": readHandler("x")},
			fail(se),
		)
		if h.res.Term != TermAdmissionRefused {
			t.Errorf("%v: term = %q, want admission_refused", se, h.res.Term)
		}
		if h.res.Retried || h.res.RetryReason != "" {
			t.Errorf("%v: retried=%v reason=%q, want false / \"\" (admission never retries)", se, h.res.Retried, h.res.RetryReason)
		}
		if h.orch.calls != 1 {
			t.Errorf("%v: Submit called %d times, want 1 (no retry)", se, h.orch.calls)
		}
		h.assertNotObserved(t)
	}
}

// (4) context cancellation never retries — a cancelled/deadline Submit classifies
// ctx_done, outside the provider_error retry arm (FR-002).
func TestRetryContextDoneNeverRetried(t *testing.T) {
	for _, ce := range []error{context.Canceled, context.DeadlineExceeded} {
		h := drive(t, 8, []tool.Tool{readTool},
			map[string]Handler{"peek": readHandler("x")},
			fail(ce),
		)
		if h.res.Term != TermCtxDone {
			t.Errorf("%v: term = %q, want ctx_done", ce, h.res.Term)
		}
		if h.res.Retried {
			t.Errorf("%v: retried=%v, want false (ctx_done never retries)", ce, h.res.Retried)
		}
		if h.orch.calls != 1 {
			t.Errorf("%v: Submit called %d times, want 1 (no retry)", ce, h.orch.calls)
		}
		h.assertNotObserved(t)
	}
}

// (5) a tool-handler infrastructure failure is NOT a transport failure (the
// model call succeeded, the handler broke): it terminates provider_error exactly
// as today, never retried — re-dispatching side-effectful handlers is forbidden
// (loop.go acting/read handler Err sites, FR-002, contract "what never retries").
func TestRetryHandlerInfraNeverRetried(t *testing.T) {
	forage := lookup(t, "forage")
	down := errors.New("inject door unavailable")
	h := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": errHandler(down)},
		resp("", call("a1", "forage", "{}")),
	)
	if h.res.Term != TermProviderError || !errors.Is(h.err, down) {
		t.Fatalf("term = %q err = %v, want provider_error / handler error", h.res.Term, h.err)
	}
	if h.res.Retried || h.res.RetryReason != "" {
		t.Errorf("retried=%v reason=%q, want false / \"\" (handler infra never retries)", h.res.Retried, h.res.RetryReason)
	}
	if h.orch.calls != 1 {
		t.Errorf("Submit called %d times, want 1 (the model call succeeded; no re-submit)", h.orch.calls)
	}
	h.assertNotObserved(t)
}

// (7) round-cap invariance (contract invariant 1): a failed attempt consumes no
// round. Failing on round 1 then running reads to the cap still completes the
// full MaxRounds provider rounds.
func TestRetryFailedAttemptConsumesNoRound(t *testing.T) {
	boom := errors.New("transient blip")
	h := drive(t, 3, []tool.Tool{readTool},
		map[string]Handler{"peek": readHandler("nothing useful")},
		fail(boom),
		resp("", call("r1", "peek", "{}")),
		resp("", call("r2", "peek", "{}")),
		resp("", call("r3", "peek", "{}")),
	)
	if h.res.Term != TermCapExhausted {
		t.Fatalf("term = %q, want cap_exhausted", h.res.Term)
	}
	if h.res.Rounds != 3 {
		t.Errorf("rounds = %d, want 3 (the failed attempt consumed no round of the cap)", h.res.Rounds)
	}
	if !h.res.Retried {
		t.Errorf("retried=%v, want true", h.res.Retried)
	}
	if h.orch.calls != 4 {
		t.Errorf("Submit called %d times, want 4 (1 failed + 3 completed rounds)", h.orch.calls)
	}
	h.assertObservedOnce(t)
}

// (8) RetryReason is non-empty iff Retried is true (data-model.md §2 invariant),
// across a model_done recovery and a no-retry success.
func TestRetryReasonInvariant(t *testing.T) {
	// Recovered into model_done: the retry succeeds, the model then finishes with
	// prose (no tool call). Retried true, reason present.
	boom := errors.New("blip")
	forage := lookup(t, "forage")
	rec := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		fail(boom),
		resp("I have nothing to do."),
	)
	if rec.res.Term != TermModelDone {
		t.Fatalf("term = %q, want model_done", rec.res.Term)
	}
	if !rec.res.Retried || rec.res.RetryReason == "" {
		t.Errorf("recovered: retried=%v reason=%q, want true / non-empty", rec.res.Retried, rec.res.RetryReason)
	}
	// No failure at all: Retried false, RetryReason empty.
	ok := drive(t, 8, []tool.Tool{forage},
		map[string]Handler{"forage": landHandler("ok")},
		resp("", call("c1", "forage", "{}")),
	)
	if ok.res.Retried || ok.res.RetryReason != "" {
		t.Errorf("no-failure run: retried=%v reason=%q, want false / \"\"", ok.res.Retried, ok.res.RetryReason)
	}
}

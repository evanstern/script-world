package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockError is an openai_compat endpoint that always fails (HTTP 500): a call
// that DISPATCHES to it and then errors — the instrument for the no-redispatch
// invariant (a post-dispatch failure is final, never re-run down the chain).
func mockError(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestChainWalkCircuitOpen (spec 024 T010): an open breaker on the head is a
// circuit-open skip; the walk moves to the next candidate and records the skip
// in chain order.
func TestChainWalkCircuitOpen(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, fallbackConfig(fast.URL, slow.URL), testStore(t))

	for i := 0; i < failuresToOpen; i++ {
		o.providers["fast"].health.fail()
	}
	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if resp.Provider != "slow" {
		t.Errorf("served by %q, want slow (fast circuit open)", resp.Provider)
	}
	if fh.Load() != 0 {
		t.Errorf("fast was called %d times despite an open circuit", fh.Load())
	}
	assertSkipped(t, resp.Skipped, RouteSkip{"fast", SkipCircuitOpen})
}

// TestChainWalkWalletExhausted (spec 024 T010 + T012 tie-in): a priced head that
// has hit the monthly ceiling is a wallet-exhausted skip; the zero-priced
// fallback serves (and bills nothing) while the priced candidate is refused.
func TestChainWalkWalletExhausted(t *testing.T) {
	var hits atomic.Int64
	srv := mockLocal(t, &hits)
	st := testStore(t)
	if err := st.SetMeta(metaKey(currentMonth()), "9999"); err != nil {
		t.Fatal(err)
	}
	o := newOrch(t, walletFallbackConfig(srv.URL), st)

	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if resp.Provider != "free" {
		t.Errorf("served by %q, want free (premium wallet-exhausted)", resp.Provider)
	}
	if resp.CostUSD != 0 {
		t.Errorf("zero-priced fallback billed %v, want 0", resp.CostUSD)
	}
	assertSkipped(t, resp.Skipped, RouteSkip{"premium", SkipWalletExhausted})
}

// TestChainWalkQueueFull (spec 024 T010): a full queue on the head is a
// queue-full skip. The single fast worker is parked so it stops draining, the
// queue is filled to capacity, and a fresh call walks to the fallback.
func TestChainWalkQueueFull(t *testing.T) {
	release := make(chan struct{})
	fast, _, fastArr := parkServer(t, release)
	defer close(release)
	var sh atomic.Int64
	slow := mockLocal(t, &sh)

	cfg := fallbackConfig(fast.URL, slow.URL)
	cfg.Providers["fast"] = ProviderConfig{Transport: ProviderOpenAICompat, Endpoint: fast.URL, Model: "fast-model", Parallel: 1}
	o := newOrch(t, cfg, testStore(t))
	fastP := o.providers["fast"]

	// Occupy the single worker so it stops draining the queue.
	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "park"})
	select {
	case <-fastArr:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never parked")
	}
	// Fill fast's queue to capacity with pre-cancelled jobs (the parked worker
	// won't dequeue them; a fresh enqueue now sees a full queue).
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < queueCap; i++ {
		fastP.queue <- job{ctx: cancelled, reply: make(chan result, 1)}
	}

	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if resp.Provider != "slow" {
		t.Errorf("served by %q, want slow (fast queue full)", resp.Provider)
	}
	assertSkipped(t, resp.Skipped, RouteSkip{"fast", SkipQueueFull})
}

// TestChainWalkBusy (spec 024 T010): a best-effort call additionally requires an
// idle slot and empty queues per candidate — a saturated head is a busy skip.
// Both of fast's two workers are parked (inflight == slots), so a best-effort
// planner call walks to the idle fallback.
func TestChainWalkBusy(t *testing.T) {
	release := make(chan struct{})
	fast, _, fastArr := parkServer(t, release)
	defer close(release)
	var sh atomic.Int64
	slow := mockLocal(t, &sh)

	o := newOrch(t, fallbackConfig(fast.URL, slow.URL), testStore(t)) // fast parallel 2

	for i := 0; i < 2; i++ {
		go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "park"})
	}
	for i := 0; i < 2; i++ {
		select {
		case <-fastArr:
		case <-time.After(5 * time.Second):
			t.Fatal("workers never parked")
		}
	}

	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x", BestEffort: true})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if resp.Provider != "slow" {
		t.Errorf("served by %q, want slow (fast busy)", resp.Provider)
	}
	assertSkipped(t, resp.Skipped, RouteSkip{"fast", SkipBusy})
}

// TestChainWalkAllInadmissibleRefusesHead (spec 024 T010): when every candidate
// is inadmissible the walk returns the HEAD's refusal error — here both breakers
// are open, so the head's circuit-open reason maps to ErrTierDown.
func TestChainWalkAllInadmissibleRefusesHead(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, fallbackConfig(fast.URL, slow.URL), testStore(t))

	for _, name := range []string{"fast", "slow"} {
		for i := 0; i < failuresToOpen; i++ {
			o.providers[name].health.fail()
		}
	}
	_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if !errors.Is(err, ErrTierDown) {
		t.Errorf("all-inadmissible walk err = %v, want the head's ErrTierDown", err)
	}
	if fh.Load() != 0 || sh.Load() != 0 {
		t.Errorf("a refused walk still reached a model: fast=%d slow=%d", fh.Load(), sh.Load())
	}
}

// TestNoFallbackConfinesToHead (spec 024 T010): a no_fallback route considers
// only its head — an open breaker there refuses rather than walking, even though
// a healthy provider exists elsewhere in the world.
func TestNoFallbackConfinesToHead(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	cfg := fallbackConfig(fast.URL, slow.URL)
	cfg.Routes[string(KindPlanner)] = RouteConfig{Chain: []string{"fast"}, NoFallback: true}
	o := newOrch(t, cfg, testStore(t))

	for i := 0; i < failuresToOpen; i++ {
		o.providers["fast"].health.fail()
	}
	_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if !errors.Is(err, ErrTierDown) {
		t.Errorf("no_fallback head down: err = %v, want ErrTierDown (no walk)", err)
	}
	if sh.Load() != 0 {
		t.Errorf("no_fallback route fell through to slow (%d hits) — it must confine to its head", sh.Load())
	}
}

// TestPinHonorsAdmissionNoFallback (spec 024 T010): a Request.Provider pin
// bypasses chain-walking but still honors the pinned provider's admission — a
// pinned provider whose breaker is open refuses, and never falls back to the
// chain head.
func TestPinHonorsAdmissionNoFallback(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, fallbackConfig(fast.URL, slow.URL), testStore(t))

	for i := 0; i < failuresToOpen; i++ {
		o.providers["slow"].health.fail()
	}
	// Pin planner to slow (not its chain head): admission is honored, so an open
	// breaker refuses — and the walk does NOT fall back to the healthy head fast.
	_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x", Provider: "slow"})
	if !errors.Is(err, ErrTierDown) {
		t.Errorf("pinned-to-down err = %v, want ErrTierDown", err)
	}
	if fh.Load() != 0 {
		t.Errorf("a pin fell back to the chain head (%d hits) — a pin must never walk", fh.Load())
	}

	// A pin to a healthy non-head provider is served by exactly that provider.
	resp, err := o.Submit(context.Background(), Request{Kind: KindConsolidation, Prompt: "x", Provider: "fast"})
	if err != nil {
		t.Fatalf("healthy pin: %v", err)
	}
	if resp.Provider != "fast" || len(resp.Skipped) != 0 {
		t.Errorf("healthy pin served by %q skipped %+v, want fast with no skips", resp.Provider, resp.Skipped)
	}
}

// TestNoRedispatchAfterDispatch (spec 024 T010): a post-dispatch provider
// failure is FINAL — the head accepted the call and errored, so the error is
// returned and the fallback is never tried (re-running could double-spend or
// double-act). The head's breaker was closed, so admission dispatched to it.
func TestNoRedispatchAfterDispatch(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockError(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, fallbackConfig(fast.URL, slow.URL), testStore(t))

	_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if err == nil {
		t.Fatal("expected the head's provider error, got nil")
	}
	if fh.Load() != 1 {
		t.Errorf("fast dispatched %d times, want exactly 1", fh.Load())
	}
	if sh.Load() != 0 {
		t.Errorf("a post-dispatch failure was re-dispatched to slow (%d hits) — dispatch is final", sh.Load())
	}
}

// assertSkipped checks the recorded skip list matches an expected ordered set.
func assertSkipped(t *testing.T, got []RouteSkip, want ...RouteSkip) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("skipped = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("skipped[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

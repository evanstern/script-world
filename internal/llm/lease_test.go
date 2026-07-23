package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// withLeaseBase points the lease pool root at a temp dir for the duration of a
// test — the seam that lets two in-process orchestrators reproduce cross-world
// endpoint contention without touching the operator's real ~/.promptworld. Not
// parallel-safe (it swaps a package var); lease tests run sequentially.
func withLeaseBase(t *testing.T, dir string) {
	t.Helper()
	prev := leaseBaseDir
	leaseBaseDir = func() (string, bool) { return dir, true }
	t.Cleanup(func() { leaseBaseDir = prev })
}

// concCounter tracks a mock endpoint's live and peak in-flight request count.
type concCounter struct {
	cur  atomic.Int32
	max  atomic.Int32
	hits atomic.Int64
}

// newConcMock is an OpenAI-compatible endpoint that records the peak number of
// requests it ever served simultaneously — the instrument proving the lease pool
// bounds COMBINED concurrency across worlds sharing it. Each request lingers for
// delay so overlap is real, not an artifact of instantaneous handlers.
func newConcMock(t *testing.T, delay time.Duration) (*httptest.Server, *concCounter) {
	t.Helper()
	cc := &concCounter{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		n := cc.cur.Add(1)
		cc.hits.Add(1)
		for { // lift the running peak to n if n is higher
			m := cc.max.Load()
			if n <= m || cc.max.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(delay)
		cc.cur.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "hi"}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, cc
}

// leaseConfig is a v2 registry with a single zero-priced provider that declares
// an endpoint_capacity, so its normalized endpoint gets an advisory lease pool.
func leaseConfig(url string, capacity, parallel int) Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Providers: map[string]ProviderConfig{
			"shared": {
				Transport: ProviderOpenAICompat, Endpoint: url, Model: "shared-model",
				Parallel: parallel, EndpointCapacity: capacity,
			},
		},
		Routes: map[string]RouteConfig{
			string(KindPlanner):       {Chain: []string{"shared"}},
			string(KindConversation):  {Chain: []string{"shared"}},
			string(KindMeeting):       {Chain: []string{"shared"}},
			string(KindConsolidation): {Chain: []string{"shared"}},
			string(KindNarrator):      {Chain: []string{"shared"}},
			string(KindDrama):         {Chain: []string{"shared"}},
			string(KindMetatron):      {Chain: []string{"shared"}},
		},
	}
}

// TestLeaseBoundsCombinedConcurrency (spec 024 US5 / SC-006, automated portion of
// quickstart §7): two orchestrators with separate configs and stores, sharing one
// endpoint + capacity through a temp lease base, never exceed the capacity in
// COMBINED in-flight calls under saturation — and contention alone never opens a
// breaker.
func TestLeaseBoundsCombinedConcurrency(t *testing.T) {
	const capacity = 3
	withLeaseBase(t, t.TempDir())
	srv, cc := newConcMock(t, 40*time.Millisecond)

	// 4 workers each × two worlds = 8 would-be concurrent callers; the shared
	// lease must clamp the endpoint to `capacity`.
	oA := newOrch(t, leaseConfig(srv.URL, capacity, 4), testStore(t))
	oB := newOrch(t, leaseConfig(srv.URL, capacity, 4), testStore(t))
	if oA.providers["shared"].leases == nil || oB.providers["shared"].leases == nil {
		t.Fatal("declared endpoint_capacity did not attach a lease pool")
	}

	const perOrch = 20
	var wg sync.WaitGroup
	submit := func(o *Orchestrator) {
		defer wg.Done()
		if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"}); err != nil {
			t.Errorf("submit: %v", err)
		}
	}
	for i := 0; i < perOrch; i++ {
		wg.Add(2)
		go submit(oA)
		go submit(oB)
	}
	wg.Wait()

	if peak := cc.max.Load(); peak > capacity {
		t.Fatalf("peak combined in-flight = %d, exceeds lease capacity %d", peak, capacity)
	}
	if peak := cc.max.Load(); peak < 2 {
		t.Fatalf("peak combined in-flight = %d — the mock never overlapped, so the bound is untested", peak)
	}
	if got, want := cc.hits.Load(), int64(2*perOrch); got != want {
		t.Errorf("served %d calls, want %d (every call must eventually acquire a slot)", got, want)
	}
	if oA.providers["shared"].health.down() || oB.providers["shared"].health.down() {
		t.Error("a breaker opened under lease contention alone (leases must never be a health signal)")
	}
}

// TestLeasePoolContendedFlag (spec 024 US5): a wait above the contended threshold
// raises the pool's contended flag; a subsequent sub-threshold wait clears it
// (hysteresis by observation, research R2). The threshold is compressed so the
// test stays fast.
func TestLeasePoolContendedFlag(t *testing.T) {
	prev := leaseContendedThreshold
	leaseContendedThreshold = 40 * time.Millisecond
	t.Cleanup(func() { leaseContendedThreshold = prev })

	lp, err := newLeasePool(t.TempDir(), "http://localhost:11434/v1", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the only slot, then a second acquire must wait past the threshold.
	rel1, _, err := lp.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	type acq struct {
		rel    func()
		waited time.Duration
	}
	got := make(chan acq, 1)
	go func() {
		rel, waited, err := lp.acquire(context.Background())
		if err != nil {
			t.Errorf("blocked acquire: %v", err)
		}
		got <- acq{rel, waited}
	}()
	time.Sleep(80 * time.Millisecond) // > threshold: the blocked sweep waits past it
	rel1()                            // free the slot; the blocked acquire now wins
	a := <-got
	if a.waited <= leaseContendedThreshold {
		t.Fatalf("blocked acquire waited %v, want > threshold %v", a.waited, leaseContendedThreshold)
	}
	if !lp.contended.Load() {
		t.Error("contended flag not set after a >threshold wait")
	}
	a.rel() // free the slot

	// A now-immediate acquire waits under the threshold and clears the flag.
	rel3, waited3, err := lp.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if waited3 > leaseContendedThreshold {
		t.Fatalf("uncontended acquire waited %v, expected near-zero", waited3)
	}
	if lp.contended.Load() {
		t.Error("contended flag not cleared after a sub-threshold wait")
	}
	rel3()
}

// TestLeaseSlotFreedOnRelease (spec 024 US5): a released slot is immediately
// reacquirable; while held, a bounded acquire times out on its context (never a
// breaker strike — that wiring is exercised through the worker in the concurrency
// test) rather than blocking forever.
func TestLeaseSlotFreedOnRelease(t *testing.T) {
	lp, err := newLeasePool(t.TempDir(), "http://localhost:11434/v1", 1)
	if err != nil {
		t.Fatal(err)
	}
	rel, _, err := lp.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	_, _, err = lp.acquire(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("saturated acquire err = %v, want DeadlineExceeded", err)
	}
	rel() // free the slot
	rel2, _, err := lp.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel2()
}

// TestLeaseReclaimOnClose (spec 024 US5 / SC-006, crash reclaim): closing the fds
// of a saturated pool's held slots — the same thing the kernel does for a dead
// process — makes those slots acquirable again with no operator action.
func TestLeaseReclaimOnClose(t *testing.T) {
	const capacity = 2
	lp, err := newLeasePool(t.TempDir(), "http://localhost:11434/v1", capacity)
	if err != nil {
		t.Fatal(err)
	}
	var held []func()
	for i := 0; i < capacity; i++ {
		rel, _, err := lp.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, rel)
	}
	// Saturated: a bounded acquire times out.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	_, _, err = lp.acquire(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("saturated pool acquire err = %v, want DeadlineExceeded", err)
	}
	// "Kill" the holders (close their fds); survivors reclaim the freed slots.
	for _, rel := range held {
		rel()
	}
	rel, _, err := lp.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire after reclaim: %v", err)
	}
	rel()
}

// TestUndeclaredCapacityNoLeases (spec 024 US5, legacy equivalence): a provider
// with no endpoint_capacity gets no pool and touches no lease directory at all —
// zero syscalls, today's behavior.
func TestUndeclaredCapacityNoLeases(t *testing.T) {
	base := t.TempDir()
	withLeaseBase(t, base)
	var hits atomic.Int64
	srv := mockLocal(t, &hits)
	o := newOrch(t, twoProviderConfig(srv.URL, srv.URL, 2, 2), testStore(t))
	if o.providers["fast"].leases != nil || o.providers["slow"].leases != nil {
		t.Error("a provider without endpoint_capacity was given a lease pool")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("undeclared capacity created %d lease dir entries, want 0", len(entries))
	}
}

// TestContendedFlowsToStatus (spec 024 US5): the pool's contended flag surfaces on
// the provider's StatusSnapshot row.
func TestContendedFlowsToStatus(t *testing.T) {
	withLeaseBase(t, t.TempDir())
	var hits atomic.Int64
	srv := mockLocal(t, &hits)
	o := newOrch(t, leaseConfig(srv.URL, 2, 1), testStore(t))
	lp := o.providers["shared"].leases
	if lp == nil {
		t.Fatal("no lease pool attached")
	}
	if row := provStatus(o.StatusSnapshot(), "shared"); row.Contended {
		t.Error("provider reported contended before any contention")
	}
	lp.contended.Store(true)
	if row := provStatus(o.StatusSnapshot(), "shared"); !row.Contended {
		t.Error("StatusSnapshot did not reflect the lease pool's contended flag")
	}
}

// TestNormalizeEndpoint (spec 024 US5, contract normalization): cosmetically
// different spellings of one endpoint normalize (and thus hash) identically;
// genuinely different paths stay distinct.
func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HTTP://LocalHost:11434/v1/", "http://localhost:11434/v1"},
		{"http://localhost:11434/v1", "http://localhost:11434/v1"},
		{"http://Example.com:80/v1", "http://example.com/v1"},
		{"https://Example.com:443/", "https://example.com"},
		{"http://localhost:11434/", "http://localhost:11434"},
		{"http://localhost:11434", "http://localhost:11434"},
	}
	for _, c := range cases {
		if got := normalizeEndpoint(c.in); got != c.want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Equivalent spellings hash to one pool dir.
	if endpointHash(normalizeEndpoint("HTTP://LocalHost:11434/v1/")) !=
		endpointHash(normalizeEndpoint("http://localhost:11434/v1")) {
		t.Error("equivalent endpoints hashed to different pools")
	}
	// Distinct paths stay distinct.
	if endpointHash(normalizeEndpoint("http://localhost:11434/v1")) ==
		endpointHash(normalizeEndpoint("http://localhost:11434/v2")) {
		t.Error("distinct endpoint paths collapsed to one pool")
	}
}

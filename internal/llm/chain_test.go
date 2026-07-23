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

// twoProviderConfig is a v2 registry (spec 024) with two openai_compat providers
// — a "fast" one (parallel 4) serving the high-volume chatty kinds and a "slow"
// one (parallel 2) serving prose kinds — both zero-priced so the wallet never
// interferes. This is the measured division of labor's shape (US2), used here to
// prove US1's chain-head dispatch.
func twoProviderConfig(fastURL, slowURL string, fastParallel, slowParallel int) Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Providers: map[string]ProviderConfig{
			"fast": {Transport: ProviderOpenAICompat, Endpoint: fastURL, Model: "fast-model", Parallel: fastParallel},
			"slow": {Transport: ProviderOpenAICompat, Endpoint: slowURL, Model: "slow-model", Parallel: slowParallel},
		},
		Routes: map[string]RouteConfig{
			string(KindPlanner):       {Chain: []string{"fast"}},
			string(KindConversation):  {Chain: []string{"fast"}},
			string(KindMeeting):       {Chain: []string{"fast"}},
			string(KindConsolidation): {Chain: []string{"slow"}},
			string(KindNarrator):      {Chain: []string{"slow"}},
			string(KindDrama):         {Chain: []string{"slow"}},
			string(KindMetatron):      {Chain: []string{"slow"}},
		},
	}
}

// TestChainHeadDispatch (spec 024 US1 AC#2): a v2 config with two providers
// routes each kind to its chain head; Response.Provider (and the serving-
// provider Model) name it; the pin field bypasses the chain to a named provider;
// an unknown pin fails with ErrUnknownProvider.
func TestChainHeadDispatch(t *testing.T) {
	var fastHits, slowHits atomic.Int64
	fast := mockLocal(t, &fastHits)
	slow := mockLocal(t, &slowHits)
	o := newOrch(t, twoProviderConfig(fast.URL, slow.URL, 4, 2), testStore(t))

	cases := []struct {
		kind     Kind
		provider string
	}{
		{KindPlanner, "fast"}, {KindConversation, "fast"}, {KindMeeting, "fast"},
		{KindConsolidation, "slow"}, {KindNarrator, "slow"}, {KindDrama, "slow"}, {KindMetatron, "slow"},
	}
	for _, c := range cases {
		resp, err := o.Submit(context.Background(), Request{Kind: c.kind, Prompt: "x"})
		if err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		if resp.Provider != c.provider {
			t.Errorf("%s served by %q, want chain head %q", c.kind, resp.Provider, c.provider)
		}
		if resp.Model != c.provider+"-model" {
			t.Errorf("%s reported model %q, want %q", c.kind, resp.Model, c.provider+"-model")
		}
	}
	// fast serves planner/conversation/meeting (3), slow serves the other four.
	if fastHits.Load() != 3 || slowHits.Load() != 4 {
		t.Errorf("hits: fast=%d slow=%d, want 3/4", fastHits.Load(), slowHits.Load())
	}

	// Worker slots per provider match the clamped parallel.
	if got := o.providers["fast"].slots; got != 4 {
		t.Errorf("fast provider slots = %d, want 4", got)
	}
	if got := o.providers["slow"].slots; got != 2 {
		t.Errorf("slow provider slots = %d, want 2", got)
	}

	// Request.Provider pin bypasses the chain: a planner call pinned to "slow"
	// is served by slow, not its chain head "fast".
	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x", Provider: "slow"})
	if err != nil {
		t.Fatalf("pinned submit: %v", err)
	}
	if resp.Provider != "slow" {
		t.Errorf("pinned planner served by %q, want slow", resp.Provider)
	}

	// An unknown pin fails fast with ErrUnknownProvider (config-drift guard).
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x", Provider: "ghost"}); !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("unknown pin: got %v, want ErrUnknownProvider", err)
	}
}

// parkServer is an openai_compat mock whose handlers park until release, tracking
// the high-water mark of simultaneous in-flight handlers — the instrument for
// proving a provider's worker concurrency.
func parkServer(t *testing.T, release <-chan struct{}) (*httptest.Server, *atomic.Int32, chan struct{}) {
	t.Helper()
	var cur, max atomic.Int32
	arrived := make(chan struct{}, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := cur.Add(1)
		for { // record the high-water mark of concurrent handlers
			m := max.Load()
			if c <= m || max.CompareAndSwap(m, c) {
				break
			}
		}
		arrived <- struct{}{}
		<-release
		cur.Add(-1)
		localReplyJSON(w)
	}))
	t.Cleanup(srv.Close)
	return srv, &max, arrived
}

// TestChainHeadPerProviderConcurrency (spec 024 US1 T007, SC-003; -race): two
// providers run their own worker pools SIMULTANEOUSLY — each provider's calls
// land on its chain head and reach exactly its declared parallelism at once, with
// neither pool's saturation bounding the other. Extends TASK-45's single-tier
// concurrency proof to two providers under load.
func TestChainHeadPerProviderConcurrency(t *testing.T) {
	const fastN, slowN = 3, 2
	release := make(chan struct{})
	fast, fastMax, fastArr := parkServer(t, release)
	slow, slowMax, slowArr := parkServer(t, release)

	o := newOrch(t, twoProviderConfig(fast.URL, slow.URL, fastN, slowN), testStore(t))

	errs := make(chan error, fastN+slowN)
	// planner routes to fast, narrator to slow — drive both pools to saturation
	// at the same instant.
	for i := 0; i < fastN; i++ {
		go func() {
			_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
			errs <- err
		}()
	}
	for i := 0; i < slowN; i++ {
		go func() {
			_, err := o.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "x"})
			errs <- err
		}()
	}

	watchdog := time.After(5 * time.Second)
	for i := 0; i < fastN; i++ {
		select {
		case <-fastArr:
		case <-watchdog:
			t.Fatalf("only %d/%d fast-provider calls reached the server", i, fastN)
		}
	}
	for i := 0; i < slowN; i++ {
		select {
		case <-slowArr:
		case <-watchdog:
			t.Fatalf("only %d/%d slow-provider calls reached the server", i, slowN)
		}
	}

	// Both pools are saturated at the same time: each hit its own parallelism.
	if got := fastMax.Load(); got != fastN {
		t.Errorf("fast provider max in-flight = %d, want %d", got, fastN)
	}
	if got := slowMax.Load(); got != slowN {
		t.Errorf("slow provider max in-flight = %d, want %d", got, slowN)
	}

	close(release)
	for i := 0; i < fastN+slowN; i++ {
		if err := <-errs; err != nil {
			t.Errorf("submit: %v", err)
		}
	}
}

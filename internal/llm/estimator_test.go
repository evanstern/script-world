package llm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// fallbackConfig is a v2 registry whose planner/conversation routes are TWO-entry
// chains (fast → slow) so the admissible-head seam and the US3 dispatch walk have
// a real fallback to exercise; both providers are zero-priced (the wallet never
// interferes). The remaining kinds stay single-entry so legacy-shaped behavior is
// still present in the same world.
func fallbackConfig(fastURL, slowURL string) Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Providers: map[string]ProviderConfig{
			"fast": {Transport: ProviderOpenAICompat, Endpoint: fastURL, Model: "fast-model", Parallel: 2},
			"slow": {Transport: ProviderOpenAICompat, Endpoint: slowURL, Model: "slow-model", Parallel: 2},
		},
		Routes: map[string]RouteConfig{
			string(KindPlanner):       {Chain: []string{"fast", "slow"}},
			string(KindConversation):  {Chain: []string{"fast", "slow"}},
			string(KindMeeting):       {Chain: []string{"fast"}},
			string(KindConsolidation): {Chain: []string{"slow"}},
			string(KindNarrator):      {Chain: []string{"slow"}},
			string(KindDrama):         {Chain: []string{"slow"}},
			string(KindMetatron):      {Chain: []string{"slow"}},
		},
	}
}

// walletFallbackConfig routes planner through a PRICED head (premium) to a
// zero-priced fallback (free), so the wallet-skip branch of admission has a
// priced candidate to refuse when the ceiling is hit.
func walletFallbackConfig(url string) Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Providers: map[string]ProviderConfig{
			"premium": {Transport: ProviderOpenAICompat, Endpoint: url, Model: "premium-model", InputUSDPerMTok: 10, OutputUSDPerMTok: 10},
			"free":    {Transport: ProviderOpenAICompat, Endpoint: url, Model: "free-model"},
		},
		Routes: map[string]RouteConfig{
			string(KindPlanner):       {Chain: []string{"premium", "free"}},
			string(KindConversation):  {Chain: []string{"free"}},
			string(KindMeeting):       {Chain: []string{"free"}},
			string(KindConsolidation): {Chain: []string{"premium"}},
			string(KindNarrator):      {Chain: []string{"premium"}},
			string(KindDrama):         {Chain: []string{"premium"}},
			string(KindMetatron):      {Chain: []string{"premium"}},
		},
	}
}

// TestEstimateForKindBreakerFallback (spec 024 T009): EstimateForKind names the
// CURRENT ADMISSIBLE chain head — the first candidate whose breaker is closed —
// and falls back to the chain head only when every candidate is inadmissible.
func TestEstimateForKindBreakerFallback(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, fallbackConfig(fast.URL, slow.URL), testStore(t))

	if name, _, ok := o.EstimateForKind(KindPlanner); !ok || name != "fast" {
		t.Errorf("both healthy: estimate head = %q (ok=%v), want fast", name, ok)
	}

	// Open fast's breaker (3 genuine failures): the estimate moves to slow.
	for i := 0; i < failuresToOpen; i++ {
		o.providers["fast"].health.fail()
	}
	if name, _, _ := o.EstimateForKind(KindPlanner); name != "slow" {
		t.Errorf("fast down: estimate head = %q, want slow (first admissible)", name)
	}

	// Open slow too: nothing is admissible, so it falls back to the chain head.
	for i := 0; i < failuresToOpen; i++ {
		o.providers["slow"].health.fail()
	}
	if name, _, _ := o.EstimateForKind(KindPlanner); name != "fast" {
		t.Errorf("all down: estimate head = %q, want the chain head fast (fallback)", name)
	}

	// EstimateForKind must be a pure read — down() never consumes a half-open
	// probe, so a fresh provider's breaker state is untouched by the queries above.
	if _, _, ok := o.EstimateForKind("sorcery"); ok {
		t.Error("unknown kind returned ok=true")
	}
}

// TestEstimateForKindWalletFallback (spec 024 T009): a priced head that has hit
// the monthly ceiling is inadmissible, so the estimate moves to the zero-priced
// fallback; with budget available the priced head is admissible again.
func TestEstimateForKindWalletFallback(t *testing.T) {
	var hits atomic.Int64
	srv := mockLocal(t, &hits)

	st := testStore(t)
	o := newOrch(t, walletFallbackConfig(srv.URL), st)
	if name, _, _ := o.EstimateForKind(KindPlanner); name != "premium" {
		t.Errorf("budget available: estimate head = %q, want premium", name)
	}

	// Exhaust the wallet: the priced head is skipped, so the free fallback heads.
	st2 := testStore(t)
	if err := st2.SetMeta(metaKey(currentMonth()), "9999"); err != nil {
		t.Fatal(err)
	}
	o2 := newOrch(t, walletFallbackConfig(srv.URL), st2)
	if name, _, _ := o2.EstimateForKind(KindPlanner); name != "free" {
		t.Errorf("budget exhausted: estimate head = %q, want free (priced head skipped)", name)
	}
	// A single-entry priced route with no fallback still names its head (the
	// routing seam must always name a provider), even when inadmissible.
	if name, _, _ := o2.EstimateForKind(KindNarrator); name != "premium" {
		t.Errorf("exhausted single-entry priced route = %q, want its head premium", name)
	}
}

// TestEstimatorAttributionConcurrent (spec 024 T009, -race): under simultaneous
// two-provider load, each provider's estimator observes ONLY its own calls — a
// fast small model is never averaged with a slow quality model. planner routes to
// fast, narrator to slow (single-entry chains), so exact sample counts prove no
// cross-attribution and the -race flag proves the per-provider feed is safe under
// concurrency.
func TestEstimatorAttributionConcurrent(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, twoProviderConfig(fast.URL, slow.URL, 4, 4), testStore(t))

	const n = 24
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"}); err != nil {
				t.Errorf("planner submit: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := o.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "x"}); err != nil {
				t.Errorf("narrator submit: %v", err)
			}
		}()
	}
	wg.Wait()

	if _, _, fastSamples, _ := o.providers["fast"].est.Stats(); fastSamples != n {
		t.Errorf("fast estimator saw %d samples, want exactly %d (its own planner calls only)", fastSamples, n)
	}
	if _, _, slowSamples, _ := o.providers["slow"].est.Stats(); slowSamples != n {
		t.Errorf("slow estimator saw %d samples, want exactly %d (its own narrator calls only)", slowSamples, n)
	}
}

// TestObserveCognitionNamedProvider (spec 024 T009): the whole-cognition
// observation lands on the NAMED serving provider's estimator — even when that
// is not the kind's chain head — and an empty name falls back to the head. This
// is the seam the tool-use loop drives with Response.Provider, so a fallback that
// served a different provider than the head is measured where the work happened.
func TestObserveCognitionNamedProvider(t *testing.T) {
	var fh, sh atomic.Int64
	fast := mockLocal(t, &fh)
	slow := mockLocal(t, &sh)
	o := newOrch(t, twoProviderConfig(fast.URL, slow.URL, 4, 4), testStore(t))
	fastEst, slowEst := o.providers["fast"].est, o.providers["slow"].est

	_, _, fs0, _ := fastEst.Stats()
	_, _, ss0, _ := slowEst.Stats()

	// planner's chain head is fast, but the loop reports "slow" served it: slow's
	// estimator gets the sample, fast's is untouched.
	o.ObserveCognition(KindPlanner, "slow", 6000)
	if _, _, fs1, _ := fastEst.Stats(); fs1 != fs0 {
		t.Errorf("named-provider observation leaked to the head estimator: fast %d -> %d", fs0, fs1)
	}
	if _, _, ss1, _ := slowEst.Stats(); ss1 != ss0+1 {
		t.Errorf("named-provider observation missed slow: %d -> %d, want +1", ss0, ss1)
	}

	// An empty name falls back to the kind's chain head (fast).
	o.ObserveCognition(KindPlanner, "", 6000)
	if _, _, fs2, _ := fastEst.Stats(); fs2 != fs0+1 {
		t.Errorf("empty-name observation missed the chain head: fast %d -> %d, want +1", fs0, fs2)
	}

	// An unknown provider name defensively falls back to the head, never a panic.
	o.ObserveCognition(KindPlanner, "ghost", 6000)
	if _, _, fs3, _ := fastEst.Stats(); fs3 != fs0+2 {
		t.Errorf("unknown-name observation must fall back to the head: fast now %d, want %d", fs3, fs0+2)
	}
}

package llm

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

// sumAttributed returns Σ(perProvider) and the unattributed remainder against a
// total — the invariant every meter path must hold: Σ + unattributed == total.
func sumAttributed(perProvider map[string]float64, total float64) (sum, unattributed float64) {
	for _, v := range perProvider {
		sum += v
	}
	return sum, total - sum
}

// TestMeterConcurrentAttribution (spec 024 T012, -race): concurrent Add calls
// attribute to the right provider with no lost or cross-counted money — under
// parallel load Σ(perProvider) + unattributed == total, and each provider's
// share is exactly its own calls' sum.
func TestMeterConcurrentAttribution(t *testing.T) {
	m, err := NewMeter(testStore(t), 1e9, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	const per = 0.01
	const nA, nB = 128, 96
	var wg sync.WaitGroup
	for i := 0; i < nA; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); mustAdd(t, m, "a", per) }()
	}
	for i := 0; i < nB; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); mustAdd(t, m, "b", per) }()
	}
	wg.Wait()

	_, spent, _, pp := m.Snapshot()
	sum, unattributed := sumAttributed(pp, spent)
	if math.Abs(unattributed) > 1e-9 {
		t.Errorf("unattributed = %v, want 0 (every dollar attributed)", unattributed)
	}
	if math.Abs(pp["a"]-nA*per) > 1e-9 {
		t.Errorf("a attributed %v, want %v", pp["a"], nA*per)
	}
	if math.Abs(pp["b"]-nB*per) > 1e-9 {
		t.Errorf("b attributed %v, want %v", pp["b"], nB*per)
	}
	if math.Abs(sum-spent) > 1e-9 || math.Abs(spent-(nA+nB)*per) > 1e-9 {
		t.Errorf("total = %v (Σ = %v), want %v", spent, sum, (nA+nB)*per)
	}
}

// TestMeterAttributionPersistsAcrossReopen (spec 024 T012): per-provider spend
// survives a store reopen (a daemon restart) with zero migration — the meter
// reloads each declared provider's persisted key, and Σ still equals the total.
func TestMeterAttributionPersistsAcrossReopen(t *testing.T) {
	st := testStore(t)
	m1, err := NewMeter(st, 1e9, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	mustAdd(t, m1, "a", 1.5)
	mustAdd(t, m1, "b", 2.5)
	mustAdd(t, m1, "a", 0.5)

	m2, err := NewMeter(st, 1e9, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	_, spent, _, pp := m2.Snapshot()
	if pp["a"] != 2.0 || pp["b"] != 2.5 {
		t.Errorf("reloaded attribution = %v, want a=2.0 b=2.5", pp)
	}
	if _, unattributed := sumAttributed(pp, spent); math.Abs(unattributed) > 1e-9 {
		t.Errorf("unattributed after reopen = %v, want 0", unattributed)
	}
	if spent != 4.5 {
		t.Errorf("total after reopen = %v, want 4.5", spent)
	}
}

// TestMeterLegacyMonthUnattributed (spec 024 T012, research R4): a legacy month —
// a persisted total with NO per-provider breakdown — surfaces the whole total as
// the unattributed remainder (never invented backfill), and a fresh Add
// attributes going forward while the legacy remainder stays unattributed.
func TestMeterLegacyMonthUnattributed(t *testing.T) {
	st := testStore(t)
	if err := st.SetMeta(metaKey(currentMonth()), "7.25"); err != nil {
		t.Fatal(err)
	}
	m, err := NewMeter(st, 1e9, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	_, spent, _, pp := m.Snapshot()
	sum, unattributed := sumAttributed(pp, spent)
	if sum != 0 || math.Abs(unattributed-7.25) > 1e-9 {
		t.Errorf("legacy month: Σ = %v unattributed = %v, want 0 / 7.25", sum, unattributed)
	}

	mustAdd(t, m, "a", 1.0)
	_, spent2, _, pp2 := m.Snapshot()
	sum2, unattributed2 := sumAttributed(pp2, spent2)
	if pp2["a"] != 1.0 || math.Abs(sum2-1.0) > 1e-9 {
		t.Errorf("post-Add attribution = %v (Σ %v), want a=1.0", pp2, sum2)
	}
	if math.Abs(unattributed2-7.25) > 1e-9 || math.Abs(spent2-8.25) > 1e-9 {
		t.Errorf("legacy remainder shifted: total=%v unattributed=%v, want 8.25 / 7.25", spent2, unattributed2)
	}
}

// TestAttributionCeilingSkipsPricedZeroPricedServes (spec 024 T012, US3 tie-in):
// with the wallet at its ceiling a priced candidate is skipped and the
// zero-priced fallback serves for free; StatusSnapshot attributes the earlier
// priced spend to the priced provider, shows the zero-priced provider at $0, and
// Σ(rows) + unattributed still equals the total.
func TestAttributionCeilingSkipsPricedZeroPricedServes(t *testing.T) {
	var hits atomic.Int64
	srv := mockLocal(t, &hits)
	cfg := walletFallbackConfig(srv.URL)
	// One premium call costs (10 in + 5 out tokens) × $10/Mtok = 1.5e-4. A budget
	// of 1e-4 lets the first priced call through, then bars the wallet.
	cfg.MonthlyBudgetUSD = 1e-4
	o := newOrch(t, cfg, testStore(t))

	r1, err := o.Submit(context.Background(), Request{Kind: KindConsolidation, Prompt: "x"})
	if err != nil {
		t.Fatalf("priced consolidation: %v", err)
	}
	if r1.Provider != "premium" || r1.CostUSD <= 0 {
		t.Fatalf("consolidation served by %q cost %v, want premium billed", r1.Provider, r1.CostUSD)
	}

	// The wallet is now exhausted: a planner walk skips premium and free serves.
	r2, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if err != nil {
		t.Fatalf("planner walk: %v", err)
	}
	if r2.Provider != "free" || r2.CostUSD != 0 {
		t.Errorf("planner served by %q cost %v, want free at $0", r2.Provider, r2.CostUSD)
	}

	st := o.StatusSnapshot()
	if got := provStatus(st, "premium").SpentUSD; math.Abs(got-r1.CostUSD) > 1e-12 {
		t.Errorf("premium attributed %v, want the consolidation cost %v", got, r1.CostUSD)
	}
	if got := provStatus(st, "free").SpentUSD; got != 0 {
		t.Errorf("free (zero-priced) attributed %v, want 0", got)
	}
	sum := provStatus(st, "premium").SpentUSD + provStatus(st, "free").SpentUSD
	if math.Abs(st.Spent-sum) > 1e-12 || math.Abs(st.Spent-r1.CostUSD) > 1e-12 {
		t.Errorf("total %v vs Σ(rows) %v, want equal and == %v", st.Spent, sum, r1.CostUSD)
	}
}

func mustAdd(t *testing.T, m *Meter, provider string, cost float64) {
	t.Helper()
	if err := m.Add(provider, cost); err != nil {
		t.Errorf("meter add %q: %v", provider, err)
	}
}

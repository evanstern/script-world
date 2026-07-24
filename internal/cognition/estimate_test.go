package cognition

import (
	"math"
	"sort"
	"testing"
)

func TestEstimatorSpikeExcluded(t *testing.T) {
	e := NewEstimator(17.0)
	e.Sample(100.0) // > 3x17 — a lag spike
	if got := e.Estimate(); got != 17.0 {
		t.Errorf("spike moved the estimate: %g", got)
	}
	_, _, samples, spikes := e.Stats()
	if samples != 1 || spikes != 1 {
		t.Errorf("counts = %d samples, %d spikes; want 1, 1", samples, spikes)
	}
}

func TestEstimatorFollowsDrift(t *testing.T) {
	e := NewEstimator(17.0)
	for i := 0; i < 50; i++ {
		e.Sample(25.0) // sustained systemic drift, under the spike bar
	}
	if got := e.Estimate(); math.Abs(got-25.0) > 0.1 {
		t.Errorf("estimate %g did not converge to 25", got)
	}
}

func TestEstimatorBreachSignalOnceAndRearm(t *testing.T) {
	// The breach signal (now carried by a non-nil Adoption — spec 031) fires
	// exactly once per episode and re-arms after the window drains. Adoption
	// itself is asserted separately; here we pin the fire-once/re-arm timing.
	e := NewEstimator(10.0)
	// Fill the window under threshold first.
	for i := 0; i < WindowSize; i++ {
		if e.Sample(10.0) != nil {
			t.Fatal("breach before any spikes")
		}
	}
	// Push spikes past 30% of the window: signal fires exactly once.
	fired := 0
	for i := 0; i < 8; i++ {
		if e.Sample(100.0) != nil {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("breach fired %d times, want exactly 1", fired)
	}
	// Recovery drains the window; the signal re-arms and can fire again.
	for i := 0; i < WindowSize; i++ {
		if e.Sample(10.0) != nil {
			t.Fatal("fired during recovery")
		}
	}
	fired = 0
	for i := 0; i < 8; i++ {
		if e.Sample(100.0) != nil {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("re-armed breach fired %d times, want exactly 1", fired)
	}
}

func TestEstimatorNoVerdictBeforeFullWindow(t *testing.T) {
	e := NewEstimator(10.0)
	for i := 0; i < WindowSize-1; i++ {
		if e.Sample(100.0) != nil {
			t.Fatal("breach verdict before the window filled")
		}
	}
}

// feed drives n samples of the same value and reports how many adoptions
// fired plus the last one — a helper for the sustained-load families below.
func feed(e *Estimator, v float64, n int) (adoptions int, last *Adoption) {
	for i := 0; i < n; i++ {
		if a := e.Sample(v); a != nil {
			adoptions++
			last = a
		}
	}
	return adoptions, last
}

// TestEstimatorAdoptsSustainedSlowdown is the world-01 freeze regression (spec
// 031 SC-001): seeded at the frozen 0.52 s/pt and fed a sustained ~12 s/pt
// stream (every sample beyond 3x the estimate, which pre-031 froze the estimate
// forever), the estimator adopts the window median exactly once, on the sample
// that completes the first full window.
func TestEstimatorAdoptsSustainedSlowdown(t *testing.T) {
	const seed = 0.52
	e := NewEstimator(seed)
	vals := make([]float64, WindowSize)
	adoptions := 0
	var got *Adoption
	for i := 0; i < WindowSize; i++ {
		v := 12.0 + 0.1*float64(i) // ~12 s/pt, varied so the median is a real median
		vals[i] = v
		if a := e.Sample(v); a != nil {
			adoptions++
			got = a
			if i != WindowSize-1 {
				t.Fatalf("adoption at sample %d, want the window-completing sample %d", i+1, WindowSize)
			}
		}
	}
	if adoptions != 1 {
		t.Fatalf("adoptions = %d, want exactly 1", adoptions)
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	wantMedian := (sorted[WindowSize/2-1] + sorted[WindowSize/2]) / 2
	if got.Prior != seed {
		t.Errorf("Prior = %g, want the untouched seed %g (every sample was a spike)", got.Prior, seed)
	}
	if got.Adopted != wantMedian {
		t.Errorf("Adopted = %g, want window median %g", got.Adopted, wantMedian)
	}
	if est := e.Estimate(); est != got.Adopted {
		t.Errorf("estimate after adoption = %g, want adopted %g", est, got.Adopted)
	}
}

// TestEstimatorFollowsDownwardStepNoAdoption: samples below the estimate are
// never spikes, so a system that got faster is followed by the ordinary EWMA
// with no adoption — the freeze is asymmetric by construction (spec 031 US1
// scenario 3, research R5).
func TestEstimatorFollowsDownwardStepNoAdoption(t *testing.T) {
	e := NewEstimator(12.0)
	adoptions, _ := feed(e, 2.0, 50)
	if adoptions != 0 {
		t.Fatalf("downward step produced %d adoptions, want 0", adoptions)
	}
	if got := e.Estimate(); math.Abs(got-2.0) > 0.1 {
		t.Errorf("estimate %g did not follow down to 2.0", got)
	}
}

// TestEstimatorReArmsAfterAdoption: after adopting a sustained slowdown, stable
// samples at the adopted level never re-adopt (re-arm is structural — a fresh
// window must refill and breach again), while a second genuine sustained step
// is followed by further adoption (spec 031 FR-003, US3 scenario 2).
func TestEstimatorReArmsAfterAdoption(t *testing.T) {
	e := NewEstimator(0.52)
	up1, first := feed(e, 12.0, WindowSize)
	if up1 != 1 || first == nil {
		t.Fatalf("first sustained window: %d adoptions, want exactly 1", up1)
	}
	// Stable at the adopted level: no breach, no adoption.
	if stable, _ := feed(e, first.Adopted, 3*WindowSize); stable != 0 {
		t.Fatalf("stable at the adopted level produced %d adoptions, want 0", stable)
	}
	// A second genuine sustained step (12 -> 40 s/pt, > 3x) re-arms and is
	// followed up to the new regime.
	if up2, _ := feed(e, 40.0, 4*WindowSize); up2 < 1 {
		t.Fatal("second sustained step produced no adoption — re-arm failed")
	}
	if est := e.Estimate(); math.Abs(est-40.0) > 0.1 {
		t.Errorf("estimate %g did not follow the second step to 40", est)
	}
}

// TestEstimatorOneShotSpikesPreserveEWMA is the one-shot-rejection regression
// (spec 031 SC-002): with <=2 spikes per window the spike rate never breaches
// (<=4/20 < 0.3), so no adoption fires and the estimate is bit-identical (full
// float64 equality) to the EWMA over the non-spike samples alone.
func TestEstimatorOneShotSpikesPreserveEWMA(t *testing.T) {
	const seed = 5.0
	normals := []float64{5.2, 4.8, 5.5, 5.1, 4.9, 5.3, 5.0, 5.4, 4.7, 5.6}
	e := NewEstimator(seed)
	ref := seed // reference EWMA over the non-spike samples only
	for round := 0; round < 5; round++ {
		for i, v := range normals {
			if i == 3 || i == 7 { // two isolated spikes per 10-sample block
				if a := e.Sample(100.0); a != nil {
					t.Fatalf("adoption on a one-shot spike (round %d, i %d)", round, i)
				}
			}
			if a := e.Sample(v); a != nil {
				t.Fatalf("adoption on a normal sample (round %d, i %d)", round, i)
			}
			ref = (1-EWMAAlpha)*ref + EWMAAlpha*v
		}
	}
	if est := e.Estimate(); est != ref {
		t.Errorf("estimate %v != reference EWMA over non-spike samples %v", est, ref)
	}
}

package cognition

import (
	"math"
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
	e := NewEstimator(10.0)
	// Fill the window under threshold first.
	for i := 0; i < WindowSize; i++ {
		if e.Sample(10.0) {
			t.Fatal("breach before any spikes")
		}
	}
	// Push spikes past 30% of the window: signal fires exactly once.
	fired := 0
	for i := 0; i < 8; i++ {
		if e.Sample(100.0) {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("breach fired %d times, want exactly 1", fired)
	}
	// Recovery drains the window; the signal re-arms and can fire again.
	for i := 0; i < WindowSize; i++ {
		if e.Sample(10.0) {
			t.Fatal("fired during recovery")
		}
	}
	fired = 0
	for i := 0; i < 8; i++ {
		if e.Sample(100.0) {
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
		if e.Sample(100.0) {
			t.Fatal("breach verdict before the window filled")
		}
	}
}

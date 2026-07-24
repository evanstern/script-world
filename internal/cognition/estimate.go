package cognition

import "sync"

// Estimator tuning (specs/007-cognition-horizon/contracts/calibration.md).
const (
	EWMAAlpha   = 0.2
	SpikeFactor = 3.0
	WindowSize  = 20
	BreachRate  = 0.3
	// Bootstrap defaults when no calibration profile exists — deliberately
	// pessimistic so an uncalibrated world fails toward reflex, never toward
	// stale action.
	BootstrapLocalSecPerPt = 20.0
	BootstrapCloudSecPerPt = 10.0
)

// windowSample is one retained ring slot: the observed per-point duration and
// its spike classification. The value is retained (spec 031 FR-001) so the
// estimator can adopt the window median at breach instead of freezing.
type windowSample struct {
	secPerPoint float64
	spike       bool
}

// Estimator is the live seconds-per-point estimate for one tier: an EWMA
// over per-point-normalized call durations with spike rejection. One-shot
// lag spikes are excluded from the estimate but counted; systemic drift is
// followed. Process-lifetime only — restarts re-seed from the calibration
// profile; the recorded baseline moves only when a human re-runs calibrate.
type Estimator struct {
	mu       sync.Mutex
	estimate float64
	window   []windowSample // ring of the last WindowSize samples (value + spike flag)
	wi, wn   int
	samples  int
	spikes   int
	breached bool // recalibration signal armed state
}

func NewEstimator(seed float64) *Estimator {
	return &Estimator{estimate: seed, window: make([]windowSample, WindowSize)}
}

// Estimate returns the current seconds-per-point.
func (e *Estimator) Estimate() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.estimate
}

// Stats returns the current estimate, rolling spike rate, and lifetime
// sample/spike counts — telemetry material.
func (e *Estimator) Stats() (estimate, spikeRate float64, samples, spikes int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.estimate, e.rateLocked(), e.samples, e.spikes
}

func (e *Estimator) rateLocked() float64 {
	if e.wn == 0 {
		return 0
	}
	n := 0
	for i := 0; i < e.wn; i++ {
		if e.window[i].spike {
			n++
		}
	}
	return float64(n) / float64(e.wn)
}

// Sample feeds one observed per-point duration in seconds. A sample beyond
// SpikeFactor times the current estimate is excluded from the EWMA but
// counted. Returns true exactly when the spike rate over a full window first
// breaches BreachRate — the cog.recalibration_recommended signal, re-armed
// once the rate falls back under the threshold.
func (e *Estimator) Sample(secPerPoint float64) (recalibrate bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.samples++
	spike := secPerPoint > SpikeFactor*e.estimate
	if spike {
		e.spikes++
	} else {
		e.estimate = (1-EWMAAlpha)*e.estimate + EWMAAlpha*secPerPoint
	}
	e.window[e.wi] = windowSample{secPerPoint: secPerPoint, spike: spike}
	e.wi = (e.wi + 1) % WindowSize
	if e.wn < WindowSize {
		e.wn++
		return false // no breach verdicts until the window is full
	}
	if e.rateLocked() > BreachRate {
		if !e.breached {
			e.breached = true
			return true
		}
		return false
	}
	e.breached = false
	return false
}

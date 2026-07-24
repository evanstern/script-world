package cognition

import (
	"sort"
	"sync"
)

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

// Adoption is the evidence a breach produced: on a sustained slowdown the
// estimator re-seeds itself from the window median instead of freezing at the
// stale seed. Sample returns it (nil when no breach). Plain values only —
// internal/cognition stays a stdlib-only leaf (no event or JSON types), so the
// orchestrator carries this to the event log via the existing recalibrate hook
// (spec 031 FR-005; research R3).
type Adoption struct {
	Prior     float64 // estimate immediately before adoption
	Adopted   float64 // window median installed as the new estimate
	SpikeRate float64 // rolling spike rate at the breaching sample
}

// Estimator is the live seconds-per-point estimate for one tier: an EWMA
// over per-point-normalized call durations with spike rejection. One-shot
// lag spikes are excluded from the estimate but counted; systemic drift is
// followed — the persistence classifier (the rolling spike-rate window) is the
// actor: when the spike rate first breaches BreachRate over a full window the
// estimator adopts the window median, so a step change larger than SpikeFactor
// is tracked rather than mistaken for an endless run of one-shot spikes (spec
// 031). Process-lifetime only — restarts re-seed from the calibration profile;
// the recorded baseline moves only when a human re-runs calibrate.
type Estimator struct {
	mu       sync.Mutex
	estimate float64
	window   []windowSample // ring of the last WindowSize samples (value + spike flag)
	wi, wn   int
	samples  int
	spikes   int
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
// counted. It returns non-nil exactly when the spike rate over a full window
// first breaches BreachRate — the cog.recalibration_recommended episode, which
// post-spec-031 also ADOPTS: the estimator re-seeds itself to the window median
// (over the retained values, spike and non-spike alike) and zeroes the ring, so
// a fresh window must refill before any further breach. Fire-once and re-arm
// are structural — the reset empties the window, and post-adoption samples in
// the new regime are no longer spikes against the adopted estimate, so no
// separate armed flag is needed (spec 031 FR-002/FR-003; research R3/R4). The
// returned Adoption is the audit evidence (prior, adopted, spike rate); nil
// means no breach. The verdict is evaluated on the sample that completes a full
// window — the earliest sample at which "rate over a full window" is defined.
// Adoption is atomic with detection under the single mutex hold, so concurrent
// completions on the shared per-provider estimator cannot double-adopt one
// episode (FR-006; spec edge case "concurrent observation").
func (e *Estimator) Sample(secPerPoint float64) *Adoption {
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
	}
	if e.wn < WindowSize {
		return nil // no breach verdicts until the window is full
	}
	rate := e.rateLocked()
	if rate > BreachRate {
		// Persistence proven: adopt the window median as the new estimate and
		// reset, rather than freezing at the stale seed (spec 031). Zeroing the
		// ring makes a fresh full window accrue before the next verdict, so this
		// fires once per breach episode and re-arms structurally.
		prior := e.estimate
		adopted := e.medianLocked()
		e.estimate = adopted
		e.wi, e.wn = 0, 0
		return &Adoption{Prior: prior, Adopted: adopted, SpikeRate: rate}
	}
	return nil
}

// medianLocked returns the median of the retained window values (all wn of
// them, spike and non-spike alike) — the adoption value. Robust to a mixed
// window and deterministic, needing no new tuning constant (spec 031 R1;
// mean rejected as spike-sensitive, max as overshooting). Caller holds mu.
func (e *Estimator) medianLocked() float64 {
	vals := make([]float64, e.wn)
	for i := 0; i < e.wn; i++ {
		vals[i] = e.window[i].secPerPoint
	}
	sort.Float64s(vals)
	n := len(vals)
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

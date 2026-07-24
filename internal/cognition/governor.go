package cognition

import (
	"time"

	"github.com/evanstern/promptworld/internal/clock"
)

// Governor doctrine (specs/028-adaptive-throttle, research R6): the hysteresis
// constants that shape adaptive throttling. Like the registry's points and
// budgets, these are doctrine — human-tuned from this feature's own telemetry,
// never runtime knobs (FR-007). Changing one is a reviewed code change, not a
// setting a player or the daemon may turn.
const (
	// GovernorCadence is the wall-clock interval at which the daemon samples
	// debt and asks the controller for a decision.
	GovernorCadence = 1 * time.Second
	// ShedThreshold is the debt (in budget-fractions) above which a breach
	// window begins to accrue.
	ShedThreshold = 1.0
	// BreachWindow is the continuous time above ShedThreshold required to shed
	// one notch of speed.
	BreachWindow = 5 * time.Second
	// RecoverHeadroom scales ShedThreshold to set the recovery ceiling: the
	// projected debt at the candidate notch must be below
	// ShedThreshold × RecoverHeadroom before recovery accrues (FR-006).
	RecoverHeadroom = 0.5
	// RecoveryWindow is the continuous headroom required to recover one notch —
	// deliberately longer than BreachWindow (asymmetric hysteresis, FR-006).
	RecoveryWindow = 20 * time.Second
)

// PendingDebtInput is one model-bound thought's contribution to debt: its call
// kind (resolved to a decision class via the registry) and the seconds of work
// predicted versus already elapsed. Plain values keep this package a leaf — the
// daemon's sampler snapshots the orchestrator's live registry into these before
// calling Debt.
type PendingDebtInput struct {
	Kind         string
	PredictedSec float64 // class points × the serving provider's current sec/pt estimate
	ElapsedSec   float64 // 0 while queued; wall-clock since dispatch while in flight
}

// Debt is the deterministic budget-fraction sum over a pending set (spec 033
// FR-001/FR-002, revising spec 028; contracts/debt-formula.md). Per thought the
// staleness seconds are piecewise: a thought still within its prediction counts
// its remaining work (PredictedSec − ElapsedSec), which drains toward zero as it
// progresses; a thought that has overrun its prediction counts its full accrued
// drift (ElapsedSec), which grows the longer it languishes. At ticksPerSecond
// game ticks per real second those seconds drift seconds × ticksPerSecond ticks
// before the reply lands, and dividing by the class's staleness budget expresses
// that drift as a dimensionless fraction of the budget:
//
//	seconds(job)  = PredictedSec − ElapsedSec   if ElapsedSec < PredictedSec  // remaining work, drains
//	              = ElapsedSec                   if ElapsedSec ≥ PredictedSec  // accrued drift, grows
//	fraction(job) = seconds(job) × ticksPerSecond / BudgetTicks(class)
//	debt          = Σ fraction(job)
//
// An overdue thought's elapsed time IS its grounded debt — the measured minimum
// staleness its reply will land with. Where spec 028 floored the overrun to zero
// ("an overdue thought invents no debt it cannot ground") the debt signal
// inverted under overload: the worse the drowning, the sooner every in-flight
// thought went overdue and vanished from the sum, so maximum drift registered
// minimum debt and the governor never shed (spec 033, world-01). The overrun is
// a measurement, not an invention. At the boundary (ElapsedSec == PredictedSec)
// the contribution deliberately JUMPS from the drained remaining work (~0) to
// the full accrued drift: the overrun proves the prediction wrong, so the honest
// floor switches from "almost done" to "already this stale" — the jump is
// doctrine, not an artifact (contracts/debt-formula.md).
//
// The unit is budget-fractions: 1.0 means one thought's drift equals its whole
// staleness budget. jobs counts only inputs that contribute a positive fraction;
// overdue thoughts now contribute, so they count (correcting the same blindness
// in the visible jobs figure). Pure arithmetic: no wall-clock reads, no
// randomness. Unknown kinds are skipped — they cannot reach a model (FR-002,
// spec 007). ticksPerSecond ≤ 0 (uncapped max speed, which the orchestrator
// refuses upstream) yields debt 0.
func Debt(pending []PendingDebtInput, ticksPerSecond float64) (debt float64, jobs int) {
	if ticksPerSecond <= 0 {
		return 0, 0
	}
	for _, p := range pending {
		dc, ok := ClassForKind(p.Kind)
		if !ok {
			continue // unregistered kinds cannot reach a model (FR-002)
		}
		seconds := p.PredictedSec - p.ElapsedSec // remaining work while within prediction
		if p.ElapsedSec >= p.PredictedSec {
			seconds = p.ElapsedSec // overrun: count the full accrued drift, not zero
		}
		fraction := seconds * ticksPerSecond / float64(dc.BudgetTicks)
		if fraction > 0 {
			debt += fraction
			jobs++
		}
	}
	return debt, jobs
}

// Action is the kind of decision the Governor returns each sample.
type Action string

const (
	// ActionNone means the effective speed holds — the common case.
	ActionNone Action = "none"
	// ActionShed drops the effective speed one capped-ladder notch (US2).
	ActionShed Action = "shed"
	// ActionRecover raises the effective speed one notch toward the ceiling
	// (US3): the debt projected at that notch holds under the recovery headroom
	// through a full RecoveryWindow.
	ActionRecover Action = "recover"
)

// Decision is one governor sample's verdict plus the measured arithmetic the
// caller records on the resulting clock.governor_* event (FR-008). To is the
// target effective speed; when Action is ActionNone it is the current speed.
type Decision struct {
	Action Action
	To     clock.Speed
	Debt   float64
	Jobs   int
}

// breachSamples is how many consecutive over-threshold samples make up one
// BreachWindow at the GovernorCadence (5s / 1s = 5): a shed fires on the sample
// that completes the window, never before.
const breachSamples = int(BreachWindow / GovernorCadence)

// recoverSamples is how many consecutive headroom samples make up one
// RecoveryWindow at the GovernorCadence (20s / 1s = 20): a recover fires on the
// sample that completes the window. Deliberately larger than breachSamples —
// the asymmetric hysteresis (FR-006) that keeps the effective speed from
// flapping between notches (US3-AC4, SC-003).
const recoverSamples = int(RecoveryWindow / GovernorCadence)

// Governor is the pure hysteresis state machine behind adaptive throttling
// (research R6). The daemon owns the wall clock and calls Sample once per
// GovernorCadence; the Governor owns only the decision logic — no goroutines,
// no wall-clock reads — so it is exhaustively unit-testable. It counts samples,
// not durations: a window is a run of consecutive qualifying samples.
type Governor struct {
	breach  int         // consecutive over-threshold samples accrued toward a shed
	recover int         // consecutive headroom samples accrued toward a recover (US3)
	lastEff clock.Speed // effective speed at the previous sample; a change resets the windows
}

// Sample feeds one debt reading to the controller and returns its decision.
// The controller counts SAMPLES, not durations: a window is a run of
// consecutive qualifying samples at the GovernorCadence.
//
// State-machine invariants (both windows are wall-side observer state, never
// persisted — spec 028 Key Entities):
//   - Each sample advances AT MOST one accumulator. An over-threshold sample
//     advances breach and zeroes recover (climbing only multiplies an already
//     breaching debt); a sample whose projection holds advances recover (breach
//     was zeroed by its own else-branch); a sample qualifying for neither zeroes
//     both.
//   - Both windows reset on any decision, any pause, and any change in the
//     effective speed between samples (the player moved the ceiling, or a prior
//     decision took effect). A paused sample resets and returns ActionNone
//     (FR-013) — elapsed pause time never counts toward either window.
//   - A construction-fresh Governor starts with zeroed windows.
//
// Shed (US2): breach accrues while debt sits over ShedThreshold AND there is a
// lower notch to shed to; at the 1x floor the governor saturates — debt over
// threshold yields no decision, visible in status (US2-AC4).
//
// Recover (US3): recovery accrues only while GOVERNED with room to climb — the
// effective speed sits below the requested ceiling on the capped ladder — and
// while the debt PROJECTED at the candidate notch (current debt scaled by the
// notch's tick-rate ratio, FR-006) stays below ShedThreshold × RecoverHeadroom.
// The candidate is one notch up, never above the requested ceiling. A recover
// fires after the full RecoveryWindow; recoverSamples > breachSamples is the
// asymmetric hysteresis that prevents flapping (US3-AC2/AC4, SC-003).
func (g *Governor) Sample(debt float64, jobs int, paused bool, effective, requested clock.Speed) Decision {
	none := Decision{Action: ActionNone, To: effective, Debt: debt, Jobs: jobs}

	// Paused: the clock and the governor with it are frozen. Reset both windows
	// so a resume starts fresh — elapsed pause time never counts (FR-013).
	if paused {
		g.reset(effective)
		return none
	}

	// A speed change since the last sample invalidates the accruing windows: the
	// player moved the ceiling, or our own prior decision has taken effect.
	if effective != g.lastEff {
		g.breach = 0
		g.recover = 0
		g.lastEff = effective
	}

	idx := clock.LadderIndex(effective)

	// --- shed path (US2) ---
	// Breach accrues only while debt sits over the threshold AND there is a lower
	// notch to shed to (effective above the 1x floor). A breaching sample can
	// never also be recovering — climbing multiplies an already over-threshold
	// debt — so it zeroes the recovery window. A single under-threshold sample
	// resets the run (a blip cannot shed by itself — the breach window is the
	// hysteresis).
	breaching := debt > ShedThreshold && idx > 0
	if breaching {
		g.breach++
		g.recover = 0
		if g.breach >= breachSamples {
			to := clock.CappedLadder()[idx-1]
			g.reset(to)
			return Decision{Action: ActionShed, To: to, Debt: debt, Jobs: jobs}
		}
	} else {
		g.breach = 0
	}

	// --- recover path (US3) ---
	// Recovery accrues only while governed with room to climb: the effective
	// speed sits below the requested ceiling on the capped ladder. The candidate
	// is the next notch up (never above requested). Accrue while the debt
	// projected at that notch — current debt scaled by candidateTPS/currentTPS
	// (FR-006) — stays under ShedThreshold × RecoverHeadroom; a failing
	// projection, or no room to climb, resets the window. A breaching sample has
	// already zeroed g.recover above and is skipped here.
	if !breaching {
		reqIdx := clock.LadderIndex(requested)
		if idx >= 0 && reqIdx > idx {
			candidate := clock.CappedLadder()[idx+1]
			projected := debt * candidate.TicksPerSecond() / effective.TicksPerSecond()
			if projected < ShedThreshold*RecoverHeadroom {
				g.recover++
				if g.recover >= recoverSamples {
					g.reset(candidate)
					return Decision{Action: ActionRecover, To: candidate, Debt: debt, Jobs: jobs}
				}
			} else {
				g.recover = 0
			}
		} else {
			// At the ceiling / ungoverned, or effective off the capped ladder:
			// recovery cannot accrue.
			g.recover = 0
		}
	}

	return none
}

// reset zeroes both windows and re-anchors the effective-speed watch to eff, so
// the next sample after a decision, pause, or speed change starts a fresh run.
func (g *Governor) reset(eff clock.Speed) {
	g.breach = 0
	g.recover = 0
	g.lastEff = eff
}

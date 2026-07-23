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

// Debt is the deterministic budget-fraction sum over a pending set (FR-001,
// FR-002, research R5). Per thought the remaining work is
// max(0, PredictedSec − ElapsedSec) seconds, which at ticksPerSecond game ticks
// per real second is expected to drift remaining × ticksPerSecond ticks before
// the reply lands; dividing by the class's staleness budget expresses that
// drift as a dimensionless fraction of the budget:
//
//	fraction(job) = max(0, PredictedSec − ElapsedSec) × ticksPerSecond / BudgetTicks(class)
//	debt          = Σ fraction(job)
//
// The unit is budget-fractions: 1.0 means one thought is predicted to consume
// exactly its whole staleness budget. jobs counts only inputs that contribute a
// positive fraction (overdue thoughts, floored to zero, do not count). Pure
// arithmetic: no wall-clock reads, no randomness. Unknown kinds are skipped —
// they cannot reach a model (FR-002, spec 007). ticksPerSecond ≤ 0 (uncapped
// max speed, which the orchestrator refuses upstream) yields debt 0.
func Debt(pending []PendingDebtInput, ticksPerSecond float64) (debt float64, jobs int) {
	if ticksPerSecond <= 0 {
		return 0, 0
	}
	for _, p := range pending {
		dc, ok := ClassForKind(p.Kind)
		if !ok {
			continue // unregistered kinds cannot reach a model (FR-002)
		}
		remaining := p.PredictedSec - p.ElapsedSec
		if remaining <= 0 {
			continue // an overdue thought invents no debt it cannot ground
		}
		fraction := remaining * ticksPerSecond / float64(dc.BudgetTicks)
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
	// (US3, T012 — not yet emitted by this slice).
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

// Governor is the pure hysteresis state machine behind adaptive throttling
// (research R6). The daemon owns the wall clock and calls Sample once per
// GovernorCadence; the Governor owns only the decision logic — no goroutines,
// no wall-clock reads — so it is exhaustively unit-testable. It counts samples,
// not durations: a window is a run of consecutive qualifying samples.
type Governor struct {
	breach  int         // consecutive over-threshold samples accrued toward a shed
	recover int         // consecutive headroom samples accrued toward a recover (US3, T012)
	lastEff clock.Speed // effective speed at the previous sample; a change resets the windows
}

// Sample feeds one debt reading to the controller and returns its decision.
// Windows reset — accruals zeroed — on any decision, any pause, and any change
// in the effective speed between samples (the player moved the ceiling, or a
// prior decision took effect); a paused sample resets and returns ActionNone
// (FR-013). A construction-fresh Governor starts with zeroed windows.
//
// This slice implements ONLY the shed side (US2). Recovery (US3) lands in T012:
// the recover window and its projection/headroom test are stubbed here so the
// state machine's shape is complete, but no ActionRecover is ever returned yet.
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
	// notch to shed to (effective above the 1x floor). At the floor the governor
	// saturates: debt over threshold yields no decision, visible in status
	// (US2-AC4). A single under-threshold sample resets the run (a blip cannot
	// shed by itself — the breach window is the hysteresis).
	if debt > ShedThreshold && idx > 0 {
		g.breach++
		if g.breach >= breachSamples {
			to := clock.CappedLadder()[idx-1]
			g.reset(to)
			return Decision{Action: ActionShed, To: to, Debt: debt, Jobs: jobs}
		}
	} else {
		g.breach = 0
	}

	// --- recover path (US3) — T012 scope ---
	// The recovery window (g.recover) accrues while governed and the debt
	// PROJECTED at the next notch up would sit below ShedThreshold × RecoverHeadroom
	// (FR-006), firing an ActionRecover after RecoveryWindow. That logic — and the
	// only reads of g.recover and requested — land in T012; this slice never
	// accrues or returns a recover.
	_ = requested

	return Decision{Action: ActionNone, To: effective, Debt: debt, Jobs: jobs}
}

// reset zeroes both windows and re-anchors the effective-speed watch to eff, so
// the next sample after a decision, pause, or speed change starts a fresh run.
func (g *Governor) reset(eff clock.Speed) {
	g.breach = 0
	g.recover = 0
	g.lastEff = eff
}

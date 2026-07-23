package cognition

import "time"

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

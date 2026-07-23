package cognition

import (
	"math"
	"testing"
	"time"
)

// TestGovernorDoctrineConstants pins the doctrine values (research R6) — a
// drift here is a reviewed doctrine change (FR-007), not a tuning tweak.
func TestGovernorDoctrineConstants(t *testing.T) {
	if GovernorCadence != time.Second {
		t.Errorf("GovernorCadence = %v, want 1s", GovernorCadence)
	}
	if ShedThreshold != 1.0 {
		t.Errorf("ShedThreshold = %v, want 1.0", ShedThreshold)
	}
	if BreachWindow != 5*time.Second {
		t.Errorf("BreachWindow = %v, want 5s", BreachWindow)
	}
	if RecoverHeadroom != 0.5 {
		t.Errorf("RecoverHeadroom = %v, want 0.5", RecoverHeadroom)
	}
	if RecoveryWindow != 20*time.Second {
		t.Errorf("RecoveryWindow = %v, want 20s", RecoveryWindow)
	}
}

func TestDebtTable(t *testing.T) {
	// Budgets read from the real registry so expected fractions are grounded
	// in doctrine, not magic literals.
	planner, _ := ClassFor("planner")           // 1200 ticks
	conversation, _ := ClassFor("conversation") // 7200 ticks
	plannerBudget := float64(planner.BudgetTicks)
	convBudget := float64(conversation.BudgetTicks)

	cases := []struct {
		name     string
		pending  []PendingDebtInput
		tps      float64
		wantDebt float64
		wantJobs int
	}{
		{
			name:     "empty pending",
			pending:  nil,
			tps:      4,
			wantDebt: 0,
			wantJobs: 0,
		},
		{
			name:     "single queued planner",
			pending:  []PendingDebtInput{{Kind: "planner", PredictedSec: 100, ElapsedSec: 0}},
			tps:      4,
			wantDebt: 100 * 4 / plannerBudget,
			wantJobs: 1,
		},
		{
			name:     "in-flight planner nets remaining",
			pending:  []PendingDebtInput{{Kind: "planner", PredictedSec: 100, ElapsedSec: 40}},
			tps:      4,
			wantDebt: 60 * 4 / plannerBudget,
			wantJobs: 1,
		},
		{
			name:     "overdue thought floored to zero",
			pending:  []PendingDebtInput{{Kind: "planner", PredictedSec: 10, ElapsedSec: 15}},
			tps:      4,
			wantDebt: 0,
			wantJobs: 0,
		},
		{
			name: "mixed classes sum from real budgets",
			pending: []PendingDebtInput{
				{Kind: "planner", PredictedSec: 100, ElapsedSec: 40},     // remaining 60
				{Kind: "conversation", PredictedSec: 200, ElapsedSec: 0}, // remaining 200
			},
			tps:      4,
			wantDebt: 60*4/plannerBudget + 200*4/convBudget,
			wantJobs: 2,
		},
		{
			name: "overdue neighbor does not count but valid does",
			pending: []PendingDebtInput{
				{Kind: "planner", PredictedSec: 5, ElapsedSec: 9},        // overdue, skipped
				{Kind: "conversation", PredictedSec: 200, ElapsedSec: 0}, // remaining 200
			},
			tps:      4,
			wantDebt: 200 * 4 / convBudget,
			wantJobs: 1,
		},
		{
			name: "unknown kind skipped",
			pending: []PendingDebtInput{
				{Kind: "oracle", PredictedSec: 100, ElapsedSec: 0}, // unregistered
				{Kind: "planner", PredictedSec: 100, ElapsedSec: 0},
			},
			tps:      4,
			wantDebt: 100 * 4 / plannerBudget,
			wantJobs: 1,
		},
		{
			name:     "ticksPerSecond zero yields zero",
			pending:  []PendingDebtInput{{Kind: "planner", PredictedSec: 100, ElapsedSec: 0}},
			tps:      0,
			wantDebt: 0,
			wantJobs: 0,
		},
		{
			name:     "negative ticksPerSecond yields zero",
			pending:  []PendingDebtInput{{Kind: "planner", PredictedSec: 100, ElapsedSec: 0}},
			tps:      -4,
			wantDebt: 0,
			wantJobs: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			debt, jobs := Debt(c.pending, c.tps)
			if math.Abs(debt-c.wantDebt) > 1e-9 {
				t.Errorf("Debt = %g, want %g", debt, c.wantDebt)
			}
			if jobs != c.wantJobs {
				t.Errorf("jobs = %d, want %d", jobs, c.wantJobs)
			}
		})
	}
}

// TestDebtIsDeterministic: the same inputs twice yield identical output — pure
// arithmetic, no wall-clock reads, no randomness (FR-002).
func TestDebtIsDeterministic(t *testing.T) {
	pending := []PendingDebtInput{
		{Kind: "planner", PredictedSec: 100, ElapsedSec: 40},
		{Kind: "conversation", PredictedSec: 200, ElapsedSec: 10},
		{Kind: "metatron", PredictedSec: 30, ElapsedSec: 0},
	}
	debt0, jobs0 := Debt(pending, 8)
	for i := 0; i < 100; i++ {
		debt, jobs := Debt(pending, 8)
		if debt != debt0 || jobs != jobs0 {
			t.Fatalf("output varied: (%g,%d) vs (%g,%d)", debt, jobs, debt0, jobs0)
		}
	}
}

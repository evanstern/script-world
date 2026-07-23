package cognition

import (
	"math"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
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

// --- Governor state machine: shed path (spec 028 US2, T009) ---
//
// The controller counts SAMPLES, not durations; breachSamples (= BreachWindow /
// GovernorCadence = 5) is one breach window. These tests drive Sample directly
// with scripted debt readings — the daemon owns the wall clock (T010).

const overThreshold = ShedThreshold + 0.5 // a comfortably-breaching debt reading

// TestGovernorShedFiresAtWindowBoundary (US2-AC1): a shed fires on the sample
// that completes the breach window — the fifth consecutive over-threshold
// sample — and not one sample earlier, carrying the measured arithmetic.
func TestGovernorShedFiresAtWindowBoundary(t *testing.T) {
	g := &Governor{}
	for i := 1; i < breachSamples; i++ {
		if d := g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("sample %d/%d shed early: %+v", i, breachSamples, d)
		}
	}
	d := g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x)
	if d.Action != ActionShed {
		t.Fatalf("sample %d did not shed: %+v", breachSamples, d)
	}
	if d.To != clock.Speed16x {
		t.Errorf("shed To = %q, want 16x (one notch down)", d.To)
	}
	if d.Debt != overThreshold || d.Jobs != 3 {
		t.Errorf("shed arithmetic = {Debt:%v Jobs:%d}, want {%v 3}", d.Debt, d.Jobs, overThreshold)
	}
}

// TestGovernorBlipNeverSheds: a breach interrupted by even one under-threshold
// sample never completes a window, so a transient spike cannot shed by itself.
func TestGovernorBlipNeverSheds(t *testing.T) {
	g := &Governor{}
	for cycle := 0; cycle < 4; cycle++ {
		for i := 0; i < breachSamples-1; i++ { // one short of a full window
			if d := g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x); d.Action != ActionNone {
				t.Fatalf("cycle %d sample %d shed during a blip: %+v", cycle, i, d)
			}
		}
		// A single under-threshold sample resets the run.
		if d := g.Sample(0.2, 0, false, clock.Speed32x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("cycle %d: shed on an under-threshold sample: %+v", cycle, d)
		}
	}
}

// TestGovernorMultiNotchDescent (US2-AC2): sustained breach sheds one notch per
// window down the capped ladder, stopping exactly at the 1x floor.
func TestGovernorMultiNotchDescent(t *testing.T) {
	g := &Governor{}
	eff := clock.Speed32x
	var sheds []clock.Speed
	for i := 0; i < 6*breachSamples; i++ {
		d := g.Sample(overThreshold, 2, false, eff, clock.Speed32x)
		if d.Action == ActionShed {
			sheds = append(sheds, d.To)
			eff = d.To // the world applies the shed; the next sample sees the new speed
		}
	}
	want := []clock.Speed{clock.Speed16x, clock.Speed8x, clock.Speed4x, clock.Speed1x}
	if len(sheds) != len(want) {
		t.Fatalf("sheds = %v, want %v", sheds, want)
	}
	for i := range want {
		if sheds[i] != want[i] {
			t.Errorf("shed %d = %q, want %q", i, sheds[i], want[i])
		}
	}
}

// TestGovernorFloorSaturates (US2-AC4): at the 1x floor, debt over threshold
// yields no decision — the governor saturates rather than pausing the world.
func TestGovernorFloorSaturates(t *testing.T) {
	g := &Governor{}
	for i := 0; i < 4*breachSamples; i++ {
		if d := g.Sample(overThreshold, 5, false, clock.Speed1x, clock.Speed1x); d.Action != ActionNone {
			t.Fatalf("sample %d shed below the 1x floor: %+v", i, d)
		}
	}
}

// TestGovernorPausedResetsWindow (FR-013): a paused sample returns no decision
// AND clears the breach window, so a resume starts a fresh window — a pause
// never converts accrued breach into an instant shed.
func TestGovernorPausedResetsWindow(t *testing.T) {
	g := &Governor{}
	for i := 0; i < breachSamples-2; i++ { // partway into a window
		g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x)
	}
	if d := g.Sample(overThreshold, 3, true /*paused*/, clock.Speed32x, clock.Speed32x); d.Action != ActionNone {
		t.Fatalf("paused sample returned a decision: %+v", d)
	}
	// A full fresh window is now required: breachSamples-1 samples stay quiet.
	for i := 1; i < breachSamples; i++ {
		if d := g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("post-pause sample %d shed before a fresh window completed: %+v", i, d)
		}
	}
	if d := g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x); d.Action != ActionShed {
		t.Fatalf("post-pause window did not shed at the boundary: %+v", d)
	}
}

// TestGovernorEffectiveChangeResets: a change in the effective speed between
// samples (the player moved the ceiling) resets the accruing breach window.
func TestGovernorEffectiveChangeResets(t *testing.T) {
	g := &Governor{}
	for i := 0; i < breachSamples-2; i++ { // accrue partway at 32x
		g.Sample(overThreshold, 3, false, clock.Speed32x, clock.Speed32x)
	}
	// Player drops to 16x: the change resets, then this sample accrues one.
	if d := g.Sample(overThreshold, 3, false, clock.Speed16x, clock.Speed16x); d.Action != ActionNone {
		t.Fatalf("shed on the speed-change sample: %+v", d)
	}
	// A full fresh window at 16x is required: three more quiet, then a shed to 8x.
	for i := 2; i < breachSamples; i++ {
		if d := g.Sample(overThreshold, 3, false, clock.Speed16x, clock.Speed16x); d.Action != ActionNone {
			t.Fatalf("sample %d shed before a fresh post-change window completed: %+v", i, d)
		}
	}
	d := g.Sample(overThreshold, 3, false, clock.Speed16x, clock.Speed16x)
	if d.Action != ActionShed || d.To != clock.Speed8x {
		t.Fatalf("post-change window did not shed 16x->8x at the boundary: %+v", d)
	}
}

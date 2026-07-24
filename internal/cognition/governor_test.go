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
			name:     "overdue thought counts its accrued drift",
			pending:  []PendingDebtInput{{Kind: "planner", PredictedSec: 10, ElapsedSec: 15}},
			tps:      4,
			wantDebt: 15 * 4 / plannerBudget, // spec 033: full elapsed, not floored to zero
			wantJobs: 1,
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
			name: "overdue neighbor counts its accrued drift alongside a queued thought",
			pending: []PendingDebtInput{
				{Kind: "planner", PredictedSec: 5, ElapsedSec: 9},        // overdue → accrued drift 9
				{Kind: "conversation", PredictedSec: 200, ElapsedSec: 0}, // remaining 200
			},
			tps:      4,
			wantDebt: 9*4/plannerBudget + 200*4/convBudget, // spec 033: the overdue thought now contributes
			wantJobs: 2,
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

// TestGovernorWorld01ZeroShedRegression (spec 033 FR-005, SC-001) pins the
// world-01 zero-shed defect red-first: the saturation shape that produced ZERO
// governor sheds while 17/31 planner thoughts landed rejected-stale. Eight
// planner thoughts predicted 1.573 s but 30 s into flight, sampled at 8 ticks/s,
// are the worked example in contracts/debt-formula.md. Under the old floored
// arithmetic each overdue thought contributes zero, so debt is 0.0, jobs 0, and
// the governor never sheds — the throttle sits blind exactly while the system
// drowns. Under accrued-drift debt each contributes 30 × 8 / 1200 = 0.2, so debt
// is 1.6 over the 1.0 shed threshold and a shed fires when the breach window
// completes. This test fails on the current arithmetic; T003 turns it green.
func TestGovernorWorld01ZeroShedRegression(t *testing.T) {
	const (
		predicted = 1.573 // frozen-optimistic prediction (spec 031 estimator freeze)
		elapsed   = 30.0  // long in flight — well past the prediction
		tps       = 8.0   // 8 ticks/sec
	)
	pending := make([]PendingDebtInput, 8)
	for i := range pending {
		pending[i] = PendingDebtInput{Kind: "planner", PredictedSec: predicted, ElapsedSec: elapsed}
	}

	debt, jobs := Debt(pending, tps)
	// planner budget 1200 ticks: each overdue thought counts 30 × 8 / 1200 = 0.2.
	if wantDebt := 8 * (elapsed * tps / 1200); math.Abs(debt-wantDebt) > 1e-9 {
		t.Errorf("world-01 debt = %g, want %g — overdue thoughts must count their accrued drift, not zero", debt, wantDebt)
	}
	if jobs != 8 {
		t.Errorf("world-01 jobs = %d, want 8 — every overdue thought contributes", jobs)
	}

	// The same debt sampled every cadence drives a shed once the breach window
	// completes: effective and requested both 8x, unpaused, so there is a lower
	// notch (4x) to shed to. A shed fires on the fifth consecutive over-threshold
	// sample (breachSamples = 5) and never before.
	g := &Governor{}
	for i := 1; i < breachSamples; i++ {
		if d := g.Sample(debt, jobs, false, clock.Speed8x, clock.Speed8x); d.Action != ActionNone {
			t.Fatalf("world-01 sample %d/%d shed early: %+v", i, breachSamples, d)
		}
	}
	d := g.Sample(debt, jobs, false, clock.Speed8x, clock.Speed8x)
	if d.Action != ActionShed {
		t.Fatalf("world-01 sample %d did not shed: %+v — the throttle stayed blind while the system drowned", breachSamples, d)
	}
	if d.To != clock.Speed4x {
		t.Errorf("world-01 shed To = %q, want 4x (one notch below 8x)", d.To)
	}
}

// --- Governor state machine: shed path (spec 028 US2, T009) ---
//
// The controller counts SAMPLES, not durations; breachSamples (= BreachWindow /
// GovernorCadence = 5) is one breach window. These tests drive Sample directly
// with scripted debt readings — the daemon owns the wall clock (T010).

const overThreshold = ShedThreshold + 0.5 // a comfortably-breaching debt reading

// recoverDebt is low enough that debt projected one capped notch up (ratio ≤ 4)
// stays under ShedThreshold × RecoverHeadroom, so recovery accrues. marginalDebt
// sits under ShedThreshold (no shed) yet projects over the headroom one notch up
// (0.4 × 2 = 0.8 > 0.5) — the steady marginal load that must park without
// oscillating (US3-AC2, SC-003).
const (
	recoverDebt  = 0.1
	marginalDebt = 0.4
)

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

// --- Governor state machine: recover path (spec 028 US3, T012) ---
//
// Recovery accrues only while governed with room to climb and while the debt
// PROJECTED at the candidate notch (debt × candidateTPS/currentTPS) stays under
// ShedThreshold × RecoverHeadroom; a recover fires after recoverSamples (= 20)
// consecutive holding samples — deliberately longer than a breach window.

// TestGovernorRecoverFiresAtWindowBoundary (US3-AC1): a recover fires on the
// sample that completes the recovery window — the twentieth consecutive holding
// sample — and not one sample earlier, carrying the measured arithmetic and
// climbing exactly one notch.
func TestGovernorRecoverFiresAtWindowBoundary(t *testing.T) {
	g := &Governor{}
	for i := 1; i < recoverSamples; i++ {
		if d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("sample %d/%d recovered early: %+v", i, recoverSamples, d)
		}
	}
	d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x)
	if d.Action != ActionRecover {
		t.Fatalf("sample %d did not recover: %+v", recoverSamples, d)
	}
	if d.To != clock.Speed16x {
		t.Errorf("recover To = %q, want 16x (one notch up)", d.To)
	}
	if d.Debt != recoverDebt || d.Jobs != 1 {
		t.Errorf("recover arithmetic = {Debt:%v Jobs:%d}, want {%v 1}", d.Debt, d.Jobs, recoverDebt)
	}
}

// TestGovernorRecoverClimbsNotchByNotch (US3-AC1): sustained headroom climbs one
// notch per full recovery window up the capped ladder toward the requested
// ceiling — two recoveries need two full windows — and stops exactly at 32x.
func TestGovernorRecoverClimbsNotchByNotch(t *testing.T) {
	g := &Governor{}
	eff := clock.Speed8x
	const req = clock.Speed32x
	var recoveries []clock.Speed
	for i := 0; i < 3*recoverSamples; i++ {
		d := g.Sample(recoverDebt, 1, false, eff, req)
		if d.Action == ActionRecover {
			recoveries = append(recoveries, d.To)
			eff = d.To // the world applies the recover; the next sample sees the new speed
		}
	}
	want := []clock.Speed{clock.Speed16x, clock.Speed32x}
	if len(recoveries) != len(want) {
		t.Fatalf("recoveries = %v, want %v", recoveries, want)
	}
	for i := range want {
		if recoveries[i] != want[i] {
			t.Errorf("recover %d = %q, want %q", i, recoveries[i], want[i])
		}
	}
}

// TestGovernorMarginalLoadParks (US3-AC2, SC-003): a steady marginal load whose
// debt is under threshold at the current notch but whose projection breaches the
// headroom one notch up parks the effective speed forever — no shed, no recover,
// no oscillation across many recovery windows.
func TestGovernorMarginalLoadParks(t *testing.T) {
	g := &Governor{}
	for i := 0; i < 10*recoverSamples; i++ {
		if d := g.Sample(marginalDebt, 2, false, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("sample %d flapped off the parked notch: %+v", i, d)
		}
	}
}

// TestGovernorQuiescentAtCeiling (US3-AC3): recovered to the requested ceiling,
// effective equals requested and debt stays low — the governor is quiescent, no
// room to climb and nothing to shed, so every sample returns ActionNone.
func TestGovernorQuiescentAtCeiling(t *testing.T) {
	g := &Governor{}
	for i := 0; i < 3*recoverSamples; i++ {
		if d := g.Sample(0.05, 0, false, clock.Speed32x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("sample %d acted at the ceiling: %+v", i, d)
		}
	}
}

// TestGovernorWindowAsymmetry (US3-AC4): the recovery window is observably longer
// than the breach window — recoverSamples exceeds breachSamples, and a full
// breach window's worth of headroom is not yet enough to recover.
func TestGovernorWindowAsymmetry(t *testing.T) {
	if recoverSamples <= breachSamples {
		t.Fatalf("recoverSamples (%d) must exceed breachSamples (%d) — asymmetric hysteresis (FR-006)", recoverSamples, breachSamples)
	}
	g := &Governor{}
	for i := 0; i < breachSamples; i++ {
		if d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("recovered within a breach window (%d samples): %+v", breachSamples, d)
		}
	}
}

// TestGovernorRecoverInterruptedRestarts: a recovery accrual interrupted by even
// one projection-failing sample restarts the window — a full fresh recovery
// window is then required before a recover can fire.
func TestGovernorRecoverInterruptedRestarts(t *testing.T) {
	g := &Governor{}
	for i := 0; i < recoverSamples-1; i++ { // one short of a full window
		g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x)
	}
	// One projection-failing sample (still under threshold, so no shed) resets it.
	if d := g.Sample(marginalDebt, 2, false, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
		t.Fatalf("interrupting sample produced a decision: %+v", d)
	}
	// A full fresh window is now required: recoverSamples-1 stay quiet, then recover.
	for i := 1; i < recoverSamples; i++ {
		if d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("post-interrupt sample %d recovered before a fresh window completed: %+v", i, d)
		}
	}
	if d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x); d.Action != ActionRecover {
		t.Fatalf("post-interrupt window did not recover at the boundary: %+v", d)
	}
}

// TestGovernorNeverTargetsMax (spec 028 US4-AC5, FR-004/FR-012): the governor
// governs only the capped ladder — every Decision it ever returns, for any
// combination of debt, effective, and requested speed and across any run of
// samples, has a To that sits ON the capped ladder and is NEVER the uncapped
// SpeedMax. The property makes the CappedLadder invariant load-bearing: shedding
// or recovering can never make max speed meaningful, so the pre-028 max refusal
// (enforced at the set_speed door) stays the whole story.
func TestGovernorNeverTargetsMax(t *testing.T) {
	ladder := clock.CappedLadder()
	// A spread of readings that provokes both shed (over-threshold) and recover
	// (deep headroom) accrual, plus a saturating floor case and a quiescent one.
	debts := []float64{0.0, 0.05, marginalDebt, ShedThreshold, overThreshold, 100.0}

	onLadder := func(sp clock.Speed) bool { return clock.LadderIndex(sp) >= 0 }

	for _, eff := range ladder {
		for _, req := range ladder {
			for _, d := range debts {
				g := &Governor{}
				// Enough samples to complete any window and fire many decisions,
				// re-anchoring effective to the decided speed so the walk descends
				// and climbs the ladder exactly as production would.
				cur := eff
				for i := 0; i < 3*recoverSamples; i++ {
					dec := g.Sample(d, 1, false, cur, req)
					if dec.To == clock.SpeedMax {
						t.Fatalf("governor targeted uncapped max: eff=%q req=%q debt=%v decision=%+v", eff, req, d, dec)
					}
					if !onLadder(dec.To) {
						t.Fatalf("governor targeted off-ladder speed: eff=%q req=%q debt=%v decision=%+v", eff, req, d, dec)
					}
					if dec.Action != ActionNone {
						cur = dec.To // the world applies the decision
					}
				}
			}
		}
	}

	// A construction-fresh governor sampled from a max-speed world (the uncapped
	// state the refusal keeps LLM worlds out of) never manufactures a decision:
	// max is off the capped ladder, so LadderIndex is -1 and neither path accrues.
	g := &Governor{}
	for i := 0; i < 3*breachSamples; i++ {
		if dec := g.Sample(overThreshold, 3, false, clock.SpeedMax, clock.SpeedMax); dec.Action != ActionNone {
			t.Fatalf("governor acted from an uncapped-max world: %+v", dec)
		}
	}
}

// TestGovernorPausedResetsRecovery (FR-013): a paused sample clears the recovery
// window just as it clears the breach window, so a resume starts a fresh
// recovery window — a pause never converts accrued headroom into an instant
// recover.
func TestGovernorPausedResetsRecovery(t *testing.T) {
	g := &Governor{}
	for i := 0; i < recoverSamples-2; i++ { // partway into a window
		g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x)
	}
	if d := g.Sample(recoverDebt, 1, true /*paused*/, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
		t.Fatalf("paused sample returned a decision: %+v", d)
	}
	// A full fresh window is now required.
	for i := 1; i < recoverSamples; i++ {
		if d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x); d.Action != ActionNone {
			t.Fatalf("post-pause sample %d recovered before a fresh window completed: %+v", i, d)
		}
	}
	if d := g.Sample(recoverDebt, 1, false, clock.Speed8x, clock.Speed32x); d.Action != ActionRecover {
		t.Fatalf("post-pause window did not recover at the boundary: %+v", d)
	}
}

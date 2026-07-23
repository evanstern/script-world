package daemon

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
)

// fakePending is a scripted pending-thought inventory: the sampler reads
// whatever the test last set, standing in for Orchestrator.PendingCognition.
type fakePending struct {
	mu   sync.Mutex
	jobs []llm.PendingThought
}

func (f *fakePending) set(jobs []llm.PendingThought) {
	f.mu.Lock()
	f.jobs = jobs
	f.mu.Unlock()
}

func (f *fakePending) PendingCognition() []llm.PendingThought {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]llm.PendingThought, len(f.jobs))
	copy(out, f.jobs)
	return out
}

// fakeStatus is a scripted status+govern door standing in for the loop's
// non-blocking command channel. It reports a mutable effective speed, requested
// ceiling, and paused flag, and records every Govern call; a Govern applies the
// target speed to the reported status the way the real loop would, so the next
// sample sees the governed speed. stopped models a stopped loop (Do error).
type fakeStatus struct {
	mu        sync.Mutex
	speed     clock.Speed
	requested clock.Speed
	paused    bool
	stopped   bool
	governs   []governCall
}

type governCall struct {
	to   clock.Speed
	debt float64
	jobs int
}

func (f *fakeStatus) Do(name string, speed clock.Speed) (sim.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return sim.Status{}, context.Canceled
	}
	return sim.Status{Speed: f.speed, RequestedSpeed: f.requested, Paused: f.paused}, nil
}

func (f *fakeStatus) Govern(to clock.Speed, debt float64, jobs int) (sim.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.governs = append(f.governs, governCall{to: to, debt: debt, jobs: jobs})
	// Model the loop applying the shed: the ceiling is recorded on the first
	// shed, and the effective speed becomes the governed notch.
	if f.requested == "" {
		f.requested = f.speed
	}
	f.speed = to
	return sim.Status{Speed: f.speed, RequestedSpeed: f.requested, Paused: f.paused}, nil
}

func (f *fakeStatus) governCalls() []governCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]governCall(nil), f.governs...)
}

func (f *fakeStatus) setPaused(p bool) {
	f.mu.Lock()
	f.paused = p
	f.mu.Unlock()
}

// TestGovernorSamplerDebt (US1-AC1): the sampler folds pending thoughts and the
// effective speed into aggregate debt matching the hand-computed budget-fraction
// sum, counting only thoughts with positive remaining drift.
func TestGovernorSamplerDebt(t *testing.T) {
	pending := &fakePending{}
	// Effective 16x → 16 ticks/s. Budgets (internal/cognition registry):
	// planner 1200 ticks, conversation 7200 ticks.
	pending.set([]llm.PendingThought{
		// in flight: remaining 20s → 20*16/1200 = 0.26666…
		{Kind: "planner", PredictedSec: 30, ElapsedSec: 10},
		// queued: remaining 100s → 100*16/7200 = 0.22222…
		{Kind: "conversation", PredictedSec: 100, ElapsedSec: 0},
		// overdue: remaining < 0 → floored to zero, does not contribute or count.
		{Kind: "planner", PredictedSec: 5, ElapsedSec: 10},
	})
	s := newGovernorSampler(pending, &fakeStatus{speed: clock.Speed16x})

	s.sample()
	got := s.Snapshot()

	wantDebt := 20.0*16/1200 + 100.0*16/7200 // = 0.488888…
	if math.Abs(got.Debt-wantDebt) > 1e-9 {
		t.Errorf("debt = %v, want %v", got.Debt, wantDebt)
	}
	if got.Jobs != 2 {
		t.Errorf("jobs = %d, want 2 (the overdue thought is not counted)", got.Jobs)
	}
}

// TestGovernorSamplerDrainsToZero (US1-AC2): once the pending set drains, the
// next sample reads exactly zero debt and zero jobs — no residue from the prior
// reading.
func TestGovernorSamplerDrainsToZero(t *testing.T) {
	pending := &fakePending{}
	pending.set([]llm.PendingThought{
		{Kind: "planner", PredictedSec: 30, ElapsedSec: 0},
	})
	s := newGovernorSampler(pending, &fakeStatus{speed: clock.Speed16x})

	s.sample()
	if snap := s.Snapshot(); snap.Debt == 0 || snap.Jobs == 0 {
		t.Fatalf("precondition: expected non-zero debt with a pending thought, got %+v", snap)
	}

	pending.set(nil) // the world quiesces
	s.sample()
	if snap := s.Snapshot(); snap.Debt != 0 || snap.Jobs != 0 {
		t.Errorf("drained snapshot = %+v, want exactly zero debt and jobs", snap)
	}
}

// TestGovernorSamplerUnknownKindSkipped: a kind that resolves to no decision
// class cannot reach a model, so it never contributes debt (FR-002).
func TestGovernorSamplerUnknownKindSkipped(t *testing.T) {
	pending := &fakePending{}
	pending.set([]llm.PendingThought{
		{Kind: "not_a_real_kind", PredictedSec: 100, ElapsedSec: 0},
	})
	s := newGovernorSampler(pending, &fakeStatus{speed: clock.Speed16x})
	s.sample()
	if snap := s.Snapshot(); snap.Debt != 0 || snap.Jobs != 0 {
		t.Errorf("unknown kind contributed to debt: %+v", snap)
	}
}

// TestGovernorSamplerStoppedLoop: a Do error (stopped loop) leaves the last
// snapshot untouched rather than clobbering it — the ctx.Done branch unwinds run.
func TestGovernorSamplerStoppedLoop(t *testing.T) {
	pending := &fakePending{}
	pending.set([]llm.PendingThought{{Kind: "planner", PredictedSec: 30, ElapsedSec: 0}})
	status := &fakeStatus{speed: clock.Speed16x}
	s := newGovernorSampler(pending, status)

	s.sample()
	before := s.Snapshot()

	status.stopped = true
	s.sample()
	if after := s.Snapshot(); after != before {
		t.Errorf("stopped-loop sample changed the snapshot: before %+v after %+v", before, after)
	}
}

// TestGovernorSamplerRunLifecycle exercises run()'s clean shutdown on ctx cancel
// and the snapshot mutex under -race: concurrent samples and reads race the
// running goroutine, then cancel must return it promptly.
func TestGovernorSamplerRunLifecycle(t *testing.T) {
	pending := &fakePending{}
	pending.set([]llm.PendingThought{{Kind: "planner", PredictedSec: 30, ElapsedSec: 0}})
	s := newGovernorSampler(pending, &fakeStatus{speed: clock.Speed16x})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.run(ctx); close(done) }()

	// Hammer the mutex concurrently with the running goroutine.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.sample()
				_ = s.Snapshot()
			}
		}()
	}
	wg.Wait()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after ctx cancel")
	}

	// The doctrine cadence is the wall-clock sampling interval — pin it so a
	// change to the constant is a reviewed decision, not an accident.
	if cognition.GovernorCadence != time.Second {
		t.Errorf("GovernorCadence = %v, want 1s", cognition.GovernorCadence)
	}
}

// TestGovernorSamplerShedsAfterBreachWindow (spec 028 US2-AC1, T010): a
// sustained breach drives exactly one Govern(shed) call after a full breach
// window of samples — the sampler→controller→loop path end to end — carrying the
// measured debt and job count, and never firing before the window completes.
func TestGovernorSamplerShedsAfterBreachWindow(t *testing.T) {
	breachSamples := int(cognition.BreachWindow / cognition.GovernorCadence)
	pending := &fakePending{}
	// One planner at 32x, remaining 80s → 80*32/1200 ≈ 2.13 > ShedThreshold.
	pending.set([]llm.PendingThought{{Kind: "planner", PredictedSec: 80, ElapsedSec: 0}})
	status := &fakeStatus{speed: clock.Speed32x}
	s := newGovernorSampler(pending, status)

	for i := 1; i < breachSamples; i++ {
		s.sample()
		if calls := status.governCalls(); len(calls) != 0 {
			t.Fatalf("shed after only %d samples, before the breach window closed: %+v", i, calls)
		}
	}
	s.sample() // the sample that completes the breach window

	calls := status.governCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly one Govern(shed) after the breach window, got %d: %+v", len(calls), calls)
	}
	if calls[0].to != clock.Speed16x {
		t.Errorf("Govern to = %q, want 16x", calls[0].to)
	}
	if calls[0].jobs != 1 {
		t.Errorf("Govern jobs = %d, want 1", calls[0].jobs)
	}
	if wantDebt := 80.0 * 32 / 1200; math.Abs(calls[0].debt-wantDebt) > 1e-9 {
		t.Errorf("Govern debt = %v, want %v", calls[0].debt, wantDebt)
	}
}

// TestGovernorSamplerPausedNoGovern (spec 028 FR-013, T010): while paused the
// sampler issues no Govern calls and resets the breach window, so a resume
// requires a full fresh window — a pause never converts accrued breach into an
// instant shed.
func TestGovernorSamplerPausedNoGovern(t *testing.T) {
	breachSamples := int(cognition.BreachWindow / cognition.GovernorCadence)
	pending := &fakePending{}
	pending.set([]llm.PendingThought{{Kind: "planner", PredictedSec: 80, ElapsedSec: 0}})
	status := &fakeStatus{speed: clock.Speed32x}
	s := newGovernorSampler(pending, status)

	// Accrue partway (one short of a window), then pause and hold.
	for i := 0; i < breachSamples-1; i++ {
		s.sample()
	}
	status.setPaused(true)
	for i := 0; i < 3*breachSamples; i++ {
		s.sample()
	}
	if calls := status.governCalls(); len(calls) != 0 {
		t.Fatalf("paused samples issued Govern calls: %+v", calls)
	}

	// Resume: the window reset by the pause means a full fresh window is needed.
	status.setPaused(false)
	for i := 1; i < breachSamples; i++ {
		s.sample()
		if calls := status.governCalls(); len(calls) != 0 {
			t.Fatalf("shed before a fresh post-resume window completed: %+v", calls)
		}
	}
	s.sample()
	if calls := status.governCalls(); len(calls) != 1 {
		t.Fatalf("expected exactly one shed after a fresh post-resume window, got %+v", calls)
	}
}

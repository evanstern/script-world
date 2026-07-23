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

// fakeStatus is a scripted status door with a fixed effective speed, standing
// in for the loop's non-blocking Do("status", …). stopped models a stopped
// loop (Do error) so the sampler's error path is exercised.
type fakeStatus struct {
	speed   clock.Speed
	stopped bool
}

func (f *fakeStatus) Do(name string, speed clock.Speed) (sim.Status, error) {
	if f.stopped {
		return sim.Status{}, context.Canceled
	}
	return sim.Status{Speed: f.speed}, nil
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

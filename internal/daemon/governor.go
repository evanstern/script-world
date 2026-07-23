package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
)

// GovernorSnapshot is the latest debt sample the daemon governor sampler has
// taken (spec 028 US1): the aggregate staleness debt (a dimensionless sum of
// budget-fractions over pending model-bound thoughts) and the count of thoughts
// contributing to it. Derived, never stored — the sampler recomputes it every
// GovernorCadence and the ipc server folds it into status.
type GovernorSnapshot struct {
	Debt float64
	Jobs int
}

// pendingSource yields the orchestrator's current pending model-bound thoughts.
// The orchestrator satisfies it via PendingCognition; tests supply fakes so the
// sampler is exercised without a live model (research R9). Narrow by design —
// the sampler needs nothing else from the orchestrator.
type pendingSource interface {
	PendingCognition() []llm.PendingThought
}

// statusSource yields the loop's current clock status without ever blocking the
// loop goroutine. The real *sim.Loop satisfies it through its non-blocking
// command door (Do("status", …)); tests supply fakes with a fixed speed.
type statusSource interface {
	Do(name string, speed clock.Speed) (sim.Status, error)
}

// governorSampler is the daemon-owned wall-clock observer behind US1: once per
// GovernorCadence it reads the loop's effective speed, snapshots the
// orchestrator's pending thoughts, and stores the aggregate staleness debt for
// status. It makes NO decisions and emits NO events — the governor state
// machine and Loop.Govern are later slices (US2/US3). It is constructed only
// when an orchestrator exists, so a no-LLM world builds zero governor machinery
// (FR-003, SC-004).
type governorSampler struct {
	pending pendingSource
	status  statusSource

	mu   sync.Mutex
	snap GovernorSnapshot
}

// newGovernorSampler wires a sampler over the orchestrator's pending inventory
// and the loop's status door. Both are narrow interfaces so tests can drive the
// sampler with fakes.
func newGovernorSampler(pending pendingSource, status statusSource) *governorSampler {
	return &governorSampler{pending: pending, status: status}
}

// run samples every GovernorCadence until the daemon ctx is canceled, then
// exits cleanly. Sampling goes through the loop's non-blocking command door, so
// it never blocks the loop or the tick schedule.
func (s *governorSampler) run(ctx context.Context) {
	ticker := time.NewTicker(cognition.GovernorCadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sample()
		}
	}
}

// sample takes one debt reading: read the loop's effective speed, snapshot the
// pending thoughts, and store the aggregate debt. A stopped loop (Do error)
// leaves the last snapshot untouched — shutdown is racing the ctx cancel and
// the next select will exit.
func (s *governorSampler) sample() {
	st, err := s.status.Do("status", "")
	if err != nil {
		return // loop stopped; the ctx.Done branch will unwind run next
	}
	// State.Speed is the effective speed the loop paces at (research R2); its
	// tick rate is what predicted drift is measured against. Uncapped max yields
	// TicksPerSecond 0 and Debt returns 0 (max is refused with an LLM anyway).
	tps := st.Speed.TicksPerSecond()

	pending := s.pending.PendingCognition()
	inputs := make([]cognition.PendingDebtInput, 0, len(pending))
	for _, p := range pending {
		inputs = append(inputs, cognition.PendingDebtInput{
			Kind:         p.Kind,
			PredictedSec: p.PredictedSec,
			ElapsedSec:   p.ElapsedSec,
		})
	}
	debt, jobs := cognition.Debt(inputs, tps)

	s.mu.Lock()
	s.snap = GovernorSnapshot{Debt: debt, Jobs: jobs}
	s.mu.Unlock()
}

// Snapshot returns the latest debt reading. Safe from any goroutine.
func (s *governorSampler) Snapshot() GovernorSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// GovernorStatus is the ipc.Governor surface the server folds into status
// (exactly as it folds the LLM StatusSnapshot). It returns the latest sampled
// debt and contributing-thought count.
func (s *governorSampler) GovernorStatus() (debt float64, jobs int) {
	snap := s.Snapshot()
	return snap.Debt, snap.Jobs
}

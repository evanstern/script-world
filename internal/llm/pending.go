package llm

import (
	"sync"
	"time"

	"github.com/evanstern/promptworld/internal/cognition"
)

// PendingThought is one accepted-but-unlanded model-bound job as the adaptive
// throttle governor (spec 028 US1) sees it: the debt sampler sums these into an
// aggregate staleness signal. PredictedSec and ElapsedSec are computed at READ
// time — PredictedSec from the serving provider's CURRENT live estimate (so the
// figure tracks the freshest estimator state, spike rejection included), and
// ElapsedSec from wall time since the worker dequeued the job. The struct is a
// value copy in the returned snapshot; mutating it never touches the registry.
type PendingThought struct {
	Kind         string  // llm call kind, resolves to a cognition decision class
	Provider     string  // the provider that accepted the job (serving or queued-at)
	PredictedSec float64 // class points × that provider's current sec/pt estimate
	ElapsedSec   float64 // 0 while queued; wall-clock since dispatch while in flight
}

// pendingEntry is the registry's record of one accepted job. dispatchAt is the
// zero time until a worker dequeues the job; PredictedSec is deliberately NOT
// stored (it is recomputed from the live estimator on every PendingCognition
// read, per FR-001/R4), so an entry carries only what read-time arithmetic
// cannot reconstruct on its own.
type pendingEntry struct {
	provider   string
	kind       Kind
	dispatchAt time.Time // zero while queued; stamped at worker dequeue
}

// pendingRegistry is the mutex-guarded inventory of accepted-but-unfinished jobs
// backing Orchestrator.PendingCognition (spec 028 US1). An entry is added when
// Submit accepts a job (after admission), stamped with a dispatch time when a
// worker dequeues it, and removed when Submit returns on ANY terminal path —
// successful reply, provider error, worker-cap kill, or caller-abandoned skip.
// Submit owns each entry's lifetime end to end (add on accept, remove on return
// via defer); the worker only stamps dispatch time. That single-owner rule is
// why the registry drains to empty once every Submit call has returned — a
// leaked entry is a bug. Bookkeeping is O(queued+inflight): one map entry per
// live job, bounded by queue caps × slots × providers.
type pendingRegistry struct {
	mu   sync.Mutex
	seq  uint64 // monotonic id source; guarded by mu
	jobs map[uint64]*pendingEntry
}

func newPendingRegistry() *pendingRegistry {
	return &pendingRegistry{jobs: make(map[uint64]*pendingEntry)}
}

// add records an accepted job against its accepting provider and enqueue moment,
// returning the id the caller carries on the job so the worker can find the
// entry to stamp and Submit can find it to remove.
func (r *pendingRegistry) add(provider string, kind Kind) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	id := r.seq
	r.jobs[id] = &pendingEntry{provider: provider, kind: kind}
	return id
}

// dispatch stamps the dispatch time on a queued entry the instant a worker
// dequeues it. It is a no-op when the entry is already gone (Submit's caller
// abandoned the job and removed it before the worker got there) — idempotent so
// the two goroutines never need to coordinate beyond the lock.
func (r *pendingRegistry) dispatch(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.jobs[id]; ok {
		e.dispatchAt = time.Now()
	}
}

// remove drops an entry. Idempotent (delete of an absent key is a no-op), so a
// queue-full candidate that was registered-then-rejected and an accepted job
// that reached a terminal path both remove cleanly.
func (r *pendingRegistry) remove(id uint64) {
	r.mu.Lock()
	delete(r.jobs, id)
	r.mu.Unlock()
}

// PendingCognition returns a point-in-time snapshot of every model-bound job the
// orchestrator has accepted and not yet finished — the debt sampler's input
// (spec 028 FR-001). Every model-bound kind participates; there is no kind
// filtering (long-budget classes contribute proportionally tiny debt fractions
// downstream by construction). PredictedSec is the job's class point cost × its
// provider's CURRENT live seconds-per-point estimate, and ElapsedSec is wall
// time since dispatch (0 while still queued), both computed here at read time.
// The returned slice is a fresh copy: mutating it never affects the registry.
func (o *Orchestrator) PendingCognition() []PendingThought {
	// Copy the registry entries out under the lock, then compute predictions
	// outside it: the estimator has its own mutex, and this keeps the registry
	// lock held for O(entries) copying only, never for arithmetic.
	o.pending.mu.Lock()
	entries := make([]pendingEntry, 0, len(o.pending.jobs))
	for _, e := range o.pending.jobs {
		entries = append(entries, *e)
	}
	o.pending.mu.Unlock()

	now := time.Now()
	out := make([]PendingThought, 0, len(entries))
	for _, e := range entries {
		var predicted float64
		if p, ok := o.providers[e.provider]; ok {
			if dc, ok := cognition.ClassForKind(string(e.kind)); ok {
				predicted = float64(dc.Points) * p.est.Estimate()
			}
		}
		var elapsed float64
		if !e.dispatchAt.IsZero() {
			elapsed = now.Sub(e.dispatchAt).Seconds()
		}
		out = append(out, PendingThought{
			Kind:         string(e.kind),
			Provider:     e.provider,
			PredictedSec: predicted,
			ElapsedSec:   elapsed,
		})
	}
	return out
}

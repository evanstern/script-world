package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// plannerPoints is the planner class's registered point cost (cognition
// registry) — the multiplier PendingCognition applies to the live estimate.
const plannerPoints = 3

// pendingByKind returns the snapshot entries whose Kind matches, so a test can
// assert over the model-bound jobs it submitted regardless of ordering (the
// registry is a map, so PendingCognition ordering is unspecified).
func pendingByKind(pend []PendingThought, kind Kind) []PendingThought {
	var out []PendingThought
	for _, p := range pend {
		if p.Kind == string(kind) {
			out = append(out, p)
		}
	}
	return out
}

// parkedLocal is an OpenAI-compatible server that signals arrival on `arrived`
// and blocks each handler on `release`, so a test can hold a call verifiably
// in flight and inspect the registry mid-flight.
func parkedLocal(t *testing.T, arrived chan<- struct{}, release <-chan struct{}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestPendingLifecycle (spec 028 US1, contract a): a queued job reports
// ElapsedSec 0, an in-flight job reports ElapsedSec > 0, and a completed job
// disappears from the inventory. Parallel is left at the default 1, so a single
// parked call holds the one worker while a second call sits queued behind it.
func TestPendingLifecycle(t *testing.T) {
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := parkedLocal(t, arrived, release)
	o := newOrch(t, testConfig(srv.URL, "http://unused.invalid", 100), testStore(t))

	// Quiescent world: nothing pending.
	if pc := o.PendingCognition(); len(pc) != 0 {
		t.Fatalf("idle orchestrator has %d pending, want 0: %+v", len(pc), pc)
	}

	// First planner call reaches the worker and parks in the model handler.
	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "in-flight"})
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("first call never reached the model handler")
	}
	// Second planner call cannot get the (single) worker and sits queued.
	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "queued"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(o.PendingCognition()) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Let wall time accrue so the in-flight job's ElapsedSec is unambiguously > 0.
	time.Sleep(20 * time.Millisecond)

	pend := o.PendingCognition()
	if len(pend) != 2 {
		t.Fatalf("want 2 pending (one in-flight, one queued), got %d: %+v", len(pend), pend)
	}
	var inflight, queued int
	for _, p := range pend {
		if p.Provider != "local" {
			t.Errorf("pending provider = %q, want local", p.Provider)
		}
		if p.Kind != string(KindPlanner) {
			t.Errorf("pending kind = %q, want planner", p.Kind)
		}
		switch {
		case p.ElapsedSec > 0:
			inflight++
		case p.ElapsedSec == 0:
			queued++
		default:
			t.Errorf("negative ElapsedSec %v", p.ElapsedSec)
		}
	}
	if inflight != 1 || queued != 1 {
		t.Fatalf("want exactly one in-flight and one queued, got in-flight=%d queued=%d", inflight, queued)
	}

	// Release the model: both jobs complete and drain from the inventory.
	close(release)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(o.PendingCognition()) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("inventory did not drain after completion: %+v", o.PendingCognition())
}

// TestPendingDrainsToEmpty (spec 028 US1, contract b): a burst of concurrent
// Submits — successes, a provider-error path, and a caller-context-cancelled
// path — leaves the registry empty once every Submit has returned. Submit owns
// each entry's removal via defer, so wg.Wait() is a sufficient barrier: no
// polling needed, a non-empty registry after it is a leak.
func TestPendingDrainsToEmpty(t *testing.T) {
	// "fail" prompts 500 (provider error path); everything else replies ok.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lastUserPrompt(r) == "fail" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = 4
	o := newOrch(t, cfg, testStore(t))

	var wg sync.WaitGroup
	const n = 24
	for i := 0; i < n; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			switch i % 3 {
			case 0: // provider-error path
				o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "fail"})
			case 1: // caller-context-cancelled path
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // dead on arrival: stale-skip / ctx.Done terminal
				o.Submit(ctx, Request{Kind: KindPlanner, Prompt: "cancelled"})
			default: // success path
				o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "ok"})
			}
		}()
	}
	wg.Wait()

	if pc := o.PendingCognition(); len(pc) != 0 {
		t.Fatalf("registry leaked %d entries after all Submits returned: %+v", len(pc), pc)
	}
}

// TestPendingSnapshotIsolation (spec 028 US1, contract c): the returned slice is
// a copy — mutating it (element fields or the slice itself) leaves the registry
// and any subsequent snapshot untouched.
func TestPendingSnapshotIsolation(t *testing.T) {
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	srv := parkedLocal(t, arrived, release)
	o := newOrch(t, testConfig(srv.URL, "http://unused.invalid", 100), testStore(t))

	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "held"})
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("call never reached the model handler")
	}

	s1 := o.PendingCognition()
	if len(s1) != 1 {
		t.Fatalf("want 1 pending, got %d", len(s1))
	}
	// Mutate the returned element and grow the returned slice.
	s1[0].Provider = "hacked"
	s1[0].PredictedSec = -999
	s1 = append(s1, PendingThought{Kind: "ghost"})

	s2 := o.PendingCognition()
	if len(s2) != 1 {
		t.Fatalf("mutating the snapshot changed the registry size: got %d, want 1", len(s2))
	}
	if s2[0].Provider != "local" || s2[0].PredictedSec == -999 {
		t.Errorf("mutation bled into the registry: %+v", s2[0])
	}
}

// TestPendingPredictionArithmetic (spec 028 US1, contract d): PredictedSec is
// the class point cost × the provider's CURRENT live estimate — read at
// snapshot time, not frozen at submit. The first call completes and moves the
// estimator off its bootstrap seed; a second call held in flight must then
// predict against the moved value, proving the read is current, not frozen.
func TestPendingPredictionArithmetic(t *testing.T) {
	// The handler serves the first call immediately (warm-up: it feeds the
	// estimator) and parks every later call in flight for inspection.
	var warmed atomic.Bool
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !warmed.CompareAndSwap(false, true) {
			arrived <- struct{}{}
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	// t.Cleanup runs AFTER the function-scoped `defer close(release)`, so the
	// parked handler is unblocked before srv.Close waits on it (a plain
	// `defer srv.Close()` would run first, LIFO, and deadlock).
	t.Cleanup(srv.Close)

	o := newOrch(t, testConfig(srv.URL, "http://unused.invalid", 100), testStore(t))
	seed := o.providers["local"].est.Estimate()

	// Warm-up: a completed planner call feeds one sample, moving the estimate.
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "warm"}); err != nil {
		t.Fatalf("warm-up call: %v", err)
	}
	moved := o.providers["local"].est.Estimate()
	if moved == seed {
		t.Fatalf("estimator did not move off seed %v; cannot prove current-estimate read", seed)
	}
	if pc := o.PendingCognition(); len(pc) != 0 {
		t.Fatalf("inventory not quiescent after warm-up: %+v", pc)
	}

	// Hold a second planner call in flight and read its prediction.
	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "predict"})
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("prediction call never reached the model handler")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(o.PendingCognition()) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	pend := pendingByKind(o.PendingCognition(), KindPlanner)
	if len(pend) != 1 {
		t.Fatalf("want 1 in-flight planner, got %d: %+v", len(pend), pend)
	}
	want := float64(plannerPoints) * o.providers["local"].est.Estimate()
	if pend[0].PredictedSec != want {
		t.Errorf("PredictedSec = %v, want points(%d) × current estimate(%v) = %v",
			pend[0].PredictedSec, plannerPoints, o.providers["local"].est.Estimate(), want)
	}
	if pend[0].PredictedSec == float64(plannerPoints)*seed {
		t.Errorf("prediction used the frozen seed %v, not the current estimate", seed)
	}
}

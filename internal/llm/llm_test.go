package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/store"
)

// mockLocal is an OpenAI-compatible chat-completions server.
func mockLocal(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "local says hi"}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockCloud is an Anthropic Messages API server (the SDK posts /v1/messages).
func mockCloud(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant",
			"model":       "claude-opus-4-8",
			"content":     []map[string]any{{"type": "text", "text": "cloud says hi"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 100, "output_tokens": 50},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func testConfig(localURL, cloudURL string, budget float64) Config {
	return Config{
		MonthlyBudgetUSD: budget,
		Local:            LocalConfig{Endpoint: localURL, Model: "test-local"},
		Cloud: CloudConfig{
			Model: "claude-opus-4-8", Endpoint: cloudURL,
			InputUSDPerMTok: 5, OutputUSDPerMTok: 25,
			APIKeyEnv: "PROMPTWORLD_TEST_KEY",
		},
	}
}

func newOrch(t *testing.T, cfg Config, st *store.Store) *Orchestrator {
	t.Helper()
	t.Setenv("PROMPTWORLD_TEST_KEY", "test-key") // hermetic: never depend on the caller's env
	o, err := New(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Close)
	return o
}

// TestRouting is AC#1: planner/conversation route local; consolidation,
// narrator, and drama route cloud — verified by which mock takes the hit.
func TestRouting(t *testing.T) {
	var localHits, cloudHits atomic.Int64
	local := mockLocal(t, &localHits)
	cloud := mockCloud(t, &cloudHits)
	o := newOrch(t, testConfig(local.URL, cloud.URL, 100), testStore(t))

	cases := []struct {
		kind Kind
		tier Tier
	}{
		{KindPlanner, TierLocal},
		{KindConversation, TierLocal},
		{KindConsolidation, TierCloud},
		{KindNarrator, TierCloud},
		{KindDrama, TierCloud},
		{KindMetatron, TierCloud},
		{KindMeeting, TierLocal},
	}
	for _, c := range cases {
		resp, err := o.Submit(context.Background(), Request{Kind: c.kind, Prompt: "hello"})
		if err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		if resp.Tier != c.tier {
			t.Errorf("%s routed to %s, want %s", c.kind, resp.Tier, c.tier)
		}
	}
	if localHits.Load() != 3 || cloudHits.Load() != 4 {
		t.Errorf("hits: local=%d cloud=%d, want 3/4", localHits.Load(), cloudHits.Load())
	}

	// Cost math: local is free; cloud bills at configured prices.
	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if err != nil || resp.CostUSD != 0 {
		t.Errorf("local call should cost $0, got %.5f err=%v", resp.CostUSD, err)
	}
	resp, err = o.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "x"})
	want := 100*5.0/1e6 + 50*25.0/1e6 // 0.00175
	if err != nil || resp.CostUSD != want {
		t.Errorf("cloud cost = %.5f, want %.5f err=%v", resp.CostUSD, want, err)
	}

	if _, err := o.Submit(context.Background(), Request{Kind: "sorcery", Prompt: "x"}); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("unknown kind: %v", err)
	}
}

// TestMeterPersistsAcrossRestart: spend survives an orchestrator (daemon)
// restart via the store's meta table.
func TestMeterPersistsAcrossRestart(t *testing.T) {
	var hits atomic.Int64
	cloud := mockCloud(t, &hits)
	st := testStore(t)
	cfg := testConfig("http://unused.invalid", cloud.URL, 100)

	o1 := newOrch(t, cfg, st)
	for i := 0; i < 3; i++ {
		if _, err := o1.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	_, spent1, _ := o1.meter.Snapshot()
	o1.Close()

	o2 := newOrch(t, cfg, st)
	_, spent2, _ := o2.meter.Snapshot()
	if spent2 != spent1 || spent2 == 0 {
		t.Errorf("spend not persisted: before=%.5f after=%.5f", spent1, spent2)
	}
}

// TestBudgetCeiling is AC#2: hitting the ceiling refuses cloud calls BEFORE
// any HTTP happens (no silent overspend); the local tier is unaffected.
func TestBudgetCeiling(t *testing.T) {
	var localHits, cloudHits atomic.Int64
	local := mockLocal(t, &localHits)
	cloud := mockCloud(t, &cloudHits)
	// One call costs $0.00175; the second must exceed the ceiling.
	o := newOrch(t, testConfig(local.URL, cloud.URL, 0.001), testStore(t))

	if _, err := o.Submit(context.Background(), Request{Kind: KindConsolidation, Prompt: "x"}); err != nil {
		t.Fatalf("first call under budget should pass: %v", err)
	}
	_, err := o.Submit(context.Background(), Request{Kind: KindConsolidation, Prompt: "x"})
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("want ErrBudgetExhausted, got %v", err)
	}
	if cloudHits.Load() != 1 {
		t.Errorf("refused call must not reach the API (hits=%d)", cloudHits.Load())
	}
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"}); err != nil {
		t.Errorf("local tier must survive cloud budget exhaustion: %v", err)
	}

	st := o.StatusSnapshot()
	if st.Spent < 0.001 || st.Budget != 0.001 {
		t.Errorf("status meter wrong: %+v", st)
	}
}

// TestDegradedAndRecovery is AC#3 at the package level: an unreachable tier
// opens the circuit (fast failures, no hangs), and a returning model closes
// it again via the half-open probe.
func TestDegradedAndRecovery(t *testing.T) {
	oldInitial := backoffInitial
	backoffInitial = 50 * time.Millisecond
	defer func() { backoffInitial = oldInitial }()

	// Reserve an address, then close it: connection refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	o := newOrch(t, testConfig("http://"+addr, "http://"+addr, 100), testStore(t))

	for i := 0; i < failuresToOpen; i++ {
		if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"}); err == nil {
			t.Fatal("call against dead endpoint should fail")
		}
	}
	if o.StatusSnapshot().Local.Up {
		t.Fatal("local tier should be marked down after consecutive failures")
	}

	// Circuit open: refusal is immediate, not a connection timeout.
	start := time.Now()
	_, err = o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	if !errors.Is(err, ErrTierDown) {
		t.Fatalf("want ErrTierDown, got %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Errorf("open circuit must fail fast, took %v", time.Since(start))
	}

	// The model comes back on the same address.
	ln2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("could not rebind %s: %v", addr, err)
	}
	revived := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "back"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	revived.Listener.Close()
	revived.Listener = ln2
	revived.Start()
	defer revived.Close()

	// After the backoff window, the half-open probe succeeds and the tier
	// recovers.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"}); err == nil {
			if resp.Text != "back" {
				t.Errorf("probe response %q", resp.Text)
			}
			if !o.StatusSnapshot().Local.Up {
				t.Error("tier should be up after successful probe")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tier never recovered after model came back")
}

// TestQueueBackpressure: a saturated tier refuses instead of piling up —
// the surface TASK-7 uses to let local throughput cap sim speed.
func TestQueueBackpressure(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer slow.Close()
	defer close(release)

	o := newOrch(t, testConfig(slow.URL, "http://unused.invalid", 100), testStore(t))

	// One request occupies the worker; queueCap more fill the queue.
	for i := 0; i < queueCap+1; i++ {
		go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
	}
	// Wait until saturation is observable, then expect fast refusal.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.StatusSnapshot().Local.Queue >= queueCap {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "overflow"})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("want ErrQueueFull, got %v", err)
	}
}

// TestCallerTimeoutIsNotModelFailure (TASK-22 live finding): callers whose
// contexts expire while queued or mid-call are starvation, not model
// failure — they must neither strike the circuit breaker nor reach the
// model once dead. A busy tier serving a long conversation must not get
// declared down by impatient planners.
func TestCallerTimeoutIsNotModelFailure(t *testing.T) {
	release := make(chan struct{})
	first := true
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if first {
			first = false
			<-release // park the worker on the first call (the "conversation")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer slow.Close()

	o := newOrch(t, testConfig(slow.URL, "http://unused.invalid", 100), testStore(t))

	// One long call occupies the worker; several short-deadline callers
	// pile up behind it and give up — more than failuresToOpen of them.
	longDone := make(chan struct{})
	go func() {
		o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "long"})
		close(longDone)
	}()
	time.Sleep(50 * time.Millisecond) // let the long call reach the worker
	for i := 0; i < failuresToOpen+2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		if _, err := o.Submit(ctx, Request{Kind: KindPlanner, Prompt: "impatient"}); err == nil {
			t.Fatal("impatient caller should have timed out")
		}
		cancel()
	}
	close(release)
	<-longDone

	if !o.StatusSnapshot().Local.Up {
		t.Fatal("caller timeouts opened the circuit — starvation counted as model failure")
	}
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "after"}); err != nil {
		t.Fatalf("healthy tier must serve the next caller: %v", err)
	}
}

// lastUserPrompt pulls the final user message out of an OpenAI-compatible
// chat-completions request body — the test servers key ordering off it.
func lastUserPrompt(r *http.Request) string {
	var body struct {
		Messages []struct {
			Role, Content string
		} `json:"messages"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	for i := len(body.Messages) - 1; i >= 0; i-- {
		if body.Messages[i].Role == "user" {
			return body.Messages[i].Content
		}
	}
	return ""
}

func localReplyJSON(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
		"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
	})
}

// TestParallelInFlight (US1, SC-004): with parallel: 4, four calls are
// verifiably in flight at the same instant. The server counts concurrent
// handlers and parks each until all four have arrived; a watchdog fails the
// test if fewer than four ever overlap (i.e. the tier is still serial).
func TestParallelInFlight(t *testing.T) {
	const n = 4
	var cur, max atomic.Int32
	arrived := make(chan struct{}, n)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := cur.Add(1)
		for { // record the high-water mark of simultaneous handlers
			m := max.Load()
			if c <= m || max.CompareAndSwap(m, c) {
				break
			}
		}
		arrived <- struct{}{}
		<-release // hold every handler open until all n have arrived
		cur.Add(-1)
		localReplyJSON(w)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = n
	o := newOrch(t, cfg, testStore(t))

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
			errs <- err
		}()
	}
	watchdog := time.After(5 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-arrived:
		case <-watchdog:
			t.Fatalf("only %d/%d calls reached the server — tier is not running %d-wide", i, n, n)
		}
	}
	if got := max.Load(); got != n {
		t.Errorf("max simultaneous in-flight = %d, want %d", got, n)
	}
	close(release)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("call %d: %v", i, err)
		}
	}
}

// TestSerialWhenParallelAbsent (US1, SC-003): with the field absent behavior
// is byte-identical to today — exactly one call is ever in flight, even with a
// full queue waiting.
func TestSerialWhenParallelAbsent(t *testing.T) {
	var cur, max atomic.Int32
	arrived := make(chan struct{}, 8)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := cur.Add(1)
		for {
			m := max.Load()
			if c <= m || max.CompareAndSwap(m, c) {
				break
			}
		}
		arrived <- struct{}{}
		<-release
		cur.Add(-1)
		localReplyJSON(w)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL, "http://unused.invalid", 100) // no Parallel → default 1
	o := newOrch(t, cfg, testStore(t))

	const submitted = 5
	errs := make(chan error, submitted)
	for i := 0; i < submitted; i++ {
		go func() {
			_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
			errs <- err
		}()
	}
	<-arrived // one handler is in flight and parked
	select {
	case <-arrived:
		t.Fatal("a second call reached the server with parallel absent — tier is not serial")
	case <-time.After(300 * time.Millisecond):
	}
	if got := max.Load(); got != 1 {
		t.Errorf("max simultaneous in-flight = %d, want 1 (serial default)", got)
	}
	close(release)
	for i := 0; i < submitted; i++ {
		if err := <-errs; err != nil {
			t.Errorf("call: %v", err)
		}
	}
}

// TestParallelOverflowPreservesOrderAndPrio (US1, FR-002): submissions beyond
// N slots queue rather than drop, and the priority (conversation) lane still
// jumps the FIFO line under saturation — a conversation submitted AFTER a full
// queue of planners is served in the first freed-slot wave, not last.
func TestParallelOverflowPreservesOrderAndPrio(t *testing.T) {
	const slots = 4
	const overflow = 8
	var mu sync.Mutex
	var order []string
	var entered atomic.Int32
	parked := make(chan struct{}, slots)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prompt := lastUserPrompt(r)
		idx := entered.Add(1)
		mu.Lock()
		order = append(order, prompt)
		mu.Unlock()
		if idx <= slots { // the first `slots` calls occupy every slot and park
			parked <- struct{}{}
			<-release
		}
		localReplyJSON(w)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = slots
	o := newOrch(t, cfg, testStore(t))

	type res struct {
		prompt string
		err    error
	}
	done := make(chan res, slots+overflow+1)
	submit := func(kind Kind, prompt string) {
		go func() {
			_, err := o.Submit(context.Background(), Request{Kind: kind, Prompt: prompt})
			done <- res{prompt, err}
		}()
	}

	// Fill every slot; wait until all `slots` are parked in the server.
	for i := 0; i < slots; i++ {
		submit(KindPlanner, "parked")
	}
	watchdog := time.After(5 * time.Second)
	for i := 0; i < slots; i++ {
		select {
		case <-parked:
		case <-watchdog:
			t.Fatalf("only %d/%d slots occupied", i, slots)
		}
	}
	// Overflow planners queue behind the full slots.
	for i := 0; i < overflow; i++ {
		submit(KindPlanner, "overflow")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.StatusSnapshot().Local.Queue >= overflow {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if q := o.StatusSnapshot().Local.Queue; q < overflow {
		t.Fatalf("overflow did not queue: queue=%d, want >=%d", q, overflow)
	}
	// A conversation submitted LAST must still jump the line via the prio lane.
	submit(KindConversation, "conversation")
	time.Sleep(100 * time.Millisecond) // let it land in prio before release
	close(release)

	got := make(map[string]int)
	for i := 0; i < slots+overflow+1; i++ {
		select {
		case r := <-done:
			if r.err != nil {
				t.Errorf("call %q: %v", r.prompt, r.err)
			}
			got[r.prompt]++
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d/%d calls completed — work was lost", i, slots+overflow+1)
		}
	}
	// Nothing lost: every submitted call ran exactly once.
	if got["parked"] != slots || got["overflow"] != overflow || got["conversation"] != 1 {
		t.Errorf("completion counts = %v, want parked:%d overflow:%d conversation:1", got, slots, overflow)
	}
	// Prio lane intact: the conversation was dequeued in the first freed-slot
	// wave, not last (pure FIFO would place it at index slots+overflow).
	mu.Lock()
	defer mu.Unlock()
	convIdx := -1
	for i, p := range order {
		if p == "conversation" {
			convIdx = i
			break
		}
	}
	if convIdx < slots || convIdx >= 2*slots {
		t.Errorf("conversation served at index %d (order=%v); want the first overflow wave [%d,%d) — prio lane not honored", convIdx, order, slots, 2*slots)
	}
}

// TestBestEffortAdmission (TASK-21): a best-effort call succeeds on a quiet
// tier but is refused immediately (ErrTierBusy) the moment anything is waiting
// — best-effort work may never displace real cognition. Scheduled musing was
// the first user of this mechanism (retired in spec 017); it remains doctrine
// for any future drop-when-busy kind, exercised here with a planner call.
func TestBestEffortAdmission(t *testing.T) {
	release := make(chan struct{})
	first := true
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if first {
			first = false
			<-release // the worker parks on the first call only
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "a quiet thought"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer slow.Close()

	o := newOrch(t, testConfig(slow.URL, "http://unused.invalid", 100), testStore(t))

	// Park the worker, then queue one planner call so the queue is non-empty.
	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "parked"})
	go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "queued"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.StatusSnapshot().Local.Queue >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "best-effort", BestEffort: true}); !errors.Is(err, ErrTierBusy) {
		t.Fatalf("busy tier must drop best-effort musings: got %v", err)
	}
	// A starved musing (fairness floor) drops BestEffort and queues like
	// any other call — admission must not refuse it.
	done := make(chan error, 1)
	go func() {
		_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "starved"})
		done <- err
	}()

	// Drain everything; a musing on a quiet tier goes through, locally.
	close(release)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.StatusSnapshot().Local.Queue == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("starved (non-best-effort) musing must be served: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("starved musing never completed")
	}
	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "best-effort", BestEffort: true})
	if err != nil {
		t.Fatalf("quiet tier must serve best-effort musings: %v", err)
	}
	if resp.Tier != TierLocal || resp.Text != "a quiet thought" {
		t.Errorf("musing response: %+v", resp)
	}
}

// TestBestEffortSlotAware (US2, FR-003, SC-002): with N slots a best-effort
// musing is admitted whenever ANY slot is free — not refused the moment one
// call is in flight — and refused only when every slot is occupied.
func TestBestEffortSlotAware(t *testing.T) {
	const slots = 4
	parked := make(chan struct{}, slots)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lastUserPrompt(r) == "park" {
			parked <- struct{}{}
			<-release // planners occupy a slot until released
		}
		localReplyJSON(w)
	}))
	defer srv.Close()
	defer close(release)

	cfg := testConfig(srv.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = slots
	o := newOrch(t, cfg, testStore(t))

	// occupy submits n parked planners and blocks until all n hold a slot.
	occupy := func(n int) {
		for i := 0; i < n; i++ {
			go o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "park"})
		}
		watchdog := time.After(5 * time.Second)
		for i := 0; i < n; i++ {
			select {
			case <-parked:
			case <-watchdog:
				t.Fatalf("only %d/%d planners occupied a slot", i, n)
			}
		}
	}

	// One slot busy, three free, queues empty → the musing is served (today's
	// serial tier would have refused it the instant anything was in flight).
	occupy(1)
	resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "best-effort", BestEffort: true})
	if err != nil {
		t.Fatalf("best-effort musing with a free slot must be served: %v", err)
	}
	if resp.Tier != TierLocal || resp.Text != "ok" {
		t.Errorf("musing response: %+v", resp)
	}

	// Occupy the remaining three slots → all four busy, queues still empty.
	occupy(slots - 1)
	start := time.Now()
	_, err = o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "best-effort", BestEffort: true})
	if !errors.Is(err, ErrTierBusy) {
		t.Fatalf("all slots busy must drop the best-effort musing: got %v", err)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("best-effort refusal must be immediate, took %v", d)
	}
}

// TestEstimatorSampleCountUnderConcurrency (US3, FR-004): under N-wide load
// the estimator observes exactly one sample per completed call — no lost or
// double counts — and the estimate moves off its seed.
func TestEstimatorSampleCountUnderConcurrency(t *testing.T) {
	var hits atomic.Int64
	local := mockLocal(t, &hits)
	cfg := testConfig(local.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = 8
	o := newOrch(t, cfg, testStore(t))

	seed := o.tiers[TierLocal].est.Estimate()
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A synchronized 40-wide burst at Parallel=8 outruns the workers'
			// dequeues, so the bounded tier queue (cap 32) fills and Submit
			// fails fast with ErrQueueFull — designed backpressure, exactly the
			// "back off and retry" that error instructs. Retrying keeps every
			// call completing, so the exact-count assertion (one sample per
			// call) still holds; the deadline stops a real regression from
			// hanging the test.
			deadline := time.Now().Add(5 * time.Second)
			for {
				_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"})
				if err == nil {
					return
				}
				if !errors.Is(err, ErrQueueFull) || time.Now().After(deadline) {
					errs <- err
					return
				}
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("planner call: %v", err)
	}
	est, _, samples, _ := o.tiers[TierLocal].est.Stats()
	if samples != n {
		t.Errorf("estimator samples = %d, want %d (one per completed call)", samples, n)
	}
	if est == seed {
		t.Errorf("estimate did not move from seed %v under concurrent load", seed)
	}
	if hits.Load() != n {
		t.Errorf("server hits = %d, want %d", hits.Load(), n)
	}
}

// TestBreakerConsecutiveUnderConcurrency (US3, FR-005): under concurrent
// failures the breaker counts exactly, opens only on failuresToOpen
// CONSECUTIVE failures, and a success resets the run — proven by driving each
// batch concurrently and gating on completion between batches.
func TestBreakerConsecutiveUnderConcurrency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(lastUserPrompt(r), "fail") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		localReplyJSON(w)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = 4
	o := newOrch(t, cfg, testStore(t))
	local := o.tiers[TierLocal]

	// fire submits n concurrent calls and blocks until all have replied, so
	// the breaker's fail()/succeed() bookkeeping is fully settled after it.
	fire := func(prompt string, n int, wantErr bool) {
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: prompt})
				if (err != nil) != wantErr {
					t.Errorf("prompt %q: err=%v, wantErr=%v", prompt, err, wantErr)
				}
			}()
		}
		wg.Wait()
	}

	// One short of the threshold: not enough to open (exact count, no over-count).
	fire("fail", failuresToOpen-1, true)
	if local.health.down() {
		t.Fatalf("breaker opened after only %d failures (threshold %d)", failuresToOpen-1, failuresToOpen)
	}
	// A success resets the consecutive run to zero.
	fire("ok", 1, false)
	if local.health.down() {
		t.Fatal("breaker down after a healthy call")
	}
	// Another sub-threshold burst: total failures now exceed the threshold,
	// but they are not consecutive — the breaker must stay closed.
	fire("fail", failuresToOpen-1, true)
	if local.health.down() {
		t.Fatalf("breaker opened on non-consecutive failures — success failed to reset the run")
	}
	// One more consecutive failure crosses the threshold.
	fire("fail", 1, true)
	if !local.health.down() {
		t.Fatalf("breaker did not open after %d consecutive failures", failuresToOpen)
	}
}

// TestReplyIntegrityUnderConcurrency (US3, FR-005): a failing call never
// corrupts another caller's reply. With the breaker held out of the way, every
// successful caller gets ITS OWN echoed prompt back and every failing caller
// gets an error — no crossed wires under N-wide load (-race validated).
func TestReplyIntegrityUnderConcurrency(t *testing.T) {
	old := failuresToOpen
	failuresToOpen = 1 << 30 // isolate reply routing from breaker behavior
	defer func() { failuresToOpen = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := lastUserPrompt(r)
		if strings.HasPrefix(p, "fail") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": p}}}, // echo
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL, "http://unused.invalid", 100)
	cfg.Local.Parallel = 8
	o := newOrch(t, cfg, testStore(t))

	// Stay within the tier's admission capacity (slots + queueCap) so this
	// stays a reply-routing test, not a backpressure one.
	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fail := i%2 == 0
			prompt := fmt.Sprintf("ok-%d", i)
			if fail {
				prompt = fmt.Sprintf("fail-%d", i)
			}
			resp, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: prompt})
			if fail {
				if err == nil {
					t.Errorf("%s: expected error, got success %q", prompt, resp.Text)
				}
				return
			}
			if err != nil {
				t.Errorf("%s: unexpected error %v", prompt, err)
				return
			}
			if resp.Text != prompt {
				t.Errorf("reply crossed wires: caller %q received %q", prompt, resp.Text)
			}
		}(i)
	}
	wg.Wait()
}

// TestMeterExactUnderConcurrency (US3, FR-006, SC-005): concurrent cloud
// completions through the 1-slot cloud tier, interleaved with direct concurrent
// Meter.Add calls, sum to the exact expected total. Prices are chosen so every
// cost is exactly representable, so the assertion is exact equality.
func TestMeterExactUnderConcurrency(t *testing.T) {
	var hits atomic.Int64
	cloud := mockCloud(t, &hits)
	cfg := testConfig("http://unused.invalid", cloud.URL, 1e9)
	cfg.Cloud.InputUSDPerMTok = 10000 // 100 input tokens → exactly $1.00 per call
	cfg.Cloud.OutputUSDPerMTok = 0
	o := newOrch(t, cfg, testStore(t))

	const calls = 20
	const adders = 10
	const perAdd = 2.0
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := o.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "x"}); err != nil {
				t.Errorf("cloud call: %v", err)
			}
		}()
	}
	for i := 0; i < adders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := o.meter.Add(perAdd); err != nil {
				t.Errorf("meter add: %v", err)
			}
		}()
	}
	wg.Wait()

	want := float64(calls)*1.0 + float64(adders)*perAdd
	_, spent, _ := o.meter.Snapshot()
	if spent != want {
		t.Errorf("meter spend = %v, want exactly %v", spent, want)
	}
	if hits.Load() != calls {
		t.Errorf("cloud hits = %d, want %d", hits.Load(), calls)
	}
}

// TestSkipObserveNoEstimatorSample (TASK-52): a loop-internal call
// (SkipObserve) feeds no per-call sample to the tier estimator and leaves the
// estimate untouched; a normal call still feeds exactly one. Both calls reach
// the model regardless — SkipObserve gates estimation only.
func TestSkipObserveNoEstimatorSample(t *testing.T) {
	var hits atomic.Int64
	local := mockLocal(t, &hits)
	o := newOrch(t, testConfig(local.URL, "http://unused.invalid", 100), testStore(t))
	lt := o.tiers[TierLocal]

	est0, _, samples0, _ := lt.est.Stats()
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x", SkipObserve: true}); err != nil {
		t.Fatal(err)
	}
	est1, _, samples1, _ := lt.est.Stats()
	if samples1 != samples0 {
		t.Errorf("SkipObserve fed the estimator: samples %d -> %d", samples0, samples1)
	}
	if est1 != est0 {
		t.Errorf("SkipObserve moved the estimate: %v -> %v", est0, est1)
	}

	// Regression: a normal call still feeds exactly one sample.
	if _, err := o.Submit(context.Background(), Request{Kind: KindPlanner, Prompt: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, samples2, _ := lt.est.Stats(); samples2 != samples0+1 {
		t.Errorf("normal call fed %d samples, want 1", samples2-samples0)
	}
	if hits.Load() != 2 {
		t.Errorf("both calls must reach the model: hits=%d", hits.Load())
	}
}

// TestObserveCognitionFeedsOnce (TASK-52): ObserveCognition reports exactly one
// whole-loop sample to the routed tier's estimator, moving it exactly as a
// direct estimator sample of the same per-point value would; unknown kinds are
// a no-op.
func TestObserveCognitionFeedsOnce(t *testing.T) {
	o := newOrch(t, testConfig("http://unused.invalid", "http://unused.invalid", 100), testStore(t))
	lt := o.tiers[TierLocal]
	_, _, samples0, _ := lt.est.Stats()

	const millis = 6000
	const points = 3 // planner's registered point cost
	ref := cognition.NewEstimator(cognition.SeedFor(nil, string(TierLocal)))
	ref.Sample(float64(millis) / 1000 / float64(points))
	want := ref.Estimate()

	o.ObserveCognition(KindPlanner, millis)
	est, _, samples, _ := lt.est.Stats()
	if samples != samples0+1 {
		t.Errorf("ObserveCognition fed %d samples, want 1", samples-samples0)
	}
	if est != want {
		t.Errorf("estimate = %v, want %v (one whole-loop sample)", est, want)
	}

	// Unknown kind has no tier: no-op, no sample.
	o.ObserveCognition("sorcery", 1000)
	if _, _, samples2, _ := lt.est.Stats(); samples2 != samples {
		t.Errorf("unknown kind fed the estimator: %d -> %d", samples, samples2)
	}
}

// TestSkipObserveStillMeters (TASK-52): SkipObserve suppresses estimator
// feeding only — a billable cloud call still records its cost in the meter, and
// the call still reaches the API.
func TestSkipObserveStillMeters(t *testing.T) {
	var hits atomic.Int64
	cloud := mockCloud(t, &hits)
	o := newOrch(t, testConfig("http://unused.invalid", cloud.URL, 100), testStore(t))
	ct := o.tiers[TierCloud]
	_, _, csamples0, _ := ct.est.Stats()
	_, spent0, _ := o.meter.Snapshot()

	resp, err := o.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "x", SkipObserve: true})
	if err != nil {
		t.Fatal(err)
	}
	if resp.CostUSD <= 0 {
		t.Errorf("SkipObserve call must still bill: cost=%v", resp.CostUSD)
	}
	if _, spent1, _ := o.meter.Snapshot(); spent1 <= spent0 {
		t.Errorf("meter did not record the SkipObserve call: %v -> %v", spent0, spent1)
	}
	if _, _, csamples1, _ := ct.est.Stats(); csamples1 != csamples0 {
		t.Errorf("SkipObserve fed the cloud estimator: %d -> %d", csamples0, csamples1)
	}
	if hits.Load() != 1 {
		t.Errorf("call must reach the API: hits=%d", hits.Load())
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llm.json")
	if cfg, err := LoadConfig(path); err != nil || cfg != nil {
		t.Fatalf("missing file should be (nil, nil), got %v %v", cfg, err)
	}
	if err := WriteDefault(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg == nil {
		t.Fatalf("load default: %v", err)
	}
	if cfg.MonthlyBudgetUSD != 100 || cfg.Cloud.Model != "claude-opus-4-8" ||
		cfg.Cloud.APIKeyEnv != "ANTHROPIC_API_KEY" || cfg.Local.Endpoint == "" {
		t.Errorf("default config wrong: %+v", cfg)
	}
}

// TestLocalWorkersNormalization (FR-001/FR-007): the parallel knob normalizes
// to an effective worker count and warns exactly on out-of-range values —
// absent/0 is the silent compat default (1); the cap is 16; nothing errors.
func TestLocalWorkersNormalization(t *testing.T) {
	cases := []struct {
		parallel int
		wantN    int
		wantWarn bool
	}{
		{0, 1, false},   // absent / explicit 0 → 1, silent (compat)
		{1, 1, false},   // floor, verbatim
		{4, 4, false},   // measured sweet spot, verbatim
		{16, 16, false}, // cap, verbatim, no warning
		{-2, 1, true},   // negative → 1 with warning
		{64, 16, true},  // above cap → 16 with warning
		{17, 16, true},  // one past the cap → clamped with warning
	}
	for _, c := range cases {
		n, warn := LocalConfig{Parallel: c.parallel}.Workers()
		if n != c.wantN {
			t.Errorf("Workers(parallel=%d) n=%d, want %d", c.parallel, n, c.wantN)
		}
		if (warn != "") != c.wantWarn {
			t.Errorf("Workers(parallel=%d) warn=%q, wantWarn=%v", c.parallel, warn, c.wantWarn)
		}
	}
	// Absent field on a struct with no Parallel set is byte-identical to 0.
	if n, warn := (LocalConfig{}).Workers(); n != 1 || warn != "" {
		t.Errorf("absent parallel → (%d,%q), want (1,\"\")", n, warn)
	}
}

// TestRoundsNormalization (TASK-52): loop_max_rounds normalizes to an
// effective cap and warns exactly on out-of-range values — absent/0 is the
// silent default (8), the cap is 16, negatives fall back to the default, and
// nothing errors (mirrors LocalConfig.Workers()).
func TestRoundsNormalization(t *testing.T) {
	cases := []struct {
		rounds   int
		wantN    int
		wantWarn bool
	}{
		{0, 8, false},   // absent / explicit 0 → default 8, silent
		{1, 1, false},   // floor, verbatim
		{8, 8, false},   // default value set explicitly, verbatim
		{16, 16, false}, // cap, verbatim, no warning
		{-3, 8, true},   // negative → default 8 with warning
		{17, 16, true},  // one past the cap → clamped with warning
		{999, 16, true}, // far above cap → clamped with warning
	}
	for _, c := range cases {
		n, warn := Config{LoopMaxRounds: c.rounds}.Rounds()
		if n != c.wantN {
			t.Errorf("Rounds(loop_max_rounds=%d) n=%d, want %d", c.rounds, n, c.wantN)
		}
		if (warn != "") != c.wantWarn {
			t.Errorf("Rounds(loop_max_rounds=%d) warn=%q, wantWarn=%v", c.rounds, warn, c.wantWarn)
		}
	}
	if n, warn := (Config{}).Rounds(); n != 8 || warn != "" {
		t.Errorf("absent loop_max_rounds → (%d,%q), want (8,\"\")", n, warn)
	}
}

// TestToolModeNormalization (TASK-52): tool_mode resolves "" and "native" to
// native and "json" to json, all silently; any unknown value falls back to
// native with an operator warning (never an error). Both tiers share the
// resolver; the warning names the scope.
func TestToolModeNormalization(t *testing.T) {
	cases := []struct {
		raw      string
		wantMode string
		wantWarn bool
	}{
		{"", ToolModeNative, false},       // absent → native, silent (compat)
		{"native", ToolModeNative, false}, // explicit native, silent
		{"json", ToolModeJSON, false},     // fallback, silent
		{"grammar", ToolModeNative, true}, // unknown → native with warning
		{"NATIVE", ToolModeNative, true},  // case-sensitive: unknown → native + warn
	}
	for _, c := range cases {
		lm, lw := LocalConfig{ToolMode: c.raw}.ToolModeResolved()
		if lm != c.wantMode || (lw != "") != c.wantWarn {
			t.Errorf("local ToolModeResolved(%q) = (%q,%q), want mode %q warn=%v", c.raw, lm, lw, c.wantMode, c.wantWarn)
		}
		cm, cw := CloudConfig{ToolMode: c.raw}.ToolModeResolved()
		if cm != c.wantMode || (cw != "") != c.wantWarn {
			t.Errorf("cloud ToolModeResolved(%q) = (%q,%q), want mode %q warn=%v", c.raw, cm, cw, c.wantMode, c.wantWarn)
		}
	}
	// The scope appears in the warning so an operator knows which tier to fix.
	if _, lw := (LocalConfig{ToolMode: "x"}).ToolModeResolved(); !strings.Contains(lw, "local.tool_mode") {
		t.Errorf("local warning missing scope: %q", lw)
	}
	if _, cw := (CloudConfig{ToolMode: "x"}).ToolModeResolved(); !strings.Contains(cw, "cloud.tool_mode") {
		t.Errorf("cloud warning missing scope: %q", cw)
	}
}

// TestNilToolsByteIdentity (TASK-52, provider-wire.md §5c): a Request that
// leaves the new transport fields (Tools/Turns) nil produces a chat-completions
// request body byte-for-byte identical to the pre-feature payload — the
// regression pin protecting every untouched single-shot kind. The expected
// body is rebuilt from the exact pre-feature payload shape (deterministic:
// json.Marshal sorts map keys).
func TestNilToolsByteIdentity(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)

	// preFeatureBody reproduces exactly what openaiCompat.call marshaled before
	// TASK-52 for the given request fields.
	preFeatureBody := func(system, prompt string, maxTokens int64, reasoning string) []byte {
		type msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		var msgs []msg
		if system != "" {
			msgs = append(msgs, msg{Role: "system", Content: system})
		}
		msgs = append(msgs, msg{Role: "user", Content: prompt})
		payload := map[string]any{"model": "m", "messages": msgs, "stream": false}
		if maxTokens > 0 {
			payload["max_tokens"] = maxTokens
		}
		if reasoning != "" {
			payload["reasoning_effort"] = reasoning
		}
		b, _ := json.Marshal(payload)
		return b
	}

	cases := []struct {
		name      string
		reasoning string
		req       Request
	}{
		{"plain", "", Request{Prompt: "hello"}},
		{"system+maxtokens", "", Request{System: "sys", Prompt: "hello", MaxTokens: 256}},
		{"reasoning", "none", Request{Prompt: "hello"}},
		// New fields present but empty must not perturb the wire at all.
		{"empty-new-fields", "", Request{Prompt: "hello", Tools: nil, Turns: nil, SkipObserve: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw = nil
			o := newOpenAICompat(srv.URL, "m", "", c.reasoning, "")
			if _, err := o.call(context.Background(), c.req); err != nil {
				t.Fatal(err)
			}
			want := preFeatureBody(c.req.System, c.req.Prompt, c.req.MaxTokens, c.reasoning)
			if !bytes.Equal(bytes.TrimSpace(raw), want) {
				t.Errorf("body drifted from pre-feature:\n got: %s\nwant: %s", raw, want)
			}
		})
	}
}

// TestConfigParallelRoundTrip (FR-007): llm.json loads with parallel present
// or absent — any integer value, including out-of-range, never fails to load;
// WriteDefault omits the field entirely (default 1).
func TestConfigParallelRoundTrip(t *testing.T) {
	load := func(local string) *Config {
		p := filepath.Join(t.TempDir(), "llm.json")
		data := `{"monthly_budget_usd": 100, "local": ` + local + `, "cloud": {"model": "m"}}`
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(p)
		if err != nil {
			t.Fatalf("LoadConfig(%s): %v", local, err)
		}
		return cfg
	}
	// Present, in range.
	if cfg := load(`{"endpoint": "http://x", "model": "m", "parallel": 4}`); cfg.Local.Parallel != 4 {
		t.Errorf("parallel not loaded: %d", cfg.Local.Parallel)
	}
	// Absent → zero value → Workers() default 1.
	if cfg := load(`{"endpoint": "http://x", "model": "m"}`); cfg.Local.Parallel != 0 {
		t.Errorf("absent parallel should be 0, got %d", cfg.Local.Parallel)
	}
	// Out-of-range values LOAD fine (clamping is Workers()'s job, not load's).
	for _, v := range []int{-2, 64, 100000} {
		cfg := load(`{"endpoint": "http://x", "model": "m", "parallel": ` + strconv.Itoa(v) + `}`)
		if cfg.Local.Parallel != v {
			t.Errorf("parallel %d not preserved on load: %d", v, cfg.Local.Parallel)
		}
	}
	// WriteDefault omits the field: the default config has no parallelism knob.
	p := filepath.Join(t.TempDir(), "llm.json")
	if err := WriteDefault(p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "parallel") {
		t.Errorf("WriteDefault must omit parallel, got:\n%s", raw)
	}
}

// TestCloudOpenAICompat: cloud.provider=openai_compat routes cloud kinds
// through the chat-completions caller with Bearer auth and stream pinned
// false (9router streams by default).
func TestCloudOpenAICompat(t *testing.T) {
	var sawAuth atomic.Bool
	var sawStreamFalse atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		sawAuth.Store(r.Header.Get("Authorization") == "Bearer sk-router-local")
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["stream"].(bool); ok && !v {
			sawStreamFalse.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "router says hi"}}},
			"usage":   map[string]any{"prompt_tokens": 7, "completion_tokens": 3},
		})
	}))
	t.Cleanup(srv.Close)

	cfg := testConfig("http://127.0.0.1:1", srv.URL, 100)
	cfg.Cloud.Provider = ProviderOpenAICompat
	cfg.Cloud.APIKey = "sk-router-local"
	cfg.Cloud.APIKeyEnv = ""
	o := newOrch(t, cfg, testStore(t))

	resp, err := o.Submit(context.Background(), Request{Kind: KindNarrator, Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "router says hi" || resp.Tier != TierCloud {
		t.Errorf("resp = %q via %s", resp.Text, resp.Tier)
	}
	if !sawAuth.Load() {
		t.Error("router never saw the Bearer key")
	}
	if !sawStreamFalse.Load() {
		t.Error("request did not pin stream:false")
	}
}

// TestConfigProviderValidation: unknown providers and openai_compat without
// an endpoint are rejected at load time, not at first call.
func TestConfigProviderValidation(t *testing.T) {
	write := func(cloud string) string {
		p := filepath.Join(t.TempDir(), "llm.json")
		data := `{"monthly_budget_usd": 100, "local": {"endpoint": "http://x", "model": "m"}, "cloud": ` + cloud + `}`
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, err := LoadConfig(write(`{"model": "m", "provider": "sorcery"}`)); err == nil {
		t.Error("unknown provider accepted")
	}
	if _, err := LoadConfig(write(`{"model": "m", "provider": "openai_compat"}`)); err == nil {
		t.Error("openai_compat without endpoint accepted")
	}
	if _, err := LoadConfig(write(`{"model": "m", "provider": "openai_compat", "endpoint": "http://r/v1", "api_key": "k"}`)); err != nil {
		t.Errorf("valid router config rejected: %v", err)
	}
}

// TestKindsCompleteAndRegistered: Kinds() exposes exactly the routing table,
// and every kind resolves to a cognition decision class — the compile-time
// half of the daemon's FR-002 startup gate.
func TestKindsCompleteAndRegistered(t *testing.T) {
	ks := Kinds()
	if len(ks) != len(routing) {
		t.Fatalf("Kinds() returned %d, routing has %d", len(ks), len(routing))
	}
	names := make([]string, 0, len(ks))
	for _, k := range ks {
		if _, ok := routing[k]; !ok {
			t.Errorf("Kinds() includes unrouted kind %q", k)
		}
		names = append(names, string(k))
	}
	if err := cognition.ValidateKinds(names); err != nil {
		t.Errorf("ValidateKinds over all llm kinds: %v", err)
	}
}

// TestTokenBudgetNormalization (spec 025 US2, FR-007/FR-008): each max_tokens
// key normalizes independently to (effective, warning) — absent/0 is the silent
// per-kind default (512/1024/1024), 1..4096 pass verbatim, negatives fall back
// to the default with a warning, and > 4096 clamps to 4096 with a warning.
// Nothing errors. The warning names max_tokens.<key>, the offending value, and
// the effective value.
func TestTokenBudgetNormalization(t *testing.T) {
	cases := []struct {
		name    string
		resolve func(Config) (int64, string)
		key     string
		def     int64
	}{
		{"planner", Config.PlannerTokens, "max_tokens.planner", 512},
		{"metatron_turn", Config.MetatronTurnTokens, "max_tokens.metatron_turn", 1024},
		{"consolidation", Config.ConsolidationTokens, "max_tokens.consolidation", 1024},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mk := func(v int64) Config {
				var b TokenBudgets
				switch tc.name {
				case "planner":
					b.Planner = v
				case "metatron_turn":
					b.MetatronTurn = v
				case "consolidation":
					b.Consolidation = v
				}
				return Config{MaxTokens: &b}
			}
			rows := []struct {
				raw      int64
				wantN    int64
				wantWarn bool
			}{
				{0, tc.def, false},   // absent / explicit 0 → default, silent
				{1, 1, false},        // floor, verbatim
				{2048, 2048, false},  // in-range, verbatim
				{4096, 4096, false},  // cap, verbatim, no warning
				{-5, tc.def, true},   // negative → default with warning
				{4097, 4096, true},   // one past the cap → clamped with warning
				{999999, 4096, true}, // far above → clamped with warning
			}
			for _, r := range rows {
				n, warn := tc.resolve(mk(r.raw))
				if n != r.wantN {
					t.Errorf("%s(%d) n=%d, want %d", tc.name, r.raw, n, r.wantN)
				}
				if (warn != "") != r.wantWarn {
					t.Errorf("%s(%d) warn=%q, wantWarn=%v", tc.name, r.raw, warn, r.wantWarn)
				}
				if warn != "" && !strings.Contains(warn, tc.key) {
					t.Errorf("%s(%d) warning %q does not name %q", tc.name, r.raw, warn, tc.key)
				}
			}
			// Absent field on a zero Config is byte-identical to 0 → default, silent.
			if n, warn := tc.resolve(Config{}); n != tc.def || warn != "" {
				t.Errorf("absent %s → (%d,%q), want (%d,\"\")", tc.name, n, warn, tc.def)
			}
		})
	}
}

// TestTokenBudgetsNormalizeIndependently (spec 025 edge case): the three knobs
// coexist and each normalizes on its own — one clamp warning does not suppress
// another, and a valid sibling stays verbatim while an invalid one warns.
func TestTokenBudgetsNormalizeIndependently(t *testing.T) {
	cfg := Config{MaxTokens: &TokenBudgets{Planner: 768, MetatronTurn: -1, Consolidation: 999999}}
	if n, warn := cfg.PlannerTokens(); n != 768 || warn != "" {
		t.Errorf("planner → (%d,%q), want (768,\"\") verbatim", n, warn)
	}
	if n, warn := cfg.MetatronTurnTokens(); n != 1024 || warn == "" {
		t.Errorf("metatron_turn → (%d,%q), want (1024, warning)", n, warn)
	}
	if n, warn := cfg.ConsolidationTokens(); n != 4096 || warn == "" {
		t.Errorf("consolidation → (%d,%q), want (4096, warning)", n, warn)
	}
}

// TestMaxTokensOmittedWhenAbsent (spec 025 contracts/llm-json.md compatibility):
// a config without max_tokens marshals WITHOUT the key (omitempty) — WriteDefault
// stays minimal and the knob stays opt-in, so every pre-025 world round-trips
// byte-for-byte.
func TestMaxTokensOmittedWhenAbsent(t *testing.T) {
	data, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "max_tokens") {
		t.Errorf("default config emitted max_tokens (should be omitempty): %s", data)
	}
}

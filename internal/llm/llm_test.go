package llm

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/evanstern/script-world/internal/cognition"
	"github.com/evanstern/script-world/internal/store"
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
			APIKeyEnv: "SCRIPTWORLD_TEST_KEY",
		},
	}
}

func newOrch(t *testing.T, cfg Config, st *store.Store) *Orchestrator {
	t.Helper()
	t.Setenv("SCRIPTWORLD_TEST_KEY", "test-key") // hermetic: never depend on the caller's env
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

// TestMusingBestEffort (TASK-21): musings route local and succeed on a quiet
// tier, but are refused immediately (ErrTierBusy) the moment anything is
// waiting — they may never displace real cognition.
func TestMusingBestEffort(t *testing.T) {
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
	if _, err := o.Submit(context.Background(), Request{Kind: KindMusing, Prompt: "musing", BestEffort: true}); !errors.Is(err, ErrTierBusy) {
		t.Fatalf("busy tier must drop best-effort musings: got %v", err)
	}
	// A starved musing (fairness floor) drops BestEffort and queues like
	// any other call — admission must not refuse it.
	done := make(chan error, 1)
	go func() {
		_, err := o.Submit(context.Background(), Request{Kind: KindMusing, Prompt: "starved musing"})
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
	resp, err := o.Submit(context.Background(), Request{Kind: KindMusing, Prompt: "musing", BestEffort: true})
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
	resp, err := o.Submit(context.Background(), Request{Kind: KindMusing, Prompt: "musing", BestEffort: true})
	if err != nil {
		t.Fatalf("best-effort musing with a free slot must be served: %v", err)
	}
	if resp.Tier != TierLocal || resp.Text != "ok" {
		t.Errorf("musing response: %+v", resp)
	}

	// Occupy the remaining three slots → all four busy, queues still empty.
	occupy(slots - 1)
	start := time.Now()
	_, err = o.Submit(context.Background(), Request{Kind: KindMusing, Prompt: "musing", BestEffort: true})
	if !errors.Is(err, ErrTierBusy) {
		t.Fatalf("all slots busy must drop the best-effort musing: got %v", err)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("best-effort refusal must be immediate, took %v", d)
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

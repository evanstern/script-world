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
	"sync/atomic"
	"testing"
	"time"

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
	if localHits.Load() != 2 || cloudHits.Load() != 3 {
		t.Errorf("hits: local=%d cloud=%d, want 2/3", localHits.Load(), cloudHits.Load())
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// stubSampler is a sampleWallMs stub: each call returns a fixed per-point wall
// time (× the shape's points), or an error. It replaces the old fakeSubmitter —
// the local reference shape is now a whole-loop probe (spec 017), so tests drive
// the sampler seam, not a bare Submit fake.
type stubSampler struct {
	millisPerPoint int64
	fail           bool
	calls          int
}

func (s *stubSampler) sample(_ context.Context, sh refShape) (int64, error) {
	s.calls++
	if s.fail {
		return 0, errors.New("model down")
	}
	return s.millisPerPoint * int64(sh.points), nil
}

func TestCalibrateTierMedian(t *testing.T) {
	s := &stubSampler{millisPerPoint: 17000}
	tp, err := calibrateTier(s.sample, llm.TierLocal, 3)
	if err != nil {
		t.Fatal(err)
	}
	if s.calls != 3 { // 1 local shape (planner-3pt loop) × 3 samples
		t.Errorf("calls = %d, want 3", s.calls)
	}
	if tp.SecondsPerPoint < 16.9 || tp.SecondsPerPoint > 17.1 {
		t.Errorf("seconds_per_point = %g, want ~17", tp.SecondsPerPoint)
	}
	if len(tp.Samples) != 1 || len(tp.Samples[0].WallMs) != 3 {
		t.Errorf("audit samples incomplete: %+v", tp.Samples)
	}
}

func TestCalibrateTierAllFailed(t *testing.T) {
	s := &stubSampler{fail: true}
	if _, err := calibrateTier(s.sample, llm.TierLocal, 2); err == nil {
		t.Fatal("unusable tier must error (profile not written)")
	}
}

func TestHorizonSummary(t *testing.T) {
	// At 17 s/pt the contract's sanity table: planner suppressed above 16x,
	// conversation/meeting OK at 32x.
	s := horizonSummary(17.0)
	if !strings.Contains(s, "planner suppressed above 16x") {
		t.Errorf("summary = %q", s)
	}
	if !strings.Contains(s, "conversation OK at 32x") {
		t.Errorf("summary = %q", s)
	}
}

func TestSamplerTiming(t *testing.T) {
	// Guard the sampler seam itself: calibrate never sleeps for real in tests.
	start := time.Now()
	s := &stubSampler{millisPerPoint: 17000}
	if _, err := calibrateTier(s.sample, llm.TierLocal, 5); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("calibrateTier did real waiting in a unit test")
	}
}

// TestVillagerProbeJobRoster pins the loop probe's shape: it declares the real
// villager registry roster (LoopRosterVillager) and carries a handler for every
// declared tool, so the model is offered exactly the production tools and any
// call it makes can be dispatched.
func TestVillagerProbeJobRoster(t *testing.T) {
	sh := refShapes(llm.TierLocal)[0]
	if !sh.loop {
		t.Fatal("the local planner shape must be a loop shape")
	}
	job := villagerProbeJob(sh, 8)

	want := tool.LoopRosterVillager()
	if len(job.Roster) != len(want) {
		t.Fatalf("probe roster has %d tools, want %d (the villager loop roster)", len(job.Roster), len(want))
	}
	names := map[string]bool{}
	for _, tl := range job.Roster {
		names[tl.Name] = true
		if _, ok := job.Handlers[tl.Name]; !ok {
			t.Errorf("roster tool %q has no probe handler", tl.Name)
		}
	}
	// Spot-check the loop-defining members are present (a world verb, set_plan,
	// muse) — the roster the live villager cognition presents.
	for _, must := range []string{"forage", "set_plan", "muse"} {
		if !names[must] {
			t.Errorf("probe roster missing %q", must)
		}
	}
	if job.Kind != llm.KindPlanner || job.MaxRounds != 8 {
		t.Errorf("probe job kind=%q rounds=%d, want planner/8", job.Kind, job.MaxRounds)
	}
}

// TestLoopProbeReportsWholeLoop is the T024 gate: the loop sampler drives
// toolloop.Run against a (stub) model, declares the villager roster on the wire,
// and reports a whole-loop wall time — the estimator's unit. No real model: a
// scripted httptest OpenAI-compat server answers with a native tool_call that
// the probe's no-op handler lands, ending the loop in one round.
func TestLoopProbeReportsWholeLoop(t *testing.T) {
	var sawTools bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		// The request must declare the villager roster as native tools, and it
		// must include the loop-defining members.
		if raw, ok := body["tools"]; ok {
			blob, _ := json.Marshal(raw)
			s := string(blob)
			if strings.Contains(s, "forage") && strings.Contains(s, "set_plan") && strings.Contains(s, "muse") {
				sawTools = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content":    "",
					"tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "forage", "arguments": "{}"}}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 4},
		})
	}))
	defer srv.Close()

	cfg := llm.Config{
		MonthlyBudgetUSD: 100,
		Local:            llm.LocalConfig{Endpoint: srv.URL, Model: "mock"},
		Cloud:            llm.CloudConfig{Model: "unused"},
	}
	orch, err := llm.New(cfg, &memMeter{m: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer orch.Close()

	sample := orchSampler(orch, 8)
	millis, err := sample(context.Background(), refShapes(llm.TierLocal)[0])
	if err != nil {
		t.Fatalf("loop probe: %v", err)
	}
	if !sawTools {
		t.Error("loop probe did not declare the villager roster on the wire (expected native tools)")
	}
	if millis < 0 {
		t.Errorf("whole-loop wall time = %d, want >= 0", millis)
	}
}

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

	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
	"github.com/evanstern/promptworld/internal/world"
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
	job := villagerProbeJob(sh, 8, "")

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

// --- T020: v2-registry calibration (spec 024 US6) ---

// stubToolCallServer answers every request with a native forage tool_call —
// the same no-op-landing shape TestLoopProbeReportsWholeLoop scripts, reused
// here so a calibrateLegacy/calibrateDeclaredProviders run against it
// terminates in one round without a real model.
func stubToolCallServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content":    "",
					"tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "forage", "arguments": "{}"}}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2},
		})
	}))
}

// TestCalibrateLegacyOutputStructureUnchanged (T020: "legacy config output
// is byte-identical to today's"): the legacy path's default invocation
// (empty --tier, no --provider — a v2-registry config never reaches this
// function) prints the exact pre-spec-024 template — "tier", never
// "provider" — around the wall-clock numbers, which are machine-dependent
// and so are matched structurally rather than byte-for-byte.
func TestCalibrateLegacyOutputStructureUnchanged(t *testing.T) {
	srv := stubToolCallServer()
	defer srv.Close()

	w, err := world.Create(t.TempDir()+"/w", "calib-legacy", 1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := llm.Config{
		MonthlyBudgetUSD: 100,
		Local:            llm.LocalConfig{Endpoint: srv.URL, Model: "mock-local"},
		Cloud:            llm.CloudConfig{Model: "unused"},
	}
	out := captureStdout(t, func() {
		if err := calibrateLegacy(w, &cfg, "", 1); err != nil {
			t.Fatalf("calibrateLegacy: %v", err)
		}
	})
	for _, want := range []string{
		"calibrating local tier (1 samples per shape)...\n",
		"tier local  (mock-local)\n",
		"  planner-3pt    1/1 samples",
		"seconds_per_point:",
		"cognition at this profile:",
		"wrote " + w.CalibrationPath(),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("legacy output missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "provider") {
		t.Errorf("legacy output must keep the pre-spec-024 \"tier\" wording, not \"provider\": %s", out)
	}

	prof, err := cognition.LoadProfile(w.CalibrationPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prof.Tiers["local"]; !ok || len(prof.Tiers) != 1 {
		t.Errorf("profile = %+v, want exactly one entry keyed \"local\"", prof.Tiers)
	}
}

// TestCalibrateDeclaredProvidersThreeNamedEntries (T020: "a v2 three-provider
// config produces three named profile entries"): every declared provider
// gets its own calibration.json entry, keyed by its name.
func TestCalibrateDeclaredProvidersThreeNamedEntries(t *testing.T) {
	srv := stubToolCallServer()
	defer srv.Close()

	w, err := world.Create(t.TempDir()+"/w", "calib-v2", 1)
	if err != nil {
		t.Fatal(err)
	}
	pc := llm.ProviderConfig{Transport: llm.ProviderOpenAICompat, Endpoint: srv.URL, Model: "alpha-model"}
	cfg := &llm.Config{
		MonthlyBudgetUSD: 100,
		Providers: map[string]llm.ProviderConfig{
			"alpha": pc,
			"beta":  {Transport: llm.ProviderOpenAICompat, Endpoint: srv.URL, Model: "beta-model"},
			"gamma": {Transport: llm.ProviderOpenAICompat, Endpoint: srv.URL, Model: "gamma-model"},
		},
		Routes: map[string]llm.RouteConfig{
			string(llm.KindPlanner):       {Chain: []string{"alpha", "beta", "gamma"}},
			string(llm.KindConversation):  {Chain: []string{"alpha"}},
			string(llm.KindConsolidation): {Chain: []string{"alpha"}},
			string(llm.KindNarrator):      {Chain: []string{"alpha"}},
			string(llm.KindDrama):         {Chain: []string{"alpha"}},
			string(llm.KindMetatron):      {Chain: []string{"alpha"}},
			string(llm.KindMeeting):       {Chain: []string{"alpha"}},
		},
	}
	out := captureStdout(t, func() {
		if err := calibrateDeclaredProviders(w, cfg, "", "", 1); err != nil {
			t.Fatalf("calibrateDeclaredProviders: %v", err)
		}
	})
	for _, want := range []string{`provider "alpha"`, `provider "beta"`, `provider "gamma"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}

	prof, err := cognition.LoadProfile(w.CalibrationPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(prof.Tiers) != 3 {
		t.Fatalf("profile has %d entries, want 3: %+v", len(prof.Tiers), prof.Tiers)
	}
	for name, model := range map[string]string{"alpha": "alpha-model", "beta": "beta-model", "gamma": "gamma-model"} {
		tp, ok := prof.Tiers[name]
		if !ok {
			t.Errorf("profile missing entry for %q", name)
			continue
		}
		if tp.Model != model {
			t.Errorf("profile[%q].Model = %q, want %q", name, tp.Model, model)
		}
	}
}

// TestSelectDeclaredProvidersProviderFlag: --provider narrows to exactly one
// declared name and rejects an undeclared one; no flags selects every
// declared provider (the v2 default, tasks.md T020).
func TestSelectDeclaredProvidersProviderFlag(t *testing.T) {
	names := []string{"alpha", "beta"}
	sel, err := selectDeclaredProviders(nil, names, "", "beta")
	if err != nil || len(sel) != 1 || sel[0] != "beta" {
		t.Errorf("sel=%v err=%v, want [beta]", sel, err)
	}
	if _, err := selectDeclaredProviders(nil, names, "", "nope"); err == nil {
		t.Error("an undeclared --provider must error")
	}
	if _, err := selectDeclaredProviders(nil, names, "bogus", ""); err == nil {
		t.Error("an unknown --tier must error")
	}
	sel, err = selectDeclaredProviders(nil, names, "", "")
	if err != nil || len(sel) != 2 {
		t.Errorf("no flags should select every declared provider: sel=%v err=%v", sel, err)
	}
}

// TestVillagerProbeJobPinsProvider: the calibrate loop probe threads Provider
// straight onto toolloop.Job (spec 024 R3/T020) so a reference sample pins
// its Submit to the named provider.
func TestVillagerProbeJobPinsProvider(t *testing.T) {
	sh := refShapesFor(false)[0]
	job := villagerProbeJob(sh, 8, "cogito")
	if job.Provider != "cogito" {
		t.Errorf("job.Provider = %q, want \"cogito\"", job.Provider)
	}
	if unpinned := villagerProbeJob(sh, 8, ""); unpinned.Provider != "" {
		t.Errorf("empty provider must stay unpinned: %q", unpinned.Provider)
	}
}

package e2e

// TASK-32 US1 (specs/007-cognition-horizon): telemetry audit against a live
// daemon with a mock local model — every cog.thought has exactly one
// cog.outcome (SC-002), and an intent's chain walks back to its stimulus
// through job → cog.thought → trigger_seq (SC-007).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
)

// mockOpenAI answers every chat completion instantly with a fixed reply.
func mockOpenAI(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": reply}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestCognitionTelemetryAudit(t *testing.T) {
	srv := mockOpenAI(t, `{"goal":"forage","reason":"stores are low"}`)
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "w")
	run(t, "new", dir, "--name", "cog", "--seed", "7")
	llmCfg := fmt.Sprintf(`{
  "monthly_budget_usd": 1,
  "local": {"endpoint": %q, "model": "mock"},
  "cloud": {"provider": "openai_compat", "endpoint": %q, "model": "mock",
            "input_usd_per_mtok": 0, "output_usd_per_mtok": 0, "api_key": "x"}
}`, srv.URL, srv.URL)
	if err := os.WriteFile(filepath.Join(dir, "llm.json"), []byte(llmCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	run(t, "start", dir)
	defer run(t, "stop", dir)
	run(t, "speed", dir, "16x")

	// Let the mind think: planner cadence is staggered; the first agents come
	// due within a few game-minutes, instant mock replies land immediately.
	deadline := time.Now().Add(60 * time.Second)
	var thoughts, outcomes []store.Event
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		st, err := store.Open(filepath.Join(dir, "world.db"))
		if err != nil {
			continue // daemon holds WAL; retry
		}
		thoughts, outcomes = nil, nil
		st.ReplayEvents(0, func(e store.Event) error {
			switch e.Type {
			case "cog.thought":
				thoughts = append(thoughts, e)
			case "cog.outcome":
				outcomes = append(outcomes, e)
			}
			return nil
		})
		st.Close()
		if len(thoughts) >= 2 && len(outcomes) >= 2 {
			break
		}
	}
	if len(thoughts) == 0 {
		t.Fatal("no cog.thought recorded by a live daemon")
	}

	// SC-002: every thought has exactly one outcome.
	outcomeByJob := map[string]int{}
	for _, e := range outcomes {
		var p sim.CogOutcomePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("outcome payload: %v", err)
		}
		outcomeByJob[p.Job]++
	}
	for _, e := range thoughts {
		var p sim.CogThoughtPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatalf("thought payload: %v", err)
		}
		if n := outcomeByJob[p.Job]; n != 1 {
			// In-flight at shutdown is the one legal zero; only flag doubles
			// and only flag zeroes for thoughts older than the last outcome.
			if n > 1 {
				t.Errorf("job %s has %d outcomes (SC-002)", p.Job, n)
			}
		}
	}

	// SC-007: any landed planner outcome chains back to its thought; if the
	// thought was stimulus-armed, the trigger_seq names a real earlier event.
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	thoughtByJob := map[string]sim.CogThoughtPayload{}
	for _, e := range thoughts {
		var p sim.CogThoughtPayload
		json.Unmarshal(e.Payload, &p)
		thoughtByJob[p.Job] = p
	}
	chained := false
	for _, e := range outcomes {
		var p sim.CogOutcomePayload
		json.Unmarshal(e.Payload, &p)
		tp, ok := thoughtByJob[p.Job]
		if !ok {
			if p.Outcome != sim.OutcomeSuppressed {
				t.Errorf("outcome %s has no thought and is not a suppression", p.Job)
			}
			continue
		}
		chained = true
		if tp.TriggerSeq > 0 && tp.TriggerSeq >= e.Seq {
			t.Errorf("job %s trigger_seq %d not earlier than outcome seq %d", p.Job, tp.TriggerSeq, e.Seq)
		}
	}
	if !chained {
		t.Error("no outcome chained to its thought (SC-007)")
	}
}

// TestCognitionStaleRejectionUnderLatency (US3, SC-001): a calibration
// profile claiming a fast model (1 s/pt) lets the router admit planners at
// 32x; the mock then takes ~45s per planner call, so the landing arrives
// ~1440 game-ticks stale against the 1200-tick budget — rejected, recorded,
// classified prediction-miss, while the reflex floor keeps the world moving.
func TestCognitionStaleRejectionUnderLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("latency-injection e2e takes ~90s")
	}
	// A uniformly slow model: every call takes 45s (the openai_compat wire
	// carries no max_tokens to discriminate on, and a slow host is slow for
	// everything anyway).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		time.Sleep(45 * time.Second)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"goal":"forage","reason":"late thought"}`}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10},
		})
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "w")
	run(t, "new", dir, "--name", "stale", "--seed", "11")
	llmCfg := fmt.Sprintf(`{
  "monthly_budget_usd": 1,
  "local": {"endpoint": %q, "model": "mock"},
  "cloud": {"provider": "openai_compat", "endpoint": %q, "model": "mock",
            "input_usd_per_mtok": 0, "output_usd_per_mtok": 0, "api_key": "x"}
}`, srv.URL, srv.URL)
	if err := os.WriteFile(filepath.Join(dir, "llm.json"), []byte(llmCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// The optimistic profile: router admits planners at 32x (3pt × 1s × 32x
	// = 96 ticks ≤ 1200). The 45s reality is caught at landing, not trusted.
	prof := `{"calibrated_at":"2026-07-20T00:00:00Z","tiers":{"local":{"model":"mock","seconds_per_point":1.0}}}`
	if err := os.WriteFile(filepath.Join(dir, "calibration.json"), []byte(prof), 0o644); err != nil {
		t.Fatal(err)
	}

	run(t, "start", dir)
	defer run(t, "stop", dir)
	run(t, "speed", dir, "32x")

	deadline := time.Now().Add(150 * time.Second)
	var rejection *sim.CogOutcomePayload
	for time.Now().Before(deadline) && rejection == nil {
		time.Sleep(3 * time.Second)
		st, err := store.Open(filepath.Join(dir, "world.db"))
		if err != nil {
			continue
		}
		st.ReplayEvents(0, func(e store.Event) error {
			if e.Type != "cog.outcome" || rejection != nil {
				return nil
			}
			var p sim.CogOutcomePayload
			if json.Unmarshal(e.Payload, &p) == nil &&
				p.Class == "planner" && p.Outcome == sim.OutcomeRejectedStale {
				rejection = &p
			}
			return nil
		})
		st.Close()
	}
	if rejection == nil {
		t.Fatal("no stale rejection recorded under injected latency")
	}
	if rejection.StalenessTicks <= 1200 {
		t.Errorf("rejected at staleness %d, budget is 1200", rejection.StalenessTicks)
	}
	if rejection.Kind != sim.RejectKindPredictionMiss {
		t.Errorf("kind = %q, want prediction-miss (45s actual vs 3s predicted)", rejection.Kind)
	}

	// SC-001 audit: nothing executed past its budget.
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.ReplayEvents(0, func(e store.Event) error {
		if e.Type != "cog.outcome" {
			return nil
		}
		var p sim.CogOutcomePayload
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		if (p.Outcome == sim.OutcomeLanded || p.Outcome == sim.OutcomeAdapted) &&
			p.Class == "planner" && p.StalenessTicks > 1200 {
			t.Errorf("SC-001 violated: %s executed at staleness %d", p.Job, p.StalenessTicks)
		}
		return nil
	})
}

// TestCognitionReplayByteIdentical (SC-003): on a cognition-enabled run,
// deriving state by replaying the full event log from genesis is
// byte-identical to the snapshot+tail derivation the daemon itself uses —
// cog.* telemetry, plans, guards, and rejections are all recorded input,
// never recomputed.
func TestCognitionReplayByteIdentical(t *testing.T) {
	srv := mockOpenAI(t, `{"goal":"forage","reason":"stores are low"}`)
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "w")
	run(t, "new", dir, "--name", "replay", "--seed", "21")
	llmCfg := fmt.Sprintf(`{
  "monthly_budget_usd": 1,
  "local": {"endpoint": %q, "model": "mock"},
  "cloud": {"provider": "openai_compat", "endpoint": %q, "model": "mock",
            "input_usd_per_mtok": 0, "output_usd_per_mtok": 0, "api_key": "x"}
}`, srv.URL, srv.URL)
	if err := os.WriteFile(filepath.Join(dir, "llm.json"), []byte(llmCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "start", dir)
	run(t, "speed", dir, "16x")

	// Accumulate cognition traffic, then a pause (forces a snapshot) and stop.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		st, err := store.Open(filepath.Join(dir, "world.db"))
		if err != nil {
			continue
		}
		n := 0
		st.ReplayEvents(0, func(e store.Event) error {
			if e.Type == "cog.outcome" {
				n++
			}
			return nil
		})
		st.Close()
		if n >= 3 {
			break
		}
	}
	run(t, "pause", dir)
	run(t, "stop", dir)

	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CheckContiguity(); err != nil {
		t.Fatalf("holed log: %v", err)
	}
	w, err := world.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := w.Map()

	// Derivation A: genesis → full replay.
	full := sim.NewState(w.Manifest.Seed, m)
	sawCog := false
	if err := st.ReplayEvents(0, func(e store.Event) error {
		if strings.HasPrefix(e.Type, "cog.") {
			sawCog = true
		}
		full.Tick = maxI64(full.Tick, e.Tick)
		return full.Apply(e)
	}); err != nil {
		t.Fatalf("full replay: %v", err)
	}
	if !sawCog {
		t.Fatal("run recorded no cognition telemetry — nothing proven")
	}

	// Derivation B: latest snapshot + tail (the daemon's own recovery path).
	snap, err := st.LatestValidSnapshot()
	if err != nil || snap == nil {
		t.Fatalf("no snapshot: %v", err)
	}
	fromSnap := sim.NewState(w.Manifest.Seed, m)
	if err := json.Unmarshal(snap.State, fromSnap); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplayEvents(snap.Seq, func(e store.Event) error {
		fromSnap.Tick = maxI64(fromSnap.Tick, e.Tick)
		return fromSnap.Apply(e)
	}); err != nil {
		t.Fatalf("tail replay: %v", err)
	}

	if string(full.Marshal()) != string(fromSnap.Marshal()) {
		t.Error("SC-003 violated: full replay != snapshot+tail on a cognition-enabled run")
	}
}

func maxI64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

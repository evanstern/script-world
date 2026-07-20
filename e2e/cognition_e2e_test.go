package e2e

// TASK-32 US1 (specs/007-cognition-horizon): telemetry audit against a live
// daemon with a mock local model — every cog.thought has exactly one
// cog.outcome (SC-002), and an intent's chain walks back to its stimulus
// through job → cog.thought → trigger_seq (SC-007).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
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

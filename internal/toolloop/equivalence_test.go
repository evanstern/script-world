package toolloop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
)

// TestToolModeEquivalenceNativeVsJSON is the fallback-mode equivalence harness
// (spec 017 US4 scenario 3 / FR-010): "engaging [the json fallback] MUST still
// satisfy FR-003 through FR-008 (bounded, gated, recorded, correlatable)".
// FR-010's guarantee lives at the layer where the two wire modes actually
// diverge — the provider transport — so this drives a REAL llm.Orchestrator
// (not the stubOrch scripted submitter loop_test.go uses) against two scripted
// httptest OpenAI-compat servers: one speaking native tool_calls
// (provider-wire.md §2), one speaking the tool_mode:"json" envelope
// (provider-wire.md §3). Both script the same logical cognition — round 1 a
// read-tool call fed back, round 2 the acting call lands — through the same
// toolloop.Run entry point with an identical Job (roster, handlers, records
// sink), and asserts the loop reports the same CallRecords, Termination,
// Final, and round count regardless of which wire shape produced them, while
// independently confirming the two servers really did see different wire
// shapes (tools+tool_calls vs. response_format+envelope) — the sanity check
// that this is testing a genuine fallback, not a no-op knob.
func TestToolModeEquivalenceNativeVsJSON(t *testing.T) {
	forage := lookup(t, "forage")

	var nativeReqs, jsonReqs []map[string]any
	nativeSrv := nativeEquivServer(t, &nativeReqs)
	jsonSrv := jsonEquivServer(t, &jsonReqs)

	nativeOrch := newEquivOrch(t, nativeSrv.URL, llm.ToolModeNative)
	jsonOrch := newEquivOrch(t, jsonSrv.URL, llm.ToolModeJSON)

	newJob := func(sink *[]CallRecord) Job {
		return Job{
			JobID:     "planner-ada-999900",
			Kind:      llm.KindPlanner,
			System:    "you are ada",
			Seed:      "what next?",
			Roster:    []tool.Tool{readTool, forage},
			Handlers:  map[string]Handler{"peek": readHandler("berries nearby"), "forage": landHandler("foraging")},
			MaxRounds: 4,
			Record:    func(r CallRecord) { *sink = append(*sink, r) },
		}
	}

	var nativeRecs, jsonRecs []CallRecord
	nativeRes, nativeErr := Run(context.Background(), nativeOrch, newJob(&nativeRecs))
	jsonRes, jsonErr := Run(context.Background(), jsonOrch, newJob(&jsonRecs))

	if nativeErr != nil || jsonErr != nil {
		t.Fatalf("native err=%v json err=%v, want both nil", nativeErr, jsonErr)
	}

	// --- equivalence: everything the loop reports ---
	if nativeRes.Term != TermLanded || jsonRes.Term != TermLanded {
		t.Fatalf("term native=%q json=%q, want landed/landed", nativeRes.Term, jsonRes.Term)
	}
	if nativeRes.Rounds != jsonRes.Rounds {
		t.Errorf("rounds native=%d json=%d, want equal", nativeRes.Rounds, jsonRes.Rounds)
	}
	if nativeRes.Final != jsonRes.Final {
		t.Errorf("final native=%q json=%q, want equal", nativeRes.Final, jsonRes.Final)
	}
	if nativeRes.Landed == nil || jsonRes.Landed == nil || nativeRes.Landed.Name != jsonRes.Landed.Name {
		t.Fatalf("landed native=%+v json=%+v, want the same tool name (IDs may differ)", nativeRes.Landed, jsonRes.Landed)
	}
	if string(nativeRes.Landed.Args) != string(jsonRes.Landed.Args) {
		t.Errorf("landed args differ: native=%s json=%s", nativeRes.Landed.Args, jsonRes.Landed.Args)
	}

	if len(nativeRecs) != len(jsonRecs) {
		t.Fatalf("record count native=%d json=%d, want equal", len(nativeRecs), len(jsonRecs))
	}
	for i := range nativeRecs {
		n, j := nativeRecs[i], jsonRecs[i]
		// CallRecord carries no wire-level call ID (record.go: JobID, Ordinal,
		// Tool, Args, Verdict, Reason, Tier) — the two modes' records are
		// directly comparable; ID divergence is confined to Landed above.
		if n.JobID != j.JobID || n.Ordinal != j.Ordinal || n.Tool != j.Tool ||
			n.Verdict != j.Verdict || n.Reason != j.Reason || n.Tier != j.Tier {
			t.Errorf("record %d differs:\n native=%+v\n json=%+v", i, n, j)
		}
		if string(n.Args) != string(j.Args) {
			t.Errorf("record %d args differ: native=%s json=%s", i, n.Args, j.Args)
		}
	}

	// --- sanity: the two paths really diverged on the wire ---
	if len(nativeReqs) != 2 || len(jsonReqs) != 2 {
		t.Fatalf("native saw %d requests, json saw %d, want 2/2", len(nativeReqs), len(jsonReqs))
	}
	for i, req := range nativeReqs {
		if _, ok := req["tools"]; !ok {
			t.Errorf("native round %d: missing tools field", i+1)
		}
		if _, ok := req["response_format"]; ok {
			t.Errorf("native round %d: unexpected response_format (native+tools must never mix)", i+1)
		}
	}
	for i, req := range jsonReqs {
		if _, ok := req["response_format"]; !ok {
			t.Errorf("json round %d: missing response_format envelope", i+1)
		}
		if _, ok := req["tools"]; ok {
			t.Errorf("json round %d: unexpected native tools field", i+1)
		}
	}

	// Round 2's native request must echo round 1 as an assistant tool_calls
	// message and carry the read result as a role:"tool" message.
	round2Native := nativeReqs[1]["messages"].([]any)
	var sawToolCallsEcho, sawToolResult bool
	for _, m := range round2Native {
		mm := m.(map[string]any)
		if mm["role"] == "assistant" {
			if _, ok := mm["tool_calls"]; ok {
				sawToolCallsEcho = true
			}
		}
		if mm["role"] == "tool" {
			sawToolResult = true
		}
	}
	if !sawToolCallsEcho || !sawToolResult {
		t.Errorf("native round 2 did not echo tool_calls / a tool result message: %v", round2Native)
	}

	// Round 2's json request must carry the tool catalog in the system prompt
	// and the round-1 result as plain user text — never a role:"tool" message.
	round2JSON := jsonReqs[1]["messages"].([]any)
	var sawSystemCatalog, sawPlainResult bool
	for _, m := range round2JSON {
		mm := m.(map[string]any)
		if mm["role"] == "system" {
			if s, ok := mm["content"].(string); ok && strings.Contains(s, "peek") {
				sawSystemCatalog = true
			}
		}
		if mm["role"] == "tool" {
			t.Errorf("json mode must never send role:tool messages: %v", mm)
		}
		if s, ok := mm["content"].(string); ok && strings.Contains(s, "Tool result (peek)") {
			sawPlainResult = true
		}
	}
	if !sawSystemCatalog || !sawPlainResult {
		t.Errorf("json round 2 missing system tool catalog / plain-text result feedback: %v", round2JSON)
	}
}

// nativeEquivServer scripts the native-mode round-trip: round 1 replies with a
// tool_calls read ("peek"); round 2 replies with a tool_calls landing act
// ("forage"). Every decoded request body is appended to *reqs so the caller
// can assert on the wire shape after the run.
func nativeEquivServer(t *testing.T, reqs *[]map[string]any) *httptest.Server {
	t.Helper()
	round := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		*reqs = append(*reqs, body)
		round++
		w.Header().Set("Content-Type", "application/json")
		switch round {
		case 1:
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": "",
						"tool_calls": []map[string]any{
							{"id": "c1", "type": "function", "function": map[string]any{"name": "peek", "arguments": "{}"}},
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2},
			})
		case 2:
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": "digging in",
						"tool_calls": []map[string]any{
							{"id": "c2", "type": "function", "function": map[string]any{"name": "forage", "arguments": "{}"}},
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4},
			})
		default:
			t.Errorf("native server: unexpected round %d", round)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// jsonEquivServer scripts the identical logical cognition through the
// tool_mode:"json" envelope: round 1's content is the envelope calling "peek",
// round 2's is the envelope landing "forage" with the same closing text the
// native server produced, so Result.Final matches exactly.
func jsonEquivServer(t *testing.T, reqs *[]map[string]any) *httptest.Server {
	t.Helper()
	round := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		*reqs = append(*reqs, body)
		round++
		var content string
		switch round {
		case 1:
			content = `{"tool":"peek","args":{}}`
		case 2:
			content = `{"tool":"forage","args":{},"say":"digging in"}`
		default:
			t.Errorf("json server: unexpected round %d", round)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 2},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// memMeter is a minimal in-memory llm.MeterStore — this test never bills a
// cloud call, but Orchestrator construction requires one.
type memMeter struct{ m map[string]string }

func newMemMeter() *memMeter { return &memMeter{m: map[string]string{}} }

func (s *memMeter) GetMeta(key string) (string, error) { return s.m[key], nil }
func (s *memMeter) SetMeta(key, value string) error    { s.m[key] = value; return nil }

// newEquivOrch builds a real *llm.Orchestrator whose local tier (KindPlanner's
// route) points at the given openai_compat server with the given tool_mode.
func newEquivOrch(t *testing.T, endpoint, toolMode string) *llm.Orchestrator {
	t.Helper()
	cfg := llm.Config{
		MonthlyBudgetUSD: 100,
		Local:            llm.LocalConfig{Endpoint: endpoint, Model: "test-model", ToolMode: toolMode},
		Cloud:            llm.CloudConfig{Model: "unused"},
	}
	o, err := llm.New(cfg, newMemMeter())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Close)
	return o
}

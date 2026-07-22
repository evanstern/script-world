package mind

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// testLoopRounds is the loop iteration cap harness minds run with.
const testLoopRounds = 8

// replyToCalls translates a legacy JSON planner reply (the string form the mind
// tests script) into the tool call the tool-use loop would receive, so migrated
// tests keep scripting behavior by intent. A {"goal":...} reply becomes one
// world-verb (or muse, via {"muse":...}) call; a {"plan":[...]} reply becomes a
// set_plan call; anything unparseable becomes no calls (the model answered in
// prose — model_done, which the mind records as unusable).
func replyToCalls(reply string) []llm.ToolCall {
	trimmed := strings.TrimSpace(reply)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var r struct {
		Goal   string `json:"goal"`
		Target string `json:"target"`
		Kind   string `json:"kind"`
		Qty    int    `json:"qty"`
		Muse   string `json:"muse"`
		Plan   []struct {
			Goal string `json:"goal"`
			Kind string `json:"kind"`
			Qty  int    `json:"qty"`
		} `json:"plan"`
	}
	if json.Unmarshal([]byte(trimmed), &r) != nil {
		return nil
	}
	switch {
	case len(r.Plan) > 0:
		steps := make([]map[string]any, 0, len(r.Plan))
		for _, s := range r.Plan {
			step := map[string]any{"goal": s.Goal}
			if s.Kind != "" {
				step["kind"] = s.Kind
			}
			if s.Qty != 0 {
				step["qty"] = s.Qty
			}
			steps = append(steps, step)
		}
		args, _ := json.Marshal(map[string]any{"steps": steps})
		return []llm.ToolCall{{ID: "c1", Name: "set_plan", Args: args}}
	case r.Muse != "":
		args, _ := json.Marshal(map[string]any{"text": r.Muse})
		return []llm.ToolCall{{ID: "c1", Name: "muse", Args: args}}
	case r.Goal != "":
		m := map[string]any{}
		if r.Target != "" {
			m["target"] = r.Target
		}
		if r.Kind != "" {
			m["kind"] = r.Kind
		}
		if r.Qty != 0 {
			m["qty"] = r.Qty
		}
		args, _ := json.Marshal(m)
		return []llm.ToolCall{{ID: "c1", Name: strings.ToLower(strings.TrimSpace(r.Goal)), Args: args}}
	}
	return nil
}

// mockLoop is the stub runLoop for harness minds: it drives the model through
// the Submitter seam (so planGate / call counting / errors still work), then
// dispatches the single translated tool call through the real handlers (real
// door). It is deliberately simple — the real toolloop.Run is covered by
// internal/toolloop and the e2e; this exercises the mind's handlers + outcome
// mapping against a scripted reply.
func mockLoop(model Submitter) func(context.Context, toolloop.Job) (toolloop.Result, error) {
	return func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		resp, err := model.Submit(ctx, llm.Request{Kind: j.Kind, System: j.System, Prompt: j.Seed})
		if err != nil {
			term := toolloop.TermProviderError
			if err == context.DeadlineExceeded || err == context.Canceled {
				term = toolloop.TermCtxDone
			}
			return toolloop.Result{Term: term}, err
		}
		calls := replyToCalls(resp.Text)
		if len(calls) == 0 {
			return toolloop.Result{Term: toolloop.TermModelDone}, nil
		}
		c := calls[0]
		h := j.Handlers[c.Name]
		if h == nil {
			j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: c.Name, Verdict: toolloop.VerdictRejectedUnknown})
			return toolloop.Result{Term: toolloop.TermModelDone}, nil
		}
		out := h(ctx, c)
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: c.Name, Verdict: out.Verdict, Reason: out.ResultForModel})
		switch {
		case out.Err != nil:
			return toolloop.Result{Term: toolloop.TermProviderError}, out.Err
		case out.Verdict == toolloop.VerdictLanded:
			cc := c
			return toolloop.Result{Term: toolloop.TermLanded, Landed: &cc}, nil
		default:
			// Rejected/unknown but never landed; the simple stub does not retry.
			return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
		}
	}
}

// noopLoop drops every planner cognition without touching the model — installed
// on minds whose tests never intend the planner to run (they drive
// conversation/consolidation directly) so a stray cadence tick can't consume a
// scripted convo reply.
func noopLoop(context.Context, toolloop.Job) (toolloop.Result, error) {
	return toolloop.Result{Term: toolloop.TermModelDone}, nil
}

// mockModel is a Submitter returning canned planner replies (or errors).
// Musing calls answer musingReply; empty means the busy-drop path (the
// orchestrator's best-effort admission), so planner-focused tests are
// untouched by the musing cadence.
type mockModel struct {
	mu          sync.Mutex
	calls       atomic.Int64
	reply       string
	musingReply string
	narrReply   string // narrator calls; empty = tier down (carry path)
	err         error
	prompts     []string
	kinds       []llm.Kind
	// planGate, when set, blocks planner calls until closed — the in-flight
	// thought for pause-semantics tests (TASK-32 US5).
	planGate chan struct{}
}

func (m *mockModel) Submit(_ context.Context, req llm.Request) (llm.Response, error) {
	m.calls.Add(1)
	m.mu.Lock()
	gate := m.planGate
	m.mu.Unlock()
	if gate != nil && req.Kind == llm.KindPlanner {
		<-gate
	}
	m.mu.Lock()
	m.prompts = append(m.prompts, req.Prompt)
	m.kinds = append(m.kinds, req.Kind)
	reply, err := m.reply, m.err
	if req.Kind == llm.KindMusing {
		reply = m.musingReply
		if reply == "" {
			err = llm.ErrTierBusy
		}
	}
	if req.Kind == llm.KindNarrator {
		reply = m.narrReply
		if reply == "" {
			err = llm.ErrTierDown
		}
	}
	m.mu.Unlock()
	if err != nil {
		return llm.Response{}, err
	}
	return llm.Response{Text: reply, Tier: llm.TierLocal, Model: "mock"}, nil
}

func (m *mockModel) lastPrompts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.prompts...)
}

// harness: real store + loop at max speed, mind wired through notify.
type harness struct {
	st    *store.Store
	loop  *sim.Loop
	mind  *Mind
	model *mockModel
	m     *worldmap.Map
	done  chan error
}

func newHarness(t *testing.T, reply string) *harness {
	return newHarnessAt(t, reply, "max")
}

// newHarnessAt runs the loop at a real speed — routing tests need a finite
// ticks-per-second (max bypasses the router; production refuses max+LLM).
func newHarnessAt(t *testing.T, reply string, speed clock.Speed) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	state.Speed = speed

	model := &mockModel{reply: reply}
	h := &harness{st: st, model: model, m: m, done: make(chan error, 1)}

	var notifyMu sync.Mutex
	var consumers []func([]store.Event)
	notify := func(evs []store.Event) {
		notifyMu.Lock()
		cs := make([]func([]store.Event), len(consumers))
		copy(cs, consumers)
		notifyMu.Unlock()
		for _, c := range cs {
			c(evs)
		}
	}
	h.loop = sim.NewLoop(state, m, st, notify)

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{}, testLoopRounds, mockLoop(model))
	if err != nil {
		t.Fatal(err)
	}
	h.mind = md
	notifyMu.Lock()
	consumers = append(consumers, md.Observe)
	notifyMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go func() { h.done <- h.loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("loop did not stop")
		}
		md.Close()
		st.Close()
	})
	return h
}

func (h *harness) waitEvents(t *testing.T, timeout time.Duration, match func(store.Event) bool) []store.Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var found []store.Event
	seen := int64(0)
	for time.Now().Before(deadline) {
		evs, err := h.st.EventsSince(seen, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			seen = e.Seq
			if match(e) {
				found = append(found, e)
			}
		}
		if len(found) > 0 {
			return found
		}
		time.Sleep(30 * time.Millisecond)
	}
	return found
}

// TestMusingsInjectThoughts is TASK-21 AC#1/#3: on their own cadence, agents
// emit recorded agent.thought events with source "musing" that never carry a
// goal — pure interiority landing through the social injection door.
func TestMusingsInjectThoughts(t *testing.T) {
	h := newHarness(t, `{"goal": "wander", "reason": "Stretching my legs."}`)
	h.model.mu.Lock()
	h.model.musingReply = "The wind smells like rain tonight."
	h.model.mu.Unlock()

	musings := h.waitEvents(t, 15*time.Second, func(e store.Event) bool {
		if e.Type != "agent.thought" {
			return false
		}
		var p sim.ThoughtPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "musing"
	})
	if len(musings) == 0 {
		t.Fatal("no musing thoughts appeared")
	}
	var p sim.ThoughtPayload
	json.Unmarshal(musings[0].Payload, &p)
	if p.Text != "The wind smells like rain tonight." {
		t.Errorf("musing text: %q", p.Text)
	}
	if p.Agent < 0 || p.Agent >= sim.AgentCount {
		t.Errorf("musing agent out of range: %d", p.Agent)
	}
}

// TestMusingDropsAreSilent is TASK-21 AC#2: a busy tier (ErrTierBusy) drops
// the musing without retry — no thought events, no intent disturbance, and
// the planner keeps working.
func TestMusingDropsAreSilent(t *testing.T) {
	h := newHarness(t, `{"goal": "wander", "reason": "Stretching my legs."}`) // musingReply empty → every musing drops busy

	intents := h.waitEvents(t, 15*time.Second, func(e store.Event) bool {
		if e.Type != "agent.intent_set" {
			return false
		}
		var p sim.IntentSetPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "planner"
	})
	if len(intents) == 0 {
		t.Fatal("planner starved while musings were dropping")
	}
	musings := h.waitEvents(t, 2*time.Second, func(e store.Event) bool {
		if e.Type != "agent.thought" {
			return false
		}
		var p sim.ThoughtPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "musing"
	})
	if len(musings) != 0 {
		t.Fatalf("dropped musings still produced %d thoughts", len(musings))
	}
}

// TestParseMusing covers the reply hygiene: plain line in, JSON and empties out.
func TestParseMusing(t *testing.T) {
	if got, err := parseMusing("  \"I miss the sound of the river.\"  \nsecond line"); err != nil || got != "I miss the sound of the river." {
		t.Errorf("parseMusing: %q %v", got, err)
	}
	for _, bad := range []string{"", "   ", `{"goal": "wander"}`} {
		if _, err := parseMusing(bad); err == nil {
			t.Errorf("parseMusing(%q): expected error", bad)
		}
	}
}

// TestPlannerDrivesAgents is AC#1: with a working model, planner-sourced
// intents appear (cadence + triggers) and the executor acts on them.
func TestPlannerDrivesAgents(t *testing.T) {
	h := newHarness(t, `{"goal": "wander", "reason": "Stretching my legs."}`)

	intents := h.waitEvents(t, 15*time.Second, func(e store.Event) bool {
		if e.Type != "agent.intent_set" {
			return false
		}
		var p sim.IntentSetPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "planner"
	})
	if len(intents) == 0 {
		t.Fatal("no planner-sourced intents appeared")
	}

	if h.model.calls.Load() == 0 {
		t.Fatal("model never called")
	}
	// Tool-era note: world-verb landings no longer carry a free-text reason
	// (the world verbs declare no reason param — interiority is the muse tool
	// now), so a "wander" landing emits no agent.thought(source planner). The
	// intent landing above is the acceptance signal; musing thoughts are
	// covered by the musing tests.
}

// TestGarbageOutputFallsToReflex: unusable model output produces no planner
// events; the reflex grace keeps the village moving (SC-004 shape).
func TestGarbageOutputFallsToReflex(t *testing.T) {
	h := newHarness(t, `I am a helpful villager and I think that...`)

	reflex := h.waitEvents(t, 15*time.Second, func(e store.Event) bool {
		if e.Type != "agent.intent_set" {
			return false
		}
		var p sim.IntentSetPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "reflex"
	})
	if len(reflex) == 0 {
		t.Fatal("reflex never covered for a garbage-spouting model")
	}
	planner := h.waitEvents(t, 1*time.Second, func(e store.Event) bool {
		if e.Type != "agent.intent_set" {
			return false
		}
		var p sim.IntentSetPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "planner"
	})
	if len(planner) != 0 {
		t.Fatal("garbage output must never become an intent")
	}
}

// TestDeadModelMeansReflexWorld: submit errors → reflex world, no planner
// events, model failures don't stop the clock.
func TestDeadModelMeansReflexWorld(t *testing.T) {
	h := newHarness(t, "")
	h.model.mu.Lock()
	h.model.err = context.DeadlineExceeded
	h.model.mu.Unlock()

	reflex := h.waitEvents(t, 15*time.Second, func(e store.Event) bool {
		if e.Type != "agent.intent_set" {
			return false
		}
		var p sim.IntentSetPayload
		json.Unmarshal(e.Payload, &p)
		return p.Source == "reflex"
	})
	if len(reflex) == 0 {
		t.Fatal("dead model must degrade to reflex, not paralysis")
	}
}

// TestPromptWindowBound is AC#3 end-to-end: a soul with 150 memories yields
// a prompt with at most WindowK memory lines.
func TestPromptWindowBound(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	for i := int64(0); i < 150; i++ {
		state.Agents[0].Memories = append(state.Agents[0].Memories,
			sim.Memory{Text: "memory", Salience: 1 + int(i%10), Tick: i * 60})
	}
	state.Tick = 150 * 60

	prompt := userPrompt(state, 0, sim.WindowK)
	lines := 0
	for _, l := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(l, "- ") {
			lines++
		}
	}
	if lines > sim.WindowK {
		t.Fatalf("prompt carries %d memory lines, window is %d (AC#3)", lines, sim.WindowK)
	}
	if lines == 0 {
		t.Fatal("prompt carries no memories at all")
	}
	if !strings.Contains(prompt, "What do you do next?") {
		t.Error("prompt missing the ask")
	}
}

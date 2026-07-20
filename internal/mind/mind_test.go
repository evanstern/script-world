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

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

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
}

func (m *mockModel) Submit(_ context.Context, req llm.Request) (llm.Response, error) {
	m.calls.Add(1)
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
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	state.Speed = "max"

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

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{})
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

	thoughts := h.waitEvents(t, 5*time.Second, func(e store.Event) bool {
		return e.Type == "agent.thought"
	})
	if len(thoughts) == 0 {
		t.Fatal("no agent.thought events recorded")
	}
	var tp sim.ThoughtPayload
	json.Unmarshal(thoughts[0].Payload, &tp)
	if tp.Text != "Stretching my legs." || tp.Source != "planner" {
		t.Errorf("thought payload: %+v", tp)
	}
	if h.model.calls.Load() == 0 {
		t.Fatal("model never called")
	}
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

// TestParseReply covers the JSON extraction contract.
func TestParseReply(t *testing.T) {
	good := []string{
		`{"goal": "forage", "reason": "hungry"}`,
		`Sure! Here's my choice: {"goal": "Forage", "reason": "x"} hope that helps`,
		"```json\n{\"goal\": \"sleep\", \"reason\": \"tired\"}\n```",
	}
	for _, g := range good {
		if _, err := parseReply(g); err != nil {
			t.Errorf("parseReply(%q): %v", g, err)
		}
	}
	bad := []string{
		"", "no json here",
		`{"goal": "fly_to_moon", "reason": "x"}`,
		`{"goal": }`,
	}
	for _, b := range bad {
		if _, err := parseReply(b); err == nil {
			t.Errorf("parseReply(%q) should fail", b)
		}
	}
	r, err := parseReply(`{"goal": "talk_to", "target": "Birch", "reason": "gossip"}`)
	if err != nil || r.Target != "Birch" {
		t.Errorf("talk_to parse: %+v %v", r, err)
	}
}

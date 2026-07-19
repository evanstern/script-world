package mind

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// scriptedModel returns queued replies in order, then errors.
type scriptedModel struct {
	mu      sync.Mutex
	replies []string
	calls   int
}

func (m *scriptedModel) Submit(_ context.Context, _ llm.Request) (llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.replies) {
		return llm.Response{}, context.DeadlineExceeded
	}
	r := m.replies[m.calls]
	m.calls++
	return llm.Response{Text: r, Tier: llm.TierLocal}, nil
}

// convoScript builds 2×ConvoTurnsPerSide utterances plus the outcome.
func convoScript(outcome string) []string {
	var replies []string
	lines := []string{"Cold morning.", "Aye, bitter.", "Fire held though.", "Barely.", "We need more wood.", "I'll fetch it."}
	for i := 0; i < 2*sim.ConvoTurnsPerSide; i++ {
		replies = append(replies, `{"say": "`+lines[i%len(lines)]+`"}`)
	}
	return append(replies, outcome)
}

func setupConvo(t *testing.T, model Submitter) (*harness, *Mind) {
	t.Helper()
	h := newHarness(t, "") // its own mock is unused; we swap the mind below
	m := h.m
	state := sim.NewState(42, m)
	// Adjacent pair with a tellable memory for agent 0 about agent 2.
	state.Agents[0].X, state.Agents[0].Y = 10, 10
	state.Agents[1].X, state.Agents[1].Y = 10, 11
	state.Agents[0].Memories = append(state.Agents[0].Memories,
		sim.Memory{Text: "Watched Cedar break his word.", Salience: 6, Tick: 10, Subject: 2, Tone: -60})

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(md.Close)
	return h, md
}

// TestConversationRunsAndLands is AC#3: capped multi-turn exchange, gist in
// both souls, tone edges, paraphrased rumor — all as one recorded batch.
func TestConversationRunsAndLands(t *testing.T) {
	model := &scriptedModel{replies: convoScript(
		`{"gist": "complained about the cold and planned firewood", "tone_a": 1, "tone_b": 1, "retold": "Cedar can't be trusted to keep his word, you know."}`)}
	h, md := setupConvo(t, model)

	md.maybeStartConversation(store.Event{
		Tick: 100, Type: "agent.talked",
		Payload: mustJSON(t, sim.TalkedPayload{A: 0, B: 1}),
	})

	convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("conversation never landed")
	}
	var cp sim.ConversationPayload
	json.Unmarshal(convs[0].Payload, &cp)
	if cp.Turns != 2*sim.ConvoTurnsPerSide {
		t.Errorf("turns = %d, want %d (cap)", cp.Turns, 2*sim.ConvoTurnsPerSide)
	}
	if cp.Gist == "" {
		t.Error("empty gist")
	}

	all, _ := h.st.EventsSince(0, 0)
	var turnCount, gistMems, toneEdges int
	var rumorText string
	for _, e := range all {
		switch e.Type {
		case "social.conversation_turn":
			turnCount++
		case "agent.memory_added":
			if strings.Contains(string(e.Payload), "Talked with") {
				gistMems++
			}
		case "social.relation_changed":
			if strings.Contains(string(e.Payload), "conversation") {
				toneEdges++
			}
		case "social.rumor_told":
			var p sim.RumorToldPayload
			json.Unmarshal(e.Payload, &p)
			rumorText = p.Text
		}
	}
	if turnCount != 2*sim.ConvoTurnsPerSide {
		t.Errorf("recorded turns = %d", turnCount)
	}
	if gistMems < 2 {
		t.Errorf("gist memories = %d, want both souls (AC#3)", gistMems)
	}
	if toneEdges != 2 {
		t.Errorf("tone edges = %d", toneEdges)
	}
	if rumorText != "Cedar can't be trusted to keep his word, you know." {
		t.Errorf("rumor should carry the paraphrase, got %q", rumorText)
	}
}

// TestConversationFailureInjectsNothing: a garbage utterance mid-dialogue
// abandons the whole conversation (the primitive talk stands alone).
func TestConversationFailureInjectsNothing(t *testing.T) {
	model := &scriptedModel{replies: []string{
		`{"say": "Hello."}`,
		`the model rambles without json`,
	}}
	h, md := setupConvo(t, model)

	md.maybeStartConversation(store.Event{
		Tick: 100, Type: "agent.talked",
		Payload: mustJSON(t, sim.TalkedPayload{A: 0, B: 1}),
	})
	time.Sleep(500 * time.Millisecond)
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if strings.HasPrefix(e.Type, "social.conversation") {
			t.Fatalf("abandoned conversation leaked %s into the log", e.Type)
		}
	}
}

// TestInjectSocialWhitelist: the door rejects non-social event types
// atomically.
func TestInjectSocialWhitelist(t *testing.T) {
	h, _ := setupConvo(t, &scriptedModel{})
	bad := []store.Event{
		{Type: "social.conversation", Payload: mustJSON(t, sim.ConversationPayload{Conv: 1, A: 0, B: 1, Gist: "x", Turns: 2})},
		{Type: "agent.died", Payload: mustJSON(t, sim.DiedPayload{Agent: 0, Cause: "murder-by-injection"})},
	}
	if err := h.loop.InjectSocial(bad); err == nil {
		t.Fatal("whitelist must reject the batch")
	}
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if e.Type == "social.conversation" || e.Type == "agent.died" {
			t.Fatal("rejected batch must inject nothing (atomicity)")
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

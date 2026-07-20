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

// TestSceneConversation (TASK-22): a third awake villager within the join
// radius enters the scene; the outcome lands per-participant fodder —
// subject-tagged toned gist memories (gossip seeds), a full mesh of tone
// edges, topics, and a durable record in state.
func TestSceneConversation(t *testing.T) {
	h := newHarness(t, "")
	m := h.m
	state := sim.NewState(42, m)
	// Cluster 0/1/2; park everyone else far away and asleep so the scene
	// is exactly three.
	for i := range state.Agents {
		state.Agents[i].X, state.Agents[i].Y = 50, 50+i
		state.Agents[i].Asleep = true
	}
	for i, pos := range [][2]int{{10, 10}, {10, 11}, {11, 10}} {
		state.Agents[i].X, state.Agents[i].Y = pos[0], pos[1]
		state.Agents[i].Asleep = false
	}

	// 3 participants × ConvoTurnsPerSide utterances, then the outcome.
	var replies []string
	for i := 0; i < 3*sim.ConvoTurnsPerSide; i++ {
		replies = append(replies, `{"say": "Aye."}`)
	}
	replies = append(replies,
		`{"gist": "argued about who tends the fire", "topics": ["fire", "chores"], "tones": [2, 0, -1], "retold": null}`)
	model := &scriptedModel{replies: replies}

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(md.Close)

	md.maybeStartConversation(store.Event{
		Tick: 100, Type: "agent.talked",
		Payload: mustJSON(t, sim.TalkedPayload{A: 0, B: 1}),
	})

	convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("scene conversation never landed")
	}
	var cp sim.ConversationPayload
	json.Unmarshal(convs[0].Payload, &cp)
	if len(cp.Participants) != 3 || cp.Turns != 3*sim.ConvoTurnsPerSide {
		t.Fatalf("scene shape: participants=%v turns=%d", cp.Participants, cp.Turns)
	}
	if len(cp.Topics) != 2 || cp.Topics[0] != "fire" || len(cp.Tones) != 3 || cp.Tones[2] != -1 {
		t.Errorf("topics/tones: %v %v", cp.Topics, cp.Tones)
	}

	all, _ := h.st.EventsSince(0, 0)
	var gistMems, toneEdges int
	var sawSubjectTagged bool
	for _, e := range all {
		switch e.Type {
		case "agent.memory_added":
			var p sim.MemoryAddedPayload
			json.Unmarshal(e.Payload, &p)
			// Filter on the gist text: the live loop's executor emits its
			// own "Talked with X." talk memories during the wait window.
			if strings.Contains(p.Text, "argued about who tends the fire") {
				gistMems++
				if p.Agent == 0 && p.Subject == 1 && p.Tone == 2*30 {
					sawSubjectTagged = true // agent 0's tone (+2) about counterpart 1
				}
			}
		case "social.relation_changed":
			if strings.Contains(string(e.Payload), "conversation: fire") {
				toneEdges++
			}
		}
	}
	if gistMems != 6 {
		t.Errorf("gist memories = %d, want 6 (3 participants × 2 counterparts)", gistMems)
	}
	if toneEdges != 6 {
		t.Errorf("tone edges = %d, want full mesh of 6 with topic reason", toneEdges)
	}
	if !sawSubjectTagged {
		t.Error("gist memory must be subject-tagged and toned (gossip seed)")
	}

	// The record ring and the gossip seed are live state: rebuild state
	// from the loop and check both.
	stateJSON, _, err := h.loop.DoState()
	if err != nil {
		t.Fatal(err)
	}
	live := sim.NewState(42, m)
	if err := json.Unmarshal(stateJSON, live); err != nil {
		t.Fatal(err)
	}
	if rec, ok := sim.LastConversationBetween(live, 0, 2); !ok || rec.Gist == "" {
		t.Fatalf("no durable record between 0 and 2: %v %v", rec, ok)
	}
	if tell, ok := sim.TellableFor(live, 0, 3); !ok || (tell.Subject != 1 && tell.Subject != 2) {
		t.Errorf("scene gist must be servable gossip about a counterpart: %+v ok=%v", tell, ok)
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

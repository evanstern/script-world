package mind

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
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

// countingModel returns queued replies and a transport error once they run
// out, counting EVERY Submit (including the errored ones) — so a test can
// prove that a transport failure was not retried.
type countingModel struct {
	mu      sync.Mutex
	replies []string
	calls   int
}

func (m *countingModel) Submit(_ context.Context, _ llm.Request) (llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls > len(m.replies) {
		return llm.Response{}, context.DeadlineExceeded
	}
	return llm.Response{Text: m.replies[m.calls-1], Tier: llm.TierLocal}, nil
}

// validSays returns the scene's utterance replies (no outcome).
func validSays() []string {
	lines := []string{"Cold morning.", "Aye, bitter.", "Fire held though.", "Barely."}
	var r []string
	for i := 0; i < 2*sim.ConvoTurnsPerSide; i++ {
		r = append(r, `{"say": "`+lines[i%len(lines)]+`"}`)
	}
	return r
}

// sceneOutcomes collects the conversation-class cog.outcome payloads from a
// full event dump, split into the single terminal record and any non-terminal
// retried markers (contract §Compatibility).
func sceneOutcomes(t *testing.T, all []store.Event) (terminal sim.CogOutcomePayload, markers []sim.CogOutcomePayload, terminals int) {
	t.Helper()
	for _, e := range all {
		if e.Type != "cog.outcome" {
			continue
		}
		var p sim.CogOutcomePayload
		if json.Unmarshal(e.Payload, &p) != nil || p.Class != "conversation" {
			continue
		}
		if p.Outcome == sim.OutcomeRetried {
			markers = append(markers, p)
			continue
		}
		terminal = p
		terminals++
	}
	return terminal, markers, terminals
}

func startConvo(t *testing.T, h *harness, md *Mind) {
	t.Helper()
	md.maybeStartConversation(store.Event{
		Tick: 100, Type: "agent.talked",
		Payload: mustJSON(t, sim.TalkedPayload{A: 0, B: 1}),
	})
}

// TestConvoOutcomeRetryLandsWhole (T007a / US1 / SC-001): a malformed first summary
// reply is re-requested once; on success the scene lands whole with
// retried:true, and the failed reply rode a non-terminal retried marker.
func TestConvoOutcomeRetryLandsWhole(t *testing.T) {
	badSummary := "the model rambles without json"
	replies := append(validSays(),
		badSummary,
		`{"gist": "planned the firewood run", "topics": ["fire"], "tones": [1, 1], "retold": null}`)
	model := &scriptedModel{replies: replies}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("scene did not land after outcome retry")
	}
	var cp sim.ConversationPayload
	json.Unmarshal(convs[0].Payload, &cp)
	if cp.Turns != 2*sim.ConvoTurnsPerSide || cp.Gist == "" {
		t.Errorf("scene not whole after retry: turns=%d gist=%q", cp.Turns, cp.Gist)
	}

	all, _ := h.st.EventsSince(0, 0)
	term, markers, n := sceneOutcomes(t, all)
	if n != 1 {
		t.Fatalf("terminal outcomes = %d, want exactly 1", n)
	}
	if term.Outcome != sim.OutcomeLanded || !term.Retried {
		t.Errorf("terminal = %q retried=%v, want landed retried=true", term.Outcome, term.Retried)
	}
	if term.Raw != "" {
		t.Errorf("landed event must never carry raw, got %q", term.Raw)
	}
	if len(markers) != 1 {
		t.Fatalf("retried markers = %d, want 1", len(markers))
	}
	if markers[0].Raw != badSummary {
		t.Errorf("marker raw = %q, want verbatim %q", markers[0].Raw, badSummary)
	}
	if !strings.HasPrefix(markers[0].Reason, "outcome:") {
		t.Errorf("marker reason should locate the site: %q", markers[0].Reason)
	}
}

// TestConvoOutcomeDoubleFailureAbandons (T007b / US1): two consecutive malformed
// summaries abandon the scene with no partial state; the terminal unusable
// carries the RETRY's raw reply.
func TestConvoOutcomeDoubleFailureAbandons(t *testing.T) {
	replies := append(validSays(), "garbage one", "garbage two")
	model := &scriptedModel{replies: replies}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	outcomes := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		var p sim.CogOutcomePayload
		return e.Type == "cog.outcome" && json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "conversation" && p.Outcome == sim.OutcomeUnusable
	})
	if len(outcomes) == 0 {
		t.Fatal("double outcome failure never abandoned")
	}
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if strings.HasPrefix(e.Type, "social.conversation") {
			t.Fatalf("abandoned scene leaked %s", e.Type)
		}
	}
	term, markers, _ := sceneOutcomes(t, all)
	if !term.Retried || term.Raw != "garbage two" {
		t.Errorf("terminal unusable: retried=%v raw=%q, want retried=true raw=%q", term.Retried, term.Raw, "garbage two")
	}
	if len(markers) != 1 || markers[0].Raw != "garbage one" {
		t.Errorf("want one retried marker carrying %q, got %+v", "garbage one", markers)
	}
}

// TestConvoOutcomeTransportErrorNoRetry (T007c / US1 / FR-007): a transport error
// at the summary site abandons immediately — no retry, no raw.
func TestConvoOutcomeTransportErrorNoRetry(t *testing.T) {
	model := &countingModel{replies: validSays()} // no summary reply → outcome errors
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		var p sim.CogOutcomePayload
		return e.Type == "cog.outcome" && json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "conversation" && p.Outcome == sim.OutcomeUnusable
	})
	all, _ := h.st.EventsSince(0, 0)
	term, markers, _ := sceneOutcomes(t, all)
	if term.Raw != "" || term.Retried {
		t.Errorf("transport abandon carried raw=%q retried=%v, want neither", term.Raw, term.Retried)
	}
	if len(markers) != 0 {
		t.Errorf("transport error must not emit a retried marker: %+v", markers)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	// 2*ConvoTurnsPerSide utterances + exactly one (failed) outcome attempt:
	// a retry would show as a further call.
	if want := 2*sim.ConvoTurnsPerSide + 1; calls != want {
		t.Errorf("Submit calls = %d, want %d (no retry on transport error)", calls, want)
	}
}

// TestConvoUtteranceRetryCompletes (T009a / US2 / SC-002): one bad utterance is
// retried on the same speaker; the scene completes with alternation intact and
// lands retried:true.
func TestConvoUtteranceRetryCompletes(t *testing.T) {
	replies := []string{"garbage", `{"say": "Cold morning."}`}
	for i := 1; i < 2*sim.ConvoTurnsPerSide; i++ {
		replies = append(replies, `{"say": "Aye."}`)
	}
	replies = append(replies, `{"gist": "planned firewood", "topics": ["fire"], "tones": [1, 1], "retold": null}`)
	model := &scriptedModel{replies: replies}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("scene did not complete after utterance retry")
	}
	all, _ := h.st.EventsSince(0, 0)
	// Alternation intact: turns come back in round-robin speaker order.
	var speakers []int
	for _, e := range all {
		if e.Type == "social.conversation_turn" {
			var p sim.ConversationTurnPayload
			json.Unmarshal(e.Payload, &p)
			speakers = append(speakers, p.Speaker)
		}
	}
	if len(speakers) != 2*sim.ConvoTurnsPerSide {
		t.Fatalf("turns = %d, want %d", len(speakers), 2*sim.ConvoTurnsPerSide)
	}
	// Founding pair A=0,B=1 → scene participants idx=[0,1], round-robin.
	for i, sp := range speakers {
		if want := i % 2; sp != want {
			t.Errorf("turn %d speaker = %d, want %d (round-robin broken)", i, sp, want)
		}
	}
	term, markers, n := sceneOutcomes(t, all)
	if n != 1 || term.Outcome != sim.OutcomeLanded || !term.Retried {
		t.Errorf("terminal = %q retried=%v (n=%d), want landed retried=true", term.Outcome, term.Retried, n)
	}
	if len(markers) != 1 || markers[0].Raw != "garbage" || !strings.HasPrefix(markers[0].Reason, "utterance turn 0:") {
		t.Errorf("want one utterance retried marker for turn 0 carrying %q: %+v", "garbage", markers)
	}
}

// TestConvoUtteranceDoubleFailureAbandons (T009b / US2): two consecutive bad
// utterances abandon with nothing injected.
func TestConvoUtteranceDoubleFailureAbandons(t *testing.T) {
	model := &scriptedModel{replies: []string{"garbage", "still garbage"}}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		var p sim.CogOutcomePayload
		return e.Type == "cog.outcome" && json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "conversation" && p.Outcome == sim.OutcomeUnusable
	})
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if strings.HasPrefix(e.Type, "social.conversation") {
			t.Fatalf("abandoned scene leaked %s", e.Type)
		}
	}
	term, _, _ := sceneOutcomes(t, all)
	if !term.Retried || term.Raw != "still garbage" {
		t.Errorf("terminal unusable: retried=%v raw=%q, want retried=true raw=%q", term.Retried, term.Raw, "still garbage")
	}
}

// TestConvoUtteranceRetryOnFinalTurn (T009c / US2 edge case): a bad reply on the
// scene's last utterance recovers and the outcome step still receives a
// well-formed, full-length transcript.
func TestConvoUtteranceRetryOnFinalTurn(t *testing.T) {
	var replies []string
	for i := 0; i < 2*sim.ConvoTurnsPerSide-1; i++ {
		replies = append(replies, `{"say": "Aye."}`)
	}
	replies = append(replies, "garbage", `{"say": "Right then."}`,
		`{"gist": "settled it", "topics": ["fire"], "tones": [1, 1], "retold": null}`)
	model := &scriptedModel{replies: replies}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("scene did not land after final-turn retry")
	}
	var cp sim.ConversationPayload
	json.Unmarshal(convs[0].Payload, &cp)
	if cp.Turns != 2*sim.ConvoTurnsPerSide {
		t.Errorf("turns = %d, want full %d", cp.Turns, 2*sim.ConvoTurnsPerSide)
	}
	all, _ := h.st.EventsSince(0, 0)
	if _, _, n := sceneOutcomes(t, all); n != 1 {
		t.Errorf("terminal outcomes = %d, want 1", n)
	}
}

// TestConvoUtteranceBudgetPerScene (US2 / FR-002 / FR-007): the utterance
// retry budget is ONE per scene, not one per turn. Turn 0 fails then recovers
// (spending the budget); a later turn-2 failure abandons the scene even though
// the two failures are non-consecutive. At most one utterance retried marker;
// no second retry Submit is made.
func TestConvoUtteranceBudgetPerScene(t *testing.T) {
	// t0 attempt (fail) → retry (ok) → t1 (ok) → t2 attempt (fail → abandon).
	model := &countingModel{replies: []string{
		"garbage", `{"say": "Cold."}`, `{"say": "Aye."}`, "garbage again",
	}}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		var p sim.CogOutcomePayload
		return e.Type == "cog.outcome" && json.Unmarshal(e.Payload, &p) == nil &&
			p.Class == "conversation" && p.Outcome == sim.OutcomeUnusable
	})
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if strings.HasPrefix(e.Type, "social.conversation") {
			t.Fatalf("abandoned scene leaked %s", e.Type)
		}
	}
	term, markers, _ := sceneOutcomes(t, all)
	if len(markers) != 1 || markers[0].Raw != "garbage" {
		t.Errorf("want exactly one utterance retried marker carrying %q, got %+v", "garbage", markers)
	}
	if !term.Retried || term.Raw != "garbage again" {
		t.Errorf("terminal unusable: retried=%v raw=%q, want retried=true raw=%q", term.Retried, term.Raw, "garbage again")
	}
	if !strings.Contains(term.Reason, "budget spent") {
		t.Errorf("terminal reason should name the spent budget: %q", term.Reason)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	// t0 attempt + t0 retry + t1 + t2 attempt = 4; a second retry would be a 5th.
	if calls != 4 {
		t.Errorf("Submit calls = %d, want 4 (no second utterance retry)", calls)
	}
}

// TestConvoRawReplyRecoverableFromStore (T011 / US3 / SC-003): a parse failure's
// verbatim reply is recoverable from the persisted event log and attributable
// to the conversation job id.
func TestConvoRawReplyRecoverableFromStore(t *testing.T) {
	badSummary := `{"gist": broken and unquoted but too weird, "extra": nonsense here too`
	replies := append(validSays(), badSummary,
		`{"gist": "recovered", "topics": ["fire"], "tones": [1, 1], "retold": null}`)
	model := &scriptedModel{replies: replies}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	all, _ := h.st.EventsSince(0, 0)
	var found bool
	for _, e := range all {
		if e.Type != "cog.outcome" {
			continue
		}
		var p sim.CogOutcomePayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Raw != "" {
			found = true
			if p.Raw != badSummary {
				t.Errorf("raw = %q, want verbatim %q", p.Raw, badSummary)
			}
			if p.Job != "conversation-100" {
				t.Errorf("raw not attributable: job = %q, want conversation-100", p.Job)
			}
		}
	}
	if !found {
		t.Fatal("no raw reply persisted for the parse failure")
	}
}

// TestConvoGoldenHappyPath (T017 / SC-004 / FR-009): an all-valid scene emits the
// pre-change batch — no raw, no retried fields, no extra Submit calls.
func TestConvoGoldenHappyPath(t *testing.T) {
	model := &scriptedModel{replies: convoScript(
		`{"gist": "planned firewood", "topics": ["fire"], "tones": [1, 1], "retold": null}`)}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	})
	if len(convs) == 0 {
		t.Fatal("happy-path scene never landed")
	}
	all, _ := h.st.EventsSince(0, 0)
	for _, e := range all {
		if e.Type != "cog.outcome" {
			continue
		}
		var p sim.CogOutcomePayload
		if json.Unmarshal(e.Payload, &p) != nil || p.Class != "conversation" {
			continue
		}
		// The new fields must be omitempty-absent from the serialized payload.
		if bytes.Contains(e.Payload, []byte(`"retried"`)) || bytes.Contains(e.Payload, []byte(`"raw"`)) {
			t.Errorf("happy-path outcome carries new fields: %s", e.Payload)
		}
		if p.Outcome != sim.OutcomeLanded {
			t.Errorf("outcome = %q, want landed", p.Outcome)
		}
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	if want := 2*sim.ConvoTurnsPerSide + 1; calls != want {
		t.Errorf("Submit calls = %d, want %d (no extra calls on the happy path)", calls, want)
	}
}

// TestConvoMemoryRecoversTranscript (spec 019 US2, T012 / SC-002): a
// participant's conversation memory carries the conversation ref (Memory.Conv),
// and the complete ordered transcript — every turn's speaker + verbatim text —
// is recoverable from the EVENT LOG ALONE via that ref, with no model call.
func TestConvoMemoryRecoversTranscript(t *testing.T) {
	lines := []string{"Cold morning.", "Aye, bitter.", "Fire held though.", "Barely.", "We need more wood.", "I'll fetch it."}
	model := &scriptedModel{replies: convoScript(
		`{"gist": "planned the firewood run", "topics": ["fire"], "tones": [1, 1], "retold": null}`)}
	h, md := setupConvo(t, model)
	startConvo(t, h, md)

	if convs := h.waitEvents(t, 10*time.Second, func(e store.Event) bool {
		return e.Type == "social.conversation"
	}); len(convs) == 0 {
		t.Fatal("conversation never landed")
	}

	all, _ := h.st.EventsSince(0, 0)

	// 1. A participant's gist memory carries the conversation ref.
	var conv int64
	for _, e := range all {
		if e.Type != "agent.memory_added" {
			continue
		}
		var p sim.MemoryAddedPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Conv != 0 && strings.Contains(p.Text, "Talked with") {
			conv = p.Conv
			break
		}
	}
	if conv == 0 {
		t.Fatal("no conversation memory carried a Conv ref")
	}

	// 2. Recover the transcript from the log alone, by the memory's Conv ref —
	// social.conversation_turn events with that conv, ordered by seq.
	type turn struct {
		speaker int
		text    string
	}
	var got []turn
	for _, e := range all { // EventsSince returns rows in seq order
		if e.Type != "social.conversation_turn" {
			continue
		}
		var p sim.ConversationTurnPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Conv == conv {
			got = append(got, turn{p.Speaker, p.Text})
		}
	}

	// 3. The recovered transcript is the complete, ordered dialogue: round-robin
	// speakers over the founding pair (0,1) and the verbatim scripted utterances.
	if len(got) != 2*sim.ConvoTurnsPerSide {
		t.Fatalf("recovered %d turns, want %d", len(got), 2*sim.ConvoTurnsPerSide)
	}
	for i, tn := range got {
		wantSpeaker := i % 2 // idx = [0,1], round-robin
		wantText := lines[i%len(lines)]
		if tn.speaker != wantSpeaker || tn.text != wantText {
			t.Errorf("turn %d = {%d, %q}, want {%d, %q}", i, tn.speaker, tn.text, wantSpeaker, wantText)
		}
	}
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

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{}, testLoopRounds, testPlannerTokens, testConsolidationTokens, noopLoop)
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

	// TASK-32 US1: the scene is one 13-point thought — its cog.thought and a
	// single landed cog.outcome (batched with the scene) share the job id.
	var sceneJob string
	var sceneOutcomes int
	for _, e := range all {
		switch e.Type {
		case "cog.thought":
			var p sim.CogThoughtPayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Class == "conversation" {
				sceneJob = p.Job
				if p.Points != 13 {
					t.Errorf("scene points = %d, want 13", p.Points)
				}
			}
		case "cog.outcome":
			var p sim.CogOutcomePayload
			if json.Unmarshal(e.Payload, &p) == nil && p.Class == "conversation" {
				sceneOutcomes++
				if p.Outcome != sim.OutcomeLanded {
					t.Errorf("scene outcome = %q, want landed", p.Outcome)
				}
				if e.Tick != convs[0].Tick {
					t.Error("scene outcome not batched with the scene")
				}
			}
		}
	}
	if sceneJob == "" {
		t.Error("no conversation cog.thought recorded")
	}
	if sceneOutcomes != 1 {
		t.Errorf("scene outcomes = %d, want exactly 1", sceneOutcomes)
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

	md, err := New(model, h.loop, h.loop, m, 42, state.Marshal(), [sim.AgentCount]string{}, testLoopRounds, testPlannerTokens, testConsolidationTokens, noopLoop)
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

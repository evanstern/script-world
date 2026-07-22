package sim

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// --- TASK-56: LastGoal/LastGoalTick reducer lifecycle (research.md R1) ---

func TestIntentSetWritesLastGoal(t *testing.T) {
	s := NewState(42, testMap(42))
	e := store.Event{Tick: 500, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 2, Goal: "chop", TargetX: 10, TargetY: 12})}
	if err := s.Apply(e); err != nil {
		t.Fatalf("apply intent_set: %v", err)
	}
	a := s.Agents[2]
	if a.Intent == nil || a.Intent.Goal != "chop" {
		t.Fatalf("Intent not set: %+v", a.Intent)
	}
	if a.LastGoal != "chop" {
		t.Errorf("LastGoal = %q, want %q", a.LastGoal, "chop")
	}
	if a.LastGoalTick != 500 {
		t.Errorf("LastGoalTick = %d, want 500", a.LastGoalTick)
	}
}

func TestIntentDonePreservesLastGoal(t *testing.T) {
	s := NewState(42, testMap(42))
	apply := func(e store.Event) {
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
	}
	apply(store.Event{Tick: 500, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 2, Goal: "chop"})})
	apply(store.Event{Tick: 600, Type: "agent.intent_done",
		Payload: mustPayload(AgentPayload{Agent: 2})})

	a := s.Agents[2]
	if a.Intent != nil {
		t.Fatalf("intent_done should clear Intent: %+v", a.Intent)
	}
	if a.LastGoal != "chop" || a.LastGoalTick != 500 {
		t.Errorf("intent_done cleared last goal: LastGoal=%q LastGoalTick=%d", a.LastGoal, a.LastGoalTick)
	}
}

func TestGruAttackedPreservesLastGoal(t *testing.T) {
	s := NewState(42, testMap(42))
	apply := func(e store.Event) {
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
	}
	apply(store.Event{Tick: 500, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 2, Goal: "forage"})})
	apply(store.Event{Tick: 700, Type: "gru.attacked",
		Payload: mustPayload(GruAttackedPayload{Agent: 2, Health: 300})})

	a := s.Agents[2]
	if a.Intent != nil {
		t.Fatalf("gru.attacked should clear Intent: %+v", a.Intent)
	}
	if a.LastGoal != "forage" || a.LastGoalTick != 500 {
		t.Errorf("gru.attacked cleared last goal: LastGoal=%q LastGoalTick=%d", a.LastGoal, a.LastGoalTick)
	}
}

// TestLastGoalSurvivesRepeatedSetDone drives a set→done→set→done timeline and
// confirms LastGoal always reflects the most recent intent_set, never the
// prior one and never cleared by the intervening done.
func TestLastGoalSurvivesRepeatedSetDone(t *testing.T) {
	s := NewState(42, testMap(42))
	apply := func(e store.Event) {
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
	}
	apply(store.Event{Tick: 100, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 0, Goal: "chop"})})
	apply(store.Event{Tick: 200, Type: "agent.intent_done",
		Payload: mustPayload(AgentPayload{Agent: 0})})
	if s.Agents[0].LastGoal != "chop" || s.Agents[0].LastGoalTick != 100 {
		t.Fatalf("after first set/done: LastGoal=%q LastGoalTick=%d", s.Agents[0].LastGoal, s.Agents[0].LastGoalTick)
	}
	apply(store.Event{Tick: 300, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 0, Goal: "hunt"})})
	if s.Agents[0].LastGoal != "hunt" || s.Agents[0].LastGoalTick != 300 {
		t.Fatalf("after second set: LastGoal=%q LastGoalTick=%d", s.Agents[0].LastGoal, s.Agents[0].LastGoalTick)
	}
	apply(store.Event{Tick: 400, Type: "agent.intent_done",
		Payload: mustPayload(AgentPayload{Agent: 0})})
	if s.Agents[0].LastGoal != "hunt" || s.Agents[0].LastGoalTick != 300 {
		t.Fatalf("after second done: LastGoal=%q LastGoalTick=%d", s.Agents[0].LastGoal, s.Agents[0].LastGoalTick)
	}
}

// TestLastGoalAbsentFieldDecodesToNever confirms a pre-feature agent JSON
// (no last_goal/last_goal_tick keys) decodes to the zero value — "no
// objective yet" — and that a never-intended agent's canonical bytes carry
// no last_goal key at all (byte-stability for pre-feature snapshots).
func TestLastGoalAbsentFieldDecodesToNever(t *testing.T) {
	const raw = `{"name":"Ash","x":1,"y":2,"needs":{},"inv":{"wood":0},"last_talk":0,"idle_since":0}`
	var a Agent
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("decode pre-feature agent: %v", err)
	}
	if a.LastGoal != "" || a.LastGoalTick != 0 {
		t.Errorf("absent fields should zero-value: LastGoal=%q LastGoalTick=%d", a.LastGoal, a.LastGoalTick)
	}

	s := NewState(42, testMap(42))
	blob := s.Marshal()
	if bytes.Contains(blob, []byte(`"last_goal"`)) {
		t.Fatal("a fresh state with no intents ever set should carry no last_goal key")
	}
}

// TestLastGoalSnapshotRoundTrip is T015: marshal a state where an agent
// finished an intent, unmarshal (simulating a fresh attach), and confirm
// LastGoal/LastGoalTick survive — FR-006's "freshly attached observer" path.
func TestLastGoalSnapshotRoundTrip(t *testing.T) {
	s := NewState(42, testMap(42))
	apply := func(e store.Event) {
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
	}
	apply(store.Event{Tick: 900, Type: "agent.intent_set",
		Payload: mustPayload(IntentSetPayload{Agent: 5, Goal: "quarry"})})
	apply(store.Event{Tick: 1000, Type: "agent.intent_done",
		Payload: mustPayload(AgentPayload{Agent: 5})})

	blob := s.Marshal()
	if !bytes.Contains(blob, []byte(`"last_goal":"quarry"`)) {
		t.Fatalf("last_goal did not serialize as expected: %s", blob)
	}
	var back State
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatal(err)
	}
	if back.Agents[5].Intent != nil {
		t.Fatalf("intent should stay cleared: %+v", back.Agents[5].Intent)
	}
	if back.Agents[5].LastGoal != "quarry" || back.Agents[5].LastGoalTick != 900 {
		t.Fatalf("last goal lost in round-trip: LastGoal=%q LastGoalTick=%d",
			back.Agents[5].LastGoal, back.Agents[5].LastGoalTick)
	}
	if back.Hash() != s.Hash() {
		t.Error("state hash not stable across round-trip")
	}
}

// TestReplayDeterminismWithLastGoal is a focused replay-determinism pass over
// a set→done→set timeline, confirming the new deterministic reducer field
// passes by construction (research.md R1).
func TestReplayDeterminismWithLastGoal(t *testing.T) {
	const seed, ticks = 271, 6000
	m := testMap(seed)
	live := NewState(seed, m)
	cmds := map[int64][]store.Event{
		100: {{Tick: 100, Type: "agent.intent_set",
			Payload: mustPayload(IntentSetPayload{Agent: 1, Goal: "chop", TargetX: live.Agents[1].X, TargetY: live.Agents[1].Y})}},
	}
	log := driveTicks(t, live, m, ticks, cmds)
	if countType(log, "agent.intent_done") == 0 && countType(log, "agent.chopped") == 0 {
		t.Skip("scenario did not clear the injected intent within the window; not a determinism failure")
	}

	replayed := NewState(seed, m)
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil)
	if live.Hash() != replayed.Hash() {
		t.Fatalf("replay diverged:\nlive:     %s\nreplayed: %s", live.Marshal(), replayed.Marshal())
	}
	if live.Agents[1].LastGoal != replayed.Agents[1].LastGoal || live.Agents[1].LastGoalTick != replayed.Agents[1].LastGoalTick {
		t.Fatalf("LastGoal diverged: live=%q/%d replayed=%q/%d",
			live.Agents[1].LastGoal, live.Agents[1].LastGoalTick,
			replayed.Agents[1].LastGoal, replayed.Agents[1].LastGoalTick)
	}
}

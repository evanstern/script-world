package sim

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

func memAt(tick int64, sal int, text string) Memory {
	return Memory{Text: text, Salience: sal, Tick: tick}
}

// TestWindowBound is AC#3's core: the window never exceeds K, no matter how
// large the soul.
func TestWindowBound(t *testing.T) {
	a := &Agent{Name: "Ash"}
	for i := int64(0); i < 500; i++ {
		a.Memories = append(a.Memories, memAt(i*60, 1+int(i%10), "m"))
	}
	w := SelectMemories(a, 42, 0, 500*60, WindowK)
	if len(w) != WindowK {
		t.Fatalf("window = %d memories, want exactly %d", len(w), WindowK)
	}
}

// TestWindowDeterministic: same state + tick → identical selection.
func TestWindowDeterministic(t *testing.T) {
	a := &Agent{Name: "Ash"}
	for i := int64(0); i < 100; i++ {
		a.Memories = append(a.Memories, memAt(i*600, 1+int(i%10), "m"))
	}
	w1 := SelectMemories(a, 7, 3, 100_000, WindowK)
	w2 := SelectMemories(a, 7, 3, 100_000, WindowK)
	if len(w1) != len(w2) {
		t.Fatal("selection sizes differ")
	}
	for i := range w1 {
		if w1[i] != w2[i] {
			t.Fatalf("selection differs at %d: %+v vs %+v", i, w1[i], w2[i])
		}
	}
	// Same cadence bucket → same serendipity picks even at nearby ticks.
	w3 := SelectMemories(a, 7, 3, 100_000+60, WindowK)
	if len(w3) != len(w1) {
		t.Fatal("bucketed selection changed size within a cadence window")
	}
}

// TestWindowFavorsSalienceAndRecency: a fresh 10★ beats an old 1★; the
// serendipity quota still surfaces something old.
func TestWindowFavorsSalienceAndRecency(t *testing.T) {
	a := &Agent{Name: "Ash"}
	// 40 old low-salience, then recent high-salience ones.
	for i := int64(0); i < 40; i++ {
		a.Memories = append(a.Memories, memAt(i*60, 1, "old-noise"))
	}
	a.Memories = append(a.Memories,
		memAt(90_000, 10, "watched-death"),
		memAt(91_000, 9, "near-death"),
		memAt(92_000, 3, "talk"))
	w := SelectMemories(a, 42, 0, 93_000, WindowK)

	var texts []string
	for _, m := range w {
		texts = append(texts, m.Text)
	}
	joined := strings.Join(texts, ",")
	if !strings.Contains(joined, "watched-death") || !strings.Contains(joined, "near-death") {
		t.Errorf("high-salience recents missing: %v", texts)
	}
	if !strings.Contains(joined, "old-noise") {
		t.Errorf("serendipity tail pick missing: %v", texts)
	}
	// Reverse-chronological presentation.
	for i := 1; i < len(w); i++ {
		if w[i].Tick > w[i-1].Tick {
			t.Fatalf("window not reverse-chronological at %d", i)
		}
	}
}

// TestMemoriesAccrete: a running village generates memories via events, and
// they land in state (AC#2's accretion half at sim level).
func TestMemoriesAccrete(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	log := driveTicks(t, s, m, 12*3600, nil)

	var memEvents int
	for _, e := range log {
		if e.Type == "agent.memory_added" {
			memEvents++
			var p MemoryAddedPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p.Salience < 1 || p.Salience > 10 || p.Text == "" {
				t.Errorf("bad memory payload: %+v", p)
			}
		}
	}
	if memEvents == 0 {
		t.Fatal("half a game-day produced no memories (fires/talks should mark souls)")
	}
	var inState int
	for _, a := range s.Agents {
		inState += len(a.Memories)
	}
	if inState != memEvents {
		t.Errorf("state carries %d memories, log emitted %d", inState, memEvents)
	}
}

// TestReflexGrace: idle agents act only after the grace window, and
// IdleSince tracks intent completion.
func TestReflexGrace(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	// From genesis (IdleSince 0), nothing may happen before the grace.
	log := driveTicks(t, s, m, reflexGraceTicks-1, nil)
	for _, e := range log {
		if e.Type == "agent.intent_set" {
			t.Fatalf("reflex acted at tick %d, inside the grace window", e.Tick)
		}
	}
	// Shortly after the grace, reflexes fire (staggered).
	log = driveTicks(t, s, m, reflexGraceTicks+40, nil)
	var acted bool
	for _, e := range log {
		if e.Type == "agent.intent_set" {
			var p IntentSetPayload
			json.Unmarshal(e.Payload, &p)
			if p.Source != "reflex" {
				t.Errorf("expected reflex source, got %q", p.Source)
			}
			acted = true
		}
	}
	if !acted {
		t.Fatal("reflex never fired after the grace window")
	}
}

// TestInjectedPlannerIntent: a planner-style command timeline (intent_set
// source planner + thought) replays deterministically like any input, and
// the executor acts on it.
func TestInjectedPlannerIntent(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	// Compute what the resolver would emit for agent 0 "forage" at tick 30.
	pre := NewState(seed, m)
	driveTicks(t, pre, m, 30, nil)
	intent, direct, err := resolveGoal(pre, m, 0, "forage", -1, 30)
	if err != nil || direct != "" || intent == nil {
		t.Fatalf("resolveGoal: %v %q %v", err, direct, intent)
	}

	timeline := map[int64][]store.Event{
		30: {
			{Tick: 30, Type: "agent.thought", Payload: mustPayload(ThoughtPayload{Agent: 0, Text: "The bushes call to me.", Source: "planner"})},
			{Tick: 30, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{
				Agent: 0, Goal: intent.Goal, TargetX: intent.TargetX, TargetY: intent.TargetY, Source: "planner"})},
		},
	}
	logA := driveTicks(t, s, m, 3600, timeline)

	// The injected intent leads to actual foraging by agent 0.
	var foraged bool
	for _, e := range logA {
		if e.Type == "agent.foraged" {
			var p HarvestPayload
			json.Unmarshal(e.Payload, &p)
			if p.Agent == 0 {
				foraged = true
			}
		}
	}
	if !foraged {
		t.Fatal("planner-injected forage intent never completed")
	}

	// Determinism with injections in the timeline (SC-005).
	s2 := NewState(seed, m)
	logB := driveTicks(t, s2, m, 3600, timeline)
	if s.Hash() != s2.Hash() {
		t.Fatal("state hash diverged with identical injected timeline")
	}
	if len(logA) != len(logB) {
		t.Fatal("event count diverged with identical injected timeline")
	}
}

// TestResolveGoalErrors: impossible goals are refused with no event.
func TestResolveGoalErrors(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	if _, _, err := resolveGoal(s, m, 0, "summon_gru", -1, 0); err == nil {
		t.Error("unknown goal should error")
	}
	if _, _, err := resolveGoal(s, m, 0, "eat", -1, 0); err == nil {
		t.Error("eat with no food should error")
	}
	if _, _, err := resolveGoal(s, m, 0, "build_fire", -1, 0); err == nil {
		t.Error("build_fire with no wood should error")
	}
	if _, _, err := resolveGoal(s, m, 0, "talk_to", 0, 0); err == nil {
		t.Error("talking to yourself should error")
	}
	if in, _, err := resolveGoal(s, m, 0, "talk_to", 1, 0); err != nil || in.Goal != "seek" {
		t.Errorf("talk_to should resolve to seek: %v %v", in, err)
	}
}

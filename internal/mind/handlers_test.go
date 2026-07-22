package mind

// T011 handler-level unit tests: each villager tool handler drives its real
// landing door (a running sim.Loop) and translates the door's verdict. World
// verbs and set_plan land through Loop.InjectIntent; muse lands through the
// social door. These exercise the door integration directly — the full
// tool-use loop driver is covered by internal/toolloop (T010) and the migrated
// runPlan path (T012).

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// loopMind builds a bare Mind wired to a real running sim.Loop (no mind
// goroutines) for handler-level tests. The loop is both the Injector and the
// SocialInjector, so every handler hits the real door.
type loopMind struct {
	md *Mind
	st *store.Store
}

func newLoopMind(t *testing.T) *loopMind {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	state.Speed = clock.Speed("max")
	loop := sim.NewLoop(state, m, st, nil)
	md := &Mind{
		loop:    loop,
		social:  loop,
		replica: state,
		m:       m,
		rearm:   make(chan int, sim.AgentCount),
	}
	md.tick.Store(state.Tick)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("loop did not stop")
		}
		st.Close()
	})
	return &loopMind{md: md, st: st}
}

// newJob builds a planner planJob against the current replica for agent i,
// mirroring plan()'s snapshot construction.
func (lm *loopMind) newJob(i int) planJob {
	md := lm.md
	tick := md.replica.Tick
	job := planJob{
		agent: i,
		name:  md.replica.Agents[i].Name,
		meta:  md.newMeta("planner", i, tick, 0, llm.KindPlanner),
	}
	job.meta.generation = md.replica.Agents[i].Generation
	for j := range md.replica.Agents {
		b := &md.replica.Agents[j]
		job.world[j] = agentSnap{x: b.X, y: b.Y, dead: b.Dead}
	}
	return job
}

func (lm *loopMind) events(t *testing.T, typ string) []store.Event {
	t.Helper()
	evs, err := lm.st.EventsSince(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Event
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func call(name, args string) llm.ToolCall {
	return llm.ToolCall{ID: name, Name: name, Args: json.RawMessage(args)}
}

// TestHandlerWorldVerbLands: a resolvable world verb lands through the door as
// an intent_set carrying the job id, and the handler reports landed.
func TestHandlerWorldVerbLands(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	d := &villagerDispatch{md: lm.md, job: job, start: time.Now()}
	h := lm.md.villagerHandlers(d)

	out := h["wander"](context.Background(), call("wander", "{}"))
	if out.Verdict != toolloop.VerdictLanded {
		t.Fatalf("wander verdict = %q, want landed (%q)", out.Verdict, out.ResultForModel)
	}
	if !d.doorOutcome {
		t.Error("door outcome flag not set on a landed verb")
	}
	intents := lm.events(t, "agent.intent_set")
	if len(intents) == 0 {
		t.Fatal("no intent_set landed")
	}
	var p sim.IntentSetPayload
	json.Unmarshal(intents[0].Payload, &p)
	if p.Source != "planner" {
		t.Errorf("intent source = %q, want planner", p.Source)
	}
	// The door emits its own cog.outcome(landed) — the handler must not add one.
	outcomes := lm.events(t, "cog.outcome")
	if len(outcomes) != 1 {
		t.Fatalf("landed verb produced %d cog.outcome events, want exactly 1", len(outcomes))
	}
}

// TestHandlerWorldVerbRejectedGate: a door rejection (talk_to a dead target,
// caught by the alive guard) comes back as rejected_gate carrying the door's
// queryable reason, and does not consume the action.
func TestHandlerWorldVerbRejectedGate(t *testing.T) {
	lm := newLoopMind(t)
	// Kill the target so its alive guard fails at the door.
	target := 1
	lm.md.replica.Agents[target].Dead = true
	job := lm.newJob(0)
	d := &villagerDispatch{md: lm.md, job: job, start: time.Now()}
	h := lm.md.villagerHandlers(d)

	tname := lm.md.replica.Agents[target].Name
	out := h["talk_to"](context.Background(), call("talk_to", `{"target":"`+tname+`"}`))
	if out.Verdict != toolloop.VerdictRejectedGate {
		t.Fatalf("verdict = %q, want rejected_gate", out.Verdict)
	}
	if out.ResultForModel == "" {
		t.Error("rejected_gate carries no reason to feed back")
	}
	if !d.doorOutcome {
		t.Error("a door rejection must set the door-outcome flag")
	}
}

// TestHandlerWorldVerbUnknownTarget: an unknown talk_to target is rejected
// before touching the door (no cog.outcome recorded), fed back for repair.
func TestHandlerWorldVerbUnknownTarget(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	d := &villagerDispatch{md: lm.md, job: job, start: time.Now()}
	h := lm.md.villagerHandlers(d)

	out := h["talk_to"](context.Background(), call("talk_to", `{"target":"Nobody"}`))
	if out.Verdict != toolloop.VerdictRejectedGate {
		t.Fatalf("verdict = %q, want rejected_gate", out.Verdict)
	}
	if d.doorOutcome {
		t.Error("an unknown target never reached the door — flag must stay unset")
	}
	if len(lm.events(t, "cog.outcome")) != 0 {
		t.Error("unknown target recorded a door outcome it never reached")
	}
}

// TestHandlerSetPlanLands: set_plan's steps translate into a landed plan_set
// carrying the job id on each step.
func TestHandlerSetPlanLands(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	d := &villagerDispatch{md: lm.md, job: job, start: time.Now()}
	h := lm.md.villagerHandlers(d)

	out := h["set_plan"](context.Background(),
		call("set_plan", `{"steps":[{"goal":"wander"},{"goal":"forage"}]}`))
	if out.Verdict != toolloop.VerdictLanded {
		t.Fatalf("set_plan verdict = %q, want landed (%q)", out.Verdict, out.ResultForModel)
	}
	plans := lm.events(t, "agent.plan_set")
	if len(plans) == 0 {
		t.Fatal("no plan_set landed")
	}
	var p sim.PlanSetPayload
	json.Unmarshal(plans[0].Payload, &p)
	if len(p.Steps) != 2 {
		t.Fatalf("plan has %d steps, want 2", len(p.Steps))
	}
	if p.Steps[0].Job != job.meta.job {
		t.Errorf("step job = %q, want %q", p.Steps[0].Job, job.meta.job)
	}
}

// TestHandlerMuseLandsThought: muse lands agent.thought + cog.outcome(landed)
// atomically through the social door, exactly as scheduled musing did.
func TestHandlerMuseLandsThought(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	social := &fakeSocial{}
	md := &Mind{social: social, replica: sim.NewState(42, m), m: m}
	job := planJob{agent: 0, name: md.replica.Agents[0].Name,
		meta: md.newMeta("planner", 0, md.replica.Tick, 0, llm.KindPlanner)}
	d := &villagerDispatch{md: md, job: job, start: time.Now()}
	h := md.villagerHandlers(d)

	out := h["muse"](context.Background(), call("muse", `{"text":"The frost is early this year."}`))
	if out.Verdict != toolloop.VerdictLanded {
		t.Fatalf("muse verdict = %q, want landed", out.Verdict)
	}
	if len(social.batches) != 1 || len(social.batches[0]) != 2 {
		t.Fatalf("muse batch = %v, want one batch of two events", social.batches)
	}
	batch := social.batches[0]
	if batch[0].Type != "agent.thought" || batch[1].Type != "cog.outcome" {
		t.Fatalf("batch types = %q,%q; want agent.thought,cog.outcome", batch[0].Type, batch[1].Type)
	}
	var tp sim.ThoughtPayload
	json.Unmarshal(batch[0].Payload, &tp)
	if tp.Text != "The frost is early this year." || tp.Source != "musing" {
		t.Errorf("thought payload = %+v", tp)
	}
}

// TestHandlerMuseEmptyRejected: an empty musing is fed back as a gate rejection
// rather than landing a blank thought.
func TestHandlerMuseEmptyRejected(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	social := &fakeSocial{}
	md := &Mind{social: social, replica: sim.NewState(42, m), m: m}
	job := planJob{agent: 0, meta: md.newMeta("planner", 0, md.replica.Tick, 0, llm.KindPlanner)}
	d := &villagerDispatch{md: md, job: job, start: time.Now()}
	h := md.villagerHandlers(d)

	out := h["muse"](context.Background(), call("muse", `{"text":"   "}`))
	if out.Verdict != toolloop.VerdictRejectedGate {
		t.Fatalf("empty muse verdict = %q, want rejected_gate", out.Verdict)
	}
	if len(social.batches) != 0 {
		t.Error("empty muse still landed a thought")
	}
}

// TestRecordSinkBuffers: the dispatch's Record sink buffers every CallRecord
// the driver hands it (T018 will land them as cog.tool_call events).
func TestRecordSinkBuffers(t *testing.T) {
	d := &villagerDispatch{}
	d.record(toolloop.CallRecord{JobID: "j", Ordinal: 1, Tool: "forage", Verdict: toolloop.VerdictLanded})
	d.record(toolloop.CallRecord{JobID: "j", Ordinal: 2, Tool: "muse", Verdict: toolloop.VerdictRejectedGate})
	if len(d.records) != 2 {
		t.Fatalf("sink buffered %d records, want 2", len(d.records))
	}
	if d.records[0].Tool != "forage" || d.records[1].Verdict != toolloop.VerdictRejectedGate {
		t.Errorf("buffered records: %+v", d.records)
	}
}

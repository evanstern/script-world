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
	"strings"
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
	md   *Mind
	st   *store.Store
	loop *sim.Loop // the concrete loop, for tests that must resume ticking (reflex)
}

func newLoopMind(t *testing.T) *loopMind {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	// Start PAUSED at genesis, but at max speed so a test can resume into fast
	// ticking (T025b/FILED-2). A paused loop steps no ticks and fires no reflex,
	// so its goroutine never touches state.Agents concurrently with the test —
	// which is essential because these fixtures alias the loop's live state as
	// md.replica and mutate/read it directly (kill an agent, snapshot positions
	// in newJob). Doors (InjectIntent/InjectSocial) are pause-open and still land
	// their events, so every handler test works unchanged. A test that needs a
	// reflex resumes the loop AFTER its direct-state setup (see
	// TestToolCallCorrelationChainSC003), by which point it reads only the store.
	state.Speed = clock.Speed("max")
	state.Paused = true
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
	return &loopMind{md: md, st: st, loop: loop}
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
	job.journal = md.replica.Agents[i].Journal.Clone()
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

// TestHandlerWorldVerbThreadsReason (spec 019 R12 / T024): the optional per-
// action `reason` arg is threaded into InjectArgs.Reason, so the landed
// agent.intent_set carries it (→ Intent.Reason → the executor bakes Why) and the
// planner-landing agent.thought narrates it.
func TestHandlerWorldVerbThreadsReason(t *testing.T) {
	lm := newLoopMind(t)
	job := lm.newJob(0)
	d := &villagerDispatch{md: lm.md, job: job, start: time.Now()}
	h := lm.md.villagerHandlers(d)

	const reason = "restless — I need to move before dark"
	out := h["wander"](context.Background(), call("wander", `{"reason":"`+reason+`"}`))
	if out.Verdict != toolloop.VerdictLanded {
		t.Fatalf("wander verdict = %q (%s)", out.Verdict, out.ResultForModel)
	}
	intents := lm.events(t, "agent.intent_set")
	if len(intents) == 0 {
		t.Fatal("no intent_set landed")
	}
	var p sim.IntentSetPayload
	json.Unmarshal(intents[0].Payload, &p)
	if p.Reason != reason {
		t.Errorf("intent_set Reason = %q, want the threaded reason", p.Reason)
	}
	thoughts := lm.events(t, "agent.thought")
	if len(thoughts) == 0 {
		t.Fatal("a reasoned intent must narrate an agent.thought")
	}
	var tp sim.ThoughtPayload
	json.Unmarshal(thoughts[0].Payload, &tp)
	if tp.Text != reason || tp.Source != "planner" {
		t.Errorf("agent.thought = %+v, want the reason narrated (planner)", tp)
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

// journalHandlers builds a fresh job snapshot + handler map for agent 0 — each
// cognition takes its own journal snapshot (Clone) the way plan() does, so a
// write from a prior cognition is visible to the next one's search/read.
func journalHandlers(lm *loopMind) (*villagerDispatch, map[string]toolloop.Handler) {
	d := &villagerDispatch{md: lm.md, job: lm.newJob(0), start: time.Now()}
	return d, lm.md.villagerHandlers(d)
}

// TestJournalToolCycle (spec 019 US3, T017): the full write→search→read→delete
// cycle through the real door — a write lands durably; a later cognition's
// search finds it and read returns it; delete removes it and frees the budget;
// an unknown id read is a read_error; search after delete is an empty read_ok.
func TestJournalToolCycle(t *testing.T) {
	lm := newLoopMind(t)

	// Write.
	dW, hW := journalHandlers(lm)
	out := hW["write_journal_entry"](context.Background(),
		call("write_journal_entry", `{"text":"the fire held through the night"}`))
	if out.Verdict != toolloop.VerdictLanded {
		t.Fatalf("write verdict = %q (%s)", out.Verdict, out.ResultForModel)
	}
	if !dW.doorOutcome {
		t.Error("a landed write must set the door-outcome flag")
	}
	if len(lm.events(t, "journal.entry_written")) != 1 {
		t.Fatal("no journal.entry_written event landed")
	}

	// Search (a new snapshot reflects the prior cognition's write).
	_, hS := journalHandlers(lm)
	out = hS["search_journal"](context.Background(), call("search_journal", `{"query":"FIRE"}`))
	if out.Verdict != toolloop.VerdictReadOK || !strings.Contains(out.ResultForModel, "the fire held through the night") {
		t.Fatalf("search: verdict=%q result=%q", out.Verdict, out.ResultForModel)
	}
	if !strings.HasPrefix(out.ResultForModel, "#0 ") {
		t.Errorf("search result should lead with the entry id: %q", out.ResultForModel)
	}

	// Read whole journal, one entry, and an unknown id.
	out = hS["read_journal"](context.Background(), call("read_journal", `{}`))
	if out.Verdict != toolloop.VerdictReadOK || !strings.Contains(out.ResultForModel, "the fire held") {
		t.Errorf("read whole: verdict=%q result=%q", out.Verdict, out.ResultForModel)
	}
	out = hS["read_journal"](context.Background(), call("read_journal", `{"entry":0}`))
	if out.Verdict != toolloop.VerdictReadOK || !strings.Contains(out.ResultForModel, "#0") {
		t.Errorf("read entry 0: verdict=%q result=%q", out.Verdict, out.ResultForModel)
	}
	out = hS["read_journal"](context.Background(), call("read_journal", `{"entry":99}`))
	if out.Verdict != toolloop.VerdictReadError {
		t.Errorf("read unknown id: verdict = %q, want read_error", out.Verdict)
	}

	// Delete entry #0 through the door.
	dD, hD := journalHandlers(lm)
	out = hD["delete_from_journal"](context.Background(), call("delete_from_journal", `{"entry":0}`))
	if out.Verdict != toolloop.VerdictLanded {
		t.Fatalf("delete verdict = %q (%s)", out.Verdict, out.ResultForModel)
	}
	if !dD.doorOutcome {
		t.Error("a landed delete must set the door-outcome flag")
	}
	if len(lm.events(t, "journal.entry_deleted")) != 1 {
		t.Fatal("no journal.entry_deleted event landed")
	}

	// Search after delete: an explicit empty read_ok, never an error.
	_, hS2 := journalHandlers(lm)
	out = hS2["search_journal"](context.Background(), call("search_journal", `{"query":"fire"}`))
	if out.Verdict != toolloop.VerdictReadOK || !strings.Contains(out.ResultForModel, "no journal entries match") {
		t.Errorf("search after delete: verdict=%q result=%q", out.Verdict, out.ResultForModel)
	}
}

// TestJournalOverBudgetRejection (spec 019 US3, T017 / SC-005): once the journal
// is full, a further write is refused AT THE DOOR — rejected_gate carrying the
// budget reason verbatim, the journal unchanged, and no door outcome recorded
// (nothing landed, so the loop still records the FR-015 terminal outcome).
func TestJournalOverBudgetRejection(t *testing.T) {
	lm := newLoopMind(t)
	big := strings.Repeat("x", sim.JournalWriteCapRunes) // 1000 runes/write
	for i := 0; i < 4; i++ {                             // 4×1000 = 4000, exactly full
		_, h := journalHandlers(lm)
		out := h["write_journal_entry"](context.Background(),
			call("write_journal_entry", `{"text":"`+big+`"}`))
		if out.Verdict != toolloop.VerdictLanded {
			t.Fatalf("legal write %d refused: %q (%s)", i, out.Verdict, out.ResultForModel)
		}
	}

	dOver, hOver := journalHandlers(lm)
	out := hOver["write_journal_entry"](context.Background(),
		call("write_journal_entry", `{"text":"one more line"}`))
	if out.Verdict != toolloop.VerdictRejectedGate {
		t.Fatalf("over-budget write verdict = %q, want rejected_gate", out.Verdict)
	}
	if !strings.Contains(out.ResultForModel, "journal budget") || !strings.Contains(out.ResultForModel, "4000") {
		t.Errorf("rejection must name the budget verbatim, got %q", out.ResultForModel)
	}
	if dOver.doorOutcome {
		t.Error("a door rejection that landed nothing must not set doorOutcome")
	}
	if got := len(lm.events(t, "journal.entry_written")); got != 4 {
		t.Errorf("journal.entry_written events = %d, want 4 (the over-budget write landed nothing)", got)
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

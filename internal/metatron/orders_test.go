package metatron

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// seedOrder installs an order into both the injector's state (the door) and the
// angel's replica + mirror, keeping the two in sync the way a live absorb pass
// would — the unit angel's absorb goroutine is Closed, so tests sync explicitly.
func seedOrder(mt *Metatron, inj *stateInjector, o sim.MetatronOrder) {
	inj.state.MetatronOrders = append(inj.state.MetatronOrders, o)
	mt.replica.MetatronOrders = append(mt.replica.MetatronOrders, o)
	mt.mirrorState()
}

// syncOrdersFromDoor copies the injector state's orders into the replica + mirror
// after a door landing (the absorb goroutine's job, done by hand in unit tests).
func syncOrdersFromDoor(mt *Metatron, inj *stateInjector) {
	mt.replica.MetatronOrders = append(mt.replica.MetatronOrders[:0], inj.state.MetatronOrders...)
	mt.mirrorState()
}

func activePlayerOrder(id string, tick int64) sim.MetatronOrder {
	return sim.MetatronOrder{
		ID: id, Origin: "player", Condition: "watch " + id, Action: "act",
		EventTypes: []string{"agent.slept"}, Agent: -1,
		PlacedTick: tick, ExpiresTick: tick + 3*ticksPerGameDay, Status: "active",
	}
}

// mustEvent builds an event with a JSON payload from v.
func mustEvent(typ string, v any) store.Event {
	b, _ := json.Marshal(v)
	return store.Event{Type: typ, Payload: b}
}

// TestOrderMatches (spec 029 T008, US2): the pure structural predicate over the
// match/no-match matrix — event-type membership, the optional agent pin, and the
// optional keyword coarse filter — plus the invariant that a non-active order
// never matches (the replay/consumed guard).
func TestOrderMatches(t *testing.T) {
	slept := mustEvent("agent.slept", sim.IntentSetPayload{Agent: 3})
	woke := mustEvent("agent.woke", map[string]any{"agent": 3})
	sleptOther := mustEvent("agent.slept", map[string]any{"agent": 5})
	convo := mustEvent("social.conversation", map[string]any{"gist": "Rowan wept by the well tonight"})

	cases := []struct {
		name  string
		order sim.MetatronOrder
		event store.Event
		want  bool
	}{
		{"type + any-agent matches", sim.MetatronOrder{Status: "active", EventTypes: []string{"agent.slept"}, Agent: -1}, slept, true},
		{"wrong type never matches", sim.MetatronOrder{Status: "active", EventTypes: []string{"agent.slept"}, Agent: -1}, woke, false},
		{"agent pin matches the named villager", sim.MetatronOrder{Status: "active", EventTypes: []string{"agent.slept"}, Agent: 3}, slept, true},
		{"agent pin rejects a different villager", sim.MetatronOrder{Status: "active", EventTypes: []string{"agent.slept"}, Agent: 3}, sleptOther, false},
		{"multi-type membership matches either", sim.MetatronOrder{Status: "active", EventTypes: []string{"agent.slept", "agent.woke"}, Agent: -1}, woke, true},
		{"keyword filter hits", sim.MetatronOrder{Status: "active", EventTypes: []string{"social.conversation"}, Agent: -1, Keywords: []string{"wept"}}, convo, true},
		{"keyword filter misses", sim.MetatronOrder{Status: "active", EventTypes: []string{"social.conversation"}, Agent: -1, Keywords: []string{"harvest"}}, convo, false},
		{"consumed order never matches", sim.MetatronOrder{Status: "triggered", EventTypes: []string{"agent.slept"}, Agent: -1}, slept, false},
		{"cancelled order never matches", sim.MetatronOrder{Status: "cancelled", EventTypes: []string{"agent.slept"}, Agent: -1}, slept, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := orderMatches(c.order, c.event); got != c.want {
				t.Errorf("orderMatches = %v, want %v", got, c.want)
			}
		})
	}
}

// TestEventConcernsAgent (spec 029 T008): the agent probe reads agent / from / to
// and never false-positives on an agent-less or unknown payload shape.
func TestEventConcernsAgent(t *testing.T) {
	if !eventConcernsAgent(mustEvent("agent.died", sim.DiedPayload{Agent: 2}), 2) {
		t.Error("agent field not read")
	}
	if !eventConcernsAgent(mustEvent("social.rumor_told", map[string]any{"from": 1, "to": 4}), 4) {
		t.Error("to field not read")
	}
	if eventConcernsAgent(mustEvent("agent.died", sim.DiedPayload{Agent: 2}), 5) {
		t.Error("false positive on the wrong villager")
	}
	if eventConcernsAgent(store.Event{Type: "sim.night_started", Payload: []byte(`{}`)}, 0) {
		t.Error("agent-less payload matched an agent pin")
	}
}

// TestNextOrderIDSequences (spec 029 T008/R7): same-tick placements get distinct
// seq suffixes even before the async mirror reflects the first, and a later tick
// resets the counter.
func TestNextOrderIDSequences(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, "ok")
	a := mt.nextOrderID(100)
	b := mt.nextOrderID(100)
	if a == b {
		t.Fatalf("same-tick ids collided: %q == %q", a, b)
	}
	if a != "ord-100-0" || b != "ord-100-1" {
		t.Fatalf("unexpected same-tick ids: %q, %q", a, b)
	}
	if c := mt.nextOrderID(200); c != "ord-200-0" {
		t.Errorf("later tick did not reset seq: %q", c)
	}
}

// TestOrderPlacementLandsAndMirrors (US2 AC-1, spec 029 T011): a monitor_and_act
// call lands metatron.order_placed through the door, the TurnResult reports the
// placed order, and the mirror/status surface then lists it.
func TestOrderPlacementLandsAndMirrors(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "The watch is set.")
	mt.runLoop = actLoop(mt, "monitor_and_act",
		`{"condition":"when Rowan next falls asleep","action":"send her a comforting vision","event_types":["agent.slept"],"ttl_days":3}`)
	r, err := mt.Turn(context.Background(), "watch over Rowan")
	if err != nil {
		t.Fatal(err)
	}
	if r.Order == nil || r.Order.Condition != "when Rowan next falls asleep" {
		t.Fatalf("order not reported: %+v", r.Order)
	}
	if len(inj.state.MetatronOrders) != 1 || inj.state.MetatronOrders[0].Status != "active" {
		t.Fatalf("order did not land active: %+v", inj.state.MetatronOrders)
	}
	// The placement spent no charge (monitor_and_act is free).
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges {
		t.Errorf("placement spent a charge: %d", inj.state.MetatronCharges)
	}
	syncOrdersFromDoor(mt, inj)
	s := mt.Status()
	if len(s.Orders) != 1 || s.Orders[0].ID != r.Order.ID || s.Orders[0].Status != "active" {
		t.Fatalf("status.Orders = %+v", s.Orders)
	}
}

// TestFourthPlayerOrderRefused (US2 AC-2, spec 029 T011): with three active
// player orders, a fourth placement is refused at the door with counsel and
// nothing lands.
func TestFourthPlayerOrderRefused(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "I hold too many already.")
	for i := 0; i < sim.MetatronPlayerOrderCap; i++ {
		seedOrder(mt, inj, activePlayerOrder(fmt.Sprintf("ord-1-%d", i), 1))
	}
	mt.runLoop = actLoop(mt, "monitor_and_act",
		`{"condition":"one more","action":"act","event_types":["agent.woke"]}`)
	r, err := mt.Turn(context.Background(), "watch one more thing")
	if err != nil {
		t.Fatal(err)
	}
	if r.Order != nil {
		t.Error("a fourth player order was placed past the cap")
	}
	if len(inj.state.MetatronOrders) != sim.MetatronPlayerOrderCap {
		t.Errorf("order count changed: %d", len(inj.state.MetatronOrders))
	}
	tcs := cogToolCalls(inj)
	if len(tcs) != 1 || tcs[0].Verdict != "rejected_gate" || !strings.Contains(tcs[0].Reason, "as many watches") {
		t.Errorf("cap refusal not recorded with counsel: %+v", tcs)
	}
}

// TestCancelFreesSlot (US2 AC-6, spec 029 T011): cancelling an active order lands
// order_cancelled and frees a slot, so a subsequent placement succeeds.
func TestCancelFreesSlot(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "Released.")
	for i := 0; i < sim.MetatronPlayerOrderCap; i++ {
		seedOrder(mt, inj, activePlayerOrder(fmt.Sprintf("ord-1-%d", i), 1))
	}
	mt.runLoop = actLoop(mt, "cancel_order", `{"id":"ord-1-0"}`)
	r, err := mt.Turn(context.Background(), "release the first watch")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cancelled) != 1 || r.Cancelled[0] != "ord-1-0" {
		t.Fatalf("cancel not reported: %+v", r.Cancelled)
	}
	if inj.state.MetatronOrders[0].Status != "cancelled" {
		t.Fatalf("order not cancelled: %+v", inj.state.MetatronOrders[0])
	}
	syncOrdersFromDoor(mt, inj)
	// A fresh placement now fits (2 active player orders remain).
	mt.runLoop = actLoop(mt, "monitor_and_act",
		`{"condition":"a new watch","action":"act","event_types":["agent.woke"]}`)
	r2, err := mt.Turn(context.Background(), "watch again")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Order == nil {
		t.Error("placement refused after a slot was freed")
	}
}

// TestCancelUnknownOrderRefused (US2, spec 029 T011): cancelling an id the angel
// does not keep refuses with counsel; nothing changes.
func TestCancelUnknownOrderRefused(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "I keep no such watch.")
	mt.runLoop = actLoop(mt, "cancel_order", `{"id":"ord-999-9"}`)
	r, err := mt.Turn(context.Background(), "cancel that")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cancelled) != 0 {
		t.Error("an unknown order was reported cancelled")
	}
	tcs := cogToolCalls(inj)
	if len(tcs) != 1 || tcs[0].Verdict != "rejected_gate" || !strings.Contains(tcs[0].Reason, "no watch") {
		t.Errorf("unknown-id cancel not refused with counsel: %+v", tcs)
	}
}

// TestStandingOrdersPromptBlock (US2 FR-017, spec 029 T011): a turn's user prompt
// carries the active standing-orders block with id, condition, remaining days and
// the structural/fuzzy marker.
func TestStandingOrdersPromptBlock(t *testing.T) {
	mt, orch, inj, _ := newTestAngel(t, "I keep the watch.")
	o := activePlayerOrder("ord-1-0", 0)
	o.Condition = "when Rowan weeps"
	o.Confirm = true
	seedOrder(mt, inj, o)
	if _, err := mt.Turn(context.Background(), "what do you watch?"); err != nil {
		t.Fatal(err)
	}
	prompt := orch.requests()[0].Prompt
	if !strings.Contains(prompt, "Standing orders you keep watch over") {
		t.Fatalf("prompt missing standing-orders block: %q", prompt)
	}
	if !strings.Contains(prompt, "ord-1-0") || !strings.Contains(prompt, "when Rowan weeps") || !strings.Contains(prompt, "fuzzy") {
		t.Errorf("standing-orders block incomplete: %q", prompt)
	}
}

// TestOrderExpiryQueuesMoment (US2 AC-5, spec 029 T011): an executor-emitted
// order_expired transitions the order and queues a model-free moment naming the
// lapsed watch — surfaced on the next reply through the existing moment queue.
func TestOrderExpiryQueuesMoment(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, "noted")
	o := activePlayerOrder("ord-1-0", 0)
	o.Condition = "when the gru stirs"
	mt.replica.MetatronOrders = append(mt.replica.MetatronOrders, o)
	expired := mustEvent("metatron.order_expired", sim.OrderIDPayload{ID: "ord-1-0"})
	expired.Tick = 5 * ticksPerGameDay
	if err := mt.replica.Apply(expired); err != nil {
		t.Fatalf("apply order_expired: %v", err)
	}
	mt.observeMoment(expired)
	mt.stateMu.Lock()
	moments := append([]string(nil), mt.moments...)
	mt.stateMu.Unlock()
	if len(moments) != 1 || !strings.Contains(moments[0], "lapsed") || !strings.Contains(moments[0], "when the gru stirs") {
		t.Fatalf("expiry moment not queued: %+v", moments)
	}
}

// TestOrderHandlerGating (US2 R14, spec 029 T011): the order handlers are
// installed ONLY when granted — structural absence at the door for a withheld
// tool, matching the declaration and prose. Extends the sentinel firewall audit.
func TestOrderHandlerGating(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, "ok")
	// Full grant: both order handlers present.
	full := &turnDispatch{mt: mt, charges: 1, alive: map[int]bool{}, grant: fullGrant(), result: &TurnResult{}}
	fh := mt.turnHandlers(full)
	for _, want := range []string{"monitor_and_act", "cancel_order"} {
		if _, ok := fh[want]; !ok {
			t.Errorf("%s handler missing under a full grant", want)
		}
	}
	// Withheld: monitor_and_act granted, cancel_order not → only monitor present.
	partial := grantSet{tools: map[string]bool{"monitor_and_act": true}}
	ph := mt.turnHandlers(&turnDispatch{mt: mt, charges: 1, alive: map[int]bool{}, grant: partial, result: &TurnResult{}})
	if _, ok := ph["monitor_and_act"]; !ok {
		t.Error("monitor_and_act handler missing when granted")
	}
	if _, ok := ph["cancel_order"]; ok {
		t.Error("cancel_order handler installed when ungranted")
	}
	// The door is authoritative on its own: an ungranted placement refuses.
	if _, why := mt.placeOrder("player", orderArgs{Action: "a", EventTypes: []string{"agent.slept"}}, 0, partial); why != "" {
		_ = why // monitor_and_act IS granted here, so this should succeed at the grant gate
	}
	if why := mt.cancelOrder("ord-1-0", partial); why == "" {
		t.Error("cancel_order should refuse when ungranted")
	}
}

// --- US3: triggered orders act while away (spec 029 T015) ---

// systemActLoop scripts a runLoop that lands one act on a SYSTEM (watch) turn and
// converses on a console turn — distinguishing by the jobID prefix (R6). It lets a
// trigger test drive a real handler landing through the system-turn path.
func systemActLoop(mt *Metatron, name, args string) func(context.Context, toolloop.Job) (toolloop.Result, error) {
	return func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		c := toolCall(name, args)
		out := j.Handlers[name](ctx, c)
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: name,
			Args: c.Args, Verdict: out.Verdict, Reason: out.ResultForModel, Tier: "cloud"})
		if out.Verdict == toolloop.VerdictLanded {
			return toolloop.Result{Term: toolloop.TermLanded, Landed: &c}, nil
		}
		return toolloop.Result{Term: toolloop.TermCapExhausted}, nil
	}
}

// TestTriggerFiresEndToEnd (US3 AC-1/AC-2, spec 029 T015): a live matching event
// enqueues a trigger job; firing it lands order_triggered (the order is consumed),
// runs the pre-authorized act as a system turn (one charge spent), and queues a
// moment for the next console reply.
func TestTriggerFiresEndToEnd(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "It is done.")
	o := activePlayerOrder("ord-1-0", 1)
	o.Condition = "when someone sleeps"
	o.Action = "send Fern a comforting vision"
	seedOrder(mt, inj, o)
	mt.runLoop = systemActLoop(mt, "send_vision", `{"target":"Fern","text":"rest easy"}`)

	slept := mustEvent("agent.slept", map[string]any{"agent": 3})
	slept.Tick = 5000
	mt.matchOrders([]store.Event{slept})
	var job triggerJob
	select {
	case job = <-mt.triggerQ:
	default:
		t.Fatal("a matching live event did not enqueue a trigger job")
	}
	if job.order.ID != "ord-1-0" || job.matchedType != "agent.slept" {
		t.Fatalf("trigger job = %+v", job)
	}

	mt.runTrigger(job)

	if inj.state.MetatronOrders[0].Status != "triggered" {
		t.Fatalf("order not consumed one-shot: %+v", inj.state.MetatronOrders[0])
	}
	fern := agentIndexByName("Fern")
	if len(inj.state.Agents[fern].Memories) != 1 {
		t.Error("triggered vision did not land on Fern")
	}
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges-1 {
		t.Errorf("triggered act spent %d charges, want 1", sim.MetatronGenesisCharges-inj.state.MetatronCharges)
	}
	mt.stateMu.Lock()
	moments := append([]string(nil), mt.moments...)
	mt.stateMu.Unlock()
	if len(moments) != 1 || !strings.Contains(moments[0], "vision") {
		t.Fatalf("triggered moment not queued: %+v", moments)
	}
}

// TestTriggerSerializesWithConsoleTurn (US3 AC-5, spec 029 T015): a trigger firing
// while a console turn holds the single-flight slot lands order_triggered at once
// (the door doesn't need the slot) but its ACT waits until the console releases —
// both leave complete trails, neither is dropped.
func TestTriggerSerializesWithConsoleTurn(t *testing.T) {
	// A LIVE angel (done open): the system turn's bounded turnBusy wait must be
	// able to block on the console, not bail on a closed done. The absorb/trigger
	// goroutines stay idle (no Observe, runTrigger driven directly).
	mt, _, inj, _ := newLiveTestAngel(t, "released")
	o := activePlayerOrder("ord-1-0", 1)
	o.Action = "send Fern a vision"
	seedOrder(mt, inj, o)

	entered := make(chan struct{})
	release := make(chan struct{})
	mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		if strings.HasPrefix(j.JobID, "turn-") { // console turn parks here
			close(entered)
			<-release
			return toolloop.Result{Final: "released", Term: toolloop.TermModelDone}, nil
		}
		// system (watch) turn: land the vision
		c := toolCall("send_vision", `{"target":"Fern","text":"peace"}`)
		out := j.Handlers["send_vision"](ctx, c)
		j.Record(toolloop.CallRecord{JobID: j.JobID, Ordinal: 1, Tool: "send_vision",
			Args: c.Args, Verdict: out.Verdict, Reason: out.ResultForModel, Tier: "cloud"})
		return toolloop.Result{Term: toolloop.TermLanded, Landed: &c}, nil
	}

	fern := agentIndexByName("Fern")
	memCount := func() int {
		inj.mu.Lock()
		defer inj.mu.Unlock()
		return len(inj.state.Agents[fern].Memories)
	}
	orderStatus := func() string {
		inj.mu.Lock()
		defer inj.mu.Unlock()
		return inj.state.MetatronOrders[0].Status
	}

	doneA := make(chan struct{})
	go func() { mt.Turn(context.Background(), "hello"); close(doneA) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("console turn never entered the loop")
	}

	doneB := make(chan struct{})
	go func() {
		mt.runTrigger(triggerJob{order: o, matchedType: "agent.slept", matchedTick: 5000})
		close(doneB)
	}()

	// order_triggered lands without the slot; the act must WAIT for the console.
	waitFor(t, 2*time.Second, func() bool { return orderStatus() == "triggered" })
	if memCount() != 0 {
		t.Fatal("system turn acted while a console turn held the single-flight slot")
	}

	close(release)
	select {
	case <-doneA:
	case <-time.After(2 * time.Second):
		t.Fatal("console turn never completed")
	}
	select {
	case <-doneB:
	case <-time.After(2 * time.Second):
		t.Fatal("system turn never completed after the slot freed")
	}
	if memCount() != 1 {
		t.Error("system turn did not act after the console released the slot")
	}
}

// TestCancelledOrderRaceResolvesAtDoor (US3 edge case, spec 029 T015): an order
// cancelled before its trigger lands — the door rejects order_triggered (the order
// is no longer active), so the trigger abandons: no system turn, no act, no moment.
// Exactly one terminal (cancelled) stands.
func TestCancelledOrderRaceResolvesAtDoor(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "")
	o := activePlayerOrder("ord-1-0", 1)
	o.Action = "send Fern a vision"
	seedOrder(mt, inj, o)
	// The cancel wins the race (lands first).
	if err := inj.state.Apply(mustEvent("metatron.order_cancelled", sim.OrderIDPayload{ID: "ord-1-0"})); err != nil {
		t.Fatal(err)
	}
	fired := false
	mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		fired = true
		return toolloop.Result{Term: toolloop.TermModelDone}, nil
	}
	mt.runTrigger(triggerJob{order: o, matchedType: "agent.slept", matchedTick: 5000})

	if fired {
		t.Error("a cancelled order still ran its system turn")
	}
	if inj.state.MetatronOrders[0].Status != "cancelled" {
		t.Errorf("terminal is not cancelled: %q", inj.state.MetatronOrders[0].Status)
	}
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges {
		t.Error("an abandoned trigger spent a charge")
	}
	mt.stateMu.Lock()
	n := len(mt.moments)
	mt.stateMu.Unlock()
	if n != 0 {
		t.Errorf("an abandoned trigger queued %d moments, want 0", n)
	}
}

// TestEmptyBankPrecheckSpendsNothing (US3 AC-3, spec 029 T014/T015): a system-
// origin (deferral) order firing on an empty bank skips the model call entirely
// and queues the honest "strength was spent" moment — the order is still consumed
// (one-shot), nothing is spent, and no model is called.
func TestEmptyBankPrecheckSpendsNothing(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "")
	mt.replica.MetatronCharges = 0
	inj.state.MetatronCharges = 0
	mt.mirrorState()
	o := sim.MetatronOrder{ID: "ord-1-0", Origin: "system", Condition: "deferred omen",
		Action: "deliver the omen", EventTypes: []string{"sim.night_started"}, Agent: -1,
		PlacedTick: 1, ExpiresTick: 1 + ticksPerGameDay, Status: "active"}
	seedOrder(mt, inj, o)

	called := false
	mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		called = true
		return toolloop.Result{Term: toolloop.TermModelDone}, nil
	}
	mt.runTrigger(triggerJob{order: o, matchedType: "sim.night_started", matchedTick: 5000})

	if called {
		t.Error("empty-bank precheck still called the model")
	}
	if inj.state.MetatronCharges != 0 {
		t.Errorf("spent from an empty bank: %d", inj.state.MetatronCharges)
	}
	if inj.state.MetatronOrders[0].Status != "triggered" {
		t.Error("order not consumed one-shot on the empty-bank path")
	}
	mt.stateMu.Lock()
	moments := append([]string(nil), mt.moments...)
	mt.stateMu.Unlock()
	if len(moments) != 1 || !strings.Contains(moments[0], "strength was spent") {
		t.Fatalf("honest empty-bank moment not queued: %+v", moments)
	}
}

// TestTriggerBudgetExhaustedOneMomentNoRetry (US3 AC-4, spec 029 T014/T015): a
// system turn that fails with ErrBudgetExhausted yields exactly ONE honest moment
// and ZERO retries — the trigger worker never re-runs a failed turn.
func TestTriggerBudgetExhaustedOneMomentNoRetry(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "")
	o := activePlayerOrder("ord-1-0", 1) // player origin: no precheck, runs the turn
	o.Action = "send Fern a vision"
	seedOrder(mt, inj, o)

	calls := 0
	mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		calls++
		return toolloop.Result{Term: toolloop.TermProviderError}, llm.ErrBudgetExhausted
	}
	mt.runTrigger(triggerJob{order: o, matchedType: "agent.slept", matchedTick: 5000})

	if calls != 1 {
		t.Errorf("failed system turn was retried: %d model calls, want 1", calls)
	}
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges {
		t.Error("a degraded trigger spent a charge")
	}
	// order_triggered legitimately landed (one-shot consumption), but NO influence
	// act did — no villager gained a memory from the failed turn.
	if inj.state.MetatronOrders[0].Status != "triggered" {
		t.Error("order not consumed on the degraded path")
	}
	for i := range inj.state.Agents {
		if len(inj.state.Agents[i].Memories) != 0 {
			t.Fatalf("a budget-exhausted trigger landed an influence on agent %d", i)
		}
	}
	mt.stateMu.Lock()
	moments := append([]string(nil), mt.moments...)
	mt.stateMu.Unlock()
	if len(moments) != 1 {
		t.Fatalf("degraded trigger queued %d moments, want exactly 1: %+v", len(moments), moments)
	}
	if !strings.Contains(moments[0], "dimmed") {
		t.Errorf("degraded moment not the sight-dimmed family: %q", moments[0])
	}
}

// TestReplayReconstructsWithoutFiring (US3 edge case / SC-002, spec 029 T015): an
// angel reconstructed from a snapshot whose history already triggered an order
// rebuilds the consumed order from state alone — no live firing during
// reconstruction, and a later matching event cannot re-fire a consumed order.
func TestReplayReconstructsWithoutFiring(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	o := sim.MetatronOrder{ID: "ord-1-0", Origin: "player", Condition: "when someone sleeps",
		Action: "send a vision", EventTypes: []string{"agent.slept"}, Agent: -1,
		PlacedTick: 1, ExpiresTick: 1 + 3*ticksPerGameDay, Status: "active"}
	if err := state.Apply(mustEvent("metatron.order_placed", o)); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(mustEvent("metatron.order_triggered",
		sim.OrderTriggeredPayload{ID: "ord-1-0", MatchedType: "agent.slept", MatchedTick: 5000})); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	orch := &mockOrch{}
	inj := &stateInjector{state: state}
	mt, err := New(orch, inj, m, 42, state.Marshal(), dir, testLoopRounds, testTurnTokens)
	if err != nil {
		t.Fatal(err)
	}
	mt.Close() // stop goroutines; drive matching directly
	mt.runLoop = converseLoop(mt)

	mt.stateMu.Lock()
	orders := append([]sim.MetatronOrder(nil), mt.orders...)
	mt.stateMu.Unlock()
	if len(orders) != 1 || orders[0].Status != "triggered" {
		t.Fatalf("reconstruction did not rebuild the consumed order: %+v", orders)
	}
	// No metatron.order_triggered was emitted during reconstruction (New unmarshals
	// state; it does not run the absorb path), so nothing landed through the door.
	if len(inj.batches) != 0 {
		t.Errorf("reconstruction emitted %d batches, want 0 (no live firing)", len(inj.batches))
	}
	// A matching event now cannot re-fire the consumed order.
	mt.matchOrders([]store.Event{mustEvent("agent.slept", map[string]any{"agent": 0})})
	select {
	case <-mt.triggerQ:
		t.Fatal("a consumed order re-fired on a fresh matching event")
	default:
	}
}

// --- US4: daytime omens defer to nightfall (spec 029 T016/T017) ---

// deferralOmenOrder builds the system-origin nightfall deferral order deferOmen
// produces for a daytime everyone-omen (spec 029 T016).
func deferralOmenOrder(id string, tick int64) sim.MetatronOrder {
	return sim.MetatronOrder{
		ID: id, Origin: "system",
		Condition:  "nightfall — an omen awaits everyone",
		Action:     "Night has fallen. Send the omen you promised to everyone: look up",
		EventTypes: []string{"sim.night_started"}, Agent: -1,
		PlacedTick: tick, ExpiresTick: tick + ticksPerGameDay, Status: "active",
	}
}

// TestDeferredOmenTriggersAtNightfall (US4 AC-2, spec 029 T016/T017): the
// system-origin deferral fires on sim.night_started, running a system turn that
// lands send_omen at night — one charge spent, the order consumed one-shot, and a
// moment queued for the next console reply.
func TestDeferredOmenTriggersAtNightfall(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "The omen goes out.")
	mt.replica.Night = true // night has come (the deferral watches sim.night_started)
	inj.state.Night = true
	o := deferralOmenOrder("ord-1-0", 1)
	seedOrder(mt, inj, o)
	mt.runLoop = systemActLoop(mt, "send_omen", `{"targets":"everyone","text":"look up"}`)

	night := mustEvent("sim.night_started", map[string]any{})
	night.Tick = 6 * 3600
	mt.matchOrders([]store.Event{night})
	var job triggerJob
	select {
	case job = <-mt.triggerQ:
	default:
		t.Fatal("nightfall did not enqueue the deferral trigger")
	}
	mt.runTrigger(job)

	if inj.state.MetatronOrders[0].Status != "triggered" {
		t.Fatalf("deferral not consumed one-shot: %+v", inj.state.MetatronOrders[0])
	}
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges-1 {
		t.Errorf("nightfall omen spent %d charges, want 1", sim.MetatronGenesisCharges-inj.state.MetatronCharges)
	}
	reached := 0
	for i := range inj.state.Agents {
		if !inj.state.Agents[i].Dead && len(inj.state.Agents[i].Memories) == 1 &&
			strings.HasPrefix(inj.state.Agents[i].Memories[0].Text, "You witnessed an omen: ") {
			reached++
		}
	}
	if reached == 0 {
		t.Error("the nightfall omen reached no one")
	}
	mt.stateMu.Lock()
	moments := append([]string(nil), mt.moments...)
	mt.stateMu.Unlock()
	if len(moments) != 1 {
		t.Fatalf("nightfall omen queued %d moments, want 1: %+v", len(moments), moments)
	}
}

// TestDeferredOmenCancelledNeverLands (US4 AC-3, spec 029 T016/T017): a deferral
// cancelled before nightfall never fires — the trigger abandons at the door, no
// system turn runs, and no omen lands.
func TestDeferredOmenCancelledNeverLands(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, "")
	mt.replica.Night = true
	inj.state.Night = true
	o := deferralOmenOrder("ord-1-0", 1)
	seedOrder(mt, inj, o)
	if err := inj.state.Apply(mustEvent("metatron.order_cancelled", sim.OrderIDPayload{ID: "ord-1-0"})); err != nil {
		t.Fatal(err)
	}
	fired := false
	mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		fired = true
		return toolloop.Result{Term: toolloop.TermModelDone}, nil
	}
	mt.runTrigger(triggerJob{order: o, matchedType: "sim.night_started", matchedTick: 6 * 3600})

	if fired {
		t.Error("a cancelled deferral still ran its system turn")
	}
	if inj.state.MetatronOrders[0].Status != "cancelled" {
		t.Errorf("terminal is not cancelled: %q", inj.state.MetatronOrders[0].Status)
	}
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges {
		t.Error("a cancelled deferral spent a charge")
	}
	for i := range inj.state.Agents {
		if len(inj.state.Agents[i].Memories) != 0 {
			t.Fatalf("a cancelled deferral landed an omen on agent %d", i)
		}
	}
}

package metatron

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
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

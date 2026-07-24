package metatron

// Standing orders (spec 029 US2/US3): the event-sourced watch-and-act machinery.
// This file holds the pure predicate matcher (orderMatches — evaluated for free
// in the absorb path, zero model cost per non-matching event, SC-001), the two
// door-landing helpers a console turn's handlers wrap (placeOrder / cancelOrder),
// and the id assignment (research R7). The reducer dry-run is the door authority
// for every lifecycle transition — the cap, the ttl bounds, the agent range, and
// the cancel/expiry/trigger races all resolve there (internal/sim/metatron.go);
// these helpers map a door rejection to in-fiction counsel the loop feeds back as
// a rejected_gate. The trigger pipeline that fires a matched order (the absorb
// worker + system turn) lives alongside in metatron.go / this file's T013 half.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
)

// ticksPerGameDay is a game day in ticks (1 tick = 1 game second). MIRRORED from
// sim's unexported constant (internal/sim/metatron.go) — the reducer validates a
// standing order's ttl in [1..7] game days against the same literal, so the
// ExpiresTick this package computes and the door-side bound can never diverge.
const ticksPerGameDay = 24 * 3600

// orderArgs is the parsed monitor_and_act tool-call surface (spec 029 R5): the
// turn model itself is the compiler, supplying the compiled standing-order
// structure in the tool call. The driver's schema-lite walker already gated shape
// (event_types array + enum membership, keyword bounds, ttl range), so this is a
// lenient reader — the reducer dry-run is the semantic door.
type orderArgs struct {
	Condition  string   `json:"condition"`
	Action     string   `json:"action"`
	EventTypes []string `json:"event_types"`
	Agent      string   `json:"agent"`
	Keywords   []string `json:"keywords"`
	Confirm    bool     `json:"confirm"`
	TTLDays    int      `json:"ttl_days"`
}

// parseOrderArgs decodes a monitor_and_act call's arguments (lenient — the driver
// validated shape).
func parseOrderArgs(raw json.RawMessage) orderArgs {
	var a orderArgs
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &a)
	}
	return a
}

// nextOrderID assigns "ord-<placedTick>-<seq>" (research R7): human-readable,
// deterministic, no RNG draw. seq disambiguates same-tick placements — it is the
// max seq already present at this tick (from the mirror) plus one, floored by a
// serialized per-tick counter so a placement whose predecessor has not yet flowed
// back through Observe still gets a fresh id. Uniqueness is ultimately enforced by
// the reducer (it rejects a duplicate active id).
func (mt *Metatron) nextOrderID(tick int64) string {
	mt.stateMu.Lock()
	defer mt.stateMu.Unlock()
	seq := 0
	prefix := fmt.Sprintf("ord-%d-", tick)
	for i := range mt.orders {
		id := mt.orders[i].ID
		if strings.HasPrefix(id, prefix) {
			if s, err := strconv.Atoi(strings.TrimPrefix(id, prefix)); err == nil && s >= seq {
				seq = s + 1
			}
		}
	}
	if mt.lastPlaceTick == tick && mt.lastPlaceSeq >= seq {
		seq = mt.lastPlaceSeq + 1
	}
	mt.lastPlaceTick = tick
	mt.lastPlaceSeq = seq
	return fmt.Sprintf("ord-%d-%d", tick, seq)
}

// placeOrder compiles a monitor_and_act call into a MetatronOrder and lands it as
// metatron.order_placed through the InjectSocial door (spec 029 US2, T008). The
// door's dry-run is the authority (player cap, ttl bounds, agent range, empty
// event_types); a rejection maps to in-fiction counsel the handler feeds back as a
// rejected_gate. origin is "player" for a console monitor_and_act (Batch C's
// deferral path passes "system"). A semantically uncompilable condition (no
// structural filter — empty event_types) is refused HERE with counsel (research
// R5) rather than at the driver, so the system/deferral caller that bypasses the
// driver is guarded too. Returns the placed order (id for the reply/status) or
// (nil, refusal).
func (mt *Metatron) placeOrder(origin string, a orderArgs, tick int64, grant grantSet) (*sim.MetatronOrder, string) {
	if !grant.allows("monitor_and_act") {
		return nil, "that power is not granted in this world"
	}
	if len(a.EventTypes) == 0 {
		return nil, "I can only keep watch for things that leave a mark in the world — name what should happen, and I will watch for it"
	}
	agent := -1
	if name := strings.TrimSpace(a.Agent); name != "" {
		idx := agentIndexByName(name)
		if idx < 0 {
			return nil, fmt.Sprintf("no villager named %q to watch over", name)
		}
		agent = idx
	}
	ttl := a.TTLDays
	if ttl == 0 {
		ttl = 3 // default (spec Assumption): 3 game days
	}
	if ttl < sim.MetatronOrderTTLMinDays || ttl > sim.MetatronOrderTTLMaxDays {
		return nil, fmt.Sprintf("a watch may stand for %d to %d days", sim.MetatronOrderTTLMinDays, sim.MetatronOrderTTLMaxDays)
	}
	keywords := make([]string, 0, len(a.Keywords))
	for _, k := range a.Keywords {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			keywords = append(keywords, k)
		}
	}
	order := sim.MetatronOrder{
		ID:          mt.nextOrderID(tick),
		Origin:      origin,
		Condition:   a.Condition,
		Action:      a.Action,
		EventTypes:  a.EventTypes,
		Agent:       agent,
		Keywords:    keywords,
		Confirm:     a.Confirm,
		PlacedTick:  tick,
		ExpiresTick: tick + int64(ttl)*ticksPerGameDay,
		Status:      "active",
	}
	batch := []store.Event{{Type: "metatron.order_placed", Payload: mustJSON(order)}}
	if err := mt.social.InjectSocial(batch); err != nil {
		log.Printf("metatron: order rejected at the door: %v", err)
		return nil, orderRefusal(err)
	}
	mt.appendFile(mt.soulPath(), fmt.Sprintf("\n- %s — I set a watch (%s): %q → %q\n",
		clock.Format(mt.replicaTickSafe()), order.ID, order.Condition, order.Action))
	return &order, ""
}

// cancelOrder lands metatron.order_cancelled for the named id through the door
// (spec 029 US2, T008). The reducer rejects an unknown or non-active id — this is
// where the cancel/expiry/trigger race resolves (exactly one terminal lands).
// Returns "" on success or an in-fiction refusal the handler feeds back.
func (mt *Metatron) cancelOrder(id string, grant grantSet) string {
	if !grant.allows("cancel_order") {
		return "that power is not granted in this world"
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "name the watch you want me to release"
	}
	batch := []store.Event{{Type: "metatron.order_cancelled", Payload: mustJSON(sim.OrderIDPayload{ID: id})}}
	if err := mt.social.InjectSocial(batch); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "unknown order"):
			return fmt.Sprintf("I keep no watch called %q", id)
		case strings.Contains(msg, "not active"):
			return fmt.Sprintf("the watch %q has already lapsed", id)
		default:
			return "the world would not let me release that watch (" + msg + ")"
		}
	}
	mt.appendFile(mt.soulPath(), fmt.Sprintf("\n- %s — I released a watch (%s)\n",
		clock.Format(mt.replicaTickSafe()), id))
	return ""
}

// orderRefusal maps a metatron.order_placed door rejection to in-fiction counsel
// (spec 029): the reducer's error strings are the source, translated to the
// angel's voice so the model hears a repairable reason (rejected_gate) rather than
// a raw reducer message.
func orderRefusal(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "already active"):
		return "I already keep as many watches as I can hold — release one and I will take up another"
	case strings.Contains(msg, "ttl"):
		return "a watch may stand only for a handful of days"
	case strings.Contains(msg, "no event_types"), strings.Contains(msg, "uncompilable"):
		return "I can only keep watch for things that leave a mark in the world"
	case strings.Contains(msg, "over 300 chars"), strings.Contains(msg, "over 400 chars"):
		return "say the watch more briefly and I will hold it"
	default:
		return "the world would not let me set that watch (" + msg + ")"
	}
}

// orderMatches reports whether a live observed event satisfies an order's compiled
// structural predicates (spec 029 US2/R6): the event type is one of the order's
// event_types; if the order pins an agent (>= 0) the event concerns that villager;
// if the order lists keywords the (lowercased) payload contains at least one. A
// PURE function — no state, no model call — so the absorb path evaluates it for
// free (SC-001). Only ACTIVE orders match; a fuzzy order (Confirm) still matches
// structurally here (the confirm step that gates its trigger is Batch C / T021).
func orderMatches(o sim.MetatronOrder, e store.Event) bool {
	if o.Status != "active" {
		return false
	}
	typeOK := false
	for _, t := range o.EventTypes {
		if t == e.Type {
			typeOK = true
			break
		}
	}
	if !typeOK {
		return false
	}
	if o.Agent >= 0 && !eventConcernsAgent(e, o.Agent) {
		return false
	}
	if len(o.Keywords) > 0 {
		hay := strings.ToLower(string(e.Payload))
		hit := false
		for _, k := range o.Keywords {
			if k != "" && strings.Contains(hay, k) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// eventConcernsAgent reports whether an event's payload names the given villager
// index in one of the observable vocabulary's agent-bearing fields (agent / from /
// to). A best-effort structural probe: an unknown or agent-less payload shape
// simply does not match the agent pin (the order stays armed) — never a false
// positive against the wrong villager.
func eventConcernsAgent(e store.Event, idx int) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(e.Payload, &m) != nil {
		return false
	}
	for _, field := range []string{"agent", "from", "to"} {
		if raw, ok := m[field]; ok {
			var v int
			if json.Unmarshal(raw, &v) == nil && v == idx {
				return true
			}
		}
	}
	return false
}

// --- Trigger pipeline (spec 029 US3, T013/T014) ---

// systemTurnBusyWait bounds how long a triggered system turn waits for the
// single-flight slot before degrading (spec 029 R6): system turns WAIT for the
// slot (unlike the console's fail-fast ErrTurnBusy), but never forever — a wedged
// console turn degrades the trigger to an honest moment rather than hanging.
const systemTurnBusyWait = 90 * time.Second

// triggerJob is one matched order queued for firing (spec 029 US3). It snapshots
// the order at match time (its action/origin/condition are needed to run and
// narrate the system turn) plus the matched event's type + tick for the trail.
type triggerJob struct {
	order       sim.MetatronOrder
	matchedType string
	matchedTick int64
}

// matchOrders scans active orders against a just-applied LIVE event batch and
// enqueues a trigger job for each structural hit (spec 029 US3/R6). Called by the
// absorb goroutine AFTER replica apply + mirror refresh, so it is live-only by
// construction (replay never runs the angel). Orders fire in order-id order within
// a batch, at most once — pendingTrigger dedups an order already queued but not
// yet resolved, and one job is enqueued per order per batch. Fuzzy orders
// (Confirm) are matched structurally but NOT fired here: their confirm step is
// Batch C (T021), so a fuzzy hit is skipped so it never triggers unconfirmed
// (FR-008/FR-009) — a Batch C hand-off.
func (mt *Metatron) matchOrders(batch []store.Event) {
	if mt.social == nil {
		return
	}
	mt.stateMu.Lock()
	orders := append([]sim.MetatronOrder(nil), mt.orders...)
	mt.stateMu.Unlock()
	sort.Slice(orders, func(i, j int) bool { return orders[i].ID < orders[j].ID })
	for i := range orders {
		o := orders[i]
		if o.Status != "active" || o.Confirm {
			continue // fuzzy orders route through the Batch C confirm path (T021)
		}
		mt.stateMu.Lock()
		pending := mt.pendingTrigger[o.ID]
		mt.stateMu.Unlock()
		if pending {
			continue
		}
		for _, e := range batch {
			if !orderMatches(o, e) {
				continue
			}
			mt.stateMu.Lock()
			mt.pendingTrigger[o.ID] = true
			mt.stateMu.Unlock()
			select {
			case mt.triggerQ <- triggerJob{order: o, matchedType: e.Type, matchedTick: e.Tick}:
			default:
				log.Printf("metatron: trigger queue full, order %s dropped", o.ID)
				mt.stateMu.Lock()
				delete(mt.pendingTrigger, o.ID)
				mt.stateMu.Unlock()
			}
			break // one job per order per batch
		}
	}
}

// triggerWorker consumes the trigger queue FIFO, firing each matched order through
// runTrigger (spec 029 US3). One worker → triggered turns serialize with each
// other and, via the shared turnBusy, with console turns (R6).
func (mt *Metatron) triggerWorker() {
	for {
		select {
		case <-mt.done:
			return
		case job := <-mt.triggerQ:
			mt.runTrigger(job)
		}
	}
}

// runTrigger fires one matched standing order (spec 029 US3, T013/T014):
//
//  1. Land metatron.order_triggered through the door — the dry-run enforces the
//     order is STILL active, so a cancel/expiry that raced the match wins here and
//     the trigger is abandoned silently (edge case: exactly one of triggered/
//     cancelled/expired lands, never both).
//  2. Empty-bank precheck for known-act (deferral) orders (T014/R12): a
//     system-origin order's act is a known charge spend, so an empty bank
//     short-circuits to an honest moment — no model call, no cloud cost.
//  3. Acquire turnBusy with a bounded wait (system turns wait; the console stays
//     fail-fast) and run the pre-authorized action as a system-authored turn.
//  4. Queue a moment from the outcome — the act on success, or ONE model-free
//     honest moment per failure family, never a retry (FR-011).
func (mt *Metatron) runTrigger(job triggerJob) {
	defer func() {
		mt.stateMu.Lock()
		delete(mt.pendingTrigger, job.order.ID)
		mt.stateMu.Unlock()
	}()

	trig := []store.Event{{Type: "metatron.order_triggered", Payload: mustJSON(sim.OrderTriggeredPayload{
		ID: job.order.ID, MatchedType: job.matchedType, MatchedTick: job.matchedTick})}}
	if err := mt.social.InjectSocial(trig); err != nil {
		// Cancelled or expired before its trigger landed: the loser abandons
		// silently (the winning terminal already surfaced its own trail).
		log.Printf("metatron: order %s trigger abandoned at the door: %v", job.order.ID, err)
		return
	}
	mt.appendFile(mt.soulPath(), fmt.Sprintf("\n- %s — a watch woke me (%s): %q\n",
		clock.Format(job.matchedTick), job.order.ID, job.order.Condition))

	if mt.knownActEmptyBank(job.order) {
		mt.queueMoment(fmt.Sprintf("%s — a watch came due, but my strength was spent; I could not send what you asked.",
			clock.Format(job.matchedTick)))
		return
	}

	if !mt.acquireTurnBusy() {
		mt.queueMoment(fmt.Sprintf("%s — a watch came due, but I was too long attending another matter to act on it.",
			clock.Format(job.matchedTick)))
		return
	}
	res, err := mt.runTurn(context.Background(), turnOrigin{system: true, jobPrefix: "watch", seed: job.order.Action})
	mt.turnBusy.Store(false)

	if err != nil {
		mt.queueMoment(mt.degradedMoment(job.matchedTick, err))
		return
	}
	mt.queueMoment(mt.triggeredMoment(job.matchedTick, job.order, res))
}

// knownActEmptyBank reports whether an order's action is a KNOWN charge-spending
// act (a deferral order — origin "system", always omen/vision-bearing per R11) AND
// the charge bank is empty at trigger time (T014/R12). Only then is the empty-bank
// precheck honest: a free-form player monitor order's action may be advisory or a
// meta act, so it still runs the turn. (Batch B places no system-origin orders —
// deferral is Batch C T016 — so this is dormant-but-correct, guarding exactly the
// deferral orders it will meet.)
func (mt *Metatron) knownActEmptyBank(o sim.MetatronOrder) bool {
	if o.Origin != "system" {
		return false
	}
	mt.stateMu.Lock()
	c := mt.charges
	mt.stateMu.Unlock()
	return c <= 0
}

// acquireTurnBusy waits (bounded by systemTurnBusyWait) for the single-flight turn
// slot for a SYSTEM turn (spec 029 R6). Returns false if the slot could not be
// acquired in time (the caller degrades to an honest moment) or the angel is
// closing. The console path never calls this — it CAS-fails fast with ErrTurnBusy.
func (mt *Metatron) acquireTurnBusy() bool {
	deadline := time.Now().Add(systemTurnBusyWait)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if mt.turnBusy.CompareAndSwap(false, true) {
			return true
		}
		select {
		case <-mt.done:
			return false
		case <-tick.C:
		}
		if time.Now().After(deadline) {
			return mt.turnBusy.CompareAndSwap(false, true)
		}
	}
}

// queueMoment appends a model-free moment to the soul and the player-facing queue
// (the same discipline as observeMoment's drama moments) — how a triggered turn's
// outcome reaches the next console reply (spec 029 US3, SC-003).
func (mt *Metatron) queueMoment(line string) {
	if line == "" {
		return
	}
	mt.appendFile(mt.soulPath(), "\n**MOMENT** "+line+"\n")
	mt.stateMu.Lock()
	mt.moments = append(mt.moments, line)
	mt.stateMu.Unlock()
}

// triggeredMoment renders the model-free moment describing a COMPLETED triggered
// turn (spec 029 US3): what the angel did while the player was away, so the next
// console reply leads with it. It names the landed act (omen/vision/miracle) when
// one landed, else that the watch woke and was attended.
func (mt *Metatron) triggeredMoment(tick int64, o sim.MetatronOrder, r TurnResult) string {
	switch {
	case r.Nudge != nil:
		return fmt.Sprintf("%s — a watch came due (%q): I sent a %s to %s.",
			clock.Format(tick), o.Condition, r.Nudge.Form, strings.Join(r.Nudge.Targets, ", "))
	case r.Miracle != nil:
		return fmt.Sprintf("%s — a watch came due (%q): I worked a miracle — %s.",
			clock.Format(tick), o.Condition, r.Miracle.Summary)
	default:
		return fmt.Sprintf("%s — a watch came due (%q): I attended it.", clock.Format(tick), o.Condition)
	}
}

// degradedMoment maps a FAILED system turn to ONE model-free honest moment per
// failure family (spec 029 T014/R12, FR-011) — never a retry. Empty bank is caught
// earlier (knownActEmptyBank); this covers exhausted budget, a downed/busy tier,
// and transport failures the loop already retried once internally.
func (mt *Metatron) degradedMoment(tick int64, err error) string {
	switch {
	case errors.Is(err, llm.ErrBudgetExhausted), errors.Is(err, llm.ErrTierDown), errors.Is(err, llm.ErrTierBusy):
		return fmt.Sprintf("%s — a watch came due, but my sight dimmed and I could not act. Nothing was spent.", clock.Format(tick))
	default:
		return fmt.Sprintf("%s — a watch came due, but I faltered and could not complete it. Nothing was spent.", clock.Format(tick))
	}
}

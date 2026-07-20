// Package mind is the driver that gives villagers planner thoughts (TASK-7):
// it watches the event stream through a replica, schedules planner calls
// (30-game-minute stagger + scene-change triggers), routes them through the
// LLM orchestrator's local tier, and injects the chosen goals back into the
// loop as recorded commands. Model output never touches deterministic space
// except through Loop.InjectIntent.
package mind

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// Submitter is the orchestrator surface the mind needs (test seam).
type Submitter interface {
	Submit(ctx context.Context, req llm.Request) (llm.Response, error)
}

// Injector is the loop surface the mind needs (test seam).
type Injector interface {
	InjectIntent(args sim.InjectArgs) error
}

const (
	encounterCooldownTicks = 2 * 3600 // per pair
	encounterRadius        = 1
	callTimeout            = 90 * time.Second
	// planDebounceTicks floors the gap between one agent's planner calls:
	// completion triggers re-arm on every finished act, and without a floor
	// the trigger→plan→act→complete→trigger loop saturates the local tier
	// (starving conversations). Pending triggers stay armed and fire once
	// the window opens.
	planDebounceTicks = 300 // 5 game-minutes

	// Musings (TASK-21): best-effort interiority between planner calls.
	// The cadence is per agent; drops (busy tier, bad reply) cost a beat
	// of silence and nothing else — musings are never queued or retried.
	// museTimeout matches the planner's callTimeout: admission (not the
	// deadline) is what keeps musings from displacing real work — once a
	// quiet tier accepts one, a slow local model may take its time.
	museCadenceTicks = 900 // 15 game-minutes
	museTimeout      = callTimeout
	museMaxTokens    = 48
	// museStarveWindow is the fairness floor: best-effort admission loses
	// every race on a saturated tier (live finding: back-to-back planner
	// calls at ~50s each admit zero musings), so a musing starved this
	// long rides the normal queue like any other call. Worst-case cost:
	// one 48-token call per window.
	museStarveWindow = 2 * time.Minute
)

type Mind struct {
	orch      Submitter
	loop      Injector
	social    SocialInjector
	convoBusy atomic.Bool
	replica   *sim.State
	m         *worldmap.Map
	personas  [sim.AgentCount]string
	k         int

	nextDue     [sim.AgentCount]int64
	lastPlanned [sim.AgentCount]int64
	pairSeen    map[[2]int]int64
	pending     [sim.AgentCount]bool // trigger armed before nextDue

	museDue    [sim.AgentCount]int64
	museBusy   atomic.Bool  // one musing in flight at a time
	lastMuseOK atomic.Int64 // wall unix-nano of the last landed musing

	events chan []store.Event
	done   chan struct{}
}

// New starts the driver from a state snapshot. Cadence is staggered so eight
// agents never think in the same game-minute.
func New(orch Submitter, loop Injector, social SocialInjector, m *worldmap.Map, seed uint64, stateJSON []byte, personas [sim.AgentCount]string) (*Mind, error) {
	replica := sim.NewState(seed, m)
	if err := json.Unmarshal(stateJSON, replica); err != nil {
		return nil, err
	}
	md := &Mind{
		orch:     orch,
		loop:     loop,
		social:   social,
		replica:  replica,
		m:        m,
		personas: personas,
		k:        sim.WindowK,
		pairSeen: map[[2]int]int64{},
		events:   make(chan []store.Event, 256),
		done:     make(chan struct{}),
	}
	for i := range md.nextDue {
		md.nextDue[i] = replica.Tick + int64(i+1)*(sim.PlannerCadenceTicks/sim.AgentCount)
		// Musing stagger sits half a slot off the planner stagger so the
		// two cadences interleave instead of colliding.
		md.museDue[i] = replica.Tick + museCadenceTicks/2 + int64(i)*(museCadenceTicks/sim.AgentCount)
	}
	go md.run()
	return md, nil
}

// Observe is the loop-notify path: never blocks (drop on overflow — the
// next batch still carries the tick forward and cadence self-heals).
func (md *Mind) Observe(events []store.Event) {
	select {
	case md.events <- events:
	default:
	}
}

func (md *Mind) Close() { close(md.done) }

func (md *Mind) run() {
	for {
		select {
		case <-md.done:
			return
		case batch := <-md.events:
			md.absorb(batch)
			md.plan()
			md.muse()
		}
	}
}

// absorb applies events to the replica and arms triggers.
func (md *Mind) absorb(batch []store.Event) {
	for _, e := range batch {
		md.replica.Apply(e)
		if e.Tick > md.replica.Tick {
			md.replica.Tick = e.Tick
		}
		switch e.Type {
		case "agent.woke":
			var p sim.AgentPayload
			if json.Unmarshal(e.Payload, &p) == nil {
				md.arm(p.Agent)
			}
		case "agent.intent_done", "agent.foraged", "agent.chopped", "agent.hunted", "agent.built":
			var p struct {
				Agent int `json:"agent"`
			}
			if json.Unmarshal(e.Payload, &p) == nil {
				md.arm(p.Agent)
			}
		case "sim.night_started":
			for i := range md.replica.Agents {
				md.arm(i)
			}
		case "agent.moved":
			md.armEncounters(e)
		case "agent.talked":
			md.maybeStartConversation(e)
		}
	}
}

func (md *Mind) arm(idx int) {
	if idx >= 0 && idx < sim.AgentCount {
		md.pending[idx] = true
	}
}

// armEncounters fires when two agents first become adjacent (pair cooldown).
func (md *Mind) armEncounters(e store.Event) {
	var p sim.AgentMovedPayload
	if json.Unmarshal(e.Payload, &p) != nil {
		return
	}
	a := p.Agent
	for b := range md.replica.Agents {
		if b == a || md.replica.Agents[b].Dead {
			continue
		}
		if absInt(md.replica.Agents[b].X-p.X)+absInt(md.replica.Agents[b].Y-p.Y) <= encounterRadius {
			key := [2]int{minInt(a, b), maxInt(a, b)}
			if e.Tick-md.pairSeen[key] >= encounterCooldownTicks {
				md.pairSeen[key] = e.Tick
				md.arm(a)
				md.arm(b)
			}
		}
	}
}

// plan runs due agents. Serialized (one model call at a time) — the local
// tier is the throughput governor, and the orchestrator's queue is the
// backpressure surface.
func (md *Mind) plan() {
	tick := md.replica.Tick
	for i := range md.replica.Agents {
		a := &md.replica.Agents[i]
		if a.Dead || a.Asleep {
			md.pending[i] = false
			continue
		}
		if !md.pending[i] && tick < md.nextDue[i] {
			continue
		}
		if tick-md.lastPlanned[i] < planDebounceTicks {
			continue // debounced; pending stays armed for a later batch
		}
		md.pending[i] = false
		md.lastPlanned[i] = tick
		md.nextDue[i] = tick + sim.PlannerCadenceTicks

		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		resp, err := md.orch.Submit(ctx, llm.Request{
			Kind:      llm.KindPlanner,
			System:    systemPrompt(a.Name, md.personas[i]),
			Prompt:    userPrompt(md.replica, i, md.k),
			MaxTokens: 256,
		})
		cancel()
		if err != nil {
			log.Printf("mind: %s planner call failed: %v", a.Name, err)
			continue // reflex grace covers; next trigger retries
		}
		reply, err := parseReply(resp.Text)
		if err != nil {
			log.Printf("mind: %s reply unusable: %v", a.Name, err)
			continue
		}
		target := -1
		if reply.Goal == "talk_to" {
			if target = md.agentIndexByName(reply.Target); target < 0 {
				log.Printf("mind: %s wants to talk to unknown %q", a.Name, reply.Target)
				continue
			}
		}
		if err := md.loop.InjectIntent(sim.InjectArgs{
			Agent: i, Goal: reply.Goal, TargetAgent: target, Reason: reply.Reason,
		}); err != nil {
			log.Printf("mind: %s goal %q rejected: %v", a.Name, reply.Goal, err)
		}
	}
}

// muse fires at most one best-effort interior thought per batch (TASK-21):
// pure flavor with no goal effect, run detached so it never blocks the
// absorb loop, and dropped — never queued — whenever the tier is busy or
// the reply is unusable. Snapshot strings are built synchronously; only the
// model call and injection leave this goroutine.
func (md *Mind) muse() {
	if md.social == nil || !md.museBusy.CompareAndSwap(false, true) {
		return
	}
	tick := md.replica.Tick
	pick := -1
	for i := range md.replica.Agents {
		a := &md.replica.Agents[i]
		if a.Dead || a.Asleep || tick < md.museDue[i] {
			continue
		}
		if pick < 0 || md.museDue[i] < md.museDue[pick] {
			pick = i // most overdue first
		}
	}
	if pick < 0 {
		md.museBusy.Store(false)
		return
	}
	// Re-arm before the call: a dropped musing is silence, not a debt.
	md.museDue[pick] = tick + museCadenceTicks
	name := md.replica.Agents[pick].Name
	system := musingSystemPrompt(name, md.personas[pick])
	prompt := userPrompt(md.replica, pick, md.k)
	// The fairness floor stands down while a conversation runs — a scene
	// is already the tier's most expensive tenant, and a queued musing
	// behind it only starves planners further.
	starved := !md.convoBusy.Load() &&
		time.Since(time.Unix(0, md.lastMuseOK.Load())) > museStarveWindow
	go func() {
		defer md.museBusy.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), museTimeout)
		resp, err := md.orch.Submit(ctx, llm.Request{
			Kind: llm.KindMusing, System: system, Prompt: prompt, MaxTokens: museMaxTokens,
			BestEffort: !starved,
		})
		cancel()
		if err != nil {
			return // best effort: busy or degraded tiers cost only silence
		}
		text, err := parseMusing(resp.Text)
		if err != nil {
			return
		}
		payload, err := json.Marshal(sim.ThoughtPayload{Agent: pick, Text: text, Source: "musing"})
		if err != nil {
			return
		}
		if err := md.social.InjectSocial([]store.Event{{Type: "agent.thought", Payload: payload}}); err != nil {
			log.Printf("mind: %s musing rejected: %v", name, err)
			return
		}
		md.lastMuseOK.Store(time.Now().UnixNano())
	}()
}

func (md *Mind) agentIndexByName(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, n := range sim.AgentNames {
		if strings.ToLower(n) == name {
			return i
		}
	}
	return -1
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

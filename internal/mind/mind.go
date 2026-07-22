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

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
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
	// callTimeout must exceed the local model's honest completion time or
	// the tier's throughput is zero: live measurement (gemma 12B, day-1
	// prompts) put planner completions just past the old 90s, so every
	// call burned its full window and produced nothing. Planners tolerate
	// staleness — a late plan beats no plan (the reflex floor covers gaps).
	callTimeout = 180 * time.Second
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
	pending     [sim.AgentCount]bool  // trigger armed before nextDue
	pendingSeq  [sim.AgentCount]int64 // event seq of the arming stimulus (0 = cadence)

	// tick mirrors the replica's tick for worker goroutines (the replica
	// itself is absorb-owned). Telemetry landing ticks read this; the loop's
	// envelope re-stamp remains the authoritative landing tick.
	tick atomic.Int64

	// Planner calls run on their own single-flight worker (TASK-9 fix): a
	// model call must never block the absorb loop, or the events channel
	// overflows at high speed and edge triggers (sleeps!) are dropped.
	planQ        chan planJob
	planInFlight [sim.AgentCount]atomic.Bool

	// Nightly consolidation (TASK-9): FIFO queue + per-agent in-flight guard.
	consolQ        chan consolJob
	consolInFlight [sim.AgentCount]atomic.Bool

	// Musings (TASK-21): best-effort interiority, single-flight.
	museDue    [sim.AgentCount]int64
	museBusy   atomic.Bool  // one musing in flight at a time
	lastMuseOK atomic.Int64 // wall unix-nano of the last landed musing

	// Chronicle narrator (TASK-11): absorb-owned chapter buffer + FIFO queue
	// to the single-flight cloud worker; narrRetry (cap 1) carries a failed
	// chapter's lines into the next one.
	narrLines []string
	narrFrom  int64
	narrQ     chan narrJob
	narrRetry chan narrCarry

	// Governance phrasing (TASK-13): bounded queue + single-flight worker
	// rephrasing enacted proposals in the proposer's voice.
	meetQ chan meetingJob

	// rearm carries landing-rejection re-plan requests from the plan worker
	// back to the absorb goroutine (which owns pending); the debounce floor
	// still applies, so a rejected agent re-thinks promptly, never hotly.
	rearm chan int

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
		orch:      orch,
		loop:      loop,
		social:    social,
		replica:   replica,
		m:         m,
		personas:  personas,
		k:         sim.WindowK,
		pairSeen:  map[[2]int]int64{},
		planQ:     make(chan planJob, sim.AgentCount),
		consolQ:   make(chan consolJob, sim.AgentCount),
		narrQ:     make(chan narrJob, 8),
		narrRetry: make(chan narrCarry, 1),
		meetQ:     make(chan meetingJob, 4),
		rearm:     make(chan int, sim.AgentCount),
		events:    make(chan []store.Event, 256),
		done:      make(chan struct{}),
	}
	for i := range md.nextDue {
		md.nextDue[i] = replica.Tick + int64(i+1)*(sim.PlannerCadenceTicks/sim.AgentCount)
		// Musing stagger sits half a slot off the planner stagger so the
		// two cadences interleave instead of colliding.
		md.museDue[i] = replica.Tick + museCadenceTicks/2 + int64(i)*(museCadenceTicks/sim.AgentCount)
	}
	go md.run()
	go md.planWorker()
	go md.consolidateWorker()
	go md.narrateWorker()
	go md.meetingWorker()
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
		case idx := <-md.rearm:
			// A landing rejection: the agent noticed the plan failed and
			// re-thinks at the next open debounce window.
			md.arm(idx, 0)
			md.plan()
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
				md.arm(p.Agent, e.Seq)
			}
		case "agent.intent_done", "agent.foraged", "agent.chopped", "agent.hunted", "agent.built":
			var p struct {
				Agent int `json:"agent"`
			}
			if json.Unmarshal(e.Payload, &p) == nil {
				md.arm(p.Agent, e.Seq)
			}
		case "sim.night_started":
			for i := range md.replica.Agents {
				md.arm(i, e.Seq)
			}
		case "agent.moved":
			md.armEncounters(e)
		case "agent.talked":
			md.maybeStartConversation(e)
		case "agent.slept":
			md.maybeConsolidate(e)
		case "meeting.proposal_resolved":
			md.maybePhraseProposal(e)
		}
		md.chronicleNote(e)
	}
	md.tick.Store(md.replica.Tick)
}

// arm marks an agent due for a planner thought; seq is the arming stimulus
// event — the causality edge recorded on the eventual cog.thought (FR-020).
func (md *Mind) arm(idx int, seq int64) {
	if idx >= 0 && idx < sim.AgentCount {
		md.pending[idx] = true
		md.pendingSeq[idx] = seq
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
				md.arm(a, e.Seq)
				md.arm(b, e.Seq)
			}
		}
	}
}

// planJob is the immutable snapshot a planner call runs against; prompts are
// built in the absorb goroutine (which owns the replica) and carried as
// strings so the worker never touches shared state.
type planJob struct {
	agent  int
	name   string
	system string
	prompt string
	meta   thoughtMeta
	// world is the snapshot view guards are built from once the reply names
	// a target — the assumptions the prompt showed the model (FR-011).
	world [sim.AgentCount]agentSnap
}

type agentSnap struct {
	x, y int
	dead bool
}

// plan enqueues due agents for the planner worker. It never blocks and never
// calls a model — that is the worker's job. Single-flight per agent; the
// worker serializes calls so the local tier remains the throughput governor.
func (md *Mind) plan() {
	tick := md.replica.Tick
	for i := range md.replica.Agents {
		a := &md.replica.Agents[i]
		if a.Dead || a.Asleep {
			md.pending[i] = false
			continue
		}
		if sim.AtMeeting(md.replica, i) {
			continue // assembled (TASK-13): pending stays armed for after close
		}
		if !md.pending[i] && tick < md.nextDue[i] {
			continue
		}
		if tick-md.lastPlanned[i] < planDebounceTicks {
			continue // debounced; pending stays armed for a later batch
		}
		if md.planInFlight[i].Load() {
			continue // one plan in flight per agent; pending stays armed
		}
		// The cognition horizon (FR-007/FR-008): a planner thought whose
		// predicted drift exceeds its staleness budget at this speed is
		// never attempted — the reflex floor is the degrade action, and the
		// suppression is recorded with its arithmetic.
		if v := md.routeVerdict("planner", llm.KindPlanner); !v.Allow {
			md.emitSuppressed("planner", i, tick, v)
			md.pending[i] = false
			md.pendingSeq[i] = 0
			// Phase-preserving (TASK-44): see nextPhasePreservingDue — a
			// speed spike that suppresses several agents in the same batch
			// must not collapse their boot-staggered cadence phases.
			md.nextDue[i] = nextPhasePreservingDue(md.nextDue[i], tick, sim.PlannerCadenceTicks)
			continue
		}
		job := planJob{
			agent:  i,
			name:   a.Name,
			system: systemPrompt(a.Name, md.personas[i]),
			prompt: userPrompt(md.replica, i, md.k),
			meta:   md.newMeta("planner", i, tick, md.pendingSeq[i], llm.KindPlanner),
		}
		job.meta.generation = a.Generation
		for j := range md.replica.Agents {
			b := &md.replica.Agents[j]
			job.world[j] = agentSnap{x: b.X, y: b.Y, dead: b.Dead}
		}
		// Future-dating (FR-016): the prompt says when the decision lands,
		// using the router's own prediction — prompt and gate never disagree.
		if job.meta.class.FutureDated {
			job.prompt = futureDated(tick, job.meta.predictedLandTick) + job.prompt
		}
		md.planInFlight[i].Store(true)
		select {
		case md.planQ <- job:
			md.pending[i] = false
			md.pendingSeq[i] = 0
			md.lastPlanned[i] = tick
			// Phase-preserving (TASK-44): a shared trigger (e.g. a busy-tier
			// backlog clearing several overdue agents in one batch) must not
			// re-arm them all onto the identical nextDue — that permanently
			// locks the cadence fallback into lockstep, same as the musing bug.
			md.nextDue[i] = nextPhasePreservingDue(md.nextDue[i], tick, sim.PlannerCadenceTicks)
		default:
			md.planInFlight[i].Store(false) // queue full; retry next batch
		}
	}
}

// planWorker drains planner jobs one model call at a time.
func (md *Mind) planWorker() {
	for {
		select {
		case <-md.done:
			return
		case job := <-md.planQ:
			md.runPlan(job)
		}
	}
}

func (md *Mind) runPlan(job planJob) {
	defer md.planInFlight[job.agent].Store(false)

	md.emitCog(cogThoughtEvent(job.meta))
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind:      llm.KindPlanner,
		System:    job.system,
		Prompt:    job.prompt,
		MaxTokens: 256,
		// Constrain the local model to the planner reply shape (TASK-58):
		// the goal enum + step cap are enforced at the sampler so a 3B model
		// can't free-generate its way past parseReply. Planner-only — musing,
		// conversation, consolidation, etc. stay unconstrained.
		ResponseSchema: plannerSchema,
		SchemaName:     "plan",
	})
	cancel()
	if err != nil {
		log.Printf("mind: %s planner call failed: %v", job.name, err)
		md.emitCog(md.cogOutcomeEvent(job.meta, sim.OutcomeUnusable, "call: "+err.Error(),
			time.Since(start).Milliseconds()))
		return // reflex grace covers; next trigger retries
	}
	reply, err := parseReply(resp.Text)
	if err != nil {
		log.Printf("mind: %s reply unusable: %v", job.name, err)
		md.emitCog(md.cogOutcomeEvent(job.meta, sim.OutcomeUnusable, "parse: "+err.Error(), resp.Millis))
		return
	}
	if len(reply.Plan) > 0 {
		md.injectPlan(job, reply, resp.Millis)
		return
	}
	target := -1
	var guards []sim.Guard
	if reply.Goal == "talk_to" {
		if target = md.agentIndexByName(reply.Target); target < 0 {
			log.Printf("mind: %s wants to talk to unknown %q", job.name, reply.Target)
			md.emitCog(md.cogOutcomeEvent(job.meta, sim.OutcomeUnusable,
				"unknown target "+reply.Target, resp.Millis))
			return
		}
		// The assumptions this thought was formed under (FR-011): the
		// target was alive and present in the prompt's worldview.
		guards = append(guards,
			sim.Guard{Type: sim.GuardTargetAlive, Target: target},
			sim.Guard{Type: sim.GuardTargetPresent, Target: target,
				X: job.world[target].x, Y: job.world[target].y},
		)
	}
	// The loop owns the landing verdict and its telemetry from here
	// (FR-010..FR-013): exactly one outcome per thought, emitted atomically
	// with the verdict. The mind only reacts — a rejection re-arms a prompt
	// re-plan (the agent noticed the plan failed), floored by the debounce.
	if err := md.loop.InjectIntent(sim.InjectArgs{
		Agent: job.agent, Goal: reply.Goal, TargetAgent: target, Reason: reply.Reason,
		Kind: reply.Kind, Qty: reply.Qty,
		Class: job.meta.class.Class, JobID: job.meta.job,
		SnapshotTick: job.meta.snapshotTick, Generation: job.meta.generation,
		PredictedWallMs: job.meta.predictedWallMs, ActualWallMs: resp.Millis,
		Guards: guards,
	}); err != nil {
		log.Printf("mind: %s goal %q rejected: %v", job.name, reply.Goal, err)
		select {
		case md.rearm <- job.agent:
		default:
		}
	}
}

// injectPlan lands a guarded conditional plan (US4): after_min becomes an
// after_tick guard anchored at the snapshot, for_min bounds each step's
// window; the loop's ladder and step validation apply as for any landing.
func (md *Mind) injectPlan(job planJob, reply planReply, actualMs int64) {
	steps := make([]sim.PlanStep, 0, len(reply.Plan))
	for _, sr := range reply.Plan {
		target := -1
		if sr.Goal == "talk_to" {
			if target = md.agentIndexByName(sr.Target); target < 0 {
				log.Printf("mind: %s plan step targets unknown %q", job.name, sr.Target)
				md.emitCog(md.cogOutcomeEvent(job.meta, sim.OutcomeUnusable,
					"plan: unknown target "+sr.Target, actualMs))
				return
			}
		}
		st := sim.PlanStep{Job: job.meta.job, Goal: sr.Goal, Target: target, Kind: sr.Kind, Qty: sr.Qty}
		start := job.meta.snapshotTick
		if sr.AfterMin > 0 {
			start += int64(sr.AfterMin * 60)
			st.When = &sim.Guard{Type: sim.GuardAfterTick, Tick: start}
		}
		if sr.ForMin > 0 {
			st.Until = start + int64(sr.ForMin*60)
		}
		steps = append(steps, st)
	}
	if err := md.loop.InjectIntent(sim.InjectArgs{
		Agent: job.agent, TargetAgent: -1, Reason: reply.Reason,
		Class: job.meta.class.Class, JobID: job.meta.job,
		SnapshotTick: job.meta.snapshotTick, Generation: job.meta.generation,
		PredictedWallMs: job.meta.predictedWallMs, ActualWallMs: actualMs,
		Plan: steps,
	}); err != nil {
		log.Printf("mind: %s plan rejected: %v", job.name, err)
		select {
		case md.rearm <- job.agent:
		default:
		}
	}
}

// nextPhasePreservingDue advances an overdue schedule to the next tick
// strictly after tick, stepping in whole cadence multiples from its own due
// — never from tick. This is the TASK-44 fix: re-arming "from now" instead
// of from the agent's own due collapses every agent a shared stall left
// overdue onto the identical due, locking the whole village into lockstep
// the next time cadence comes around. Preserving due's phase (due mod
// cadence) keeps each agent's boot offset intact forever, regardless of how
// many cadences it had to skip. Arithmetic equivalent of:
//
//	for due <= tick { due += cadence }
func nextPhasePreservingDue(due, tick, cadence int64) int64 {
	if cadence <= 0 || due > tick {
		return due
	}
	return due + (tick-due)/cadence*cadence + cadence
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
		if a.Dead || a.Asleep || tick < md.museDue[i] || sim.AtMeeting(md.replica, i) {
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
	// Phase-preserving (TASK-44): step forward from this agent's own due,
	// not from tick, so a shared stall that leaves several agents overdue
	// at once doesn't collapse their boot-staggered phases together.
	md.museDue[pick] = nextPhasePreservingDue(md.museDue[pick], tick, museCadenceTicks)
	// Router gate (FR-007): musings survive far higher speeds than planners
	// (1 point vs 3), but the horizon still applies.
	if v := md.routeVerdict("musing", llm.KindMusing); !v.Allow {
		md.emitSuppressed("musing", pick, tick, v)
		md.museBusy.Store(false)
		return
	}
	name := md.replica.Agents[pick].Name
	system := musingSystemPrompt(name, md.personas[pick])
	prompt := userPrompt(md.replica, pick, md.k)
	meta := md.newMeta("musing", pick, tick, 0, llm.KindMusing)
	// The fairness floor stands down while a conversation runs — a scene
	// is already the tier's most expensive tenant, and a queued musing
	// behind it only starves planners further.
	starved := !md.convoBusy.Load() &&
		time.Since(time.Unix(0, md.lastMuseOK.Load())) > museStarveWindow
	go func() {
		defer md.museBusy.Store(false)
		md.emitCog(cogThoughtEvent(meta))
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), museTimeout)
		resp, err := md.orch.Submit(ctx, llm.Request{
			Kind: llm.KindMusing, System: system, Prompt: prompt, MaxTokens: museMaxTokens,
			BestEffort: !starved,
		})
		cancel()
		if err != nil {
			// Best effort: busy or degraded tiers cost only silence — but
			// recorded silence (FR-015).
			md.emitCog(md.cogOutcomeEvent(meta, sim.OutcomeUnusable, "call: "+err.Error(),
				time.Since(start).Milliseconds()))
			return
		}
		text, err := parseMusing(resp.Text)
		if err != nil {
			md.emitCog(md.cogOutcomeEvent(meta, sim.OutcomeUnusable, "parse: "+err.Error(), resp.Millis))
			return
		}
		payload, err := json.Marshal(sim.ThoughtPayload{Agent: pick, Text: text, Source: "musing"})
		if err != nil {
			md.emitCog(md.cogOutcomeEvent(meta, sim.OutcomeUnusable, "marshal: "+err.Error(), resp.Millis))
			return
		}
		// The musing and its terminal record land atomically.
		if err := md.social.InjectSocial([]store.Event{
			{Type: "agent.thought", Payload: payload},
			md.cogOutcomeEvent(meta, sim.OutcomeLanded, "", resp.Millis),
		}); err != nil {
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

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
	"github.com/evanstern/promptworld/internal/tool"
	"github.com/evanstern/promptworld/internal/toolloop"
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

	// Musing is no longer a scheduled channel (spec 017 R10): a villager muses
	// only by choosing the muse tool inside its planner tool-use loop, so it
	// carries the same opportunity cost as any other action. The cadence-fired
	// best-effort musing path (its queue, stagger, and fairness floor) is gone.
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

	// runLoop drives one villager tool-use loop (spec 017, TASK-52). Production
	// wires it to toolloop.Run against the orchestrator; tests that stub the
	// model through the Submitter seam install a scripted driver. loopRounds is
	// the hard iteration cap (llm.json loop_max_rounds, normalized).
	runLoop    func(ctx context.Context, j toolloop.Job) (toolloop.Result, error)
	loopRounds int

	// Nightly consolidation (TASK-9): FIFO queue + per-agent in-flight guard.
	consolQ        chan consolJob
	consolInFlight [sim.AgentCount]atomic.Bool

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
// The variadic runLoopOverride is a test seam: it installs a scripted loop
// driver BEFORE any goroutine starts (race-free), for tests that stub the model
// through the Submitter interface rather than a real *llm.Orchestrator.
// Production omits it — New wires runLoop from the concrete orchestrator.
func New(orch Submitter, loop Injector, social SocialInjector, m *worldmap.Map, seed uint64, stateJSON []byte, personas [sim.AgentCount]string, loopRounds int, runLoopOverride ...func(context.Context, toolloop.Job) (toolloop.Result, error)) (*Mind, error) {
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
	md.loopRounds = loopRounds
	// The tool-use loop needs the concrete orchestrator (toolloop.Run's
	// contract surface). Production passes it; test seams that stub the model
	// through the Submitter interface install their own runLoop after New.
	if o, ok := orch.(*llm.Orchestrator); ok {
		md.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
			return toolloop.Run(ctx, o, j)
		}
	}
	if len(runLoopOverride) > 0 && runLoopOverride[0] != nil {
		md.runLoop = runLoopOverride[0]
	}
	for i := range md.nextDue {
		md.nextDue[i] = replica.Tick + int64(i+1)*(sim.PlannerCadenceTicks/sim.AgentCount)
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
	// journal is a race-free snapshot of the acting agent's journal as of the
	// cognition's start (spec 019, US3): search_journal / read_journal handlers
	// run in the planner worker goroutine and must not touch the absorb-owned
	// replica. Only reads see it; writes/deletes land through the live door.
	journal *sim.Journal
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
		job.journal = a.Journal.Clone() // race-free snapshot for search/read handlers
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

// loopMaxTokens is the per-round token budget for a villager tool-use loop.
// The pre-loop planner used 256 with a bare JSON reply; a tool-era round must
// carry a tool_use block (the call name + JSON arguments) alongside any prose,
// so 256 truncates a structured call mid-arguments. 512 gives headroom for the
// call plus a short accompanying line without inviting the model to ramble.
const loopMaxTokens = 512

// runPlan drives one villager cognition through the bounded tool-use loop
// (spec 017, FR-002/FR-004). The model may look things up with read tools, then
// commit to one acting tool — a world verb, a plan (set_plan), or a passing
// thought (muse) — which lands through its existing door. The mind opens the
// cognition with a cog.thought and lets the landing door (or the muse social
// batch) own the terminal cog.outcome; it only supplies the FR-015 failure
// outcome when nothing reached a door, and re-arms a re-plan when a landing was
// gate-refused, mirroring the pre-loop rejection/failure paths.
func (md *Mind) runPlan(job planJob) {
	defer md.planInFlight[job.agent].Store(false)

	md.emitCog(cogThoughtEvent(job.meta))
	start := time.Now()
	d := &villagerDispatch{md: md, job: job, start: start}
	handlers := md.villagerHandlers(d)

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	res, err := md.runLoop(ctx, toolloop.Job{
		JobID:     job.meta.job,
		Kind:      llm.KindPlanner,
		System:    job.system,
		Seed:      job.prompt,
		Roster:    tool.LoopRosterVillager(),
		Handlers:  handlers,
		MaxRounds: md.loopRounds,
		MaxTokens: loopMaxTokens,
		Record:    d.record,
	})
	cancel()

	// Land every buffered CallRecord as a cog.tool_call event (spec 017
	// FR-007, T018), unconditional on termination path — the AC#5 scenario
	// requires rejected / never-grounded calls recorded even when nothing
	// landed. Emitted here, before the terminal-outcome switch: the door
	// already emitted any grounding + cog.outcome during the loop, and the
	// default case's terminal outcome follows the switch, so this dedicated
	// batch reorders neither.
	md.emitToolCalls(d.records, job.meta.snapshotTick)

	// Termination -> outcome + rearm, mirroring the pre-loop paths:
	//   - landed: the landing door (world/set_plan) or the muse social batch
	//     already emitted the sole cog.outcome; the mind adds none and does not
	//     rearm (matches today's landed path — the door owned the outcome).
	//   - a door recorded rejection(s) but nothing landed: re-think, exactly as
	//     today's rejection path rearms; the door's rejection is the record, so
	//     the mind adds no outcome.
	//   - nothing reached a door (plain text, reads only, unknown target, infra
	//     error): record the terminal unusable outcome (FR-015) and let the
	//     reflex floor cover — no rearm, as today's call/parse-failure path.
	switch {
	case res.Term == toolloop.TermLanded:
		// sole outcome already emitted by the landing path.
	case d.doorOutcome:
		if err != nil {
			log.Printf("mind: %s loop ended %s after a rejection: %v", job.name, res.Term, err)
		}
		md.rearmAgent(job.agent)
	default:
		if err != nil {
			log.Printf("mind: %s loop failed (%s): %v", job.name, res.Term, err)
		}
		md.emitCog(md.cogOutcomeEvent(job.meta, sim.OutcomeUnusable, loopFailReason(res, err), res.TotalMillis))
	}
}

// rearmAgent asks the absorb goroutine to re-plan an agent whose cognition
// landed nothing (debounce-floored). Never blocks: a full channel means a
// re-plan is already pending.
func (md *Mind) rearmAgent(agent int) {
	select {
	case md.rearm <- agent:
	default:
	}
}

// loopFailReason describes a non-landed, no-door-outcome termination for the
// recorded unusable outcome — existing failure vocabulary (OutcomeUnusable);
// the reason string carries the detail so no failure is ever silent (FR-015).
func loopFailReason(res toolloop.Result, err error) string {
	switch res.Term {
	case toolloop.TermModelDone:
		return "loop: model produced no tool call"
	case toolloop.TermCapExhausted:
		return "loop: round cap reached with no acting call landed"
	case toolloop.TermAdmissionRefused:
		return "loop: admission refused"
	case toolloop.TermCtxDone:
		return "loop: context ended"
	default:
		if err != nil {
			return "loop: " + err.Error()
		}
		return "loop: no action landed"
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
